package domain

import (
	"context"
	"errors"
	"fmt"
	"time"

	"persistent-memory/internal/persistence"
	"persistent-memory/util"

	"gorm.io/gorm"
)

func (s *MemService) CreateOrRejectMemory(ctx context.Context, payload CreateMemory) (*uint, Result) {
	normalizedFact := util.NormalizeStr(payload.Fact)
	if err := isEmptyAfterNorm(normalizedFact, "Fact"); err != nil {
		return nil, *err
	}

	if len(normalizedFact) > s.tuned.TopMemoryCharacterCount {
		return nil, RejectedMemoryUpsertResult{
			Reason: fmt.Sprintf("Fact can not be longer than %v characters", s.tuned.TopMemoryCharacterCount),
		}
	}

	factVector, err := s.embedService.Embed(normalizedFact)
	if err != nil {
		return nil, UnexpectedErrorResult{
			Err: err,
		}
	}

	var createdId *uint

	tres := s.withTransaction(ctx, func(repos *persistence.Store) Result {
		memories, err := repos.Memories.GetAll(ctx)
		if err != nil {
			return UnexpectedErrorResult{
				Err: err,
			}
		}

		cosineSortedMemories, err := s.GetCosineSortedMemories(ctx, *factVector, *memories)
		if err != nil {
			return UnexpectedErrorResult{
				Err: err,
			}
		}

		topic, err := repos.Topics.Get(ctx, payload.TopicId)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				selectedMemories := (*cosineSortedMemories)[:min(s.tuned.AcceptedCountThreshold, len(*cosineSortedMemories))]
				topTopics, err := getTopicsOfMemories(ctx, selectedMemories, repos)
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

		var similarMemories []SimilarityMemory

		for _, r := range *cosineSortedMemories {
			if r.CosineRank >= s.tuned.InsertMemoryThreshold {
				similarMemories = append(similarMemories, r)
			}
		}

		if payload.Force {
			tcreatedId, err := createMemory(ctx, normalizedFact, payload.TopicId, *factVector, repos)
			if err != nil {
				return UnexpectedErrorResult{
					Err: err,
				}
			}

			createdId = tcreatedId
			return nil
		}

		if len(similarMemories) > 0 {
			return RejectedMemoryUpsertResult{
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

		res := checkForMemoryTopicFitAndSuggest(
			ctx,
			SimilarityTopic{
				Id:          topic.Id,
				Name:        topic.Name,
				Description: topic.Description,
			},
			*cosineSortedMemories,
			repos,
			true,
			s.tuned.RejectionThresholdPercentage,
			nil,
			payload.Force,
		)

		if res != nil {
			return res
		}

		tcreatedId, err := createMemory(ctx, normalizedFact, payload.TopicId, *factVector, repos)
		if err != nil {
			return UnexpectedErrorResult{
				Err: err,
			}
		}

		createdId = tcreatedId
		return nil
	})

	if tres != nil {
		return nil, tres
	}

	return createdId, nil
}

func (s *MemService) CreateOrRejectTopic(ctx context.Context, payload CreateTopic) (*uint, Result) {
	normalizedName := util.NormalizeStr(payload.Name)

	if err := isEmptyAfterNorm(normalizedName, "Topic name"); err != nil {
		return nil, *err
	}

	if len(normalizedName) > s.tuned.TopMemoryCharacterCount {
		return nil, RejectedMemoryUpsertResult{
			Reason: fmt.Sprintf("Topic name can not be longer than %v characters", s.tuned.TopMemoryCharacterCount),
		}
	}

	normalizedDesc := util.NormalizeStr(payload.Description)

	if err := isEmptyAfterNorm(normalizedDesc, "Topic description"); err != nil {
		return nil, *err
	}

	nameVector, err := s.embedService.Embed(normalizedName)
	if err != nil {
		return nil, UnexpectedErrorResult{
			Err: err,
		}
	}

	var createdId *uint

	res := s.withTransaction(ctx, func(repos *persistence.Store) Result {
		topicWithTheSameName, _ := getTopicByName(ctx, normalizedName, repos)
		if topicWithTheSameName != nil {
			return RejectedMemoryUpsertResult{
				Reason: fmt.Sprintf(`There is already a topic with this exact same name,
				duplicates are not allowed.
				Topic with the same name: %+v
				Your payload: %+v
				`, topicWithTheSameName, payload),
			}
		}

		if payload.Force {
			tcreatedId, err := createTopic(ctx, normalizedName, normalizedDesc, *nameVector, repos)
			if err != nil {
				return UnexpectedErrorResult{
					Err: err,
				}
			}

			createdId = tcreatedId
			return nil
		}

		topics, err := repos.Topics.GetAll(ctx)
		if err != nil {
			return UnexpectedErrorResult{
				Err: err,
			}
		}

		cosineSortedTopics, err := s.GetCosineSortedTopics(ctx, *nameVector, *topics)
		if err != nil {
			return UnexpectedErrorResult{
				Err: err,
			}
		}

		var similarTopics []SimilarityTopic

		for _, r := range *cosineSortedTopics {
			if r.CosineRank >= s.tuned.InsertTopicThreshold {
				similarTopics = append(similarTopics, r)
			}
		}

		if len(similarTopics) > 0 {
			return RejectedMemoryUpsertResult{
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

		tcreatedId, err := createTopic(ctx, normalizedName, normalizedDesc, *nameVector, repos)
		if err != nil {
			return UnexpectedErrorResult{
				Err: err,
			}
		}
		createdId = tcreatedId

		return nil
	})

	if res != nil {
		return nil, res
	}

	return createdId, nil
}

func (s *MemService) UpdateTopic(ctx context.Context, payload UpdateTopic) (*SimilarityTopic, Result) {
	isNameProcessable := false
	var normalizedName string
	var nameVector *[]float32
	if util.IsStrPtrNotEmpty(payload.Name) {
		normalizedName = util.NormalizeStr(*payload.Name)

		var err error
		nameVector, err = s.embedService.Embed(normalizedName)
		if err != nil {
			return nil, UnexpectedErrorResult{
				Err: err,
			}
		}
		isNameProcessable = true
	}

	var similarityTopic *SimilarityTopic

	res := s.withTransaction(ctx, func(repos *persistence.Store) Result {
		existingTopic, err := repos.Topics.Get(ctx, payload.TopicId)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return RejectedMemoryUpsertResult{
					Reason: "a topic with that ID is not found",
				}
			}

			return UnexpectedErrorResult{
				Err: err,
			}
		}

		toBeUpdated := *existingTopic
		nameChanged := false
		descChanged := false

		if isNameProcessable {
			nameChanged = true
			topicWithTheSameName, _ := getTopicByName(ctx, normalizedName, repos)
			if topicWithTheSameName != nil && topicWithTheSameName.Id != payload.TopicId {
				return RejectedMemoryUpsertResult{
					Reason: "There is already a topic with this exact same name, duplicates are not allowed",
				}
			}

			if payload.Force {
				toBeUpdated.Name = normalizedName
				toBeUpdated.NormalizedVector = *nameVector
			} else {
				topics, err := repos.Topics.GetAll(ctx)
				if err != nil {
					return UnexpectedErrorResult{
						Err: err,
					}
				}

				cosineSortedTopics, err := s.GetCosineSortedTopics(ctx, *nameVector, *topics)
				if err != nil {
					return UnexpectedErrorResult{
						Err: err,
					}
				}

				var similarTopics []SimilarityTopic

				for _, r := range *cosineSortedTopics {
					if r.Id != payload.TopicId && r.CosineRank >= s.tuned.InsertTopicThreshold {
						similarTopics = append(similarTopics, r)
					}
				}

				if len(similarTopics) > 0 {
					return RejectedMemoryUpsertResult{
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

		if util.IsStrPtrNotEmpty(payload.Description) {
			descChanged = true
			normalizedDesc := util.NormalizeStr(*payload.Description)

			toBeUpdated.Description = normalizedDesc
		}

		if nameChanged || descChanged {
			err := repos.Topics.Update(ctx, &toBeUpdated)
			if err != nil {
				return UnexpectedErrorResult{
					Err: err,
				}
			}

			similarityTopic = &SimilarityTopic{
				Name:        toBeUpdated.Name,
				Description: toBeUpdated.Description,
				Id:          toBeUpdated.Id,
			}
		} else {
			return RejectedMemoryUpsertResult{
				Reason: "Both the name and the description can not be empty",
			}
		}

		return nil
	})

	if res != nil {
		return nil, res
	}

	return similarityTopic, nil
}

func (s *MemService) UpdateMemory(ctx context.Context, payload UpdateMemory) (*SimilarityMemory, Result) {
	isContentUpdateProcessable := false
	var normalizedFact string
	var factVector *[]float32

	if util.IsStrPtrNotEmpty(payload.Fact) {
		normalizedFact = util.NormalizeStr(*payload.Fact)

		if len(normalizedFact) > s.tuned.TopMemoryCharacterCount {
			return nil, RejectedMemoryUpsertResult{
				Reason: fmt.Sprintf("Fact can not be longer than %v characters", s.tuned.TopMemoryCharacterCount),
			}
		}

		var err error
		factVector, err = s.embedService.Embed(normalizedFact)
		if err != nil {
			return nil, UnexpectedErrorResult{
				Err: err,
			}
		}
		isContentUpdateProcessable = true
	}

	var similarityMemory *SimilarityMemory

	res := s.withTransaction(ctx, func(repos *persistence.Store) Result {
		existingMemory, err := repos.Memories.Get(ctx, payload.MemoryId)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return RejectedMemoryUpsertResult{
					Reason: "a memory with that ID is not found",
				}
			}

			return UnexpectedErrorResult{
				Err: err,
			}
		}

		if factVector == nil || !isContentUpdateProcessable {
			factVector = (*[]float32)(&existingMemory.NormalizedVector)
		}

		var topic *persistence.Topic
		if payload.TopicId != nil {
			topic, err = repos.Topics.Get(ctx, *payload.TopicId)
		} else {
			topic, err = repos.Topics.Get(ctx, existingMemory.TopicId)
		}

		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return RejectedMemoryUpsertResult{
					Reason: "the given to-be-updated topic ID is not found",
				}
			}

			return UnexpectedErrorResult{
				Err: err,
			}
		}
		memories, err := repos.Memories.GetAll(ctx)
		if err != nil {
			return UnexpectedErrorResult{
				Err: err,
			}
		}

		cosineSortedMemories, err := s.GetCosineSortedMemories(ctx, *factVector, *memories)
		if err != nil {
			return UnexpectedErrorResult{
				Err: err,
			}
		}

		factChanged := false
		topicChanged := false

		toBeUpdated := *existingMemory

		if isContentUpdateProcessable && normalizedFact != existingMemory.Content {
			factChanged = true

			if payload.Force {
				toBeUpdated.Content = normalizedFact
				toBeUpdated.NormalizedVector = *factVector
			} else {
				var similarMemories []SimilarityMemory

				for _, r := range *cosineSortedMemories {
					if r.Id != payload.MemoryId && r.CosineRank >= s.tuned.InsertMemoryThreshold {
						similarMemories = append(similarMemories, r)
					}
				}

				if len(similarMemories) > 0 {
					return RejectedMemoryUpsertResult{
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

		if payload.TopicId != nil && *payload.TopicId != existingMemory.TopicId {
			topicChanged = true
			toBeUpdated.TopicId = *payload.TopicId
		}

		// I have to do this because neither gorm nor sqlite is smart enough to
		// not update records on partial updates where the given column values
		// are the same as existing column values
		if !factChanged && !topicChanged {
			return RejectedMemoryUpsertResult{
				Reason: "there is nothing to update on the memory with your payload",
			}
		}

		res := checkForMemoryTopicFitAndSuggest(
			ctx,
			SimilarityTopic{
				Id:          topic.Id,
				Name:        topic.Name,
				Description: topic.Description,
			},
			*cosineSortedMemories,
			repos,
			false,
			s.tuned.RejectionThresholdPercentage,
			&SimilarityMemory{
				Content: existingMemory.Content,
				Id:      existingMemory.Id,
				TopicId: existingMemory.TopicId,
			},
			payload.Force,
		)

		if res != nil {
			return res
		}

		err = repos.Memories.Update(ctx, &toBeUpdated)
		if err != nil {
			return UnexpectedErrorResult{
				Err: err,
			}
		}

		similarityMemory = &SimilarityMemory{
			TopicId: toBeUpdated.TopicId,
			Content: toBeUpdated.Content,
			Id:      toBeUpdated.Id,
		}
		return nil
	})

	if res != nil {
		return nil, res
	}

	return similarityMemory, nil
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

func (s *MemService) GetTopicWithMemories(ctx context.Context, id uint, sessionId string) (*SimilarityTopicWithMemories, error) {
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

	s.markAsShown(ctx, sessionId, memories)

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
	res := s.withTransaction(ctx, func(repos *persistence.Store) Result {
		memories, err := repos.Memories.GetAll(ctx)
		if err != nil {
			return UnexpectedErrorResult{
				Err: err,
			}
		}

		var memoryErr error
		if len(*memories) > 0 {
			var toBeDeletedMemIds []uint
			for _, m := range *memories {
				if time.Duration(time.Since(m.LastRetrievedAt)).Hours() > float64(24*s.tuned.DecayAfterDays) && m.HitCount < s.tuned.DecayBelowHitCount {
					toBeDeletedMemIds = append(toBeDeletedMemIds, m.Id)
				}
			}

			memoryErr = repos.Memories.BulkDelete(ctx, toBeDeletedMemIds)
		}

		topics, err := repos.Topics.GetTopicsWithoutMemories(ctx)
		if err != nil {
			return UnexpectedErrorResult{
				Err: err,
			}
		}

		var topicErr error
		if len(*topics) > 0 {
			var toBeDeletedTopicsIds []uint
			for _, t := range *topics {
				toBeDeletedTopicsIds = append(toBeDeletedTopicsIds, t.Id)
			}

			topicErr = repos.Topics.BulkDelete(ctx, toBeDeletedTopicsIds)
		}

		if memoryErr != nil {
			return UnexpectedErrorResult{
				Err: fmt.Errorf("error while deleting one of the decayed memories: %w", memoryErr),
			}
		}

		if topicErr != nil {
			return UnexpectedErrorResult{
				Err: topicErr,
			}
		}

		return nil
	})

	switch r := res.(type) {
	case UnexpectedErrorResult:
		return r.Err
	case RejectedMemoryUpsertResult:
		return errors.New(r.Reason)
	default:
		return nil
	}
}

func (s *MemService) Export(ctx context.Context) error {
	// TODO: stub, implement
	return nil
}

func (s *MemService) Tidy(ctx context.Context) error {
	// this is just calling decay for now but I made it a two fold call
	// just in case I wanna implement something else to it in the future
	return s.decay(ctx)
}
