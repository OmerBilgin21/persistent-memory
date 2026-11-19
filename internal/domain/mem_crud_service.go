package domain

import (
	"context"
	"errors"
	"fmt"

	"persistent-memory/internal/persistence"
	"persistent-memory/util"

	"gorm.io/gorm"
)

func (s *MemService) isEmptyAfterNorm(str, attrName string) *RejectedMemoryUpsertResult {
	if len(str) == 0 || str == " " {
		return &RejectedMemoryUpsertResult{
			Reason: attrName + " can not be empty",
		}
	}

	return nil
}

func (s *MemService) createTopic(ctx context.Context, normalizedName, normalizedDesc string, nameVector []float32) (*uint, error) {
	toBeCreated := persistence.Topic{
		Name:             normalizedName,
		Description:      normalizedDesc,
		NormalizedVector: nameVector,
	}

	err := s.store.Topics.Create(ctx, &toBeCreated)
	if err != nil {
		return nil, err
	}

	return &toBeCreated.Id, nil
}

func (s *MemService) createMemory(ctx context.Context, normalizedFact string, topicId uint, factVector []float32) (*uint, error) {
	toBeCreated := persistence.Memory{
		TopicId:          topicId,
		NormalizedVector: factVector,
		Content:          normalizedFact,
	}

	err := s.store.Memories.Create(ctx, &toBeCreated)
	if err != nil {
		return nil, err
	}

	return &toBeCreated.Id, nil
}

