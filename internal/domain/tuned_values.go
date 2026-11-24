package domain

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

//go:embed tuned.json
var embeddedTunedValues []byte

type TunedValues struct {
	InsertMemoryThreshold                float32 `json:"insert_memory_threshold"`
	InsertTopicThreshold                 float32 `json:"insert_topic_threshold"`
	RejectionThresholdPercentage         float32 `json:"rejection_threshold_percentage"`
	MemoryInjectionThreshold             float32 `json:"memory_injection_threshold"`
	MemoryInjectionSuperiorityPercentage float32 `json:"memory_injection_superiority_percentage"`
	AcceptedCountThreshold               int     `json:"accepted_count_threshold"`
	TopMemoryCharacterCount              int     `json:"top_memory_character_count"`
	MemoryRetrievalLimit                 int     `json:"memory_retrieval_limit"`
}

type rawTunedValues struct {
	InsertMemoryThreshold                *float32 `json:"insert_memory_threshold"`
	InsertTopicThreshold                 *float32 `json:"insert_topic_threshold"`
	RejectionThresholdPercentage         *float32 `json:"rejection_threshold_percentage"`
	MemoryInjectionThreshold             *float32 `json:"memory_injection_threshold"`
	MemoryInjectionSuperiorityPercentage *float32 `json:"memory_injection_superiority_percentage"`
	AcceptedCountThreshold               *int     `json:"accepted_count_threshold"`
	TopMemoryCharacterCount              *int     `json:"top_memory_character_count"`
	MemoryRetrievalLimit                 *int     `json:"memory_retrieval_limit"`
}

func LoadTunedValues() (TunedValues, error) {
	return ParseTunedValues(embeddedTunedValues)
}

func ParseTunedValues(data []byte) (TunedValues, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var raw rawTunedValues
	if err := decoder.Decode(&raw); err != nil {
		return TunedValues{}, fmt.Errorf("error while parsing tuned values: %w", err)
	}

	required := []struct {
		key     string
		present bool
	}{
		{"insert_memory_threshold", raw.InsertMemoryThreshold != nil},
		{"insert_topic_threshold", raw.InsertTopicThreshold != nil},
		{"rejection_threshold_percentage", raw.RejectionThresholdPercentage != nil},
		{"memory_injection_threshold", raw.MemoryInjectionThreshold != nil},
		{"memory_injection_superiority_percentage", raw.MemoryInjectionSuperiorityPercentage != nil},
		{"accepted_count_threshold", raw.AcceptedCountThreshold != nil},
		{"top_memory_character_count", raw.TopMemoryCharacterCount != nil},
		{"memory_retrieval_limit", raw.MemoryRetrievalLimit != nil},
	}

	var missing []string
	for _, r := range required {
		if !r.present {
			missing = append(missing, r.key)
		}
	}

	if len(missing) > 0 {
		return TunedValues{}, fmt.Errorf("tuned values are missing keys: %s", strings.Join(missing, ", "))
	}

	values := TunedValues{
		InsertMemoryThreshold:                *raw.InsertMemoryThreshold,
		InsertTopicThreshold:                 *raw.InsertTopicThreshold,
		RejectionThresholdPercentage:         *raw.RejectionThresholdPercentage,
		MemoryInjectionThreshold:             *raw.MemoryInjectionThreshold,
		MemoryInjectionSuperiorityPercentage: *raw.MemoryInjectionSuperiorityPercentage,
		AcceptedCountThreshold:               *raw.AcceptedCountThreshold,
		TopMemoryCharacterCount:              *raw.TopMemoryCharacterCount,
		MemoryRetrievalLimit:                 *raw.MemoryRetrievalLimit,
	}

	if err := values.validate(); err != nil {
		return TunedValues{}, err
	}

	return values, nil
}

func (v TunedValues) validate() error {
	var problems []string

	thresholds := []struct {
		key   string
		value float32
	}{
		{"insert_memory_threshold", v.InsertMemoryThreshold},
		{"insert_topic_threshold", v.InsertTopicThreshold},
		{"memory_injection_threshold", v.MemoryInjectionThreshold},
	}
	for _, t := range thresholds {
		if t.value <= 0 || t.value > 1 {
			problems = append(problems, fmt.Sprintf("%s must be in (0, 1], got %v", t.key, t.value))
		}
	}

	percentages := []struct {
		key   string
		value float32
	}{
		{"rejection_threshold_percentage", v.RejectionThresholdPercentage},
		{"memory_injection_superiority_percentage", v.MemoryInjectionSuperiorityPercentage},
	}
	for _, p := range percentages {
		if p.value < 0 || p.value > 1 {
			problems = append(problems, fmt.Sprintf("%s must be in [0, 1], got %v", p.key, p.value))
		}
	}

	counts := []struct {
		key   string
		value int
	}{
		{"accepted_count_threshold", v.AcceptedCountThreshold},
		{"top_memory_character_count", v.TopMemoryCharacterCount},
		{"memory_retrieval_limit", v.MemoryRetrievalLimit},
	}
	for _, c := range counts {
		if c.value <= 0 {
			problems = append(problems, fmt.Sprintf("%s must be greater than 0, got %v", c.key, c.value))
		}
	}

	if len(problems) > 0 {
		return errors.New("invalid tuned values: " + strings.Join(problems, "; "))
	}

	return nil
}
