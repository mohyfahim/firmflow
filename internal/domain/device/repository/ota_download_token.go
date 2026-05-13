package repository

import (
	"context"
	"errors"
	"time"

	deviceModel "firmflow/internal/domain/device/model"
	projectmodel "firmflow/internal/domain/project/model"

	"gorm.io/gorm"
)

var (
	ErrOtaTokenNotFound = errors.New("ota_download_token_not_found")
	ErrOtaTokenExpired  = errors.New("ota_download_token_expired")
	ErrOtaTokenConsumed = errors.New("ota_download_token_consumed")
	ErrOtaDeviceBlocked = errors.New("ota_download_device_blocked")
)

func (r *Repository) InsertOtaDownloadToken(ctx context.Context, row *deviceModel.OtaDownloadToken) error {
	return r.db.WithContext(ctx).Create(row).Error
}

// ConsumeOtaDownloadTokenTx validates a raw OTA download token, ensures the device may fetch
// firmware, marks the token consumed exactly once, and returns the persisted row metadata.
func (r *Repository) ConsumeOtaDownloadTokenTx(ctx context.Context, tokenHash string, now time.Time) (*deviceModel.OtaDownloadToken, error) {
	var out *deviceModel.OtaDownloadToken
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row deviceModel.OtaDownloadToken
		if err := tx.Clauses().Where("token_hash = ?", tokenHash).First(&row).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrOtaTokenNotFound
			}
			return err
		}
		if row.ConsumedAt != nil {
			return ErrOtaTokenConsumed
		}
		if !now.Before(row.ExpiresAt) {
			return ErrOtaTokenExpired
		}

		var n int64
		if err := tx.Model(&projectmodel.Device{}).
			Where("id = ? AND project_id = ? AND deleted_at IS NULL AND blocked = ?", row.DeviceID, row.ProjectID, false).
			Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return ErrOtaDeviceBlocked
		}

		cons := now
		res := tx.Model(&deviceModel.OtaDownloadToken{}).
			Where("id = ? AND consumed_at IS NULL", row.ID).
			Updates(map[string]interface{}{
				"consumed_at": cons,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrOtaTokenConsumed
		}
		row.ConsumedAt = &cons
		out = &row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
