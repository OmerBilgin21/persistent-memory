package domain

import (
	"context"
	"persistent-memory/internal/persistence"
	"slices"
)

type SimilarityTopic struct {
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	Id              uint    `json:"id"`
	CosineRank      float32 `json:"cosine_rank"`
	RFFRank         float32 `json:"rff_rank"`
	DidBM25EffectIt bool    `json:"did_bm_25_effect_it"`
}

type SimilarityMemory struct {
	TopicId         uint    `json:"topic_id"`
	Content         string  `json:"content"`
	Id              uint    `json:"id"`
	CosineRank      float32 `json:"cosine_rank"`
	RFFRank         float32 `json:"rff_rank"`
	DidBM25EffectIt bool    `json:"did_bm_25_effect_it"`
}

type SimilarityMemoryIndex map[uint]SimilarityMemory

type SimilarityResult struct {
	Topics   []SimilarityTopic
	Memories []SimilarityMemory
	Err      error
}

type CreateMemory struct {
	Content string `json:"content"`
	TopicId uint   `json:"topic_id"`
}

type CreateTopic struct {
	Name        string
	Description string
}

type MemService struct {
	embedService                     EmbedServiceInterface
	memoriesRepo                     persistence.MemoriesRepositoryInterface
	topicsRepo                       persistence.TopicsRepositoryInterface
	similarityAcceptedCountThreshold int
}

func NewMemService(memoriesRepository *persistence.MemoriesRepository, topicsRepository *persistence.TopicsRepository,
	embedService *EmbedService, acceptedCountThreshold int,
) (*MemService, error) {
	return &MemService{
		memoriesRepo:                     memoriesRepository,
		topicsRepo:                       topicsRepository,
		embedService:                     embedService,
		similarityAcceptedCountThreshold: acceptedCountThreshold,
	}, nil
}

func (s *MemService) GetTopicByName(ctx context.Context, name string) (*SimilarityTopic, error) {
	topic, err := s.topicsRepo.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}

	return &SimilarityTopic{
		Description: topic.Description,
		Id:          topic.Id,
		Name:        topic.Name,
	}, nil
}

func (s *MemService) CreateTopic(ctx context.Context, topic CreateTopic) error {
	topicVector, err := s.embedService.Embed(topic.Name + " " + topic.Description)
	if err != nil {
		return err
	}

	newTopic := persistence.Topic{
		Name:             topic.Name,
		Description:      topic.Description,
		NormalizedVector: *topicVector,
	}

	err = s.topicsRepo.Create(ctx, &newTopic)
	if err != nil {
		return err
	}

	return nil
}

// TODO: update to upsert
func (s *MemService) CreateMemory(ctx context.Context, fact CreateMemory) error {
	factVector, err := s.embedService.Embed(fact.Content)
	if err != nil {
		return err
	}

	factMemory := persistence.Memory{
		Content:          fact.Content,
		TopicId:          fact.TopicId,
		NormalizedVector: *factVector,
	}

	err = s.memoriesRepo.Create(ctx, &factMemory)
	if err != nil {
		return err
	}

	return nil
}

func getRFFRank(position int) float32 {
	return float32(float32(1) / (float32(60) + float32(position)))
}

func (s *MemService) Search(ctx context.Context, fact string) SimilarityResult {
	encodedFact, err := s.embedService.Embed(fact)
	if err != nil {
		return SimilarityResult{
			Err: err,
		}
	}

	memories, err := s.memoriesRepo.GetAll(ctx)
	if err != nil {
		return SimilarityResult{
			Err: err,
		}
	}

	cosineSorted := make([]SimilarityMemory, 0, len(*memories))
	normalizedEncodedFact := s.embedService.Normalize(*encodedFact)

	for _, m := range *memories {
		sim, err := s.embedService.NormalizedCosineSimilarity(normalizedEncodedFact,
			m.NormalizedVector)
		if err != nil {
			continue
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

	bm25Sorted, err := s.memoriesRepo.GetAllByBM25Desc(ctx, fact)
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
			RFFRank: getRFFRank(i),
		})
	}

	for i := range cosineSorted {
		cosineSorted[i].RFFRank = getRFFRank(i)
	}

	bm25Index := make(SimilarityMemoryIndex, len(bm25Ranked))

	for _, m := range bm25Ranked {
		bm25Index[m.Id] = m
	}

	for i, m := range cosineSorted {
		bm25Equivalent, ok := bm25Index[m.Id]

		if ok {
			cosineSorted[i].RFFRank = m.RFFRank + bm25Equivalent.RFFRank
			cosineSorted[i].DidBM25EffectIt = true
		} else {
			cosineSorted[i].RFFRank = m.RFFRank
			cosineSorted[i].DidBM25EffectIt = false
		}
	}

	slices.SortStableFunc(cosineSorted, func(a, b SimilarityMemory) int {
		if a.RFFRank < b.RFFRank {
			return 1
		}
		if a.RFFRank > b.RFFRank {
			return -1
		}

		return 0
	})

	selectedMemories := cosineSorted[:min(s.similarityAcceptedCountThreshold, len(cosineSorted))]

	var ids []uint
	for _, m := range selectedMemories {
		ids = append(ids, m.TopicId)
	}

	persTopics, err := s.topicsRepo.GetByIds(ctx, ids)
	if err != nil {
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

	return SimilarityResult{
		Topics:   topics,
		Memories: selectedMemories,
		Err:      nil,
	}
}
