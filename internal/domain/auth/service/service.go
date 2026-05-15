package service

import (
	"context"
	encodingjson "encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	apperrors "firmflow/internal/common/errors"
	"firmflow/internal/config"
	"firmflow/internal/domain/auth/model"
	"firmflow/internal/domain/auth/repository"
	"firmflow/internal/domain/auth/security"
	"firmflow/internal/platform/mailer"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

const minPasswordLength = 12

type Service struct {
	cfg         config.AuthConfig
	repo        *repository.Repository
	mailer      mailer.Mailer
	jwt         *security.TokenManager
	totpManager *security.TOTPManager
	log         *logrus.Logger
}

type SessionMeta struct {
	UserAgent string
	IP        string
}

type TokenPair struct {
	AccessToken       string    `json:"access_token"`
	AccessExpiresAt   time.Time `json:"access_expires_at"`
	RefreshToken      string    `json:"refresh_token"`
	RefreshExpiresAt  time.Time `json:"refresh_expires_at"`
	SessionID         uuid.UUID `json:"session_id"`
	TwoFactorRequired bool      `json:"two_factor_required"`
}

func New(cfg config.AuthConfig, repo *repository.Repository, mailer mailer.Mailer, log *logrus.Logger) *Service {
	return &Service{
		cfg:         cfg,
		repo:        repo,
		mailer:      mailer,
		jwt:         security.NewTokenManager(cfg.JWTIssuer, cfg.JWTSecret, cfg.AccessTokenTTL),
		totpManager: security.NewTOTPManager(cfg.JWTIssuer, cfg.TOTPEncryptionKey),
		log:         log,
	}
}

func (s *Service) Register(ctx context.Context, email, password string) error {
	email = repository.NormalizeEmail(email)
	if err := security.ValidatePasswordStrength(password, minPasswordLength); err != nil {
		return apperrors.BadRequest(err.Error(), nil)
	}
	hash, err := security.HashPassword(password, s.cfg.BcryptCost)
	if err != nil {
		return err
	}

	user := &model.User{Email: email, PasswordHash: hash}
	profile := &model.UserProfile{Timezone: "UTC", PreferredLanguage: "en"}
	if err := s.repo.CreateUserWithProfile(ctx, user, profile); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			return apperrors.New("email_in_use", "email already in use", 409, nil)
		}
		return err
	}

	rawToken, err := security.GenerateSecureToken(32)
	if err != nil {
		return err
	}
	token := &model.EmailVerificationToken{
		UserID:    user.ID,
		TokenHash: security.HashToken(rawToken),
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}
	s.log.WithFields(logrus.Fields{
		"token":   rawToken,
		"user_id": user.ID,
		"email":   email,
	}).Debug("email verification token created")

	if err := s.repo.CreateEmailVerificationToken(ctx, token); err != nil {
		return err
	}
	go func(emailAddr, tokenRaw string) {
		_ = s.mailer.SendEmailVerification(context.Background(), emailAddr, tokenRaw)
	}(email, rawToken)
	_ = s.audit(ctx, &user.ID, "auth.registered", "user", user.ID.String(), map[string]interface{}{"email": user.Email})
	return nil
}

