package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Campaign is an OTA rollout for a firmware within a project.
type Campaign struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	ProjectID uuid.UUID `gorm:"type:uuid;index:idx_campaign_project_status;not null"`

	Name string `gorm:"size:200;not null"`

	FirmwareID uuid.UUID `gorm:"type:uuid;index;not null"`

	RolloutKind      string     `gorm:"size:32;not null;index"` // immediate | time_scheduled | percentage
	RolloutPercent   *int       `gorm:"index"`                  // 1-100 when rollout_kind=percentage
	ScheduledStartAt *time.Time `gorm:"index"`                  // UTC; required for time_scheduled; optional deferral for others

	Status string `gorm:"size:32;not null;index:idx_campaign_project_status"`

	TargetDeviceCount int64 `gorm:"not null"` // assignments created (after percentage slice)

	ActivatedAt *time.Time `gorm:"index"`
	PausedAt    *time.Time `gorm:"index"`
	CancelledAt *time.Time `gorm:"index"`
	CompletedAt *time.Time `gorm:"index"`

	CreatedByUserID uuid.UUID `gorm:"type:uuid;index;not null"`

	CreatedAt time.Time      `gorm:"not null;index"`
	UpdatedAt time.Time      `gorm:"not null"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (Campaign) TableName() string { return "campaigns" }

func (c *Campaign) BeforeCreate(_ *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

// CampaignTargetGroup records a device group included in campaign targeting.
type CampaignTargetGroup struct {
	CampaignID    uuid.UUID `gorm:"type:uuid;primaryKey"`
	DeviceGroupID uuid.UUID `gorm:"type:uuid;primaryKey"`
	CreatedAt     time.Time `gorm:"not null"`
}

func (CampaignTargetGroup) TableName() string { return "campaign_target_groups" }

// CampaignTargetDevice records an explicitly targeted device.
type CampaignTargetDevice struct {
	CampaignID uuid.UUID `gorm:"type:uuid;primaryKey"`
	DeviceID   uuid.UUID `gorm:"type:uuid;primaryKey"`
	CreatedAt  time.Time `gorm:"not null"`
}

func (CampaignTargetDevice) TableName() string { return "campaign_target_devices" }

// CampaignDeviceAssignment is one device's place in a rollout (stable per campaign).
type CampaignDeviceAssignment struct {
	CampaignID uuid.UUID `gorm:"type:uuid;primaryKey;index:idx_cda_campaign_status"`
	DeviceID   uuid.UUID `gorm:"type:uuid;primaryKey;index:idx_cda_device"`

	Status string `gorm:"size:32;not null;index:idx_cda_campaign_status"`

	LastReportCode *uint16 `gorm:"index"`
	LastReportMsg  string    `gorm:"size:256"`

	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (CampaignDeviceAssignment) TableName() string { return "campaign_device_assignments" }

func (a *CampaignDeviceAssignment) BeforeCreate(_ *gorm.DB) error {
	now := time.Now().UTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = now
	}
	return nil
}
