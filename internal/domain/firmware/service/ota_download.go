package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"time"

	apperrors "firmflow/internal/common/errors"
	security "firmflow/internal/domain/auth/security"
	devicemodel "firmflow/internal/domain/device/model"
	devicerepo "firmflow/internal/domain/device/repository"
	firmwaremodel "firmflow/internal/domain/firmware/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// IssueOtaDownloadToken creates a short-lived secret used to download firmware bytes over HTTPS.
// The returned token string is shown once to the device; only a SHA-256 hash is stored.
func (s *Service) IssueOtaDownloadToken(ctx context.Context, projectID, deviceID, campaignID, firmwareID uuid.UUID, ttl time.Duration) (rawToken string, expiresAt time.Time, err error) {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	if _, err := s.firmwareRepo.GetFirmware(ctx, projectID, firmwareID); err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", time.Time{}, apperrors.NotFound("firmware not found")
		}
		return "", time.Time{}, err
	}

	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", time.Time{}, err
	}
	rawToken = hex.EncodeToString(secret[:])
	tokenHash := security.HashToken(rawToken)
	expiresAt = time.Now().UTC().Add(ttl)

	row := &devicemodel.OtaDownloadToken{
		TokenHash:  tokenHash,
		ProjectID:  projectID,
		DeviceID:   deviceID,
		CampaignID: campaignID,
		FirmwareID: firmwareID,
		ExpiresAt:  expiresAt,
	}
	if err := s.deviceRepo.InsertOtaDownloadToken(ctx, row); err != nil {
		return "", time.Time{}, err
	}
	return rawToken, expiresAt, nil
}

// OpenFirmwareWithOtaDownloadToken consumes a one-time OTA download token and opens the firmware object stream.
func (s *Service) OpenFirmwareWithOtaDownloadToken(ctx context.Context, rawToken string) (io.ReadCloser, *firmwaremodel.Firmware, error) {
	if len(rawToken) != 64 {
		return nil, nil, apperrors.New("invalid_ota_token", "invalid or expired download token", 401, nil)
	}
	for i := 0; i < len(rawToken); i++ {
		c := rawToken[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return nil, nil, apperrors.New("invalid_ota_token", "invalid or expired download token", 401, nil)
		}
	}
	hash := security.HashToken(rawToken)
	row, err := s.deviceRepo.ConsumeOtaDownloadTokenTx(ctx, hash, time.Now().UTC())
	if err != nil {
		if errors.Is(err, devicerepo.ErrOtaTokenNotFound) ||
			errors.Is(err, devicerepo.ErrOtaTokenExpired) ||
			errors.Is(err, devicerepo.ErrOtaTokenConsumed) ||
			errors.Is(err, devicerepo.ErrOtaDeviceBlocked) {
			return nil, nil, apperrors.New("invalid_ota_token", "invalid or expired download token", 401, nil)
		}
		return nil, nil, err
	}

	fw, err := s.firmwareRepo.GetFirmware(ctx, row.ProjectID, row.FirmwareID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil, apperrors.New("invalid_ota_token", "invalid or expired download token", 401, nil)
		}
		return nil, nil, err
	}
	if fw.DeletedAt.Valid {
		return nil, nil, apperrors.New("invalid_ota_token", "invalid or expired download token", 401, nil)
	}

	rc, err := s.objects.Open(ctx, fw.StorageKey)
	if err != nil {
		return nil, nil, apperrors.New("firmware_storage_unavailable", "firmware binary could not be read", 500, nil)
	}
	return rc, fw, nil
}
