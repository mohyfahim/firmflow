package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DeviceType can be global predefined (ProjectID nil, IsPredefined true) or project-scoped custom.
type DeviceType struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey"`

	ProjectID     *uuid.UUID `gorm:"type:uuid;index"`
	IsPredefined  bool       `gorm:"not null;default:false;index"`
	Name          string     `gorm:"size:200;not null;index"`
	NameNormalized string    `gorm:"size:200;not null;index"`

	ProcessorArchitecture string `gorm:"size:64;not null"`
	HardwareBoardVersion  string `gorm:"size:64;not null"`
	FlashSizeBytes        int64  `gorm:"not null"`
	MemoryNotes           string `gorm:"size:1024"`

	DeletedAt gorm.DeletedAt `gorm:"index"`
	CreatedAt time.Time      `gorm:"not null"`
	UpdatedAt time.Time      `gorm:"not null"`
}

func (t *DeviceType) BeforeCreate(_ *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

// DeviceAuthToken stores a hashed device authentication token.
// Raw tokens are never persisted.
type DeviceAuthToken struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey"`

	DeviceID uuid.UUID `gorm:"type:uuid;index;not null"`
	TokenHash string   `gorm:"size:255;uniqueIndex;not null"`

	// RevokedAt makes old tokens unusable immediately after rotation.
	RevokedAt *time.Time `gorm:"index"`
	// CreatedAt is the token issuance time.
	CreatedAt time.Time `gorm:"not null;index"`

	// Optional: link to the new token created by rotation (helps audit / bulk rotation later).
	ReplacedByTokenID *uuid.UUID `gorm:"type:uuid;index"`
	RevokedByUserID   *uuid.UUID `gorm:"type:uuid;index"`
}

func (t *DeviceAuthToken) BeforeCreate(_ *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	return nil
}

// DeviceConnectionLog stores poll/report (and later other) connections from a device.
// It is append-only and queryable for audit / debugging / analytics.
type DeviceConnectionLog struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey"`

	ProjectID uuid.UUID `gorm:"type:uuid;index;not null"`
	DeviceID  uuid.UUID `gorm:"type:uuid;index;not null"`

	IP        string `gorm:"size:64;index"`
	UserAgent string `gorm:"size:512"`

	Action   string `gorm:"size:32;index;not null"`    // poll | report
	Endpoint string `gorm:"size:256;index;not null"`    // full path/action

	CreatedAt time.Time `gorm:"not null;index"`
}

func (l *DeviceConnectionLog) BeforeCreate(_ *gorm.DB) error {
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	if l.CreatedAt.IsZero() {
		l.CreatedAt = time.Now().UTC()
	}
	return nil
}

// DeviceBlockEvent preserves block/unblock history.
type DeviceBlockEvent struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey"`

	ProjectID uuid.UUID `gorm:"type:uuid;index;not null"`
	DeviceID  uuid.UUID `gorm:"type:uuid;index;not null"`

	// Action is blocked | unblocked
	Action string `gorm:"size:16;not null;index"`

	ActorUserID *uuid.UUID `gorm:"type:uuid;index"`
	CreatedAt   time.Time  `gorm:"not null;index"`
}

func (e *DeviceBlockEvent) BeforeCreate(_ *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	return nil
}

// DeviceGroup is a named collection of devices within one project.
type DeviceGroup struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	ProjectID uuid.UUID `gorm:"type:uuid;index;not null"`

	Name           string `gorm:"size:200;not null;index"`
	NameNormalized string `gorm:"size:200;not null;index"`
	Description    string `gorm:"size:1024"`

	CreatedAt time.Time      `gorm:"not null"`
	UpdatedAt time.Time      `gorm:"not null"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (g *DeviceGroup) BeforeCreate(_ *gorm.DB) error {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	return nil
}

// DeviceGroupMembership links a device to a group (many-to-many).
// Composite primary key; idempotent inserts rely on unique (device_group_id, device_id).
type DeviceGroupMembership struct {
	DeviceGroupID uuid.UUID `gorm:"type:uuid;primaryKey;index"`
	DeviceID      uuid.UUID `gorm:"type:uuid;primaryKey;index"`
	CreatedAt     time.Time `gorm:"not null"`
}

func (m *DeviceGroupMembership) BeforeCreate(_ *gorm.DB) error {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	return nil
}

