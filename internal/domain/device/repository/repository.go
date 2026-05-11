package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	projectmodel "firmflow/internal/domain/project/model"
	deviceModel "firmflow/internal/domain/device/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) DB() *gorm.DB { return r.db }

// Device types
func (r *Repository) ListPredefinedDeviceTypes(ctx context.Context) ([]deviceModel.DeviceType, error) {
	var types []deviceModel.DeviceType
	err := r.db.WithContext(ctx).
		Where("is_predefined = ? AND project_id IS NULL", true).
		Order("name ASC").
		Find(&types).Error
	return types, err
}

func (r *Repository) ListCustomDeviceTypes(ctx context.Context, projectID uuid.UUID) ([]deviceModel.DeviceType, error) {
	var types []deviceModel.DeviceType
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND is_predefined = ?", projectID, false).
		Order("name ASC").
		Find(&types).Error
	return types, err
}

func (r *Repository) GetDeviceType(ctx context.Context, deviceTypeID uuid.UUID) (*deviceModel.DeviceType, error) {
	var dt deviceModel.DeviceType
	err := r.db.WithContext(ctx).
		First(&dt, "id = ?", deviceTypeID).Error
	if err != nil {
		return nil, err
	}
	return &dt, nil
}

func (r *Repository) CustomDeviceTypeNameTaken(ctx context.Context, projectID uuid.UUID, normalizedName string, excludeID *uuid.UUID) (bool, error) {
	q := r.db.WithContext(ctx).Model(&deviceModel.DeviceType{}).
		Where("project_id = ? AND is_predefined = ? AND name_normalized = ?", projectID, false, normalizedName)
	if excludeID != nil {
		q = q.Where("id <> ?", *excludeID)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *Repository) CreateCustomDeviceType(ctx context.Context, projectID uuid.UUID, name string, processorArchitecture string, boardVersion string, flashSizeBytes int64, memoryNotes string) (*deviceModel.DeviceType, error) {
	norm := normalizeDeviceTypeName(name)
	t := &deviceModel.DeviceType{
		ProjectID:              &projectID,
		IsPredefined:           false,
		Name:                   name,
		NameNormalized:        norm,
		ProcessorArchitecture: processorArchitecture,
		HardwareBoardVersion:  boardVersion,
		FlashSizeBytes:        flashSizeBytes,
		MemoryNotes:           memoryNotes,
		CreatedAt:              time.Now().UTC(),
		UpdatedAt:              time.Now().UTC(),
	}
	if err := r.db.WithContext(ctx).Create(t).Error; err != nil {
		return nil, err
	}
	return r.GetDeviceType(ctx, t.ID)
}

func (r *Repository) UpdateCustomDeviceType(ctx context.Context, projectID uuid.UUID, deviceTypeID uuid.UUID, name *string, processorArchitecture *string, boardVersion *string, flashSizeBytes *int64, memoryNotes *string) (*deviceModel.DeviceType, error) {
	var dt deviceModel.DeviceType
	if err := r.db.WithContext(ctx).First(&dt, "id = ? AND project_id = ? AND is_predefined = ?", deviceTypeID, projectID, false).Error; err != nil {
		return nil, err
	}
	updates := map[string]interface{}{
		"updated_at": time.Now().UTC(),
	}
	if name != nil {
		norm := normalizeDeviceTypeName(*name)
		updates["name"] = *name
		updates["name_normalized"] = norm
	}
	if processorArchitecture != nil {
		updates["processor_architecture"] = *processorArchitecture
	}
	if boardVersion != nil {
		updates["hardware_board_version"] = *boardVersion
	}
	if flashSizeBytes != nil {
		updates["flash_size_bytes"] = *flashSizeBytes
	}
	if memoryNotes != nil {
		updates["memory_notes"] = *memoryNotes
	}
	if err := r.db.WithContext(ctx).Model(&dt).Updates(updates).Error; err != nil {
		return nil, err
	}
	return r.GetDeviceType(ctx, deviceTypeID)
}

func (r *Repository) DeleteCustomDeviceType(ctx context.Context, projectID uuid.UUID, deviceTypeID uuid.UUID) error {
	var dt deviceModel.DeviceType
	if err := r.db.WithContext(ctx).First(&dt, "id = ? AND project_id = ? AND is_predefined = ?", deviceTypeID, projectID, false).Error; err != nil {
		return err
	}
	// Only prevent deletion if devices exist with that type.
	var n int64
	if err := r.db.WithContext(ctx).Model(&projectmodel.Device{}).
		Where("device_type_id = ? AND deleted_at IS NULL", deviceTypeID).
		Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return errors.New("device_type_in_use")
	}
	return r.db.WithContext(ctx).Delete(&dt).Error
}

