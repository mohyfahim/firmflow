package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	firmwaremodel "firmflow/internal/domain/firmware/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) DB() *gorm.DB { return r.db }

// FirmwareVersionExists reports whether an active (non-deleted) firmware uses versionNormalized.
func (r *Repository) FirmwareVersionExists(ctx context.Context, projectID uuid.UUID, versionNormalized string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&firmwaremodel.Firmware{}).
		Where("project_id = ? AND version_normalized = ?", projectID, versionNormalized).
		Count(&n).Error
	return n > 0, err
}

func (r *Repository) CreateFirmwareWithTypes(ctx context.Context, fw *firmwaremodel.Firmware, deviceTypeIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(fw).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, dt := range deviceTypeIDs {
			row := firmwaremodel.FirmwareDeviceType{
				FirmwareID:   fw.ID,
				DeviceTypeID: dt,
				CreatedAt:    now,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) GetFirmware(ctx context.Context, projectID, firmwareID uuid.UUID) (*firmwaremodel.Firmware, error) {
	var fw firmwaremodel.Firmware
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND id = ?", projectID, firmwareID).
		First(&fw).Error
	if err != nil {
		return nil, err
	}
	return &fw, nil
}

func (r *Repository) ListDeviceTypeIDsForFirmware(ctx context.Context, firmwareID uuid.UUID) ([]uuid.UUID, error) {
	var rows []firmwaremodel.FirmwareDeviceType
	if err := r.db.WithContext(ctx).Where("firmware_id = ?", firmwareID).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.DeviceTypeID)
	}
	return out, nil
}

func (r *Repository) CountDeviceTypesByFirmwareIDs(ctx context.Context, firmwareIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	out := make(map[uuid.UUID]int64)
	if len(firmwareIDs) == 0 {
		return out, nil
	}
	type row struct {
		FirmwareID uuid.UUID
		Cnt        int64
	}
	var rows []row
	err := r.db.WithContext(ctx).Model(&firmwaremodel.FirmwareDeviceType{}).
		Select("firmware_id, count(*) as cnt").
		Where("firmware_id IN ?", firmwareIDs).
		Group("firmware_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, rrow := range rows {
		out[rrow.FirmwareID] = rrow.Cnt
	}
	return out, nil
}

func (r *Repository) ListFirmware(ctx context.Context, projectID uuid.UUID, page, pageSize int, sort string) ([]firmwaremodel.Firmware, int64, error) {
	var total int64
	q := r.db.WithContext(ctx).Model(&firmwaremodel.Firmware{}).Where("project_id = ?", projectID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	order := listFirmwareOrder(sort)
	var list []firmwaremodel.Firmware
	offset := (page - 1) * pageSize
	err := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order(order).
		Offset(offset).
		Limit(pageSize).
		Find(&list).Error
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func listFirmwareOrder(sort string) string {
	switch strings.TrimSpace(sort) {
	case "created_at":
		return "created_at ASC, id ASC"
	case "-created_at", "":
		return "created_at DESC, id DESC"
	case "version":
		return "semver_major ASC NULLS LAST, semver_minor ASC, semver_patch ASC, semver_prerelease ASC, version_normalized ASC, created_at ASC"
	case "-version":
		return "semver_major DESC NULLS LAST, semver_minor DESC, semver_patch DESC, semver_prerelease DESC, version_normalized DESC, created_at DESC"
	default:
		return "created_at DESC, id DESC"
	}
}

func (r *Repository) SoftDeleteFirmwareAndReturnKey(ctx context.Context, projectID, firmwareID uuid.UUID) (storageKey string, err error) {
	var fw firmwaremodel.Firmware
	if err := r.db.WithContext(ctx).Where("project_id = ? AND id = ?", projectID, firmwareID).First(&fw).Error; err != nil {
		return "", err
	}
	key := fw.StorageKey
	if err := r.db.WithContext(ctx).Delete(&fw).Error; err != nil {
		return "", err
	}
	return key, nil
}

// --- helpers for tests / integrity ---

func FirmwareStorageKey(projectID, firmwareID uuid.UUID) string {
	return fmt.Sprintf("projects/%s/firmware/%s/blob", projectID.String(), firmwareID.String())
}
