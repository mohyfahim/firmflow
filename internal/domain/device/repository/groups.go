package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	deviceModel "firmflow/internal/domain/device/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func normalizeGroupName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (r *Repository) GetDeviceGroup(ctx context.Context, projectID, groupID uuid.UUID) (*deviceModel.DeviceGroup, error) {
	var g deviceModel.DeviceGroup
	err := r.db.WithContext(ctx).
		First(&g, "id = ? AND project_id = ? AND deleted_at IS NULL", groupID, projectID).Error
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (r *Repository) DeviceGroupNameTaken(ctx context.Context, projectID uuid.UUID, normalized string, excludeID *uuid.UUID) (bool, error) {
	q := r.db.WithContext(ctx).Model(&deviceModel.DeviceGroup{}).
		Where("project_id = ? AND name_normalized = ? AND deleted_at IS NULL", projectID, normalized)
	if excludeID != nil {
		q = q.Where("id <> ?", *excludeID)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *Repository) CreateDeviceGroup(ctx context.Context, projectID uuid.UUID, name, description string) (*deviceModel.DeviceGroup, error) {
	now := time.Now().UTC()
	norm := normalizeGroupName(name)
	if norm == "" {
		return nil, errors.New("invalid group name")
	}
	g := &deviceModel.DeviceGroup{
		ProjectID:      projectID,
		Name:           strings.TrimSpace(name),
		NameNormalized: norm,
		Description:    strings.TrimSpace(description),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := r.db.WithContext(ctx).Create(g).Error; err != nil {
		return nil, err
	}
	return r.GetDeviceGroup(ctx, projectID, g.ID)
}

func (r *Repository) UpdateDeviceGroup(ctx context.Context, projectID, groupID uuid.UUID, name *string, description *string) (*deviceModel.DeviceGroup, error) {
	var g deviceModel.DeviceGroup
	if err := r.db.WithContext(ctx).First(&g, "id = ? AND project_id = ? AND deleted_at IS NULL", groupID, projectID).Error; err != nil {
		return nil, err
	}
	updates := map[string]interface{}{"updated_at": time.Now().UTC()}
	if name != nil {
		updates["name"] = strings.TrimSpace(*name)
		updates["name_normalized"] = normalizeGroupName(*name)
	}
	if description != nil {
		updates["description"] = strings.TrimSpace(*description)
	}
	if len(updates) == 1 {
		return &g, nil
	}
	if err := r.db.WithContext(ctx).Model(&g).Updates(updates).Error; err != nil {
		return nil, err
	}
	return r.GetDeviceGroup(ctx, projectID, groupID)
}

// DeleteDeviceGroup removes memberships then soft-deletes the group.
func (r *Repository) DeleteDeviceGroup(ctx context.Context, projectID, groupID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var g deviceModel.DeviceGroup
		if err := tx.First(&g, "id = ? AND project_id = ? AND deleted_at IS NULL", groupID, projectID).Error; err != nil {
			return err
		}
		if err := tx.Where("device_group_id = ?", groupID).Delete(&deviceModel.DeviceGroupMembership{}).Error; err != nil {
			return err
		}
		return tx.Delete(&g).Error
	})
}

func (r *Repository) ListDeviceGroups(ctx context.Context, projectID uuid.UUID) ([]deviceModel.DeviceGroup, error) {
	var groups []deviceModel.DeviceGroup
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND deleted_at IS NULL", projectID).
		Order("name ASC").
		Find(&groups).Error
	return groups, err
}

// CountMembersByGroupIDs returns member counts for the given groups scoped to a project.
func (r *Repository) CountMembersByGroupIDs(ctx context.Context, projectID uuid.UUID, groupIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	out := make(map[uuid.UUID]int64)
	if len(groupIDs) == 0 {
		return out, nil
	}
	type row struct {
		GroupID uuid.UUID `gorm:"column:device_group_id"`
		Cnt     int64     `gorm:"column:cnt"`
	}
	var rows []row
	err := r.db.WithContext(ctx).Raw(`
		SELECT m.device_group_id, COUNT(*) AS cnt
		FROM device_group_memberships m
		INNER JOIN device_groups g ON g.id = m.device_group_id AND g.project_id = ? AND g.deleted_at IS NULL
		WHERE m.device_group_id IN ?
		GROUP BY m.device_group_id
	`, projectID, groupIDs).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, rw := range rows {
		out[rw.GroupID] = rw.Cnt
	}
	return out, nil
}

// AddDevicesToGroup inserts memberships idempotently (skips duplicates).
func (r *Repository) AddDevicesToGroup(ctx context.Context, groupID uuid.UUID, deviceIDs []uuid.UUID) (inserted int64, err error) {
	if len(deviceIDs) == 0 {
		return 0, nil
	}
	now := time.Now().UTC()
	for _, did := range deviceIDs {
		m := deviceModel.DeviceGroupMembership{
			DeviceGroupID: groupID,
			DeviceID:      did,
			CreatedAt:     now,
		}
		res := r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "device_group_id"}, {Name: "device_id"}},
			DoNothing: true,
		}).Create(&m)
		if res.Error != nil {
			return inserted, res.Error
		}
		inserted += res.RowsAffected
	}
	return inserted, nil
}

func (r *Repository) RemoveDevicesFromGroup(ctx context.Context, groupID uuid.UUID, deviceIDs []uuid.UUID) (int64, error) {
	if len(deviceIDs) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).
		Where("device_group_id = ? AND device_id IN ?", groupID, deviceIDs).
		Delete(&deviceModel.DeviceGroupMembership{})
	return res.RowsAffected, res.Error
}

func (r *Repository) CountGroupMembers(ctx context.Context, groupID uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&deviceModel.DeviceGroupMembership{}).
		Where("device_group_id = ?", groupID).
		Count(&n).Error
	return n, err
}
