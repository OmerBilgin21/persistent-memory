package main

import (
	"os"
	"persistent-memory/internal/domain"
	"persistent-memory/internal/persistence"

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

	store := persistence.NewStore(db)

	_, err = domain.NewMemService(store, embedService, 5)
	if err != nil {
		panic(err)
	}
}