func (s *Service) VerifyEmail(ctx context.Context, rawToken string) error {
	_, err := s.repo.ConsumeEmailVerificationToken(ctx, security.HashToken(rawToken))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.BadRequest("invalid or expired token", nil)
	}
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) Login(ctx context.Context, email, password, totpCode, recoveryCode string, meta SessionMeta) (*TokenPair, error) {
	email = repository.NormalizeEmail(email)
	locked, _, err := s.repo.IsEmailLocked(ctx, email)
	if err != nil {
		return nil, err
	}
	if locked {
		return nil, apperrors.New("account_locked", "too many failed attempts; try again later", 429, nil)
	}

	user, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil {
		_, _, _ = s.repo.CreateOrIncrementThrottle(ctx, email, s.cfg.LoginAttemptWindow, s.cfg.MaxLoginAttempts)
		return nil, apperrors.Unauthorized()
	}
	if user.EmailVerifiedAt == nil {
		return nil, apperrors.New("email_not_verified", "email is not verified", 403, nil)
	}
	if err := security.ComparePassword(user.PasswordHash, password); err != nil {
		_, _, _ = s.repo.CreateOrIncrementThrottle(ctx, email, s.cfg.LoginAttemptWindow, s.cfg.MaxLoginAttempts)
		return nil, apperrors.Unauthorized()
	}

	if user.TwoFactorEnabled {
		if totpCode == "" && recoveryCode == "" {
			return &TokenPair{TwoFactorRequired: true}, nil
		}
		setting, err := s.repo.GetTwoFactorByUser(ctx, user.ID)
		if err != nil {
			return nil, apperrors.Unauthorized()
		}
		secret, err := s.totpManager.Decrypt(setting.SecretEncrypted)
		if err != nil {
			return nil, err
		}
		ok := false
		if totpCode != "" {
			ok = s.totpManager.Verify(secret, totpCode)
		}
		if !ok && recoveryCode != "" {
			ok, err = s.repo.ConsumeRecoveryCode(ctx, setting.ID, security.HashToken(recoveryCode))
			if err != nil {
				return nil, err
			}
		}
		if !ok {
			return nil, apperrors.Unauthorized()
		}
	}

	_ = s.repo.ResetThrottle(ctx, email)
	return s.issueSessionTokens(ctx, user.ID, meta)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string, meta SessionMeta) (*TokenPair, error) {
	current, err := s.repo.GetSessionByRefreshHash(ctx, security.HashToken(refreshToken))
	if err != nil {
		return nil, apperrors.Unauthorized()
	}
	newRefresh, err := security.GenerateSecureToken(32)
	if err != nil {
		return nil, err
	}
	newSession := &model.AuthSession{
		UserID:           current.UserID,
		RefreshTokenHash: security.HashToken(newRefresh),
		ExpiresAt:        time.Now().UTC().Add(s.cfg.RefreshTokenTTL),
		UserAgent:        meta.UserAgent,
		IP:               meta.IP,
		LastSeenAt:       time.Now().UTC(),
	}
	if err := s.repo.RotateSession(ctx, current.ID, newSession); err != nil {
		return nil, err
	}
	access, accessExp, err := s.jwt.IssueAccessToken(current.UserID, newSession.ID)
	if err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:      access,
		AccessExpiresAt:  accessExp,
		RefreshToken:     newRefresh,
		RefreshExpiresAt: time.Now().UTC().Add(s.cfg.RefreshTokenTTL),
		SessionID:        newSession.ID,
	}, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	session, err := s.repo.GetSessionByRefreshHash(ctx, security.HashToken(refreshToken))
	if err != nil {
		return nil
	}
	return s.repo.RevokeSession(ctx, session.ID)
}

func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	email = repository.NormalizeEmail(email)
	user, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil {
		return nil
	}
	raw, err := security.GenerateSecureToken(32)
	if err != nil {
		return err
	}
	token := &model.PasswordResetToken{
		UserID:    user.ID,
		TokenHash: security.HashToken(raw),
		ExpiresAt: time.Now().UTC().Add(1 * time.Hour),
	}
	s.log.WithFields(logrus.Fields{
		"token":   raw,
		"user_id": user.ID,
		"email":   email,
	}).Debug("password reset token created")

	if err := s.repo.CreateResetToken(ctx, token); err != nil {
		return err
	}
	go func() { _ = s.mailer.SendPasswordReset(context.Background(), email, raw) }()
	return nil
}

func (s *Service) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	if err := security.ValidatePasswordStrength(newPassword, minPasswordLength); err != nil {
		return apperrors.BadRequest(err.Error(), nil)
	}
	hash, err := security.HashPassword(newPassword, s.cfg.BcryptCost)
	if err != nil {
		return err
	}
	uid, err := s.repo.ConsumeResetTokenAndSetPassword(ctx, security.HashToken(rawToken), hash)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.BadRequest("invalid or expired token", nil)
	}
	if err != nil {
		return err
	}
	_ = s.audit(ctx, &uid, "auth.password_reset", "user", uid.String(), nil)
	return nil
}