func (s *MemService) getTopicsOfMemories(ctx context.Context, memories []SimilarityMemory) (*[]SimilarityTopic, error) {
	var topicIds []uint
	for _, m := range memories {
		topicIds = append(topicIds, m.TopicId)
	}
	topics, err := s.store.Topics.GetByIds(ctx, topicIds)
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

func (s *MemService) handleCreateMemoryResult(ctx context.Context, err error, cosineSortedMemories *[]SimilarityMemory, payload CreateMemory) Result {
	if errors.Is(err, gorm.ErrForeignKeyViolated) {
		selectedMemories := (*cosineSortedMemories)[:min(s.similarityAcceptedCountThreshold, len(*cosineSortedMemories))]
		topTopics, err := s.getTopicsOfMemories(ctx, selectedMemories)
		if err != nil {
			return RejectedMemoryUpsertResult{
				Reason: fmt.Sprintf(`The topic you are trying to create this memory in does not exist.
							Your payload:
							%+v
							`, payload),
			}
		}

		return RejectedMemoryUpsertResult{
			Reason: fmt.Sprintf(`The topic you are trying to create this memory in does not exist.
							Your payload:
							%+v
							Topics of top matching memories:
							%+v
							`, payload, topTopics),
		}
	}
	return UnexpectedErrorResult{
		Err: err,
	}
}

func (s *MemService) CreateOrRejectMemory(ctx context.Context, payload CreateMemory) (*uint, Result) {
	normalizedFact := util.NormalizeStr(payload.Fact)
	if err := s.isEmptyAfterNorm(normalizedFact, "Fact"); err != nil {
		return nil, *err
	}

	if len(normalizedFact) > 280 {
		return nil, RejectedMemoryUpsertResult{
			Reason: "Fact can not be longer than 280 characters",
		}
	}

	factVector, err := s.embedService.Embed(normalizedFact)
	if err != nil {
		return nil, UnexpectedErrorResult{
			Err: err,
		}
	}

	memories, err := s.store.Memories.GetAll(ctx)
	if err != nil {
		return nil, UnexpectedErrorResult{
			Err: err,
		}
	}

	cosineSortedMemories, err := s.GetCosineSortedMemories(ctx, *factVector, *memories)
	if err != nil {
		return nil, UnexpectedErrorResult{
			Err: err,
		}
	}

	var similarMemories []SimilarityMemory

	for _, r := range *cosineSortedMemories {
		if r.CosineRank >= InsertThreshold {
			similarMemories = append(similarMemories, r)
		}
	}

	if payload.Force {
		createdId, err := s.createMemory(ctx, normalizedFact, payload.TopicId, *factVector)
		if err != nil {
			return nil, s.handleCreateMemoryResult(ctx, err, cosineSortedMemories, payload)
		}

		return createdId, nil
	}

	if len(similarMemories) > 0 {
		return nil, RejectedMemoryUpsertResult{
			Reason: fmt.Sprintf(`The fact you have given 
				has many very similar memories to it.
				Therefore the create has been rejected.
				Consider either updating the memories or not creating a new one.
				If you think this is a mistake, use force: true for force-creating a memory.
				Your given fact:
				%v
				Similar memories that have been found:
				%+v
				`, payload.Fact, similarMemories),
		}
	}

	createdId, err := s.createMemory(ctx, normalizedFact, payload.TopicId, *factVector)
	if err != nil {
		return nil, s.handleCreateMemoryResult(ctx, err, cosineSortedMemories, payload)
	}

	return createdId, nil
}

func (s *MemService) CreateOrRejectTopic(ctx context.Context, payload CreateTopic) (*uint, Result) {
	normalizedName := util.NormalizeStr(payload.Name)

	if err := s.isEmptyAfterNorm(normalizedName, "Topic name"); err != nil {
		return nil, *err
	}

	if len(normalizedName) > 280 {
		return nil, RejectedMemoryUpsertResult{
			Reason: "Topic name can not be longer than 280 characters",
		}
	}

	normalizedDesc := util.NormalizeStr(payload.Description)

	if err := s.isEmptyAfterNorm(normalizedDesc, "Topic description"); err != nil {
		return nil, *err
	}

	topicWithTheSameName, _ := s.GetTopicByName(ctx, normalizedName)
	if topicWithTheSameName != nil {
		return nil, RejectedMemoryUpsertResult{
			Reason: fmt.Sprintf(`There is already a topic with this exact same name,
				duplicates are not allowed.
				Topic with the same name: %+v
				Your payload: %+v
				`, topicWithTheSameName, payload),
		}
	}

	nameVector, err := s.embedService.Embed(normalizedName)
	if err != nil {
		return nil, UnexpectedErrorResult{
			Err: err,
		}
	}

	if payload.Force {
		createdId, err := s.createTopic(ctx, normalizedName, normalizedDesc, *nameVector)
		if err != nil {
			return nil, UnexpectedErrorResult{
				Err: err,
			}
		}

		return createdId, nil
	}

	topics, err := s.store.Topics.GetAll(ctx)
	if err != nil {
		return nil, UnexpectedErrorResult{
			Err: err,
		}
	}

	cosineSortedTopics, err := s.GetCosineSortedTopics(ctx, *nameVector, *topics)
	if err != nil {
		return nil, UnexpectedErrorResult{Err: err}
	}

	var similarTopics []SimilarityTopic

	for _, r := range *cosineSortedTopics {
		if r.CosineRank >= InsertThreshold {
			similarTopics = append(similarTopics, r)
		}
	}

	if len(similarTopics) > 0 {
		return nil, RejectedMemoryUpsertResult{
			Reason: fmt.Sprintf(`The topic name you have given 
				has many very similar topics to it.
				Therefore the create has been rejected.
				Consider either updating the given topic or not creating a new one.
				If you think this is a mistake, use force: true for force-creating a topic.
				Your given topic name:
				%v
				Similar topics that have been found:
				%+v
				`, payload.Name, similarTopics),
		}
	}

	createdId, err := s.createTopic(ctx, normalizedName, normalizedDesc, *nameVector)
	if err != nil {
		return nil, UnexpectedErrorResult{
			Err: err,
		}
	}

	return createdId, nil
}

func (s *MemService) UpdateTopic(ctx context.Context, payload UpdateTopic) (*SimilarityTopic, Result) {
	existingTopic, err := s.store.Topics.Get(ctx, payload.TopicId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, RejectedMemoryUpsertResult{
				Reason: "a topic with that ID is not found",
			}
		}

		return nil, UnexpectedErrorResult{
			Err: err,
		}
	}

	toBeUpdated := *existingTopic
	if payload.Name != nil || payload.Description != nil {
		if payload.Name != nil {
			normalizedName := util.NormalizeStr(*payload.Name)
			if err := s.isEmptyAfterNorm(normalizedName, "Topic name"); err != nil {
				return nil, *err
			}

			topicWithTheSameName, _ := s.GetTopicByName(ctx, normalizedName)
			if topicWithTheSameName != nil && topicWithTheSameName.Id != payload.TopicId {
				return nil, RejectedMemoryUpsertResult{
					Reason: "There is already a topic with this exact same name, duplicates are not allowed",
				}
			}

			nameVector, err := s.embedService.Embed(normalizedName)
			if err != nil {
				return nil, UnexpectedErrorResult{
					Err: err,
				}
			}

			if payload.Force {
				toBeUpdated.Name = normalizedName
				toBeUpdated.NormalizedVector = *nameVector
			} else {
				topics, err := s.store.Topics.GetAll(ctx)
				if err != nil {
					return nil, UnexpectedErrorResult{
						Err: err,
					}
				}

				cosineSortedTopics, err := s.GetCosineSortedTopics(ctx, *nameVector, *topics)
				if err != nil {
					return nil, UnexpectedErrorResult{
						Err: err,
					}
				}

				var similarTopics []SimilarityTopic

				for _, r := range *cosineSortedTopics {
					if r.Id != payload.TopicId && r.CosineRank >= InsertThreshold {
						similarTopics = append(similarTopics, r)
					}
				}

				if len(similarTopics) > 0 {
					return nil, RejectedMemoryUpsertResult{
						Reason: fmt.Sprintf(`The topic name you have given has many very similar topics to it.
								Therefore the update has been rejected.
								Consider either merging the found topic(s) by removing some and updating the rest or not updating this one.
								If you think this is a mistake, use force: true for force-updating the topic.
								Your given topic name:
								%v
								Similar topics that have been found:
								%+v
								`, *payload.Name, similarTopics),
					}
				}

				toBeUpdated.Name = normalizedName
				toBeUpdated.NormalizedVector = *nameVector
			}
		}

		if payload.Description != nil {
			normalizedDesc := util.NormalizeStr(*payload.Description)
			if err := s.isEmptyAfterNorm(normalizedDesc, "Topic description"); err != nil {
				return nil, *err
			}

			toBeUpdated.Description = normalizedDesc
		}

		err := s.store.Topics.Update(ctx, &toBeUpdated)
		if err != nil {
			return nil, UnexpectedErrorResult{
				Err: err,
			}
		}

		return &SimilarityTopic{
			Name:        toBeUpdated.Name,
			Description: toBeUpdated.Description,
			Id:          toBeUpdated.Id,
		}, nil
	}

	return nil, RejectedMemoryUpsertResult{
		Reason: "Both the name and the description can not be empty",
	}
}

