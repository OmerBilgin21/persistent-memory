package persistence

import (
	"context"
	"database/sql"
	"errors"
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
	CreatedAt time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

type Tx struct {
	db *gorm.DB
}

func (t *Tx) Commit() error {
	return t.db.Commit().Error
}

func (t *Tx) Rollback() error {
	if err := t.db.Rollback().Error; err != nil && !errors.Is(err, sql.ErrTxDone) {
		return err
	}

	return nil
}

type Store struct {
	db         *gorm.DB
	Memories   MemoriesRepositoryInterface
	Topics     TopicsRepositoryInterface
	Injections InjectionsRepositoryInterface
}

func NewStore(db *gorm.DB) *Store {
	return &Store{
		db:         db,
		Memories:   NewMemoriesRepository(db),
		Topics:     NewTopicsRepository(db),
		Injections: NewInjectionsRepository(db),
	}
}

// Begin returns a second Store whose repositories all share one transaction,
// alongside the handle that commits or rolls it back.
func (s *Store) Begin(ctx context.Context) (*Store, *Tx, error) {
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, nil, tx.Error
	}

	return NewStore(tx), &Tx{db: tx}, nil
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

func (r *baseRepository[t]) Get(ctx context.Context, id uint) (*t, error) {
	var result t

	err := r.db.WithContext(ctx).First(&result, map[string]any{"id": id}).Error
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (r *baseRepository[t]) Update(ctx context.Context, entity *t) error {
	return r.db.WithContext(ctx).Save(entity).Error
}

func (r *baseRepository[t]) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(new(t), id).Error
}

func (r *baseRepository[t]) BulkDelete(ctx context.Context, ids []uint) error {
	return r.db.WithContext(ctx).Where("id IN ?", ids).Delete(new(t)).Error
}

func (r *baseRepository[t]) GetAll(ctx context.Context) (*[]t, error) {
	var results []t
	err := r.db.WithContext(ctx).Find(&results).Error
	return &results, err
}
