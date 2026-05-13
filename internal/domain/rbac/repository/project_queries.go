package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	campaignmodel "firmflow/internal/domain/campaign/model"
	projectmodel "firmflow/internal/domain/project/model"
	rbacmodel "firmflow/internal/domain/rbac/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ListMembershipsWithProjects returns memberships for a user excluding soft-deleted projects.
func (r *Repository) ListMembershipsWithProjects(ctx context.Context, userID uuid.UUID, search string, sortField string, desc bool, offset, limit int) ([]rbacmodel.ProjectMembership, int64, error) {
	base := r.db.WithContext(ctx).Model(&rbacmodel.ProjectMembership{}).
		Joins("JOIN projects ON projects.id = project_memberships.project_id AND projects.deleted_at IS NULL").
		Where("project_memberships.user_id = ?", userID)

	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		base = base.Where("LOWER(projects.name) LIKE ?", like)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	order := orderClause(sortField, desc)
	var ms []rbacmodel.ProjectMembership
	err := base.Order(order).
		Preload("Role").
		Preload("Project").
		Offset(offset).Limit(limit).
		Find(&ms).Error
	return ms, total, err
}

func orderClause(sortField string, desc bool) string {
	dir := "ASC"
	if desc {
		dir = "DESC"
	}
	switch strings.ToLower(sortField) {
	case "name":
		return "projects.name " + dir
	case "updated_at":
		return "projects.updated_at " + dir
	default:
		return "projects.created_at " + dir
	}
}

func (r *Repository) UpdateProjectFields(ctx context.Context, projectID uuid.UUID, name *string, description *string) error {
	updates := map[string]interface{}{"updated_at": time.Now().UTC()}
	if name != nil {
		updates["name"] = strings.TrimSpace(*name)
	}
	if description != nil {
		updates["description"] = *description
	}
	if len(updates) == 1 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&rbacmodel.Project{}).Where("id = ?", projectID).Updates(updates).Error
}

func (r *Repository) SetProjectArchived(ctx context.Context, projectID uuid.UUID, archived bool) error {
	now := time.Now().UTC()
	var t *time.Time
	if archived {
		t = &now
	}
	return r.db.WithContext(ctx).Model(&rbacmodel.Project{}).Where("id = ?", projectID).
		Updates(map[string]interface{}{
			"archived_at": t,
			"updated_at":  now,
		}).Error
}

func (r *Repository) RevokeDevicesAndDisablePolling(ctx context.Context, tx *gorm.DB, projectID uuid.UUID) error {
	if tx == nil {
		tx = r.db.WithContext(ctx)
	}
	now := time.Now().UTC()
	return tx.Model(&projectmodel.Device{}).
		Where("project_id = ? AND deleted_at IS NULL", projectID).
		Updates(map[string]interface{}{
			"token_revoked_at":    now,
			"polling_disabled_at": now,
			"updated_at":          now,
		}).Error
}

func (r *Repository) SetDevicesPollingForArchive(ctx context.Context, projectID uuid.UUID, disable bool) error {
	now := time.Now().UTC()
	var pd *time.Time
	if disable {
		pd = &now
	}
	return r.db.WithContext(ctx).Model(&projectmodel.Device{}).
		Where("project_id = ? AND deleted_at IS NULL", projectID).
		Updates(map[string]interface{}{
			"polling_disabled_at": pd,
			"updated_at":          now,
		}).Error
}

// DeleteProjectSoft runs guarded soft-delete with device revocation and campaign checks.
func (r *Repository) DeleteProjectSoft(ctx context.Context, projectID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		n, err := r.countBlockingCampaignsTx(tx, projectID)
		if err != nil {
			return err
		}
		if n > 0 {
			return errors.New("blocking_campaigns")
		}
		if err := r.RevokeDevicesAndDisablePolling(ctx, tx, projectID); err != nil {
			return err
		}
		return tx.Where("id = ?", projectID).Delete(&rbacmodel.Project{}).Error
	})
}

func (r *Repository) countBlockingCampaignsTx(tx *gorm.DB, projectID uuid.UUID) (int64, error) {
	var n int64
	err := tx.Model(&campaignmodel.Campaign{}).
		Where("project_id = ? AND deleted_at IS NULL AND status NOT IN ?", projectID, []string{
			campaignmodel.StatusCompleted,
			campaignmodel.StatusCancelled,
		}).
		Count(&n).Error
	return n, err
}

// ProjectSummary holds aggregate dashboard metrics for a project.
type ProjectSummary struct {
	DevicesTotal      int64
	DevicesOnline     int64
	DevicesOffline    int64
	UpdatesSuccess24h int64
	UpdatesFailure24h int64
}

func (r *Repository) GetProjectSummary(ctx context.Context, projectID uuid.UUID) (*ProjectSummary, error) {
	s := &ProjectSummary{}
	db := r.db.WithContext(ctx)

	if err := db.Model(&projectmodel.Device{}).
		Where("project_id = ? AND deleted_at IS NULL", projectID).
		Count(&s.DevicesTotal).Error; err != nil {
		return nil, err
	}
	// Must stay consistent with device online/offline logic.
	// If you change this threshold, update `internal/domain/device/service` too.
	threshold := time.Now().UTC().Add(-5 * time.Minute)

	if err := db.Model(&projectmodel.Device{}).
		Where("project_id = ? AND deleted_at IS NULL AND last_seen_at IS NOT NULL AND last_seen_at >= ?", projectID, threshold).
		Count(&s.DevicesOnline).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&projectmodel.Device{}).
		Where("project_id = ? AND deleted_at IS NULL AND (last_seen_at IS NULL OR last_seen_at < ?)", projectID, threshold).
		Count(&s.DevicesOffline).Error; err != nil {
		return nil, err
	}

	since := time.Now().UTC().Add(-24 * time.Hour)
	if err := db.Model(&projectmodel.DeviceUpdateReport{}).
		Where("project_id = ? AND outcome = ? AND created_at >= ?", projectID, "success", since).
		Count(&s.UpdatesSuccess24h).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&projectmodel.DeviceUpdateReport{}).
		Where("project_id = ? AND outcome = ? AND created_at >= ?", projectID, "failure", since).
		Count(&s.UpdatesFailure24h).Error; err != nil {
		return nil, err
	}
	return s, nil
}
