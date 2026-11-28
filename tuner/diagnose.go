package main

import (
	"context"
	"fmt"
	"persistent-memory/internal/domain"
	"persistent-memory/util"
	"strings"
)

const (
	green = "\033[32m"
	red   = "\033[31m"
	reset = "\033[0m"
)

func shorten(text string) string {
	runes := []rune(text)
	if len(runes) <= 90 {
		return text
	}
	return string(runes[:90]) + "…"
}

func colored(color, text string) string {
	return color + text + reset
}

func diagnoseInject(
	ctx context.Context,
	s *section,
	embedder *cachingEmbedder,
	base domain.TunedValues,
	ex *examples,
) error {
	if _, err := loadStore(ctx, s, embedder, base, ex.Inject.Store, nil); err != nil {
		return err
	}

	service, err := domain.NewMemService(s.store, embedder, base)
	if err != nil {
		return err
	}

	memories, err := s.store.Memories.GetAll(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("gate: minimum %v, gap %v, over %d facts\n", base.MemoryInjectionThreshold,
		base.MemoryInjectionSuperiorityPercentage, len(*memories))

	runner := &injectRunner{ctx: ctx, section: s, embedder: embedder, base: base, cases: ex.Inject.Cases}
	injectedPerCase, err := runner.run(base.MemoryInjectionThreshold, base.MemoryInjectionSuperiorityPercentage)
	if err != nil {
		return err
	}

	var summary diagnoseSummary
	for c, ic := range ex.Inject.Cases {
		if len(ic.Expected) == 0 {
			summary.quiet++
			if len(injectedPerCase[c]) > 0 {
				summary.noisy = append(summary.noisy, ic.Id)
			}
			continue
		}

		vector, err := embedder.Embed(util.NormalizeStr(ic.Prompt))
		if err != nil {
			return err
		}
		sorted, err := service.GetCosineSortedMemories(ctx, *vector, *memories)
		if err != nil {
			return err
		}

		summary.mustInject++
		summary.facts = summary.facts.add(scoreFacts(injectedPerCase[c], ic.Expected))
		if printInjectCase(ic, *sorted, injectedPerCase[c], base) {
			summary.exact++
		}
	}

	summary.print()
	return nil
}

type diagnoseSummary struct {
	facts      factScore
	exact      int
	mustInject int
	quiet      int
	noisy      []string
}

func (d diagnoseSummary) print() {
	silent := d.quiet - len(d.noisy)
	fmt.Printf("\nmust-inject: %d/%d exact, %d/%d expected facts injected, %d wrong facts injected\n",
		d.exact, d.mustInject, d.facts.hits, d.facts.expected, d.facts.wrong)
	fmt.Printf("silence: %d/%d quiet prompts stayed silent (%.0f%%)",
		silent, d.quiet, 100*float64(silent)/float64(max(d.quiet, 1)))
	if len(d.noisy) > 0 {
		fmt.Printf(", injected on: %s", strings.Join(d.noisy, ", "))
	}
	fmt.Println()
}

func printInjectCase(
	ic injectCase,
	ranked []domain.SimilarityMemory,
	injectedFacts []string,
	base domain.TunedValues,
) bool {
	median := ranked[len(ranked)/2].CosineRank
	bar := max(base.MemoryInjectionThreshold, median+median*base.MemoryInjectionSuperiorityPercentage)

	injected := map[string]bool{}
	for _, m := range injectedFacts {
		injected[m] = true
	}
	expected := map[string]bool{}
	for _, e := range ic.Expected {
		expected[util.NormalizeStr(e)] = true
	}

	exact := sameSet(injectedFacts, ic.Expected)
	header := colored(red, fmt.Sprintf("%s %q", ic.Id, shorten(ic.Prompt)))
	if exact {
		header = colored(green, fmt.Sprintf("%s %q", ic.Id, shorten(ic.Prompt)))
	}
	fmt.Printf("\n%s\n  median %.3f, bar %.3f\n", header, median, bar)

	for rank, m := range ranked {
		line := fmt.Sprintf("#%-3d %.3f %s", rank+1, m.CosineRank, shorten(m.Content))
		switch {
		case expected[m.Content] && injected[m.Content]:
			fmt.Printf("  %s\n", colored(green, "injected  "+line))
		case expected[m.Content]:
			fmt.Printf("  %s\n", colored(red, "missed    "+line))
		case injected[m.Content]:
			fmt.Printf("  %s\n", colored(red, "wrong     "+line))
		}
	}

	return exact
}
