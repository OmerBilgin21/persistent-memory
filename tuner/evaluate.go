package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"persistent-memory/internal/domain"
	"persistent-memory/internal/persistence"
	"persistent-memory/util"
	"sort"
	"strings"

	"gorm.io/gorm"
)

type cachingEmbedder struct {
	domain.EmbedServiceInterface
	cache map[string]*[]float32
}

func newCachingEmbedder(embedService domain.EmbedServiceInterface) *cachingEmbedder {
	return &cachingEmbedder{
		EmbedServiceInterface: embedService,
		cache:                 map[string]*[]float32{},
	}
}

func (c *cachingEmbedder) Embed(fact string) (*[]float32, error) {
	if vector, ok := c.cache[fact]; ok {
		return vector, nil
	}

	vector, err := c.EmbedServiceInterface.Embed(fact)
	if err != nil {
		return nil, err
	}

	c.cache[fact] = vector
	return vector, nil
}

type section struct {
	db    *gorm.DB
	store *persistence.Store
	dir   string
}

func openSection(name string) (*section, error) {
	dir, err := os.MkdirTemp("", "mem-tune-"+name+"-")
	if err != nil {
		return nil, fmt.Errorf("error while creating a temp dir for %s: %w", name, err)
	}

	db, err := persistence.DB(filepath.Join(dir, "tune.db"))
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("error while opening the %s database: %w", name, err)
	}

	return &section{
		db:    db,
		store: persistence.NewStore(db),
		dir:   dir,
	}, nil
}

func (s *section) close() {
	if sqlDB, err := s.db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	_ = os.RemoveAll(s.dir)
}

func (s *section) wipe() error {
	for _, table := range []string{"injections", "memories", "topics"} {
		if err := s.db.Exec("DELETE FROM " + table).Error; err != nil {
			return fmt.Errorf("error while wiping %s: %w", table, err)
		}
	}

	return nil
}

func forceTopic(ctx context.Context, setup *domain.MemService, name, description string) (uint, error) {
	createdId, res := setup.CreateOrRejectTopic(ctx, domain.CreateTopic{
		Name:        name,
		Description: description,
		Force:       true,
	})

	kind, err := classify(createdId, res)
	if err != nil {
		return 0, fmt.Errorf("error while creating topic %q: %w", name, err)
	}
	if kind != accepted {
		return 0, fmt.Errorf("topic %q was rejected as %s even with force", name, kind)
	}

	return *createdId, nil
}

func forceMemory(ctx context.Context, setup *domain.MemService, topicId uint, fact string) error {
	createdId, res := setup.CreateOrRejectMemory(ctx, domain.CreateMemory{
		Fact:    fact,
		TopicId: topicId,
		Force:   true,
	})

	kind, err := classify(createdId, res)
	if err != nil {
		return fmt.Errorf("error while creating memory %q: %w", fact, err)
	}
	if kind != accepted {
		return fmt.Errorf("memory %q was rejected as %s even with force", fact, kind)
	}

	return nil
}

func loadStore(
	ctx context.Context,
	s *section,
	embedder *cachingEmbedder,
	base domain.TunedValues,
	store []storeTopic,
	exclude map[string]bool,
) (map[string]uint, error) {
	setup, err := domain.NewMemService(s.store, embedder, base)
	if err != nil {
		return nil, err
	}

	topicIds := map[string]uint{}
	for _, t := range store {
		topicId, err := forceTopic(ctx, setup, t.Topic, t.Description)
		if err != nil {
			return nil, err
		}
		topicIds[util.NormalizeStr(t.Topic)] = topicId

		for _, f := range t.Facts {
			if exclude[util.NormalizeStr(f)] {
				continue
			}
			if err := forceMemory(ctx, setup, topicId, f); err != nil {
				return nil, err
			}
		}
	}

	return topicIds, nil
}

type outcome string

const (
	accepted             outcome = "accepted"
	rejectedDuplicate    outcome = "duplicate"
	rejectedFit          outcome = "fit"
	rejectedSimilarTopic outcome = "similar topic"
	rejectedExactTopic   outcome = "exact topic name"
)

func classify(createdId *uint, res domain.Result) (outcome, error) {
	if res == nil {
		if createdId == nil {
			return "", fmt.Errorf("the call succeeded without returning an id")
		}
		return accepted, nil
	}

	switch r := res.(type) {
	case domain.RejectedMemoryUpsertResult:
		switch {
		case strings.Contains(r.Reason, "has many very similar memories"):
			return rejectedDuplicate, nil
		case strings.Contains(r.Reason, "Despite the given topic may be a fit"):
			return rejectedFit, nil
		case strings.Contains(r.Reason, "has many very similar topics"):
			return rejectedSimilarTopic, nil
		case strings.Contains(r.Reason, "already a topic with this exact same name"):
			return rejectedExactTopic, nil
		}
		return "", fmt.Errorf("unknown rejection reason: %s", r.Reason)
	case domain.UnexpectedErrorResult:
		return "", fmt.Errorf("unexpected error: %w", r.Err)
	}

	return "", fmt.Errorf("unknown result type %T", res)
}

func newMatrix(rows, columns int) [][]bool {
	matrix := make([][]bool, rows)
	for i := range matrix {
		matrix[i] = make([]bool, columns)
	}
	return matrix
}

