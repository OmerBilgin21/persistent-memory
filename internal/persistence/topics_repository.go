package persistence

import (
	"context"
	"time"
)

type Topic struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	Description     string    `gorm:"not null" json:"description"`
	Name            string    `gorm:"uniqueIndex;not null" json:"name"`
	LastRetrievedAt time.Time `gorm:"not null" json:"lastRetrievedAt"`
	HitCount        int       `gorm:"not null;default: 0" json:"hitCount"`
	Memories        []Memory  `gorm:"foreignKey:TopicId" json:"memories"`
}

type topicsRepository struct {
	*baseRepository[Topic]
}

type TopicsRepository interface {
	Create(ctx context.Context, entity *Topic) error
	Update(ctx context.Context, entity *Topic) error
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context) (*[]Topic, error)
}
