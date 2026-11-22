package persistence

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Setting struct {
	Key   string `gorm:"primaryKey" json:"key"`
	Value string `gorm:"not null" json:"value"`
}

type SettingsRepository struct {
	db *gorm.DB
}

type SettingsRepositoryInterface interface {
	Get(ctx context.Context, key string) (*Setting, error)
	Set(ctx context.Context, key, value string) error
}

func NewSettingsRepository(db *gorm.DB) *SettingsRepository {
	return &SettingsRepository{db: db}
}

func (r *SettingsRepository) Get(ctx context.Context, key string) (*Setting, error) {
	var result Setting
	err := r.db.WithContext(ctx).First(&result, map[string]string{
		"key": key,
	}).Error
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (r *SettingsRepository) Set(ctx context.Context, key, value string) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&Setting{
		Key:   key,
		Value: value,
	}).Error
}
