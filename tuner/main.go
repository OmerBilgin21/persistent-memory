package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"persistent-memory/internal/domain"
	"persistent-memory/util"
	"strings"

	"github.com/townsendmerino/aikit/encoder"
)

const (
	examplesPath = "tuner/examples.json"
	tunedPath    = "internal/domain/tuned.json"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type tuner struct {
	ctx      context.Context
	embedder *cachingEmbedder
	base     domain.TunedValues
	ex       *examples
}

type tuningResult struct {
	memory binaryPick
	topic  binaryPick
	fit    binaryPick
	inject injectPick
}

func run() error {
	diagnose := flag.Bool("diagnose", false, "print per-prompt inject results for the current tuned values")
	flag.Parse()

	base, ex, err := loadInputs()
	if err != nil {
		return err
	}

	embedder, closeModel, err := loadEmbedder()
	if err != nil {
		return err
	}
	defer closeModel()

	t := &tuner{ctx: context.Background(), embedder: embedder, base: base, ex: ex}

	if *diagnose {
		_, err := withSection("inject", func(s *section) (struct{}, error) {
			return struct{}{}, diagnoseInject(t.ctx, s, embedder, base, ex)
		})
		return err
	}

	result, err := t.tuneAll()
	if err != nil {
		return err
	}

	return t.write(result)
}

func loadInputs() (domain.TunedValues, *examples, error) {
	if _, err := os.Stat(examplesPath); err != nil {
		return domain.TunedValues{}, nil,
			fmt.Errorf("%s not found, run the tuner from the repo root (make tune): %w", examplesPath, err)
	}

	base, err := domain.LoadTunedValues()
	if err != nil {
		return domain.TunedValues{}, nil, err
	}

	ex, err := loadExamples(examplesPath)
	if err != nil {
		return domain.TunedValues{}, nil, err
	}

	if err := validateExamples(ex, base); err != nil {
		return domain.TunedValues{}, nil, err
	}

	return base, ex, nil
}

func loadEmbedder() (*cachingEmbedder, func(), error) {
	modelDir := os.Getenv("gte_small_directory")
	if modelDir == "" {
		return nil, nil, fmt.Errorf("gte_small_directory environment variable is not set")
	}

	model, err := encoder.LoadBERT(modelDir)
	if err != nil {
		return nil, nil, err
	}

	embedService, err := domain.NewEmbedService(model)
	if err != nil {
		model.Close()
		return nil, nil, err
	}

	return newCachingEmbedder(embedService), func() { model.Close() }, nil
}

func (t *tuner) tuneAll() (tuningResult, error) {
	var result tuningResult
	var err error

	if result.memory, err = t.tuneMemory(); err != nil {
		return result, err
	}
	if result.topic, err = t.tuneTopic(); err != nil {
		return result, err
	}
	if result.fit, err = t.tuneFit(result.memory.value); err != nil {
		return result, err
	}
	if result.inject, err = t.tuneInjectGate(); err != nil {
		return result, err
	}

	return result, nil
}

func (t *tuner) tuneMemory() (binaryPick, error) {
	fmt.Println("tuning insert_memory_threshold...")
	return withSection("memory", func(s *section) (binaryPick, error) {
		evaluate := func(values []float32) ([][]bool, error) {
			return evaluateMemoryPairs(t.ctx, s, t.embedder, t.base, t.ex.MemoryPairs, values)
		}
		return tuneBinary("insert_memory_threshold", 0.70, 0.99, pairLabels(t.ex.MemoryPairs), evaluate)
	})
}

func (t *tuner) tuneTopic() (binaryPick, error) {
	fmt.Println("tuning insert_topic_threshold...")
	return withSection("topic", func(s *section) (binaryPick, error) {
		evaluate := func(values []float32) ([][]bool, error) {
			return evaluateTopicPairs(t.ctx, s, t.embedder, t.base, t.ex.TopicPairs, values)
		}
		return tuneBinary("insert_topic_threshold", 0.70, 0.99, pairLabels(t.ex.TopicPairs), evaluate)
	})
}

func (t *tuner) tuneFit(memoryThreshold float32) (binaryPick, error) {
	fmt.Println("tuning rejection_threshold_percentage...")
	fitBase := t.base
	fitBase.InsertMemoryThreshold = memoryThreshold

	return withSection("fit", func(s *section) (binaryPick, error) {
		caseFacts := map[string]bool{}
		for _, c := range t.ex.Fit.Cases {
			caseFacts[util.NormalizeStr(c.Fact)] = true
		}
		topicIds, err := loadStore(t.ctx, s, t.embedder, fitBase, t.ex.Fit.Store, caseFacts)
		if err != nil {
			return binaryPick{}, err
		}
		evaluate := func(values []float32) ([][]bool, error) {
			return evaluateFit(t.ctx, s, t.embedder, fitBase, topicIds, t.ex.Fit.Cases, values)
		}
		return tuneBinary("rejection_threshold_percentage", 0.0, 0.30, fitLabels(t.ex.Fit.Cases), evaluate)
	})
}

func (t *tuner) tuneInjectGate() (injectPick, error) {
	fmt.Println("tuning memory_injection_threshold and memory_injection_superiority_percentage...")
	return withSection("inject", func(s *section) (injectPick, error) {
		if _, err := loadStore(t.ctx, s, t.embedder, t.base, t.ex.Inject.Store, nil); err != nil {
			return injectPick{}, err
		}
		runner := &injectRunner{
			ctx:      t.ctx,
			section:  s,
			embedder: t.embedder,
			base:     t.base,
			cases:    t.ex.Inject.Cases,
		}
		return tuneInject(runner, injectLabels(t.ex.Inject.Cases))
	})
}

func (t *tuner) write(result tuningResult) error {
	tuned := t.base
	tuned.InsertMemoryThreshold = result.memory.value
	tuned.InsertTopicThreshold = result.topic.value
	tuned.RejectionThresholdPercentage = result.fit.value
	tuned.MemoryInjectionThreshold = result.inject.minimum
	tuned.MemoryInjectionSuperiorityPercentage = result.inject.gap

	data, err := json.MarshalIndent(tuned, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if _, err := domain.ParseTunedValues(data); err != nil {
		return fmt.Errorf("refusing to write tuned values the binary would reject: %w", err)
	}

	fmt.Println()
	reportBinary("insert_memory_threshold", t.base.InsertMemoryThreshold, result.memory)
	reportBinary("insert_topic_threshold", t.base.InsertTopicThreshold, result.topic)
	reportBinary("rejection_threshold_percentage", t.base.RejectionThresholdPercentage, result.fit)
	reportInject(t.base, result.inject, t.ex.Inject.Cases, injectLabels(t.ex.Inject.Cases))

	if err := os.WriteFile(tunedPath, data, 0o644); err != nil {
		return fmt.Errorf("error while writing %s: %w", tunedPath, err)
	}
	fmt.Printf("\nwrote %s\n", tunedPath)

	return nil
}

func withSection[T any](name string, fn func(s *section) (T, error)) (T, error) {
	s, err := openSection(name)
	if err != nil {
		var zero T
		return zero, err
	}
	defer s.close()

	return fn(s)
}

func pairLabels(pairs []pairExample) []label {
	labels := make([]label, 0, len(pairs))
	for _, p := range pairs {
		labels = append(labels, label{id: p.Id, reject: p.Reject, check: isCheck(p.Id)})
	}
	return labels
}

func fitLabels(cases []fitCase) []label {
	labels := make([]label, 0, len(cases))
	for _, c := range cases {
		labels = append(labels, label{id: c.Id, reject: c.Reject, check: isCheck(c.Id)})
	}
	return labels
}

func injectLabels(cases []injectCase) []injectLabel {
	labels := make([]injectLabel, 0, len(cases))
	for _, c := range cases {
		labels = append(labels, injectLabel{id: c.Id, expected: c.Expected, check: isCheck(c.Id)})
	}
	return labels
}

func formatRates(r rates) string {
	return fmt.Sprintf("false rejects %d/%d (%.0f%%), misses %d/%d (%.0f%%)",
		r.falseRejects, r.shouldAccept, r.falseRejectRate()*100,
		r.misses, r.shouldReject, r.missRate()*100)
}

func reportBinary(name string, old float32, pick binaryPick) {
	fmt.Printf("%s: %v -> %v (best plateau %v to %v)\n", name, old, pick.value, pick.plateauLow, pick.plateauHigh)
	fmt.Printf("  tuning: %s\n", formatRates(pick.tuning))
	fmt.Printf("  check:  %s\n", formatRates(pick.check))
	for _, w := range pick.tuning.wrong {
		fmt.Printf("  wrong (tuning): %s\n", w)
	}
	for _, w := range pick.check.wrong {
		fmt.Printf("  wrong (check):  %s\n", w)
	}
}

func formatFacts(f factScore) string {
	return fmt.Sprintf("score %.2f (expected facts injected %d/%d, wrong facts injected %d)",
		f.value(), f.hits, f.expected, f.wrong)
}

func reportInject(base domain.TunedValues, pick injectPick, cases []injectCase, labels []injectLabel) {
	fmt.Printf("memory_injection_threshold: %v -> %v\n", base.MemoryInjectionThreshold, pick.minimum)
	fmt.Printf("memory_injection_superiority_percentage: %v -> %v\n", base.MemoryInjectionSuperiorityPercentage, pick.gap)
	fmt.Printf("  best region: %d points in the grid\n", pick.regionSize)
	fmt.Printf("  tuning: %s, %d/%d exact matches\n", formatFacts(pick.tuningFacts), pick.tuningRight, pick.tuningTotal)
	fmt.Printf("  check:  %s, %d/%d exact matches\n", formatFacts(pick.checkFacts), pick.checkRight, pick.checkTotal)

	printWrong := func(part string, indexes []int) {
		for _, c := range indexes {
			var expected []string
			for _, e := range labels[c].expected {
				expected = append(expected, util.NormalizeStr(e))
			}
			fmt.Printf("  wrong (%s): %s %q\n", part, cases[c].Id, cases[c].Prompt)
			fmt.Printf("    expected: [%s]\n", strings.Join(expected, " | "))
			fmt.Printf("    injected: [%s]\n", strings.Join(pick.injectedHere[c], " | "))
		}
	}
	printWrong("tuning", pick.wrongTuning)
	printWrong("check", pick.wrongCheck)
}
