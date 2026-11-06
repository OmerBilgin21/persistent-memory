package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Memory struct {
	BaseModel
	TopicId          uint      `gorm:"constraint:OnDelete:CASCADE" json:"topicId"`
	Content          string    `gorm:"not null" json:"content"`
	LastRetrievedAt  time.Time `gorm:"not null;autoCreateTime" json:"lastRetrievedAt"`
	HitCount         int       `gorm:"not null;default: 0" json:"hitCount"`
	NormalizedVector Vector    `gorm:"not null" json:"normalizedVector"`
}

type MemoriesRepository struct {
	*baseRepository[Memory]
}

type MemoriesRepositoryInterface interface {
	Create(ctx context.Context, entity *Memory) error
	Update(ctx context.Context, entity *Memory) error
	Delete(ctx context.Context, id uint) error
	GetAll(ctx context.Context) (*[]Memory, error)
	UpdateHitCountAndLRA(ctx context.Context, ids []uint) error
}

func NewMemoriesRepository(db *gorm.DB) *MemoriesRepository {
	return &MemoriesRepository{
		baseRepository: newBaseRepository[Memory](db),
	}
}

func recursiveGetRidOfTwoSpaces(str string) string {
	str = strings.ReplaceAll(str, "  ", " ")
	if strings.Contains(str, "  ") {
		str = recursiveGetRidOfTwoSpaces(str)
	}

	return str
}

func normalizeStr(searchQuery string) string {
	searchQuery = strings.ReplaceAll(searchQuery, "\t", " ")
	searchQuery = strings.ReplaceAll(searchQuery, "\n", " ")
	searchQuery = strings.ReplaceAll(searchQuery, "\"", " ")
	searchQuery = recursiveGetRidOfTwoSpaces(searchQuery)
	return searchQuery
}

func conditionBuilderForBM25(searchQuery string) (*string, error) {
	searchQuery = normalizeStr(searchQuery)

	if searchQuery == "" || searchQuery == " " {
		return nil, fmt.Errorf("empty string search is not possible")
	}

	splitQuery := strings.Split(searchQuery, " ")

	safeAndQuoted := ""

	// my brain can not comprehend how I'd do this with join
	// so I'll just do it manually
	for i, kw := range splitQuery {
		if kw == "" {
			safeAndQuoted, _ = strings.CutSuffix(safeAndQuoted, "OR")
			continue
		}

		// fair game
		safeAndQuoted += `"` + kw + `"`

		// last word don't append anything
		if i == len(splitQuery)-1 {
			// yea, I don't panic break, I continue, I'm fancy like that
			continue
		}
		safeAndQuoted += "OR"

	}

	return &safeAndQuoted, nil
}

// TODO: test this, it may be that I have to add .Model(&Memory{})
// every gorm doc does so, but my experience with the generics API hasn't forced me to
// so just test it
func (r *MemoriesRepository) UpdateHitCountAndLRA(ctx context.Context, ids []uint) error {
	return r.db.WithContext(ctx).Where("id IN ?", ids).Updates(map[string]any{
		"hit_count":         gorm.Expr("hit_count + ?", 1),
		"last_retrieved_at": time.Now(),
	}).Error
}
