package service

import (
	"context"

	apperrors "firmflow/internal/common/errors"
	rbacmodel "firmflow/internal/domain/rbac/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *ProjectService) ensureProjectNotArchived(ctx context.Context, projectID uuid.UUID) error {
	p, err := s.rbac.GetProject(ctx, projectID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFound("project not found")
		}
		return err
	}
	if p.ArchivedAt != nil {
		return apperrors.New("project_archived", "project is archived; unarchive to modify", 409, nil)
	}
	return nil
}

func (s *ProjectService) mustBeProjectOwner(ctx context.Context, projectID, userID uuid.UUID) error {
	m, err := s.rbac.GetMembershipForUser(ctx, projectID, userID)
	if err != nil {
		return err
	}
	return MustBeOwner(m)
}

// ProjectListItem is one row for GET /projects.
type ProjectListItem struct {
	Project rbacmodel.Project `json:"project"`
	Role    RoleBrief         `json:"role"`
}

type RoleBrief struct {
	ID   uuid.UUID `json:"id"`
	Slug string    `json:"slug"`
	Name string    `json:"name"`
}

// ProjectDetail bundles project metadata with the current user's membership view.
type ProjectDetail struct {
	Project    rbacmodel.Project `json:"project"`
	Membership RoleBrief         `json:"membership"`
}

// ProjectSummaryDTO is returned by the dashboard endpoint.
type ProjectSummaryDTO struct {
	DevicesTotal      int64 `json:"devices_total"`
	DevicesOnline     int64 `json:"devices_online"`
	DevicesOffline    int64 `json:"devices_offline"`
	UpdatesSuccess24h int64 `json:"updates_success_24h"`
	UpdatesFailure24h int64 `json:"updates_failure_24h"`
}
