package domain

import (
	"fmt"
	"math"
)

func calculateLength(vector []float32) float32 {
	if len(vector) == 0 {
		return 0
	}
	var sumOfSquared float32
	for _, elem := range vector {
		sumOfSquared += elem * elem
	}

	return float32(math.Sqrt(float64(sumOfSquared)))
}

func CosineSimilarity(vectorOne, vectorTwo []float32) (float32, error) {
	if len(vectorOne) != len(vectorTwo) {
		return 0, fmt.Errorf("unequal lengths between vectors")
	}

	var numerator float32
	var denominator float32

	for i := range vectorOne {
		numerator += vectorOne[i] * vectorTwo[i]
	}

	denominator = calculateLength(vectorOne) * calculateLength(vectorTwo)

	return numerator / denominator, nil
}
