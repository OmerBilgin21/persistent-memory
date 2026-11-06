package persistence

import (
	"context"
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

// TODO: test this, it may be that I have to add .Model(&Memory{})
// every gorm doc does so, but my experience with the generics API hasn't forced me to
// so just test it
func (r *MemoriesRepository) UpdateHitCountAndLRA(ctx context.Context, ids []uint) error {
	return r.db.WithContext(ctx).Where("id IN ?", ids).Updates(map[string]any{
		"hit_count":         gorm.Expr("hit_count + ?", 1),
		"last_retrieved_at": time.Now(),
	}).Error
}
