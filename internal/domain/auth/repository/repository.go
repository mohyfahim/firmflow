package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"firmflow/internal/domain/auth/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) DB() *gorm.DB { return r.db }

func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func (r *Repository) CreateUserWithProfile(ctx context.Context, user *model.User, profile *model.UserProfile) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		profile.UserID = user.ID
		return tx.Create(profile).Error
	})
}

func (r *Repository) CreateEmailVerificationToken(ctx context.Context, token *model.EmailVerificationToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

func (r *Repository) FindUserByEmail(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("email = ?", NormalizeEmail(email)).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) FindUserByID(ctx context.Context, userID uuid.UUID) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).First(&user, "id = ?", userID).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) GetProfile(ctx context.Context, userID uuid.UUID) (*model.UserProfile, error) {
	var profile model.UserProfile
	if err := r.db.WithContext(ctx).First(&profile, "user_id = ?", userID).Error; err != nil {
		return nil, err
	}
	return &profile, nil
}

func (r *Repository) UpdateProfile(ctx context.Context, profile *model.UserProfile) error {
	return r.db.WithContext(ctx).Save(profile).Error
}

func (r *Repository) ConsumeEmailVerificationToken(ctx context.Context, tokenHash string) (*model.User, error) {
	var user *model.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var token model.EmailVerificationToken
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("token_hash = ? AND consumed_at IS NULL AND expires_at > ?", tokenHash, time.Now().UTC()).
			First(&token).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := tx.Model(&token).Update("consumed_at", now).Error; err != nil {
			return err
		}
		var u model.User
		if err := tx.First(&u, "id = ?", token.UserID).Error; err != nil {
			return err
		}
		if err := tx.Model(&u).Update("email_verified_at", now).Error; err != nil {
			return err
		}
		user = &u
		return nil
	})
	return user, err
}

func (r *Repository) CreateSession(ctx context.Context, session *model.AuthSession) error {
	return r.db.WithContext(ctx).Create(session).Error
}

func (r *Repository) GetSessionByRefreshHash(ctx context.Context, hash string) (*model.AuthSession, error) {
	var session model.AuthSession
	err := r.db.WithContext(ctx).
		Where("refresh_token_hash = ? AND revoked_at IS NULL AND expires_at > ?", hash, time.Now().UTC()).
		First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *Repository) RevokeSession(ctx context.Context, sessionID uuid.UUID) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&model.AuthSession{}).Where("id = ? AND revoked_at IS NULL", sessionID).
		Updates(map[string]interface{}{"revoked_at": now, "last_seen_at": now}).Error
}

func (r *Repository) RotateSession(ctx context.Context, oldSessionID uuid.UUID, newSession *model.AuthSession) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(newSession).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		return tx.Model(&model.AuthSession{}).Where("id = ? AND revoked_at IS NULL", oldSessionID).Updates(map[string]interface{}{
			"revoked_at":             now,
			"replaced_by_session_id": newSession.ID,
		}).Error
	})
}

func (r *Repository) CreateResetToken(ctx context.Context, token *model.PasswordResetToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

func (r *Repository) ConsumeResetTokenAndSetPassword(ctx context.Context, tokenHash string, passwordHash string) (uuid.UUID, error) {
	var userID uuid.UUID
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var token model.PasswordResetToken
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("token_hash = ? AND consumed_at IS NULL AND expires_at > ?", tokenHash, time.Now().UTC()).
			First(&token).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := tx.Model(&token).Update("consumed_at", now).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.User{}).Where("id = ?", token.UserID).Update("password_hash", passwordHash).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.AuthSession{}).Where("user_id = ? AND revoked_at IS NULL", token.UserID).
			Update("revoked_at", now).Error; err != nil {
			return err
		}
		userID = token.UserID
		return nil
	})
	return userID, err
}

func (r *Repository) UpdatePassword(ctx context.Context, userID uuid.UUID, newHash string) error {
	return r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).Update("password_hash", newHash).Error
}

func (r *Repository) RevokeAllOtherSessions(ctx context.Context, userID uuid.UUID, currentSessionID uuid.UUID) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&model.AuthSession{}).
		Where("user_id = ? AND id <> ? AND revoked_at IS NULL", userID, currentSessionID).
		Update("revoked_at", now).Error
}

func (r *Repository) RevokeAllSessions(ctx context.Context, userID uuid.UUID) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&model.AuthSession{}).Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", now).Error
}

func (r *Repository) ListActiveSessions(ctx context.Context, userID uuid.UUID) ([]model.AuthSession, error) {
	var out []model.AuthSession
	err := r.db.WithContext(ctx).Where("user_id = ? AND revoked_at IS NULL", userID).Order("created_at desc").Find(&out).Error
	return out, err
}

