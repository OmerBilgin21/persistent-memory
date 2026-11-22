package persistence

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Injection struct {
	SessionId string    `gorm:"primaryKey" json:"session_id"`
	MemoryId  uint      `gorm:"primaryKey;autoIncrement:false" json:"memory_id"`
	Memory    *Memory   `gorm:"constraint:OnDelete:CASCADE;foreignKey:MemoryId" json:"-"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
}

type InjectionsRepository struct {
	db *gorm.DB
}

type InjectionsRepositoryInterface interface {
	CreateMany(ctx context.Context, sessionId string, memoryIds []uint) error
	GetMemoryIdsBySessionId(ctx context.Context, sessionId string) ([]uint, error)
	DeleteBySessionId(ctx context.Context, sessionId string) error
}

func NewInjectionsRepository(db *gorm.DB) *InjectionsRepository {
	return &InjectionsRepository{db: db}
}

func (r *InjectionsRepository) CreateMany(ctx context.Context, sessionId string, memoryIds []uint) error {
	if len(memoryIds) == 0 {
		return nil
	}

	toBeCreated := make([]Injection, 0, len(memoryIds))
	for _, id := range memoryIds {
		toBeCreated = append(toBeCreated, Injection{
			SessionId: sessionId,
			MemoryId:  id,
		})
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&toBeCreated).Error
}

func (r *InjectionsRepository) GetMemoryIdsBySessionId(ctx context.Context, sessionId string) ([]uint, error) {
	var ids []uint
	err := r.db.WithContext(ctx).Model(&Injection{}).Where("session_id = ?", sessionId).Pluck("memory_id", &ids).Error
	if err != nil {
		return nil, err
	}

	return ids, nil
}

func (r *InjectionsRepository) DeleteBySessionId(ctx context.Context, sessionId string) error {
	return r.db.WithContext(ctx).Where("session_id = ?", sessionId).Delete(&Injection{}).Error
}
