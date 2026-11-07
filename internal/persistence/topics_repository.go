package persistence

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type Topic struct {
	BaseModel
	Description      string    `gorm:"not null" json:"description"`
	Name             string    `gorm:"uniqueIndex;not null" json:"name"`
	LastRetrievedAt  time.Time `gorm:"not null;autoCreateTime" json:"lastRetrievedAt"`
	HitCount         int       `gorm:"not null;default: 0" json:"hitCount"`
	Memories         []Memory  `gorm:"foreignKey:TopicId" json:"memories"`
	NormalizedVector Vector    `gorm:"not null" json:"normalizedVector"`
}

type TopicsRepository struct {
	*baseRepository[Topic]
}

type TopicsRepositoryInterface interface {
	Create(ctx context.Context, entity *Topic) error
	Update(ctx context.Context, entity *Topic) error
	Delete(ctx context.Context, id uint) error
	GetAll(ctx context.Context) (*[]Topic, error)
	GetByName(ctx context.Context, name string) (*Topic, error)
	UpdateHitCountAndLRA(ctx context.Context, ids []uint) error
	GetByIds(ctx context.Context, ids []uint) (*[]Topic, error)
}

func NewTopicsRepository(db *gorm.DB) *TopicsRepository {
	return &TopicsRepository{
		baseRepository: newBaseRepository[Topic](db),
	}
}

func (r *TopicsRepository) GetByIds(ctx context.Context, ids []uint) (*[]Topic, error) {
	var result []Topic
	err := r.db.WithContext(ctx).Find(&result, map[string]any{
		"id": ids,
	}).Error
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (r *TopicsRepository) GetByName(ctx context.Context, name string) (*Topic, error) {
	var result Topic
	err := r.db.WithContext(ctx).Find(&result, map[string]string{
		"name": name,
	}).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// TODO: test this, it may be that I have to add .Model(&Topic{})
// every gorm doc does so, but my experience with the generics API hasn't forced me to
// so just test it
func (r *TopicsRepository) UpdateHitCountAndLRA(ctx context.Context, ids []uint) error {
	return r.db.WithContext(ctx).Where("id IN ?", ids).Updates(map[string]any{
		"hit_count":         gorm.Expr("hit_count + ?", 1),
		"last_retrieved_at": time.Now(),
	}).Error
}
