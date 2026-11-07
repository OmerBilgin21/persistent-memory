package main

import (
	"context"
	"crypto/rand"
	"os"
	"persistent-memory/internal/domain"
	"persistent-memory/internal/persistence"
	"persistent-memory/util"
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

	memService, err := domain.NewMemService(persistence.NewMemoriesRepository(db),
		persistence.NewTopicsRepository(db), embedService, 5)
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err != nil {
		panic(err)
	}

	insertNow := time.Now()
	topicTitle := rand.Text()
	err = memService.CreateTopic(ctx, domain.CreateTopic{
		Description: rand.Text(),
		Name:        topicTitle,
	})
	if err != nil {
		panic(err)
	}
	topic, err := memService.GetTopicByName(ctx, topicTitle)
	if err != nil {
		panic(err)
	}
	for i := 0; i < 100; i++ {
		err = memService.CreateMemory(ctx, domain.CreateMemory{
			TopicId: topic.Id,
			Content: "butterflies " + rand.Text(),
		})

		if err != nil {
			panic(err)
		}
	}
	util.Logger("elapsed time for inserts: \n", time.Since(insertNow))

	now := time.Now()
	searchResult := memService.Search(ctx, "butterflies SAJDKSDJ")

	util.Logger("searchResult error: ", searchResult)
	util.Logger("elapsed time for search: \n", time.Since(now))
}
