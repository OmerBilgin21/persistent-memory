package main

import (
	"context"
	"fmt"
	"os"
	"persistent-memory/internal/domain"
	"persistent-memory/internal/persistence"
	"time"

	"github.com/townsendmerino/aikit/encoder"
)

func main() {
	db, err := persistence.DB()
	if err != nil {
		panic(err)
	}

	gteSmallDir := os.Getenv("gte_small_directory")
	if gteSmallDir == "" {
		panic("gte_small_directory environment variable is not set, please set it before execution")
	}
	loadedEncoder, err := encoder.LoadBERT(gteSmallDir)
	if err != nil {
		panic(err)
	}
	defer loadedEncoder.Close()

	embedService, err := domain.NewEmbedService(loadedEncoder)
	if err != nil {
		panic(err)
	}
	res, err := embedService.CosineSimilarity([]float32{1, 0}, []float32{0, 1})
	if err != nil {
		panic(err)
	}
	fmt.Printf("res: %+v\n", res)

	memService, err := domain.NewMemService(persistence.NewMemoriesRepository(db), persistence.NewTopicsRepository(db), embedService, 0.1, 5)
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	topicTitle := "yo completely unrelated topic title"
	err = memService.CreateTopic(ctx, domain.CreateTopic{
		Description: "yo completely unrelated topic desc",
		Name:        topicTitle,
	})

	if err != nil {
		panic(err)
	}

	topic, err := memService.GetTopicByName(ctx, topicTitle)

	if err != nil {
		panic(err)
	}

	err = memService.CreateMemory(ctx, domain.CreateMemory{
		TopicId: topic.Id,
		Content: "butterflies are likeable",
	})
	if err != nil {
		panic(err)
	}

	searchResult := memService.MeaningSearch(ctx, "I like butterflies")
	fmt.Printf("searchResult: %+v\n", searchResult)
}
