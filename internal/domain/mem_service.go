package domain

import (
	"context"
	"persistent-memory/internal/persistence"
	"persistent-memory/util"
	"slices"

	"golang.org/x/sync/errgroup"
)
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