func evaluateMemoryPairs(
	ctx context.Context,
	s *section,
	embedder *cachingEmbedder,
	base domain.TunedValues,
	pairs []pairExample,
	values []float32,
) ([][]bool, error) {
	rejected := newMatrix(len(values), len(pairs))

	setup, err := domain.NewMemService(s.store, embedder, base)
	if err != nil {
		return nil, err
	}

	for p, pair := range pairs {
		if err := s.wipe(); err != nil {
			return nil, err
		}

		topicId, err := forceTopic(ctx, setup, "pairs", "tuning")
		if err != nil {
			return nil, err
		}

		if err := forceMemory(ctx, setup, topicId, pair.Existing); err != nil {
			return nil, err
		}

		for v, value := range values {
			tuned := base
			tuned.InsertMemoryThreshold = value

			service, err := domain.NewMemService(s.store, embedder, tuned)
			if err != nil {
				return nil, err
			}

			createdId, res := service.CreateOrRejectMemory(ctx, domain.CreateMemory{
				Fact:    pair.New,
				TopicId: topicId,
			})

			kind, err := classify(createdId, res)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", pair.Id, err)
			}

			switch kind {
			case accepted:
				if err := s.store.Memories.Delete(ctx, *createdId); err != nil {
					return nil, fmt.Errorf("%s: error while deleting the created memory: %w", pair.Id, err)
				}
			case rejectedDuplicate:
				rejected[v][p] = true
			default:
				return nil, fmt.Errorf("%s: unexpected %s rejection", pair.Id, kind)
			}
		}
	}

	return rejected, nil
}

func evaluateTopicPairs(
	ctx context.Context,
	s *section,
	embedder *cachingEmbedder,
	base domain.TunedValues,
	pairs []pairExample,
	values []float32,
) ([][]bool, error) {
	rejected := newMatrix(len(values), len(pairs))

	setup, err := domain.NewMemService(s.store, embedder, base)
	if err != nil {
		return nil, err
	}

	for p, pair := range pairs {
		if err := s.wipe(); err != nil {
			return nil, err
		}

		if _, err := forceTopic(ctx, setup, pair.Existing, "tuning"); err != nil {
			return nil, err
		}

		for v, value := range values {
			tuned := base
			tuned.InsertTopicThreshold = value

			service, err := domain.NewMemService(s.store, embedder, tuned)
			if err != nil {
				return nil, err
			}

			createdId, res := service.CreateOrRejectTopic(ctx, domain.CreateTopic{
				Name:        pair.New,
				Description: "tuning",
			})

			kind, err := classify(createdId, res)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", pair.Id, err)
			}

			switch kind {
			case accepted:
				if err := s.store.Topics.Delete(ctx, *createdId); err != nil {
					return nil, fmt.Errorf("%s: error while deleting the created topic: %w", pair.Id, err)
				}
			case rejectedSimilarTopic:
				rejected[v][p] = true
			default:
				return nil, fmt.Errorf("%s: unexpected %s rejection", pair.Id, kind)
			}
		}
	}

	return rejected, nil
}

func evaluateFit(
	ctx context.Context,
	s *section,
	embedder *cachingEmbedder,
	base domain.TunedValues,
	topicIds map[string]uint,
	cases []fitCase,
	values []float32,
) ([][]bool, error) {
	rejected := newMatrix(len(values), len(cases))
	duplicates := map[string]bool{}

	for v, value := range values {
		tuned := base
		tuned.RejectionThresholdPercentage = value

		service, err := domain.NewMemService(s.store, embedder, tuned)
		if err != nil {
			return nil, err
		}

		for c, fc := range cases {
			createdId, res := service.CreateOrRejectMemory(ctx, domain.CreateMemory{
				Fact:    fc.Fact,
				TopicId: topicIds[util.NormalizeStr(fc.Topic)],
			})

			kind, err := classify(createdId, res)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", fc.Id, err)
			}

			switch kind {
			case accepted:
				if err := s.store.Memories.Delete(ctx, *createdId); err != nil {
					return nil, fmt.Errorf("%s: error while deleting the created memory: %w", fc.Id, err)
				}
			case rejectedFit:
				rejected[v][c] = true
			case rejectedDuplicate:
				duplicates[fc.Id] = true
			default:
				return nil, fmt.Errorf("%s: unexpected %s rejection", fc.Id, kind)
			}
		}
	}

	if len(duplicates) > 0 {
		var ids []string
		for id := range duplicates {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return nil, fmt.Errorf("fit cases rejected as duplicates of a store fact, reword them: %s", strings.Join(ids, ", "))
	}

	return rejected, nil
}

type injectRunner struct {
	ctx      context.Context
	section  *section
	embedder *cachingEmbedder
	base     domain.TunedValues
	cases    []injectCase
	sessions int
}

func (r *injectRunner) run(minimum, gap float32) ([][]string, error) {
	tuned := r.base
	tuned.MemoryInjectionThreshold = minimum
	tuned.MemoryInjectionSuperiorityPercentage = gap

	service, err := domain.NewMemService(r.section.store, r.embedder, tuned)
	if err != nil {
		return nil, err
	}

	injected := make([][]string, len(r.cases))
	for c, ic := range r.cases {
		r.sessions++
		sessionId := fmt.Sprintf("tune-%d", r.sessions)

		res := service.Inject(r.ctx, ic.Prompt, sessionId)
		if res.Err != nil {
			return nil, fmt.Errorf("%s: %w", ic.Id, res.Err)
		}

		for _, m := range res.Memories {
			injected[c] = append(injected[c], m.Content)
		}

		if err := service.ClearInjected(r.ctx, sessionId); err != nil {
			return nil, fmt.Errorf("%s: error while clearing injections: %w", ic.Id, err)
		}
	}

	return injected, nil
}

func sameSet(injected, expected []string) bool {
	if len(injected) != len(expected) {
		return false
	}

	want := map[string]bool{}
	for _, e := range expected {
		want[util.NormalizeStr(e)] = true
	}

	for _, i := range injected {
		if !want[i] {
			return false
		}
	}

	return true
}
