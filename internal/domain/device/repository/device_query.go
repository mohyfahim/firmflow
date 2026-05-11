package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	projectmodel "firmflow/internal/domain/project/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const onlineThreshold = 5 * time.Minute

// MaxBulkDevicesSync caps synchronous bulk operations; larger sets can move to async jobs later.
const MaxBulkDevicesSync = 500

// DeviceListFilter is shared by list + bulk "apply_to_filter" resolution.
type DeviceListFilter struct {
	Online *bool // true = last_seen within threshold; false = offline

	DeviceTypeID *uuid.UUID
	GroupID      *uuid.UUID

	Blocked *bool

	FirmwareVersion *string // exact match

	LastSeenFrom *time.Time
	LastSeenTo   *time.Time

	Search *string // matches name or hardware identifier (case-insensitive)
}

func (r *Repository) applyDeviceListFilter(db *gorm.DB, projectID uuid.UUID, f DeviceListFilter, now time.Time) *gorm.DB {
	q := db.Model(&projectmodel.Device{}).
		Where("devices.project_id = ? AND devices.deleted_at IS NULL", projectID)

	if f.DeviceTypeID != nil {
		q = q.Where("devices.device_type_id = ?", *f.DeviceTypeID)
	}

	if f.Blocked != nil {
		q = q.Where("devices.blocked = ?", *f.Blocked)
	}

	if f.FirmwareVersion != nil && strings.TrimSpace(*f.FirmwareVersion) != "" {
		q = q.Where("devices.current_firmware_version = ?", strings.TrimSpace(*f.FirmwareVersion))
	}

	if f.LastSeenFrom != nil {
		q = q.Where("devices.last_seen_at IS NOT NULL AND devices.last_seen_at >= ?", *f.LastSeenFrom)
	}
	if f.LastSeenTo != nil {
		q = q.Where("devices.last_seen_at IS NOT NULL AND devices.last_seen_at <= ?", *f.LastSeenTo)
	}

	if f.Online != nil {
		th := now.Add(-onlineThreshold)
		if *f.Online {
			q = q.Where("devices.last_seen_at IS NOT NULL AND devices.last_seen_at >= ?", th)
		} else {
			q = q.Where("devices.last_seen_at IS NULL OR devices.last_seen_at < ?", th)
		}
	}

	if f.GroupID != nil {
		q = q.Where(`
			EXISTS (
				SELECT 1 FROM device_group_memberships dgm
				INNER JOIN device_groups dg ON dg.id = dgm.device_group_id AND dg.deleted_at IS NULL
				WHERE dgm.device_id = devices.id
				  AND dgm.device_group_id = ?
				  AND dg.project_id = ?
			)`, *f.GroupID, projectID)
	}

	if f.Search != nil {
		s := strings.TrimSpace(*f.Search)
		if s != "" {
			like := "%" + strings.ToLower(s) + "%"
			q = q.Where(`
				LOWER(devices.name) LIKE ? OR
				LOWER(devices.hardware_identifier) LIKE ? OR
				LOWER(devices.hardware_identifier_norm) LIKE ?
			`, like, like, like)
		}
	}

	return q
}

func parseDeviceSort(raw string) (column string, desc bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "devices.created_at", true
	}
	if strings.HasPrefix(raw, "-") {
		desc = true
		raw = strings.TrimPrefix(raw, "-")
	} else if strings.HasPrefix(raw, "+") {
		raw = strings.TrimPrefix(raw, "+")
	}
	switch strings.ToLower(raw) {
	case "name":
		return "devices.name", desc
	case "last_seen_at", "last_seen":
		return "devices.last_seen_at", desc
	case "current_firmware_version", "firmware":
		return "devices.current_firmware_version", desc
	case "created_at":
		return "devices.created_at", desc
	default:
		return "devices.created_at", true
	}
}

func (r *Repository) CountDevicesWithFilter(ctx context.Context, projectID uuid.UUID, f DeviceListFilter) (int64, error) {
	var n int64
	now := time.Now().UTC()
	q := r.applyDeviceListFilter(r.db.WithContext(ctx), projectID, f, now)
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *Repository) ListDevicesWithFilter(ctx context.Context, projectID uuid.UUID, f DeviceListFilter, page, pageSize int, sortRaw string) ([]projectmodel.Device, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize
	col, desc := parseDeviceSort(sortRaw)
	dir := "ASC"
	if desc {
		dir = "DESC"
	}
	order := fmt.Sprintf("%s %s", col, dir)

	now := time.Now().UTC()
	var devices []projectmodel.Device
	q := r.applyDeviceListFilter(r.db.WithContext(ctx), projectID, f, now).
		Order(order).
		Offset(offset).
		Limit(pageSize)
	if err := q.Find(&devices).Error; err != nil {
		return nil, err
	}
	return devices, nil
}

func (r *Repository) ListDeviceIDsWithFilter(ctx context.Context, projectID uuid.UUID, f DeviceListFilter, limit int) ([]uuid.UUID, error) {
	if limit < 1 {
		limit = MaxBulkDevicesSync
	}
	if limit > MaxBulkDevicesSync {
		limit = MaxBulkDevicesSync
	}
	now := time.Now().UTC()
	var ids []uuid.UUID
	err := r.applyDeviceListFilter(r.db.WithContext(ctx), projectID, f, now).
		Model(&projectmodel.Device{}).
		Select("devices.id").
		Order("devices.created_at ASC").
		Limit(limit).
		Pluck("devices.id", &ids).Error
	return ids, err
}

func (r *Repository) CountDeviceIDsWithFilter(ctx context.Context, projectID uuid.UUID, f DeviceListFilter) (int64, error) {
	now := time.Now().UTC()
	var n int64
	err := r.applyDeviceListFilter(r.db.WithContext(ctx), projectID, f, now).Count(&n).Error
	return n, err
}

// DevicesBelongToProject returns true if every device id exists in the project (non-deleted).
func (r *Repository) DevicesBelongToProject(ctx context.Context, projectID uuid.UUID, deviceIDs []uuid.UUID) (bool, error) {
	if len(deviceIDs) == 0 {
		return true, nil
	}
	var n int64
	err := r.db.WithContext(ctx).Model(&projectmodel.Device{}).
		Where("project_id = ? AND deleted_at IS NULL AND id IN ?", projectID, deviceIDs).
		Count(&n).Error
	if err != nil {
		return false, err
	}
	return n == int64(len(deviceIDs)), nil
}

// FilterDevicesInProject splits requested device IDs into those that exist in the project vs missing.
func (r *Repository) FilterDevicesInProject(ctx context.Context, projectID uuid.UUID, deviceIDs []uuid.UUID) (valid []uuid.UUID, missing []uuid.UUID, err error) {
	if len(deviceIDs) == 0 {
		return nil, nil, nil
	}
	var found []uuid.UUID
	err = r.db.WithContext(ctx).Model(&projectmodel.Device{}).
		Where("project_id = ? AND deleted_at IS NULL AND id IN ?", projectID, deviceIDs).
		Pluck("id", &found).Error
	if err != nil {
		return nil, nil, err
	}
	foundSet := make(map[uuid.UUID]struct{}, len(found))
	for _, id := range found {
		foundSet[id] = struct{}{}
	}
	for _, id := range deviceIDs {
		if _, ok := foundSet[id]; ok {
			valid = append(valid, id)
		} else {
			missing = append(missing, id)
		}
	}
	return valid, missing, nil
}
