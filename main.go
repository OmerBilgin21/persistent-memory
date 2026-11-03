package main

import (
	"fmt"
	"persistent-memory/internal/domain"
	"persistent-memory/internal/persistence"
)

func main() {
	persistence.DB()
	res, err := domain.CosineSimilarity([]float32{1, 0}, []float32{0, 1})
	if err != nil {
		panic(err)
	}
	fmt.Printf("res: %+v\n", res)
}
