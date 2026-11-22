package domain

import (
	"context"
	"fmt"

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

func checkForMemoryTopicFitAndSuggest(
	ctx context.Context,
	topic SimilarityTopic,
	cosineSortedMemories []SimilarityMemory,
	store *persistence.Store,
	createMode bool,
	// between 0-1 represantation
	rejectionThresholdPercentage float32,
	existingMemory *SimilarityMemory,
	force bool,
) Result {
	if rejectionThresholdPercentage > 1 || rejectionThresholdPercentage < 0 {
		return nil
	}

	if force {
		return nil
	}

	if existingMemory == nil && createMode == false {
		// illogical invocation of the function
		// we can't operate without ID on update mode
		return nil
	}
	if len(cosineSortedMemories) == 0 {
		return nil
	}

	var factsOfGivenTopic []SimilarityMemory
	var bestOfGivenTopic SimilarityMemory
	var bestMatch SimilarityMemory
	if !createMode && cosineSortedMemories[0].Id == existingMemory.Id {
		if len(cosineSortedMemories) > 1 {
			bestMatch = cosineSortedMemories[1]
		} else {
			return nil
		}
	} else {
		bestMatch = cosineSortedMemories[0]
	}

	for _, m := range cosineSortedMemories {
		if existingMemory != nil && existingMemory.Id == m.Id {
			continue
		}

		if m.TopicId == topic.Id {
			if m.CosineRank > bestOfGivenTopic.CosineRank {
				bestOfGivenTopic = m
			}
			factsOfGivenTopic = append(factsOfGivenTopic, m)
		}
	}

	if len(factsOfGivenTopic) == 0 ||
		bestMatch.TopicId == topic.Id ||
		bestOfGivenTopic.CosineRank+(bestOfGivenTopic.CosineRank*rejectionThresholdPercentage) > bestMatch.CosineRank {
		return nil
	}

	bestMatchTopic, err := store.Topics.Get(ctx, bestMatch.TopicId)
	if err != nil {
		// I think I'd rather not reject the model that is trying to either
		// move a memory or create a memory just because my
		// check for finding a better fit for a fact has failed.
		// If everything before or after this is going to succeed,
		// we just let it succeed. Though it'd certainly be weird
		// if this had ever happened.
		return nil
	}

	var reasonStr string
	bestMatchSTopic := SimilarityTopic{
		CosineRank:      bestMatch.CosineRank,
		Id:              bestMatchTopic.Id,
		Name:            bestMatchTopic.Name,
		Description:     bestMatchTopic.Description,
		DidBM25EffectIt: false,
	}
	if createMode {
		reasonStr = fmt.Sprintf(`Despite the given topic may be a fit for the fact
			that you are trying to create it in, there may be a better match for this fact
			Consider adding it to the topic:
			%+v
			Your given topic:
			%+v`, bestMatchSTopic, topic)
	} else {
		reasonStr = fmt.Sprintf(`Despite the given topic may be a fit for the fact
			that you are trying to update, there may be a better match for this fact content:
			%+v
			Your given topic:
			%+v`, bestMatchSTopic, topic)
	}

	return RejectedMemoryUpsertResult{
		Reason: reasonStr,
	}
}