func (s *Service) ChangePassword(ctx context.Context, userID, currentSessionID uuid.UUID, currentPassword, newPassword string) error {
	if err := security.ValidatePasswordStrength(newPassword, minPasswordLength); err != nil {
		return apperrors.BadRequest(err.Error(), nil)
	}
	user, err := s.repo.FindUserByID(ctx, userID)
	if err != nil {
		return apperrors.Unauthorized()
	}
	if err := security.ComparePassword(user.PasswordHash, currentPassword); err != nil {
		return apperrors.Unauthorized()
	}
	hash, err := security.HashPassword(newPassword, s.cfg.BcryptCost)
	if err != nil {
		return err
	}
	if err := s.repo.UpdatePassword(ctx, userID, hash); err != nil {
		return err
	}
	if err := s.repo.RevokeAllOtherSessions(ctx, userID, currentSessionID); err != nil {
		return err
	}
	_ = s.audit(ctx, &userID, "auth.password_changed", "user", userID.String(), map[string]interface{}{"policy": "revoke_other_sessions"})
	return nil
}

func (s *Service) GetProfile(ctx context.Context, userID uuid.UUID) (*model.UserProfile, error) {
	return s.repo.GetProfile(ctx, userID)
}

func (s *Service) UpdateProfile(ctx context.Context, userID uuid.UUID, in model.UserProfile) (*model.UserProfile, error) {
	if in.Timezone != "" {
		if _, err := time.LoadLocation(in.Timezone); err != nil {
			return nil, apperrors.BadRequest("invalid timezone", nil)
		}
	}
	p, err := s.repo.GetProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	if in.FirstName != "" {
		p.FirstName = in.FirstName
	}
	if in.LastName != "" {
		p.LastName = in.LastName
	}
	if in.AvatarURL != "" {
		p.AvatarURL = in.AvatarURL
	}
	if in.CompanyName != "" {
		p.CompanyName = in.CompanyName
	}
	if in.PhoneNumber != "" {
		p.PhoneNumber = in.PhoneNumber
	}
	if in.Timezone != "" {
		p.Timezone = in.Timezone
	}
	if in.PreferredLanguage != "" {
		p.PreferredLanguage = in.PreferredLanguage
	}
	if err := s.repo.UpdateProfile(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Service) ListSessions(ctx context.Context, userID uuid.UUID, currentSessionID uuid.UUID) ([]map[string]interface{}, error) {
	sessions, err := s.repo.ListActiveSessions(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(sessions))
	for _, ss := range sessions {
		out = append(out, map[string]interface{}{
			"session_id":   ss.ID,
			"ip":           ss.IP,
			"user_agent":   ss.UserAgent,
			"created_at":   ss.CreatedAt,
			"last_seen_at": ss.LastSeenAt,
			"is_current":   ss.ID == currentSessionID,
		})
	}
	return out, nil
}

func (s *Service) RevokeSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	sessions, err := s.repo.ListActiveSessions(ctx, userID)
	if err != nil {
		return err
	}
	allowed := false
	for _, ss := range sessions {
		if ss.ID == sessionID {
			allowed = true
			break
		}
	}
	if !allowed {
		return apperrors.NotFound("session not found")
	}
	return s.repo.RevokeSession(ctx, sessionID)
}

func (s *Service) RevokeOtherSessions(ctx context.Context, userID, currentSessionID uuid.UUID) error {
	return s.repo.RevokeAllOtherSessions(ctx, userID, currentSessionID)
}

func (s *Service) BeginEnable2FA(ctx context.Context, userID uuid.UUID, password string) (map[string]interface{}, error) {
	user, err := s.repo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, apperrors.Unauthorized()
	}
	if err := security.ComparePassword(user.PasswordHash, password); err != nil {
		return nil, apperrors.Unauthorized()
	}
	secret, uri, qr, err := s.totpManager.Generate(user.Email)
	if err != nil {
		return nil, err
	}
	s.log.WithFields(logrus.Fields{
		"user_id": user.ID,
		"email":   user.Email,
		"secret":  secret,
		"uri":     uri,
		"qr":      qr,
	}).Debug("2fa generated")
	enc, err := s.totpManager.Encrypt(secret)
	if err != nil {
		return nil, err
	}
	setting := &model.TwoFactorSetting{UserID: userID, SecretEncrypted: enc}
	if err := s.repo.UpsertTwoFactor(ctx, setting); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"otpauth_uri": uri,
		"qr_data_url": qr,
	}, nil
}

