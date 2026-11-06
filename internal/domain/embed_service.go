package domain

import (
	"fmt"
	"math"
	"persistent-memory/util"

	"github.com/townsendmerino/aikit/encoder"
)
type EmbedService struct {
	loadedEncoder *encoder.BERT
}

type EmbedServiceInterface interface {
	CalculateVectorLength(vector []float32) float32
	CosineSimilarity(vectorOne, vectorTwo []float32) (float32, error)
	NormalizedCosineSimilarity(vectorOne, vectorTwo []float32) (float32, error)
	Normalize(vector []float32) []float32
	Embed(fact string) (*[]float32, error)
}
func NewEmbedService(loadedEncoder *encoder.BERT) (*EmbedService, error) {
	return &EmbedService{
		loadedEncoder: loadedEncoder,
	}, nil
}
func (s *EmbedService) CalculateVectorLength(vector []float32) float32 {
	if len(vector) == 0 {
		return 0
	}
	var sumOfSquared float32
	for _, elem := range vector {
		sumOfSquared += elem * elem
	}

	return float32(math.Sqrt(float64(sumOfSquared)))
}

func (s *EmbedService) CosineSimilarity(vectorOne, vectorTwo []float32) (float32, error) {
	if len(vectorOne) != len(vectorTwo) {
		return 0, fmt.Errorf("unequal lengths between vectors")
	}

	var numerator float32
	var denominator float32

	for i := range vectorOne {
		numerator += vectorOne[i] * vectorTwo[i]
	}

	denominator = s.CalculateVectorLength(vectorOne) * s.CalculateVectorLength(vectorTwo)

	return numerator / denominator, nil
}

func (s *EmbedService) NormalizedCosineSimilarity(vectorOne, vectorTwo []float32) (float32, error) {
	if len(vectorOne) != len(vectorTwo) {
		return 0, fmt.Errorf("unequal lengths between vectors")
	}

	var numerator float32

	for i := range vectorOne {
		numerator += vectorOne[i] * vectorTwo[i]
	}

	return numerator, nil
}

