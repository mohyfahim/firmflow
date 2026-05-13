package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Firmware is a project-scoped firmware artifact (binary + metadata).
type Firmware struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	ProjectID uuid.UUID `gorm:"type:uuid;index:idx_firmware_project;not null"`

	Version           string `gorm:"size:128;not null"`
	VersionNormalized string `gorm:"size:128;not null;index:idx_firmware_version_norm"`

	Changelog string `gorm:"type:text"`

	FileSizeBytes    int64  `gorm:"not null"`
	ChecksumSHA256   string `gorm:"size:64;not null;index"`
	OriginalFilename string `gorm:"size:255;not null"`

	StorageProvider string `gorm:"size:32;not null"` // local | s3 (future)
	StorageKey      string `gorm:"size:512;not null;index"`

	// SemverMajor/Minor/Patch are set when Version parses as semver (core only).
	SemverMajor      *int   `gorm:"index"`
	SemverMinor      *int   `gorm:"index"`
	SemverPatch      *int   `gorm:"index"`
	SemverPrerelease string `gorm:"size:128"` // optional; ordering vs releases is best-effort in SQL

	UploadedByUserID uuid.UUID `gorm:"type:uuid;index;not null"`

	CreatedAt time.Time      `gorm:"not null;index"`
	UpdatedAt time.Time      `gorm:"not null"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (Firmware) TableName() string { return "firmwares" }

func (f *Firmware) BeforeCreate(_ *gorm.DB) error {
	if f.ID == uuid.Nil {
		f.ID = uuid.New()
	}
	return nil
}

// FirmwareDeviceType links a firmware to compatible device types (M2M).
type FirmwareDeviceType struct {
	FirmwareID   uuid.UUID `gorm:"type:uuid;primaryKey"`
	DeviceTypeID uuid.UUID `gorm:"type:uuid;primaryKey"`
	CreatedAt    time.Time `gorm:"not null"`
}

func (FirmwareDeviceType) TableName() string { return "firmware_device_types" }

func (m *FirmwareDeviceType) BeforeCreate(_ *gorm.DB) error {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	return nil
}