func (s *Service) ConfirmEnable2FA(ctx context.Context, userID uuid.UUID, code string) ([]string, error) {
	setting, err := s.repo.GetTwoFactorByUser(ctx, userID)
	if err != nil {
		return nil, apperrors.BadRequest("2fa setup not initialized", nil)
	}
	secret, err := s.totpManager.Decrypt(setting.SecretEncrypted)
	if err != nil {
		return nil, err
	}
	if !s.totpManager.Verify(secret, code) {
		return nil, apperrors.BadRequest("invalid 2fa code", nil)
	}
	codes, err := security.NewRecoveryCodes(8)
	if err != nil {
		return nil, err
	}
	hashes := make([]string, 0, len(codes))
	for _, c := range codes {
		hashes = append(hashes, security.HashToken(c))
	}
	if err := s.repo.ReplaceRecoveryCodes(ctx, setting.ID, hashes); err != nil {
		return nil, err
	}
	if err := s.repo.MarkTwoFactorEnabled(ctx, userID); err != nil {
		return nil, err
	}
	return codes, nil
}

func (s *Service) Disable2FA(ctx context.Context, userID uuid.UUID, password, totpCode, recoveryCode string) error {
	user, err := s.repo.FindUserByID(ctx, userID)
	if err != nil {
		return apperrors.Unauthorized()
	}
	if err := security.ComparePassword(user.PasswordHash, password); err != nil {
		return apperrors.Unauthorized()
	}
	setting, err := s.repo.GetTwoFactorByUser(ctx, userID)
	if err != nil {
		return apperrors.BadRequest("2fa is not enabled", nil)
	}
	valid := false
	if totpCode != "" {
		secret, decErr := s.totpManager.Decrypt(setting.SecretEncrypted)
		if decErr == nil {
			valid = s.totpManager.Verify(secret, totpCode)
		}
	}
	if !valid && recoveryCode != "" {
		valid, err = s.repo.ConsumeRecoveryCode(ctx, setting.ID, security.HashToken(recoveryCode))
		if err != nil {
			return err
		}
	}
	if !valid {
		return apperrors.BadRequest("invalid 2fa confirmation", nil)
	}
	return s.repo.DisableTwoFactor(ctx, userID)
}

func (s *Service) DeleteAccount(ctx context.Context, userID uuid.UUID, password string, graceDays int) error {
	user, err := s.repo.FindUserByID(ctx, userID)
	if err != nil {
		return apperrors.Unauthorized()
	}
	if err := security.ComparePassword(user.PasswordHash, password); err != nil {
		return apperrors.Unauthorized()
	}
	var grace *time.Time
	if graceDays > 0 {
		t := time.Now().UTC().Add(time.Duration(graceDays) * 24 * time.Hour)
		grace = &t
	}
	if err := s.repo.SoftDeleteUser(ctx, userID, grace); err != nil {
		return err
	}
	_ = s.audit(ctx, &userID, "auth.account_deleted", "user", userID.String(), map[string]interface{}{"grace_days": graceDays})
	return nil
}

func (s *Service) ParseAccessToken(raw string) (*security.AccessClaims, error) {
	return s.jwt.ParseAccessToken(raw)
}

func (s *Service) issueSessionTokens(ctx context.Context, userID uuid.UUID, meta SessionMeta) (*TokenPair, error) {
	refresh, err := security.GenerateSecureToken(32)
	if err != nil {
		return nil, err
	}
	session := &model.AuthSession{
		UserID:           userID,
		RefreshTokenHash: security.HashToken(refresh),
		ExpiresAt:        time.Now().UTC().Add(s.cfg.RefreshTokenTTL),
		UserAgent:        meta.UserAgent,
		IP:               meta.IP,
		LastSeenAt:       time.Now().UTC(),
	}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, err
	}
	access, accessExp, err := s.jwt.IssueAccessToken(userID, session.ID)
	if err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:      access,
		AccessExpiresAt:  accessExp,
		RefreshToken:     refresh,
		RefreshExpiresAt: time.Now().UTC().Add(s.cfg.RefreshTokenTTL),
		SessionID:        session.ID,
	}, nil
}

func (s *Service) audit(ctx context.Context, actor *uuid.UUID, event, targetType, targetID string, metadata map[string]interface{}) error {
	b, _ := encodingjson.Marshal(metadata)
	return s.repo.AppendAuditLog(ctx, &model.AuditLog{
		ActorUserID: actor,
		Event:       event,
		TargetType:  targetType,
		TargetID:    targetID,
		Metadata:    b,
	})
}

func ParseUUID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid uuid")
	}
	return id, nil
}
