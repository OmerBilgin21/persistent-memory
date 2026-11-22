package domain

import (
	"context"
	"fmt"
	"slices"

	"persistent-memory/internal/persistence"
	"persistent-memory/util"
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
	// TODO: stub, implement
	return SimilarityResult{}
}

func (s *MemService) ClearInjected(ctx context.Context, sessionId string) error {
	// TODO: stub, implement
	return nil
}
