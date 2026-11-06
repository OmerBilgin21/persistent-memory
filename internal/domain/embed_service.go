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

func (s *EmbedService) Normalize(vector []float32) []float32 {
	normalized := make([]float32, len(vector))
	length := s.CalculateVectorLength(vector)

	// this would mean that the vector is basically (0,0,0...)
	// and we don't want to do 0/0, hence the 0 initialized
	// normalized arr is appropriate to return here
	if length == 0 {
		return normalized
	}

	for i, elem := range vector {
		normalized[i] = elem / length
	}

	return normalized
}

func (s *EmbedService) Embed(fact string) (*[]float32, error) {
	encoded, err := s.loadedEncoder.Encode(fact)

	if err != nil {
		return nil, err
	}

	return util.Ptr(s.Normalize(encoded)), nil
}
