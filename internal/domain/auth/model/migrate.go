package model

import (
	"context"

	"gorm.io/gorm"
)

type Migrator struct{}

func (Migrator) Migrate(_ context.Context, db *gorm.DB) error {
	return db.AutoMigrate(
		&User{},
		&UserProfile{},
		&AuthSession{},
		&EmailVerificationToken{},
		&PasswordResetToken{},
		&AuthThrottle{},
		&TwoFactorSetting{},
		&RecoveryCode{},
		&OauthIdentity{},
		&AuditLog{},
	)
}
