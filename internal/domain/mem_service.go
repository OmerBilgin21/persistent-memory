package domain

import (
	"context"
	"persistent-memory/internal/persistence"
	"persistent-memory/util"
	"slices"

	"golang.org/x/sync/errgroup"
)

type SimilarityItem interface {
	similarityItem()
	GetRank() *float32
}

func (s SimilarityTopic) similarityItem() {}
func (s SimilarityTopic) GetRank() *float32 {
	return s.Rank
}
func (m SimilarityMemory) similarityItem() {}
func (m SimilarityMemory) GetRank() *float32 {
	return m.Rank
}

type SimilarityTopic struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Id          uint   `json:"id"`
	Rank        *float32
}

type SimilarityMemory struct {
	TopicId uint   `json:"topic_id"`
	Content string `json:"content"`
	Id      uint   `json:"id"`
	Rank    *float32
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
	similarityAcceptanceThreshold    float32
	similarityAcceptedCountThreshold int
}

func NewMemService(memoriesRepository *persistence.MemoriesRepository, topicsRepository *persistence.TopicsRepository, embedService *EmbedService, acceptanceThreshold float32, acceptedCountThreshold int) (*MemService, error) {

	return &MemService{
		memoriesRepo:                     memoriesRepository,
		topicsRepo:                       topicsRepository,
		embedService:                     embedService,
		similarityAcceptanceThreshold:    acceptanceThreshold,
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
		Id:          topic.ID,
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

func (s *MemService) GetAllResources(ctx context.Context) ([]persistence.Topic, []persistence.Memory, error) {
	g, subCtx := errgroup.WithContext(ctx)

	var memories []persistence.Memory
	g.Go(func() error {
		res, err := s.memoriesRepo.GetAll(subCtx)
		if err != nil {
			return err
		}
		memories = *res
		return nil
	})

	var topics []persistence.Topic
	g.Go(func() error {
		res, err := s.topicsRepo.GetAll(subCtx)
		if err != nil {
			return err
		}
		topics = *res
		return nil
	})
	err := g.Wait()

	return topics, memories, err
}

func (s *MemService) MergeResources(memories []persistence.Memory, topics []persistence.Topic) []persistence.PersSimilarityItem {
	items := make([]persistence.PersSimilarityItem, 0, len(memories)+len(topics))

	for _, m := range memories {
		items = append(items, m)
	}
	for _, t := range topics {
		items = append(items, t)
	}

	return items
}

func (s *MemService) MeaningSearch(ctx context.Context, fact string) []SimilarityItem {
	encodedFact, err := s.embedService.Embed(fact)
	if err != nil {
		return nil
	}

	topics, memories, err := s.GetAllResources(ctx)
	if err != nil {
		return nil
	}

	items := s.MergeResources(memories, topics)

	results := make([]SimilarityItem, 0, len(items))
	normalizedEncodedFact := s.embedService.Normalize(*encodedFact)

	for _, item := range items {
		sim, err := s.embedService.NormalizedCosineSimilarity(normalizedEncodedFact, item.GetNormalizedVector())
		if err != nil {
			continue
		}
		if sim >= s.similarityAcceptanceThreshold {
			switch v := item.(type) {
			case persistence.Memory:
				results = append(results, SimilarityMemory{
					TopicId: v.TopicId,
					Content: v.Content,
					Id:      v.ID,
					Rank:    util.Ptr(sim),
				})
			case persistence.Topic:
				results = append(results, SimilarityTopic{
					Id:          v.ID,
					Name:        v.Name,
					Description: v.Description,
					Rank:        util.Ptr(sim),
				})
			}
		}
	}

	slices.SortStableFunc(results, func(a, b SimilarityItem) int {
		if a.GetRank() == nil || b.GetRank() == nil {
			return 0
		}

		if *a.GetRank() < *b.GetRank() {
			return 1
		}
		if *a.GetRank() > *b.GetRank() {
			return -1
		}

		return 0
	})

	return results[:min(s.similarityAcceptedCountThreshold, len(results))]
}
