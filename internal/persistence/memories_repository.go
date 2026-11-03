package persistence

import (
	"context"
	"time"
)

type Memory struct {
	Id              uint      `gorm:"primarykey" json:"id"`
	TopicId         uint      `gorm:"constraint:OnDelete:CASCADE" json:"topicId"`
	Content         string    `gorm:"not null" json:"content"`
	LastRetrievedAt time.Time `gorm:"not null" json:"lastRetrievedAt"`
	HitCount        int       `gorm:"not null;default: 0" json:"hitCount"`
}

type memoriesRepository struct {
	*baseRepository[Memory]
}

type MemoriesRepository interface {
	Create(ctx context.Context, entity *Memory) error
	Update(ctx context.Context, entity *Memory) error
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context) (*[]Memory, error)
}