// Devices
func (r *Repository) GetDevice(ctx context.Context, projectID uuid.UUID, deviceID uuid.UUID) (*projectmodel.Device, error) {
	var d projectmodel.Device
	if err := r.db.WithContext(ctx).
		First(&d, "id = ? AND project_id = ? AND deleted_at IS NULL", deviceID, projectID).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *Repository) GetDeviceByActiveTokenHash(ctx context.Context, tokenHash string) (*projectmodel.Device, *deviceModel.DeviceAuthToken, error) {
	var token deviceModel.DeviceAuthToken
	err := r.db.WithContext(ctx).
		Where("token_hash = ? AND revoked_at IS NULL", tokenHash).
		First(&token).Error
	if err != nil {
		return nil, nil, err
	}

	var d projectmodel.Device
	// Fetch device by id; auth middleware gates are applied below.
	err = r.db.WithContext(ctx).First(&d, "id = ? AND deleted_at IS NULL", token.DeviceID).Error
	if err != nil {
		return nil, nil, err
	}

	// Hard gates enforced at auth middleware time:
	if d.Blocked {
		return &d, &token, errors.New("device_blocked")
	}
	if d.TokenRevokedAt != nil {
		return &d, &token, errors.New("device_token_revoked")
	}
	if d.PollingDisabledAt != nil {
		return &d, &token, errors.New("device_polling_disabled")
	}
	return &d, &token, nil
}

func (r *Repository) GetDeviceByHardwareIdentifier(ctx context.Context, projectID uuid.UUID, hardwareIdentifierNorm string) (*projectmodel.Device, error) {
	var d projectmodel.Device
	err := r.db.WithContext(ctx).
		First(&d, "project_id = ? AND hardware_identifier_norm = ? AND deleted_at IS NULL", projectID, hardwareIdentifierNorm).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func normalizeDeviceTypeName(name string) string {
	// Keep device types normalized similarly to roles: lowercase + trim.
	return strings.ToLower(strings.TrimSpace(name))
}

func normalizeHardwareIdentifier(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.ToLower(raw)
	return raw
}

func (r *Repository) CreateDeviceAndAuthTokenTx(ctx context.Context, projectID uuid.UUID, deviceTypeID uuid.UUID, name string, hardwareIdentifier string, hardwareIdentifierNorm string, rawTokenHash string, rawTokenIssuedAt time.Time, actorUserID *uuid.UUID) (*projectmodel.Device, *deviceModel.DeviceAuthToken, error) {
	// Note: rawTokenHash is already hashed; naming kept to make call sites explicit.
	now := time.Now().UTC()
	tokenID := uuid.New()
	var createdToken *deviceModel.DeviceAuthToken
	var createdDevice *projectmodel.Device

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		d := &projectmodel.Device{
			ProjectID:              projectID,
			Name:                   name,
			DeviceTypeID:           deviceTypeID,
			HardwareIdentifier:    hardwareIdentifier,
			HardwareIdentifierNorm: hardwareIdentifierNorm,
			Blocked:                false,
			CurrentFirmwareVersion: "",
			LastSeenAt:             nil,
			ConnectionStatus:       "offline",
			ActiveAuthTokenID:      &tokenID,
			ActiveAuthTokenIssuedAt: &rawTokenIssuedAt,
		}
		if err := tx.Create(d).Error; err != nil {
			return err
		}

		t := &deviceModel.DeviceAuthToken{
			ID:              tokenID,
			DeviceID:       d.ID,
			TokenHash:      rawTokenHash,
			CreatedAt:      rawTokenIssuedAt,
			RevokedAt:      nil,
			RevokedByUserID: actorUserID,
		}
		createdToken = t
		if err := tx.Create(t).Error; err != nil {
			return err
		}

		if err := tx.Model(&projectmodel.Device{}).
			Where("id = ? AND project_id = ? AND deleted_at IS NULL", d.ID, projectID).
			Updates(map[string]interface{}{
				"active_auth_token_id":        tokenID,
				"active_auth_token_issued_at": &rawTokenIssuedAt,
				"updated_at":                  now,
			}).Error; err != nil {
			return err
		}

		createdDevice = d
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	loadedDevice, err := r.GetDevice(ctx, projectID, createdDevice.ID)
	if err != nil {
		return nil, nil, err
	}
	return loadedDevice, createdToken, nil
}

func (r *Repository) SetDeviceBlockedTx(ctx context.Context, projectID uuid.UUID, deviceID uuid.UUID, blocked bool, actorUserID uuid.UUID) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Update device current state.
		var d projectmodel.Device
		if err := tx.First(&d, "id = ? AND project_id = ? AND deleted_at IS NULL", deviceID, projectID).Error; err != nil {
			return err
		}
		d.Blocked = blocked
		if blocked {
			d.BlockedAt = &now
		} else {
			d.BlockedAt = nil
		}
		if err := tx.Model(&d).Updates(map[string]interface{}{
			"blocked": d.Blocked,
			"blocked_at": d.BlockedAt,
			"updated_at": now,
		}).Error; err != nil {
			return err
		}
		evt := &deviceModel.DeviceBlockEvent{
			ProjectID: projectID,
			DeviceID: deviceID,
			Action: func() string { if blocked { return "blocked" }; return "unblocked" }(),
			ActorUserID: &actorUserID,
			CreatedAt: now,
		}
		return tx.Create(evt).Error
	})
}

