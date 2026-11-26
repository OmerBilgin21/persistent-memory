package domain

import (
	"strings"
	"testing"
)

func TestLoadTunedValues(test *testing.T) {
	values, err := LoadTunedValues()
	if err != nil {
		test.Logf("the embedded tuned values do not parse: %+v", err)
		test.FailNow()
	}

	if values.TopMemoryCharacterCount == 0 || values.InsertMemoryThreshold == 0 {
		test.Logf("the embedded tuned values came back empty: %+v", values)
		test.FailNow()
	}
}

func TestParseTunedValues(test *testing.T) {
	valid := `{
		"insert_memory_threshold": 0.85,
		"insert_topic_threshold": 0.85,
		"rejection_threshold_percentage": 0,
		"memory_injection_threshold": 0.8,
		"memory_injection_superiority_percentage": 0.08,
		"accepted_count_threshold": 5,
		"top_memory_character_count": 280,
		"memory_retrieval_limit": 5
	}`

	test.Run("valid values parse, including a legitimate 0", func(t *testing.T) {
		values, err := ParseTunedValues([]byte(valid))
		if err != nil {
			t.Logf("expected no error, got: %+v", err)
			t.FailNow()
		}

		if values.RejectionThresholdPercentage != 0 || values.TopMemoryCharacterCount != 280 {
			t.Logf("unexpected values: %+v", values)
			t.FailNow()
		}
	})

	testCases := []struct {
		name          string
		data          string
		expectedInErr string
	}{
		{
			name:          "a missing key fails and is named",
			data:          strings.Replace(valid, `"memory_retrieval_limit": 5`, `"accepted_count_threshold": 5`, 1),
			expectedInErr: "memory_retrieval_limit",
		},
		{
			name:          "an unknown key fails",
			data:          strings.Replace(valid, `"memory_retrieval_limit": 5`, `"memory_retrieval_limit": 5, "typo_threshold": 1`, 1),
			expectedInErr: "typo_threshold",
		},
		{
			name:          "a threshold above 1 fails",
			data:          strings.Replace(valid, `"insert_topic_threshold": 0.85`, `"insert_topic_threshold": 1.2`, 1),
			expectedInErr: "insert_topic_threshold",
		},
		{
			name:          "a negative percentage fails",
			data:          strings.Replace(valid, `"memory_injection_superiority_percentage": 0.08`, `"memory_injection_superiority_percentage": -0.1`, 1),
			expectedInErr: "memory_injection_superiority_percentage",
		},
		{
			name:          "a zero count fails",
			data:          strings.Replace(valid, `"top_memory_character_count": 280`, `"top_memory_character_count": 0`, 1),
			expectedInErr: "top_memory_character_count",
		},
		{
			name:          "invalid json fails",
			data:          `{"insert_memory_threshold": `,
			expectedInErr: "error while parsing tuned values",
		},
	}

	for _, tCase := range testCases {
		test.Run(tCase.name, func(t *testing.T) {
			_, err := ParseTunedValues([]byte(tCase.data))
			if err == nil {
				t.Logf("expected an error containing %q, got none", tCase.expectedInErr)
				t.FailNow()
			}

			if !strings.Contains(err.Error(), tCase.expectedInErr) {
				t.Logf("expected an error containing %q, got: %+v", tCase.expectedInErr, err)
				t.FailNow()
			}
		})
	}
}
