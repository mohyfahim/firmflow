package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Device is a minimal project-scoped device row for summary counts and delete/revocation flows.
// A full device module can extend this model later.
type Device struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey"`
	ProjectID         uuid.UUID      `gorm:"type:uuid;index;not null"`
	ConnectionStatus  string         `gorm:"size:16;not null;default:'offline';index"` // online | offline
	TokenRevokedAt    *time.Time     `gorm:"index"`
	PollingDisabledAt *time.Time     `gorm:"index"` // set when project deleted/archived for polling gate
	CreatedAt         time.Time      `gorm:"not null"`
	UpdatedAt         time.Time      `gorm:"not null"`
	DeletedAt         gorm.DeletedAt `gorm:"index"`
}

func (d *Device) BeforeCreate(_ *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return nil
}

// Campaign is a minimal row used to block project deletion when non-terminal campaigns exist.
type Campaign struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey"`
	ProjectID uuid.UUID      `gorm:"type:uuid;index;not null"`
	Status    string         `gorm:"size:32;not null;index"` // draft | scheduled | active | paused | completed | cancelled
	CreatedAt time.Time      `gorm:"not null"`
	UpdatedAt time.Time      `gorm:"not null"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (c *Campaign) BeforeCreate(_ *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

// DeviceUpdateReport stores coarse-grained OTA outcomes for dashboard summaries.
type DeviceUpdateReport struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	ProjectID uuid.UUID `gorm:"type:uuid;index;not null"`
	Outcome   string    `gorm:"size:16;not null;index"` // success | failure
	CreatedAt time.Time `gorm:"not null;index"`
}

func (r *DeviceUpdateReport) BeforeCreate(_ *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}
