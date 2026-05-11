package model

import (
	"time"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Device is a minimal project-scoped device row for summary counts and delete/revocation flows.
// A full device module can extend this model later.
type Device struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey"`
	ProjectID                uuid.UUID      `gorm:"type:uuid;index;not null;uniqueIndex:ux_project_device_hw,priority:1"`
	// ConnectionStatus is a cached label for dashboard queries. The source of truth
	// for online/offline remains LastSeenAt (see GetProjectSummary).
	ConnectionStatus         string         `gorm:"size:16;not null;default:'offline';index"` // online | offline
	// Hardware identity: stored normalized for uniqueness checks and lookup.
	Name                     string         `gorm:"size:200;not null;index"`
	DeviceTypeID             uuid.UUID      `gorm:"type:uuid;index;not null"`
	HardwareIdentifier      string         `gorm:"size:128;not null;index"`
	HardwareIdentifierNorm  string         `gorm:"size:128;not null;index;uniqueIndex:ux_project_device_hw,priority:2"`
	Blocked                  bool           `gorm:"not null;default:false;index"`
	BlockedAt                *time.Time     `gorm:"index"`
	LastSeenAt               *time.Time     `gorm:"index"`
	CurrentFirmwareVersion  string         `gorm:"size:64;not null;default:'';index"`
	// Token metadata (never raw token). The active token is rotated immediately by updating these fields.
	ActiveAuthTokenID        *uuid.UUID    `gorm:"type:uuid;index"`
	ActiveAuthTokenIssuedAt  *time.Time    `gorm:"index"`
	// Gate device connections when project is deleted/archived.
	TokenRevokedAt          *time.Time     `gorm:"index"`
	PollingDisabledAt      *time.Time     `gorm:"index"` // set when project deleted/archived for polling gate
	CreatedAt                time.Time      `gorm:"not null"`
	UpdatedAt                time.Time      `gorm:"not null"`
	DeletedAt                gorm.DeletedAt `gorm:"index"`
}

func (d *Device) BeforeCreate(_ *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	// Normalize for lookup + uniqueness.
	d.HardwareIdentifierNorm = normalizeHardwareIdentifier(d.HardwareIdentifier)
	if d.ActiveAuthTokenIssuedAt != nil && d.ActiveAuthTokenID == nil {
		// Defensive: token metadata must be consistent.
		d.ActiveAuthTokenIssuedAt = nil
	}
	return nil
}

func normalizeHardwareIdentifier(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.ToLower(raw)
	return raw
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