func (s *MemService) UpdateMemory(ctx context.Context, payload UpdateMemory) (*SimilarityMemory, Result) {
	existingMemory, err := s.store.Memories.Get(ctx, payload.MemoryId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, RejectedMemoryUpsertResult{
				Reason: "a memory with that ID is not found",
			}
		}

		return nil, UnexpectedErrorResult{
			Err: err,
		}
	}

	factExists := false
	topicExists := false

	toBeUpdated := *existingMemory

	if payload.Fact != nil || payload.TopicId != nil {
		if payload.Fact != nil {
			normalizedFact := util.NormalizeStr(*payload.Fact)

			if err := s.isEmptyAfterNorm(normalizedFact, "Fact"); err != nil {
				return nil, *err
			}

			if len(normalizedFact) > 280 {
				return nil, RejectedMemoryUpsertResult{
					Reason: "Fact can not be longer than 280 characters",
				}
			}

			if normalizedFact != existingMemory.Content {
				factExists = true
				factVector, err := s.embedService.Embed(normalizedFact)
				if err != nil {
					return nil, UnexpectedErrorResult{
						Err: err,
					}
				}

				if payload.Force {
					toBeUpdated.Content = normalizedFact
					toBeUpdated.NormalizedVector = *factVector
				} else {
					memories, err := s.store.Memories.GetAll(ctx)
					if err != nil {
						return nil, UnexpectedErrorResult{
							Err: err,
						}
					}

					cosineSortedMemories, err := s.GetCosineSortedMemories(ctx, *factVector, *memories)
					if err != nil {
						return nil, UnexpectedErrorResult{
							Err: err,
						}
					}

					var similarMemories []SimilarityMemory

					for _, r := range *cosineSortedMemories {
						if r.Id != payload.MemoryId && r.CosineRank >= InsertThreshold {
							similarMemories = append(similarMemories, r)
						}
					}

					if len(similarMemories) > 0 {
						return nil, RejectedMemoryUpsertResult{
							Reason: fmt.Sprintf(`The fact you have given has many very similar memories to it.
								Therefore the update has been rejected.
								Consider either updating one of the found memories instead or not updating this one.
								If you think this is a mistake, use force: true for force-updating the memory.
								Your given fact:
								%v
								Similar memories that have been found:
								%+v
								`, *payload.Fact, similarMemories),
						}
					}

					toBeUpdated.Content = normalizedFact
					toBeUpdated.NormalizedVector = *factVector
				}
			}
		}

		if payload.TopicId != nil && *payload.TopicId != existingMemory.TopicId {
			_, err := s.store.Topics.Get(ctx, *payload.TopicId)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, RejectedMemoryUpsertResult{
						Reason: "the given to-be-updated topic ID is not found",
					}
				}

				return nil, UnexpectedErrorResult{
					Err: err,
				}
			}

			topicExists = true
			toBeUpdated.TopicId = *payload.TopicId
		}

		if !factExists && !topicExists {
			return nil, RejectedMemoryUpsertResult{
				Reason: "there is nothing to update on the memory with your payload",
			}
		}

		err := s.store.Memories.Update(ctx, &toBeUpdated)
		if err != nil {
			return nil, UnexpectedErrorResult{
				Err: err,
			}
		}

		return &SimilarityMemory{
			TopicId: toBeUpdated.TopicId,
			Content: toBeUpdated.Content,
			Id:      toBeUpdated.Id,
		}, nil
	}

	return nil, RejectedMemoryUpsertResult{
		Reason: "there is nothing to update on the memory with your payload",
	}
}

