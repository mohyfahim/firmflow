package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OtaDownloadToken is a short-lived secret allowing a device to fetch firmware bytes
// over HTTPS without the long-lived device session token in the URL.
type OtaDownloadToken struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey"`

	TokenHash string `gorm:"size:64;not null;uniqueIndex"`

	ProjectID  uuid.UUID `gorm:"type:uuid;index;not null"`
	DeviceID   uuid.UUID `gorm:"type:uuid;index;not null"`
	CampaignID uuid.UUID `gorm:"type:uuid;index;not null"`
	FirmwareID uuid.UUID `gorm:"type:uuid;index;not null"`

	ExpiresAt  time.Time  `gorm:"not null;index"`
	ConsumedAt *time.Time `gorm:"index"`

	CreatedAt time.Time `gorm:"not null"`
}

func (OtaDownloadToken) TableName() string { return "ota_download_tokens" }

func (t *OtaDownloadToken) BeforeCreate(_ *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	return nil
}
