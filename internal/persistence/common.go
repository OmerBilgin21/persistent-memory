package persistence

import (
	"context"

	"gorm.io/gorm"
)

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