func (s *MemService) DeleteTopic(ctx context.Context, id uint) error {
	return s.store.Topics.Delete(ctx, id)
}

func (s *MemService) DeleteMemory(ctx context.Context, id uint) error {
	return s.store.Memories.Delete(ctx, id)
}

func (s *MemService) GetMemory(ctx context.Context, id uint) (*SimilarityMemory, error) {
	memory, err := s.store.Memories.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return &SimilarityMemory{
		TopicId: memory.TopicId,
		Content: memory.Content,
		Id:      memory.Id,
	}, nil
}

func (s *MemService) GetTopicWithMemories(ctx context.Context, id uint) (*SimilarityTopicWithMemories, error) {
	topic, err := s.store.Topics.GetWithMemories(ctx, id)
	if err != nil {
		return nil, err
	}

	var memories []SimilarityMemory
	for _, m := range topic.Memories {
		memories = append(memories, SimilarityMemory{
			TopicId: m.TopicId,
			Content: m.Content,
			Id:      m.Id,
		})
	}
	return &SimilarityTopicWithMemories{
		SimilarityTopic: SimilarityTopic{
			Id:          topic.Id,
			Name:        topic.Name,
			Description: topic.Description,
		},
		Memories: memories,
	}, nil
}

func (s *MemService) decay(ctx context.Context) error {
	// TODO: stub, implement
	return nil
}

func (s *MemService) Export(ctx context.Context) error {
	// TODO: stub, implement
	return nil
}

func (s *MemService) Tidy(ctx context.Context) error {
	s.decay(ctx)
	// TODO: stub, implement
	return nil
}
