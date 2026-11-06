package domain

import (
	"context"
	"persistent-memory/internal/persistence"
	"persistent-memory/util"
	"slices"

	"golang.org/x/sync/errgroup"
)
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
