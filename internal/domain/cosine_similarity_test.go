package domain

import (
	"os"

	"encoding/json"
	"path/filepath"
	"testing"
)

type llamaEmbeddingResultEmbedding struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
	Object    string    `json:"object"`
}

type llamaEmbeddingResultUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type llamaEmbeddingResult struct {
	Model  string                          `json:"model"`
	Object string                          `json:"object"`
	Usage  llamaEmbeddingResultUsage       `json:"usage"`
	Data   []llamaEmbeddingResultEmbedding `json:"data"`
}

func TestCosineSimilarity(test *testing.T) {

	testCases := []struct {
		name           string
		vectorOne      []float32
		vectorTwo      []float32
		expectedResult float32
	}{
		{
			name:           "case 1",
			vectorOne:      []float32{3, 4},
			vectorTwo:      []float32{6, 8},
			expectedResult: 1,
		},
		{
			name:           "case 2",
			vectorOne:      []float32{1, 0},
			vectorTwo:      []float32{0, 1},
			expectedResult: 0,
		}, {
			name:           "case 3",
			vectorOne:      []float32{1, 0},
			vectorTwo:      []float32{-1, 0},
			expectedResult: -1,
		},
	}

	for _, tCase := range testCases {
		test.Run(tCase.name, func(t *testing.T) {
			result, err := CosineSimilarity(tCase.vectorOne, tCase.vectorTwo)
			if err != nil {
				t.Logf("error while calculating cosine similarity %+v", err)
				t.FailNow()
			}

			if result != tCase.expectedResult {
				t.Logf("incorrect result! expected %+v, got: %+v", tCase.expectedResult, result)
				t.FailNow()
			}
		})
	}

	test.Run("the llama one", func(t *testing.T) {
		dir, err := os.Getwd()

		if err != nil {
			t.Logf("error while getting the current dir %+v", err)
		}

		example := filepath.Join(dir, "/cosine_similarity_example.json")
		data, err := os.ReadFile(example)

		if err != nil {
			t.Logf("error while reading the example data %+v", err)
		}

		var parsed llamaEmbeddingResult

		err = json.Unmarshal(data, &parsed)
		if err != nil {
			t.Logf("error while parsing the example data %+v", err)
		}

		llamaSimResult, err := CosineSimilarity(parsed.Data[0].Embedding, parsed.Data[1].Embedding)
		if err != nil {
			t.Logf("could not calculate cosine similarity %+v", err)
		}

		var expectedRes float32
		expectedRes = 0.7749478
		if llamaSimResult != expectedRes {
			t.Logf("incorrect result! expected %+v, got: %+v", expectedRes, llamaSimResult)
			t.FailNow()
		}
	})

	test.Run("unequal vectors", func(t *testing.T) {
		_, err := CosineSimilarity([]float32{1, 2, 3}, []float32{1, 2, 3, 4})

		if err == nil {
			t.Logf("somehow no error while calculating the cosine similarity of vectors of unequal length")
		}

	})

}
