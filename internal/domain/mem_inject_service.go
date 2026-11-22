package domain

import "context"

func (s *MemService) Inject(ctx context.Context, prompt, sessionId string) SimilarityResult {
	// TODO: stub, implement
	return SimilarityResult{}
}

func (s *MemService) ClearInjected(ctx context.Context, sessionId string) error {
	// TODO: stub, implement
	return nil
}
