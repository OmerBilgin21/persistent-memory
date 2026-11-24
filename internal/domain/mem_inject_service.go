package domain

import (
	"context"
	"fmt"
	"persistent-memory/internal/persistence"
	"persistent-memory/util"
	"slices"
)

func (s *MemService) markAsShown(ctx context.Context, sessionId string, memories []SimilarityMemory) Result {
	if len(memories) == 0 {
		return nil
	}

	memoryIds := make([]uint, 0, len(memories))
	topicIds := make([]uint, 0, len(memories))
	for _, m := range memories {
		memoryIds = append(memoryIds, m.Id)
		topicIds = append(topicIds, m.TopicId)
	}

	return s.withTransaction(ctx, func(repos *persistence.Store) Result {
		if sessionId != "" {
			if err := repos.Injections.CreateMany(ctx, sessionId, memoryIds); err != nil {
				return UnexpectedErrorResult{
					Err: fmt.Errorf("error while recording shown memories: %w", err),
				}
			}
		}

		if err := repos.Memories.UpdateHitCountAndLRA(ctx, memoryIds); err != nil {
			return UnexpectedErrorResult{
				Err: fmt.Errorf("error while updating memory usage: %w", err),
			}
		}

		if err := repos.Topics.UpdateHitCountAndLRA(ctx, topicIds); err != nil {
			return UnexpectedErrorResult{
				Err: fmt.Errorf("error while updating topic usage: %w", err),
			}
		}

		return nil
	})
}

func (s *MemService) Inject(ctx context.Context, prompt, sessionId string) SimilarityResult {
	memories, err := s.store.Memories.GetAll(ctx)
	if err != nil {
		return SimilarityResult{
			Err: err,
		}
	}

	normalizedPrompt := util.NormalizeStr(prompt)
	if len(normalizedPrompt) == 0 || normalizedPrompt == " " {
		return SimilarityResult{
			Err: fmt.Errorf("The prompt is empty"),
		}
	}

	promptVector, err := s.embedService.Embed(normalizedPrompt)
	if err != nil {
		return SimilarityResult{
			Err: err,
		}
	}

	cosineSortedMemories, err := s.GetCosineSortedMemories(ctx, *promptVector, *memories)
	if err != nil {
		return SimilarityResult{
			Err: err,
		}
	}

	if len(*cosineSortedMemories) == 0 {
		return SimilarityResult{}
	}

	medianValue := (*cosineSortedMemories)[int(len(*cosineSortedMemories)/2)].CosineRank
	seenValues, err := s.store.Injections.GetMemoryIdsBySessionId(ctx, sessionId)
	if err != nil {
		return SimilarityResult{
			Err: err,
		}
	}

	var toBeInjected []SimilarityMemory
	for _, m := range *cosineSortedMemories {
		if slices.Contains(seenValues, m.Id) {
			continue
		}

		if len(toBeInjected) >= s.tuned.MemoryRetrievalLimit || m.CosineRank < s.tuned.MemoryInjectionThreshold || m.CosineRank < medianValue+medianValue*s.tuned.MemoryInjectionSuperiorityPercentage {
			break
		}

		toBeInjected = append(toBeInjected, SimilarityMemory{
			Id:              m.Id,
			CosineRank:      m.CosineRank,
			TopicId:         m.TopicId,
			Content:         m.Content,
			DidBM25EffectIt: false,
		})
	}

	s.markAsShown(ctx, sessionId, toBeInjected)

	topics, err := getTopicsOfMemories(ctx, toBeInjected, s.store)
	if err != nil {
		return SimilarityResult{
			Err: err,
		}
	}

	return SimilarityResult{
		Memories: toBeInjected,
		Topics:   *topics,
		Err:      nil,
	}
}

func (s *MemService) ClearInjected(ctx context.Context, sessionId string) error {
	return s.store.Injections.DeleteBySessionId(ctx, sessionId)
}
