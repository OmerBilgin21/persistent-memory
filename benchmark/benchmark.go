package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"persistent-memory/internal/domain"
	"persistent-memory/util"

	"github.com/townsendmerino/aikit/encoder"
)

type benchmarkData struct {
	Text string    `json:"text"`
	Vec  []float32 `json:"vec"`
}

func main() {
	dir, err := os.Getwd()

	if err != nil {
		panic(err)
	}

	example := filepath.Join(dir, "reference.json")
	util.Logger("example: ", example)
	data, err := os.ReadFile(example)

	if err != nil {
		panic(err)
	}

	var parsed []benchmarkData

	err = json.Unmarshal(data, &parsed)
	if err != nil {
		panic(err)
	}

	model, err := encoder.LoadBERT("gte-small")
	if err != nil {
		panic(err)
	}
	defer model.Close()

	embedService, err := domain.NewEmbedService(model)
	if err != nil {
		panic(err)
	}

	results := map[string]float32{}
	for _, refres := range parsed {
		gores, err := model.Encode(refres.Text)

		if err != nil {
			panic(err)
		}

		simres, err := embedService.CosineSimilarity(gores, refres.Vec)
		if err != nil {
			panic(err)
		}

		results[refres.Text] = simres
	}

	pretty, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(pretty))
}
