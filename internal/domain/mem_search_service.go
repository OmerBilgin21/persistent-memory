package domain

import (
	"context"
	"slices"
	"sync"

	"persistent-memory/internal/persistence"
	"persistent-memory/util"
)

type MemService struct {
	embedService                     EmbedServiceInterface
	store                            *persistence.Store
	similarityAcceptedCountThreshold int
}

func NewMemService(store *persistence.Store, embedService *EmbedService,
	acceptedCountThreshold int,
) (*MemService, error) {
	return &MemService{
		store:                            store,
		embedService:                     embedService,
		similarityAcceptedCountThreshold: acceptedCountThreshold,
	}, nil
}

// withTransaction runs fn against repositories bound to a single transaction.
// Anything reached through s.store instead of repos stays on the pool.
func (s *MemService) withTransaction(ctx context.Context,
	fn func(repos *persistence.Store) error,
) error {
	txStore, tx, err := s.store.Begin(ctx)
	if err != nil {
		return err
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := fn(txStore); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true

	return nil
}

func (s *MemService) GetTopicByName(ctx context.Context, name string) (*SimilarityTopic, error) {
	topic, err := s.store.Topics.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}

	return &SimilarityTopic{
		Description: topic.Description,
		Id:          topic.Id,
		Name:        topic.Name,
	}, nil
}

func (s *MemService) GetRFFRank(position int) float32 {
	return float32(float32(1) / (float32(60) + float32(position)))
}

func (s *MemService) GetCosineSortedMemories(ctx context.Context, normalizedSearchedVector []float32, memories []persistence.Memory) (*[]SimilarityMemory, error) {
	cosineSorted := make([]SimilarityMemory, 0, len(memories))

	for _, m := range memories {
		sim, err := s.embedService.NormalizedCosineSimilarity(normalizedSearchedVector,
			m.NormalizedVector)
		if err != nil {
			return nil, err
		}

		cosineSorted = append(cosineSorted, SimilarityMemory{
			TopicId:    m.TopicId,
			Content:    m.Content,
			Id:         m.Id,
			CosineRank: sim,
		})
	}

	slices.SortStableFunc(cosineSorted, func(a, b SimilarityMemory) int {
		if a.CosineRank < b.CosineRank {
			return 1
		}
		if a.CosineRank > b.CosineRank {
			return -1
		}

		return 0
	})

	return &cosineSorted, nil
}

func (s *MemService) GetCosineSortedTopics(ctx context.Context, normalizedSearchedVector []float32, topics []persistence.Topic) (*[]SimilarityTopic, error) {
	cosineSorted := make([]SimilarityTopic, 0, len(topics))

	for _, r := range topics {
		sim, err := s.embedService.NormalizedCosineSimilarity(normalizedSearchedVector,
			r.NormalizedVector)
		if err != nil {
			return nil, err
		}

		cosineSorted = append(cosineSorted, SimilarityTopic{
			Id:          r.Id,
			Name:        r.Name,
			Description: r.Description,
			CosineRank:  sim,
		})
	}

	slices.SortStableFunc(cosineSorted, func(a, b SimilarityTopic) int {
		if a.CosineRank < b.CosineRank {
			return 1
		}
		if a.CosineRank > b.CosineRank {
			return -1
		}

		return 0
	})

	return &cosineSorted, nil
}

func (s *MemService) Search(ctx context.Context, fact string) SimilarityResult {
	normalizedFact := util.NormalizeStr(fact)
	factVector, err := s.embedService.Embed(normalizedFact)
	if err != nil {
		return SimilarityResult{
			Err: err,
		}
	}

	memories, err := s.store.Memories.GetAll(ctx)
	if err != nil {
		return SimilarityResult{
			Err: err,
		}
	}
	// insert here
	cosineSorted, err := s.GetCosineSortedMemories(ctx, *factVector, *memories)
	if err != nil {
		return SimilarityResult{
			Err: err,
		}
	}
	bm25Sorted, err := s.store.Memories.GetAllByBM25Desc(ctx, normalizedFact)
	if err != nil {
		return SimilarityResult{
			Err: err,
		}
	}

	var bm25Ranked []SimilarityMemory

	for i, m := range *bm25Sorted {
		bm25Ranked = append(bm25Ranked, SimilarityMemory{
			TopicId: m.TopicId,
			Content: m.Content,
			Id:      m.Id,
			RFFRank: s.GetRFFRank(i),
		})
	}

	for i := range *cosineSorted {
		(*cosineSorted)[i].RFFRank = s.GetRFFRank(i)
	}

	bm25Index := make(SimilarityMemoryIndex, len(bm25Ranked))

	for _, m := range bm25Ranked {
		bm25Index[m.Id] = m
	}

	for i, m := range *cosineSorted {
		bm25Equivalent, ok := bm25Index[m.Id]

		if ok {
			(*cosineSorted)[i].RFFRank = m.RFFRank + bm25Equivalent.RFFRank
			(*cosineSorted)[i].DidBM25EffectIt = true
		} else {
			(*cosineSorted)[i].RFFRank = m.RFFRank
			(*cosineSorted)[i].DidBM25EffectIt = false
		}
	}

	slices.SortStableFunc(*cosineSorted, func(a, b SimilarityMemory) int {
		if a.RFFRank < b.RFFRank {
			return 1
		}
		if a.RFFRank > b.RFFRank {
			return -1
		}

		return 0
	})

	selectedMemories := (*cosineSorted)[:min(s.similarityAcceptedCountThreshold, len(*cosineSorted))]

	var topicIds []uint
	var memoryIds []uint
	for _, m := range selectedMemories {
		topicIds = append(topicIds, m.TopicId)
		memoryIds = append(memoryIds, m.Id)
	}
	var wg sync.WaitGroup

	wg.Go(func() {
		// I ignore these specific errors cause what am I gonna do about it, abort ship
		_ = s.store.Memories.UpdateHitCountAndLRA(ctx, memoryIds)
		_ = s.store.Topics.UpdateHitCountAndLRA(ctx, topicIds)
	})

	persTopics, err := s.store.Topics.GetByIds(ctx, topicIds)
	if err != nil {
		wg.Wait()
		return SimilarityResult{
			Err: err,
		}
	}

	var topics []SimilarityTopic
	for _, t := range *persTopics {
		topics = append(topics, SimilarityTopic{
			Name:        t.Name,
			Description: t.Description,
			Id:          t.Id,
		})
	}

	wg.Wait()

	return SimilarityResult{
		Topics:   topics,
		Memories: selectedMemories,
		Err:      nil,
	}
}
