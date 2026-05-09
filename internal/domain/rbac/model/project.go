package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Project is a tenant boundary for devices, firmware, and campaigns.
// Deletion is soft (DeletedAt) so historical audit and referential rows can be retained; device/campaign tables still treat deleted projects as inactive.
type Project struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey"`
	Name        string         `gorm:"size:200;not null"`
	Description string         `gorm:"type:text"`
	ArchivedAt  *time.Time     `gorm:"index"`
	CreatedAt   time.Time      `gorm:"not null"`
	UpdatedAt   time.Time      `gorm:"not null"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (p *Project) BeforeCreate(_ *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}
