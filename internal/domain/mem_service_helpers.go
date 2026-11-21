package domain

import (
	"context"

	"persistent-memory/internal/persistence"
)

func createMemory(ctx context.Context, normalizedFact string, topicId uint, factVector []float32, store *persistence.Store) (*uint, error) {
	toBeCreated := persistence.Memory{
		TopicId:          topicId,
		NormalizedVector: factVector,
		Content:          normalizedFact,
	}

	err := store.Memories.Create(ctx, &toBeCreated)
	if err != nil {
		return nil, err
	}

	return &toBeCreated.Id, nil
}

func getTopicsOfMemories(ctx context.Context, memories []SimilarityMemory, store *persistence.Store) (*[]SimilarityTopic, error) {
	var topicIds []uint
	for _, m := range memories {
		topicIds = append(topicIds, m.TopicId)
	}

	topics, err := store.Topics.GetByIds(ctx, topicIds)
	if err != nil {
		return nil, err
	}

	var similarityTopics []SimilarityTopic
	for _, t := range *topics {
		similarityTopics = append(similarityTopics, SimilarityTopic{
			Name:        t.Name,
			Id:          t.Id,
			Description: t.Description,
		})
	}

	return &similarityTopics, nil
}

func getTopicByName(ctx context.Context, name string, store *persistence.Store) (*SimilarityTopic, error) {
	topic, err := store.Topics.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}

	return &SimilarityTopic{
		Description: topic.Description,
		Id:          topic.Id,
		Name:        topic.Name,
	}, nil
}

func isEmptyAfterNorm(str, attrName string) *RejectedMemoryUpsertResult {
	if len(str) == 0 || str == " " {
		return &RejectedMemoryUpsertResult{
			Reason: attrName + " can not be empty",
		}
	}

	return nil
}

func createTopic(ctx context.Context, normalizedName, normalizedDesc string, nameVector []float32, store *persistence.Store) (*uint, error) {
	toBeCreated := persistence.Topic{
		Name:             normalizedName,
		Description:      normalizedDesc,
		NormalizedVector: nameVector,
	}

	err := store.Topics.Create(ctx, &toBeCreated)
	if err != nil {
		return nil, err
	}

	return &toBeCreated.Id, nil
}