func (r *Repository) CreateOrIncrementThrottle(ctx context.Context, email string, window time.Duration, maxAttempts int) (bool, time.Time, error) {
	var lockUntil time.Time
	locked := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		key := NormalizeEmail(email)
		now := time.Now().UTC()
		var throttle model.AuthThrottle
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("email_key = ?", key).First(&throttle).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			throttle = model.AuthThrottle{EmailKey: key, FailedAttempts: 1}
			return tx.Create(&throttle).Error
		}
		if err != nil {
			return err
		}
		if throttle.FirstFailedAt == nil || now.Sub(*throttle.FirstFailedAt) > window {
			throttle.FailedAttempts = 1
			throttle.FirstFailedAt = &now
			throttle.LockUntil = nil
			return tx.Save(&throttle).Error
		}
		throttle.FailedAttempts++
		if throttle.FailedAttempts >= maxAttempts {
			t := now.Add(window)
			throttle.LockUntil = &t
			lockUntil = t
			locked = true
		}
		return tx.Save(&throttle).Error
	})
	return locked, lockUntil, err
}

func (r *Repository) IsEmailLocked(ctx context.Context, email string) (bool, time.Time, error) {
	var throttle model.AuthThrottle
	err := r.db.WithContext(ctx).Where("email_key = ?", NormalizeEmail(email)).First(&throttle).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, time.Time{}, nil
	}
	if err != nil {
		return false, time.Time{}, err
	}
	if throttle.LockUntil != nil && throttle.LockUntil.After(time.Now().UTC()) {
		return true, *throttle.LockUntil, nil
	}
	return false, time.Time{}, nil
}

func (r *Repository) ResetThrottle(ctx context.Context, email string) error {
	return r.db.WithContext(ctx).Where("email_key = ?", NormalizeEmail(email)).Delete(&model.AuthThrottle{}).Error
}

func (r *Repository) SaveTwoFactor(ctx context.Context, tfa *model.TwoFactorSetting) error {
	return r.db.WithContext(ctx).Where("user_id = ?", tfa.UserID).Delete(&model.TwoFactorSetting{}).Error
}

func (r *Repository) UpsertTwoFactor(ctx context.Context, tfa *model.TwoFactorSetting) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_ = tx.Where("user_id = ?", tfa.UserID).Delete(&model.TwoFactorSetting{}).Error
		return tx.Create(tfa).Error
	})
}

func (r *Repository) ReplaceRecoveryCodes(ctx context.Context, settingID uuid.UUID, hashes []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("two_factor_setting_id = ?", settingID).Delete(&model.RecoveryCode{}).Error; err != nil {
			return err
		}
		for _, h := range hashes {
			if err := tx.Create(&model.RecoveryCode{TwoFactorSettingID: settingID, CodeHash: h}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) GetTwoFactorByUser(ctx context.Context, userID uuid.UUID) (*model.TwoFactorSetting, error) {
	var t model.TwoFactorSetting
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *Repository) MarkTwoFactorEnabled(ctx context.Context, userID uuid.UUID) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.TwoFactorSetting{}).Where("user_id = ?", userID).Update("enabled_at", now).Error; err != nil {
			return err
		}
		return tx.Model(&model.User{}).Where("id = ?", userID).Update("two_factor_enabled", true).Error
	})
}

func (r *Repository) ConsumeRecoveryCode(ctx context.Context, settingID uuid.UUID, hash string) (bool, error) {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).Model(&model.RecoveryCode{}).
		Where("two_factor_setting_id = ? AND code_hash = ? AND used_at IS NULL", settingID, hash).
		Update("used_at", now)
	return res.RowsAffected > 0, res.Error
}

func (r *Repository) DisableTwoFactor(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var setting model.TwoFactorSetting
		if err := tx.Where("user_id = ?", userID).First(&setting).Error; err != nil {
			return err
		}
		if err := tx.Where("two_factor_setting_id = ?", setting.ID).Delete(&model.RecoveryCode{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&setting).Error; err != nil {
			return err
		}
		return tx.Model(&model.User{}).Where("id = ?", userID).Update("two_factor_enabled", false).Error
	})
}

func (r *Repository) SoftDeleteUser(ctx context.Context, userID uuid.UUID, grace *time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.User{}).Where("id = ?", userID).Update("deletion_grace_at", grace).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", userID).Delete(&model.User{}).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		return tx.Model(&model.AuthSession{}).Where("user_id = ? AND revoked_at IS NULL", userID).Update("revoked_at", now).Error
	})
}

func (r *Repository) AppendAuditLog(ctx context.Context, log *model.AuditLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}
