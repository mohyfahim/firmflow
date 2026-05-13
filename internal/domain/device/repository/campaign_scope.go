package repository

import (
	"context"

	deviceModel "firmflow/internal/domain/device/model"
	projectmodel "firmflow/internal/domain/project/model"

	"github.com/google/uuid"
)

// ListDeviceIDsInGroupsForProject returns distinct device IDs that are members of any listed group within the project.
func (r *Repository) ListDeviceIDsInGroupsForProject(ctx context.Context, projectID uuid.UUID, groupIDs []uuid.UUID) ([]uuid.UUID, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	var ids []uuid.UUID
	err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT m.device_id
		FROM device_group_memberships AS m
		INNER JOIN devices AS d ON d.id = m.device_id
		WHERE d.project_id = ? AND d.deleted_at IS NULL AND m.device_group_id IN ?
	`, projectID, groupIDs).Scan(&ids).Error
	return ids, err
}

// ListDeviceIDsExplicitInProject returns device IDs that exist in the project (non-deleted).
func (r *Repository) ListDeviceIDsExplicitInProject(ctx context.Context, projectID uuid.UUID, deviceIDs []uuid.UUID) ([]uuid.UUID, error) {
	if len(deviceIDs) == 0 {
		return nil, nil
	}
	var ids []uuid.UUID
	err := r.db.WithContext(ctx).Model(&projectmodel.Device{}).
		Where("project_id = ? AND deleted_at IS NULL AND id IN ?", projectID, deviceIDs).
		Pluck("id", &ids).Error
	return ids, err
}

// FilterDeviceIDsByCompatibleTypes keeps devices whose device_type_id is in allowedTypes.
func (r *Repository) FilterDeviceIDsByCompatibleTypes(ctx context.Context, projectID uuid.UUID, deviceIDs []uuid.UUID, allowedTypes map[uuid.UUID]struct{}) ([]uuid.UUID, error) {
	if len(deviceIDs) == 0 || len(allowedTypes) == 0 {
		return nil, nil
	}
	typeIDs := make([]uuid.UUID, 0, len(allowedTypes))
	for id := range allowedTypes {
		typeIDs = append(typeIDs, id)
	}
	var out []uuid.UUID
	err := r.db.WithContext(ctx).Model(&projectmodel.Device{}).
		Where("project_id = ? AND deleted_at IS NULL AND id IN ? AND device_type_id IN ?", projectID, deviceIDs, typeIDs).
		Pluck("id", &out).Error
	return out, err
}

// GetDeviceTypeID returns the device type for a project device.
func (r *Repository) GetDeviceTypeID(ctx context.Context, projectID, deviceID uuid.UUID) (uuid.UUID, error) {
	var d projectmodel.Device
	err := r.db.WithContext(ctx).
		Select("device_type_id").
		Where("project_id = ? AND id = ? AND deleted_at IS NULL", projectID, deviceID).
		First(&d).Error
	return d.DeviceTypeID, err
}

// DeviceGroupExistsInProject checks the group belongs to the project.
func (r *Repository) DeviceGroupExistsInProject(ctx context.Context, projectID, groupID uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&deviceModel.DeviceGroup{}).
		Where("project_id = ? AND id = ? AND deleted_at IS NULL", projectID, groupID).
		Count(&n).Error
	return n > 0, err
}
