package persistence

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type PersSimilarityItem interface {
	persSimilarityItem()
	GetNormalizedVector() []float32
}

func (t Topic) persSimilarityItem()  {}
func (m Memory) persSimilarityItem() {}
func (t Topic) GetNormalizedVector() []float32 {
	return t.NormalizedVector
}
func (m Memory) GetNormalizedVector() []float32 {
	return m.NormalizedVector
}

type BaseModel struct {
	Id        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime" json:"updatedAt"`
}

type baseRepository[t any] struct {
	db *gorm.DB
}

func newBaseRepository[t any](db *gorm.DB) *baseRepository[t] {
	return &baseRepository[t]{db: db}
}

func (r *baseRepository[t]) Create(ctx context.Context, entity *t) error {
	return r.db.WithContext(ctx).Create(entity).Error
}

func (r *baseRepository[t]) Update(ctx context.Context, entity *t) error {
	return r.db.WithContext(ctx).Save(entity).Error
}

func (r *baseRepository[t]) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(new(t), id).Error
}

func (r *baseRepository[t]) GetAll(ctx context.Context) (*[]t, error) {
	var results []t
	err := r.db.WithContext(ctx).Find(&results).Error
	return &results, err
}
