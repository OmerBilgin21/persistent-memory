package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"persistent-memory/internal/domain"
	"persistent-memory/util"
	"strings"
)

type storeTopic struct {
	Topic       string   `json:"topic"`
	Description string   `json:"description"`
	Facts       []string `json:"facts"`
}

type pairExample struct {
	Id       string `json:"id"`
	Existing string `json:"existing"`
	New      string `json:"new"`
	Reject   bool   `json:"reject"`
}

type fitCase struct {
	Id     string `json:"id"`
	Fact   string `json:"fact"`
	Topic  string `json:"topic"`
	Reject bool   `json:"reject"`
}

type injectCase struct {
	Id       string   `json:"id"`
	Prompt   string   `json:"prompt"`
	Expected []string `json:"expected"`
}

type fitSection struct {
	Store []storeTopic `json:"store"`
	Cases []fitCase    `json:"cases"`
}

type injectSection struct {
	Store []storeTopic `json:"store"`
	Cases []injectCase `json:"cases"`
}

type examples struct {
	MemoryPairs []pairExample `json:"memory_pairs"`
	TopicPairs  []pairExample `json:"topic_pairs"`
	Fit         fitSection    `json:"fit"`
	Inject      injectSection `json:"inject"`
}

func loadExamples(path string) (*examples, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error while reading the examples: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var parsed examples
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("error while parsing the examples: %w", err)
	}

	return &parsed, nil
}

func isCheck(id string) bool {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(id))
	return hash.Sum32()%5 == 0
}

type exampleValidator struct {
	problems []string
	seenIds  map[string]bool
}

func (v *exampleValidator) add(format string, args ...any) {
	v.problems = append(v.problems, fmt.Sprintf(format, args...))
}

func (v *exampleValidator) checkId(id string) {
	if v.seenIds[id] {
		v.add("%s: duplicate id", id)
	}
	v.seenIds[id] = true
}

func (v *exampleValidator) labelled(section string, labels []label) {
	var hasReject, hasAccept bool
	for _, l := range labels {
		v.checkId(l.id)
		if l.check {
			continue
		}
		hasReject = hasReject || l.reject
		hasAccept = hasAccept || !l.reject
	}
	if !hasReject || !hasAccept {
		v.add("%s: the tuning split needs both reject and accept cases", section)
	}
}

func (v *exampleValidator) inject(section injectSection, limit int) {
	facts := map[string]bool{}
	for _, t := range section.Store {
		for _, f := range t.Facts {
			facts[util.NormalizeStr(f)] = true
		}
	}

	var hasEmpty, hasNonEmpty bool
	for _, c := range section.Cases {
		v.checkId(c.Id)
		if len(c.Expected) > limit {
			v.add("%s: expects %d facts, the limit is %d", c.Id, len(c.Expected), limit)
		}
		for _, e := range c.Expected {
			if !facts[util.NormalizeStr(e)] {
				v.add("%s: expected fact %q is not in the inject store", c.Id, e)
			}
		}
		if !isCheck(c.Id) {
			hasEmpty = hasEmpty || len(c.Expected) == 0
			hasNonEmpty = hasNonEmpty || len(c.Expected) > 0
		}
	}
	if !hasEmpty || !hasNonEmpty {
		v.add("inject: the tuning split needs cases that expect nothing and cases that expect facts")
	}
}

func validateExamples(ex *examples, tuned domain.TunedValues) error {
	v := &exampleValidator{seenIds: map[string]bool{}}
	v.labelled("memory_pairs", pairLabels(ex.MemoryPairs))
	v.labelled("topic_pairs", pairLabels(ex.TopicPairs))
	v.labelled("fit", fitLabels(ex.Fit.Cases))
	v.inject(ex.Inject, tuned.MemoryRetrievalLimit)

	if len(v.problems) > 0 {
		return fmt.Errorf("invalid examples:\n  %s", strings.Join(v.problems, "\n  "))
	}

	return nil
}
