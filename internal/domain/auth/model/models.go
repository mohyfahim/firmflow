package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type User struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey"`
	Email            string    `gorm:"size:320;uniqueIndex;not null"`
	PasswordHash     string    `gorm:"size:255;not null"`
	EmailVerifiedAt  *time.Time
	TwoFactorEnabled bool `gorm:"not null;default:false"`
	DeletionGraceAt  *time.Time
	DeletedAt        gorm.DeletedAt `gorm:"index"`
	CreatedAt        time.Time      `gorm:"not null"`
	UpdatedAt        time.Time      `gorm:"not null"`
	Profile          UserProfile    `gorm:"constraint:OnDelete:CASCADE"`
}

type UserProfile struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID            uuid.UUID `gorm:"type:uuid;uniqueIndex;not null"`
	FirstName         string    `gorm:"size:100"`
	LastName          string    `gorm:"size:100"`
	AvatarURL         string    `gorm:"size:512"`
	CompanyName       string    `gorm:"size:150"`
	PhoneNumber       string    `gorm:"size:50"`
	Timezone          string    `gorm:"size:100;not null;default:'UTC'"`
	PreferredLanguage string    `gorm:"size:32;not null;default:'en'"`
	CreatedAt         time.Time `gorm:"not null"`
	UpdatedAt         time.Time `gorm:"not null"`
}

type AuthSession struct {
	ID                  uuid.UUID  `gorm:"type:uuid;primaryKey"`
	UserID              uuid.UUID  `gorm:"type:uuid;index;not null"`
	RefreshTokenHash    string     `gorm:"size:255;not null;uniqueIndex"`
	ExpiresAt           time.Time  `gorm:"index;not null"`
	UserAgent           string     `gorm:"size:512"`
	IP                  string     `gorm:"size:64"`
	LastSeenAt          time.Time  `gorm:"not null"`
	RevokedAt           *time.Time `gorm:"index"`
	ReplacedBySessionID *uuid.UUID `gorm:"type:uuid"`
	CreatedAt           time.Time  `gorm:"not null"`
	UpdatedAt           time.Time  `gorm:"not null"`
}

type EmailVerificationToken struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey"`
	UserID     uuid.UUID  `gorm:"type:uuid;index;not null"`
	TokenHash  string     `gorm:"size:255;uniqueIndex;not null"`
	ExpiresAt  time.Time  `gorm:"index;not null"`
	ConsumedAt *time.Time `gorm:"index"`
	CreatedAt  time.Time  `gorm:"not null"`
}

type PasswordResetToken struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey"`
	UserID     uuid.UUID  `gorm:"type:uuid;index;not null"`
	TokenHash  string     `gorm:"size:255;uniqueIndex;not null"`
	ExpiresAt  time.Time  `gorm:"index;not null"`
	ConsumedAt *time.Time `gorm:"index"`
	CreatedAt  time.Time  `gorm:"not null"`
}

type AuthThrottle struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	EmailKey       string    `gorm:"size:320;uniqueIndex;not null"`
	FailedAttempts int       `gorm:"not null;default:0"`
	FirstFailedAt  *time.Time
	LockUntil      *time.Time `gorm:"index"`
	UpdatedAt      time.Time  `gorm:"not null"`
	CreatedAt      time.Time  `gorm:"not null"`
}

type TwoFactorSetting struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID          uuid.UUID `gorm:"type:uuid;uniqueIndex;not null"`
	SecretEncrypted string    `gorm:"size:512;not null"`
	EnabledAt       *time.Time
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
}

type RecoveryCode struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TwoFactorSettingID uuid.UUID  `gorm:"type:uuid;index;not null"`
	CodeHash           string     `gorm:"size:255;uniqueIndex;not null"`
	UsedAt             *time.Time `gorm:"index"`
	CreatedAt          time.Time  `gorm:"not null"`
}

type OauthIdentity struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID         uuid.UUID `gorm:"type:uuid;index;not null"`
	Provider       string    `gorm:"size:100;not null"`
	ProviderUserID string    `gorm:"size:255;not null"`
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

type AuditLog struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey"`
	ActorUserID *uuid.UUID     `gorm:"type:uuid;index"`
	Event       string         `gorm:"size:100;index;not null"`
	TargetType  string         `gorm:"size:100;not null"`
	TargetID    string         `gorm:"size:100;not null"`
	Metadata    datatypes.JSON `gorm:"type:jsonb"`
	CreatedAt   time.Time      `gorm:"not null;index"`
}

func (u *User) BeforeCreate(_ *gorm.DB) error                   { u.ID = uuid.New(); return nil }
func (u *UserProfile) BeforeCreate(_ *gorm.DB) error            { u.ID = uuid.New(); return nil }
func (a *AuthSession) BeforeCreate(_ *gorm.DB) error            { a.ID = uuid.New(); return nil }
func (e *EmailVerificationToken) BeforeCreate(_ *gorm.DB) error { e.ID = uuid.New(); return nil }
func (p *PasswordResetToken) BeforeCreate(_ *gorm.DB) error     { p.ID = uuid.New(); return nil }
func (a *AuthThrottle) BeforeCreate(_ *gorm.DB) error           { a.ID = uuid.New(); return nil }
func (t *TwoFactorSetting) BeforeCreate(_ *gorm.DB) error       { t.ID = uuid.New(); return nil }
func (r *RecoveryCode) BeforeCreate(_ *gorm.DB) error           { r.ID = uuid.New(); return nil }
func (o *OauthIdentity) BeforeCreate(_ *gorm.DB) error          { o.ID = uuid.New(); return nil }
func (a *AuditLog) BeforeCreate(_ *gorm.DB) error               { a.ID = uuid.New(); return nil }