// Token rotation + token revocation
func (r *Repository) RotateDeviceAuthTokenTx(ctx context.Context, projectID uuid.UUID, deviceID uuid.UUID, actorUserID uuid.UUID, newTokenHash string, newTokenID uuid.UUID, newIssuedAt time.Time) (*projectmodel.Device, *deviceModel.DeviceAuthToken, error) {
	now := time.Now().UTC()
	var createdToken *deviceModel.DeviceAuthToken
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Ensure device exists and belongs to project.
		var d projectmodel.Device
		if err := tx.First(&d, "id = ? AND project_id = ? AND deleted_at IS NULL", deviceID, projectID).Error; err != nil {
			return err
		}

		// Revoke all currently active tokens.
		if err := tx.Model(&deviceModel.DeviceAuthToken{}).
			Where("device_id = ? AND revoked_at IS NULL", deviceID).
			Updates(map[string]interface{}{
				"revoked_at":          now,
				"replaced_by_token_id": newTokenID,
				"revoked_by_user_id": actorUserID,
			}).Error; err != nil {
			return err
		}

		createdToken = &deviceModel.DeviceAuthToken{
			ID:                newTokenID,
			DeviceID:         deviceID,
			TokenHash:        newTokenHash,
			CreatedAt:        newIssuedAt,
			RevokedAt:        nil,
			ReplacedByTokenID: nil,
			RevokedByUserID:  &actorUserID,
		}
		if err := tx.Create(createdToken).Error; err != nil {
			return err
		}

		if err := tx.Model(&projectmodel.Device{}).
			Where("id = ? AND project_id = ? AND deleted_at IS NULL", deviceID, projectID).
			Updates(map[string]interface{}{
				"active_auth_token_id":       newTokenID,
				"active_auth_token_issued_at": &newIssuedAt,
				"updated_at":                now,
			}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	// Reload device for return.
	d, err := r.GetDevice(ctx, projectID, deviceID)
	if err != nil {
		return nil, nil, err
	}
	return d, createdToken, nil
}

// Connection logs
func (r *Repository) CreateDeviceConnectionLog(ctx context.Context, log *deviceModel.DeviceConnectionLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

func (r *Repository) UpdateDeviceSeenAndFirmware(ctx context.Context, projectID uuid.UUID, deviceID uuid.UUID, firmwareVersion *string, lastSeenAt time.Time, connectionStatus string) error {
	now := time.Now().UTC()
	updates := map[string]interface{}{
		"last_seen_at":       &lastSeenAt,
		"connection_status":  connectionStatus,
		"updated_at":          now,
	}
	if firmwareVersion != nil {
		updates["current_firmware_version"] = *firmwareVersion
	}
	return r.db.WithContext(ctx).Model(&projectmodel.Device{}).
		Where("id = ? AND project_id = ? AND deleted_at IS NULL", deviceID, projectID).
		Updates(updates).Error
}

func (r *Repository) ListRecentDeviceConnectionLogs(ctx context.Context, projectID uuid.UUID, deviceID uuid.UUID, limit int) ([]deviceModel.DeviceConnectionLog, error) {
	var logs []deviceModel.DeviceConnectionLog
	if limit <= 0 {
		limit = 10
	}
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND device_id = ?", projectID, deviceID).
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error
	return logs, err
}

// Repository methods intentionally do not enforce authorization.

