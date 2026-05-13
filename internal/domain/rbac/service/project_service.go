package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	apperrors "firmflow/internal/common/errors"
	authmodel "firmflow/internal/domain/auth/model"
	authrepo "firmflow/internal/domain/auth/repository"
	rbacmodel "firmflow/internal/domain/rbac/model"
	rbacperm "firmflow/internal/domain/rbac/permission"
	rbacrepo "firmflow/internal/domain/rbac/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProjectService orchestrates projects, memberships, custom roles, and ownership transfer.
type ProjectService struct {
	rbac  *rbacrepo.Repository
	auth  *authrepo.Repository
	authz *Authorizer
}

func NewProjectService(rbacRepo *rbacrepo.Repository, authRepo *authrepo.Repository, authz *Authorizer) *ProjectService {
	return &ProjectService{rbac: rbacRepo, auth: authRepo, authz: authz}
}

func (s *ProjectService) audit(ctx context.Context, actor *uuid.UUID, event, targetType, targetID string, metadata map[string]interface{}) error {
	b, _ := json.Marshal(metadata)
	return s.auth.AppendAuditLog(ctx, &authmodel.AuditLog{
		ActorUserID: actor,
		Event:       event,
		TargetType:  targetType,
		TargetID:    targetID,
		Metadata:    b,
	})
}

func (s *ProjectService) CreateProject(ctx context.Context, actorUserID uuid.UUID, name, description string) (*rbacmodel.Project, error) {
	name = trim(name)
	if name == "" {
		return nil, apperrors.BadRequest("project name is required", nil)
	}
	ownerRole, err := s.rbac.GetPredefinedRoleBySlug(ctx, rbacmodel.SlugOwner)
	if err != nil {
		return nil, err
	}
	p := &rbacmodel.Project{Name: name, Description: trim(description)}
	err = s.rbac.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(p).Error; err != nil {
			return err
		}
		m := &rbacmodel.ProjectMembership{
			ProjectID: p.ID,
			UserID:    actorUserID,
			RoleID:    ownerRole.ID,
		}
		return tx.Create(m).Error
	})
	if err != nil {
		return nil, err
	}
	_ = s.audit(ctx, &actorUserID, "project.created", "project", p.ID.String(), map[string]interface{}{"name": name, "description": p.Description})
	return p, nil
}

func (s *ProjectService) GetProjectDetail(ctx context.Context, actorUserID, projectID uuid.UUID) (*ProjectDetail, error) {
	m, err := s.authz.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.ProjectRead)
	if err != nil {
		return nil, err
	}
	p, err := s.rbac.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return &ProjectDetail{
		Project: *p,
		Membership: RoleBrief{
			ID:   m.Role.ID,
			Slug: m.Role.Slug,
			Name: m.Role.Name,
		},
	}, nil
}

func (s *ProjectService) ListMyProjects(ctx context.Context, actorUserID uuid.UUID, search string, sortRaw string, page, pageSize int) ([]ProjectListItem, int64, error) {
	field, desc := parseProjectSort(sortRaw)
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
	ms, total, err := s.rbac.ListMembershipsWithProjects(ctx, actorUserID, search, field, desc, offset, pageSize)
	if err != nil {
		return nil, 0, err
	}
	out := make([]ProjectListItem, 0, len(ms))
	for _, m := range ms {
		out = append(out, ProjectListItem{
			Project: m.Project,
			Role: RoleBrief{
				ID:   m.Role.ID,
				Slug: m.Role.Slug,
				Name: m.Role.Name,
			},
		})
	}
	return out, total, nil
}

func parseProjectSort(raw string) (field string, desc bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "created_at", true
	}
	if strings.HasPrefix(raw, "-") {
		desc = true
		raw = strings.TrimPrefix(raw, "-")
	} else if strings.HasPrefix(raw, "+") {
		raw = strings.TrimPrefix(raw, "+")
	}
	switch strings.ToLower(raw) {
	case "name", "updated_at", "created_at":
		return strings.ToLower(raw), desc
	default:
		return "created_at", true
	}
}

func (s *ProjectService) UpdateProject(ctx context.Context, actorUserID, projectID uuid.UUID, name *string, description *string) error {
	if _, err := s.authz.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.ProjectUpdate); err != nil {
		return err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return err
	}
	if err := s.rbac.UpdateProjectFields(ctx, projectID, name, description); err != nil {
		return err
	}
	_ = s.audit(ctx, &actorUserID, "project.updated", "project", projectID.String(), map[string]interface{}{
		"name": name, "description": description,
	})
	return nil
}

func (s *ProjectService) ArchiveProject(ctx context.Context, actorUserID, projectID uuid.UUID, archive bool) error {
	if _, err := s.authz.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.ProjectUpdate); err != nil {
		return err
	}
	p, err := s.rbac.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	if archive && p.ArchivedAt != nil {
		return nil
	}
	if !archive && p.ArchivedAt == nil {
		return nil
	}
	if err := s.rbac.SetProjectArchived(ctx, projectID, archive); err != nil {
		return err
	}
	if err := s.rbac.SetDevicesPollingForArchive(ctx, projectID, archive); err != nil {
		return err
	}
	ev := "project.unarchived"
	if archive {
		ev = "project.archived"
	}
	_ = s.audit(ctx, &actorUserID, ev, "project", projectID.String(), map[string]interface{}{"archived": archive})
	return nil
}

func (s *ProjectService) DeleteProject(ctx context.Context, actorUserID, projectID uuid.UUID) error {
	if _, err := s.authz.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.ProjectDelete); err != nil {
		return err
	}
	if err := s.mustBeProjectOwner(ctx, projectID, actorUserID); err != nil {
		return err
	}
	if err := s.rbac.DeleteProjectSoft(ctx, projectID); err != nil {
		return mapDeleteProjectError(err)
	}
	_ = s.audit(ctx, &actorUserID, "project.deleted", "project", projectID.String(), map[string]interface{}{
		"strategy": "soft_delete_with_device_revocation",
	})
	return nil
}

func mapDeleteProjectError(err error) error {
	if err == nil {
		return nil
	}
	if err.Error() == "blocking_campaigns" {
		return apperrors.New("campaigns_active", "cannot delete project while OTA campaigns are in a non-terminal state (not completed or cancelled)", 409, nil)
	}
	return err
}

func (s *ProjectService) GetProjectSummary(ctx context.Context, actorUserID, projectID uuid.UUID) (*ProjectSummaryDTO, error) {
	if _, err := s.authz.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.DashboardRead); err != nil {
		return nil, err
	}
	sum, err := s.rbac.GetProjectSummary(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return &ProjectSummaryDTO{
		DevicesTotal:      sum.DevicesTotal,
		DevicesOnline:     sum.DevicesOnline,
		DevicesOffline:    sum.DevicesOffline,
		UpdatesSuccess24h: sum.UpdatesSuccess24h,
		UpdatesFailure24h: sum.UpdatesFailure24h,
	}, nil
}

func (s *ProjectService) ListProjectAuditLogs(ctx context.Context, actorUserID, projectID uuid.UUID, actorFilter *uuid.UUID, eventPrefix string, from, to *time.Time, page, pageSize int) ([]authmodel.AuditLog, int64, error) {
	if _, err := s.authz.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.AuditRead); err != nil {
		return nil, 0, err
	}
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
	return s.auth.ListAuditLogs(ctx, authrepo.AuditLogFilters{
		TargetType:  "project",
		TargetID:    projectID.String(),
		ActorID:     actorFilter,
		EventPrefix: eventPrefix,
		From:        from,
		To:          to,
		Offset:      offset,
		Limit:       pageSize,
	})
}

func (s *ProjectService) InviteMember(ctx context.Context, actorUserID, projectID uuid.UUID, email string, roleID uuid.UUID) error {
	if _, err := s.authz.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.MemberInvite); err != nil {
		return err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return err
	}
	role, err := s.rbac.GetRole(ctx, roleID)
	if err != nil {
		return apperrors.BadRequest("invalid role", nil)
	}
	if err := ValidateAssignableRole(projectID, role); err != nil {
		return err
	}
	if role.IsPredefined && role.Slug == rbacmodel.SlugOwner {
		actorMem, err := s.rbac.GetMembershipForUser(ctx, projectID, actorUserID)
		if err != nil {
			return err
		}
		if err := MustBeOwner(actorMem); err != nil {
			return apperrors.Forbidden()
		}
	}
	user, err := s.auth.FindUserByEmail(ctx, email)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFound("user not found")
		}
		return err
	}
	var existing rbacmodel.ProjectMembership
	err = s.rbac.DB().WithContext(ctx).
		Where("project_id = ? AND user_id = ?", projectID, user.ID).
		First(&existing).Error
	if err == nil {
		return apperrors.New("already_member", "user is already a member", 409, nil)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	m := &rbacmodel.ProjectMembership{
		ProjectID: projectID,
		UserID:    user.ID,
		RoleID:    roleID,
	}
	if err := s.rbac.CreateMembership(ctx, m); err != nil {
		return err
	}
	_ = s.audit(ctx, &actorUserID, "member.invited", "project", projectID.String(), map[string]interface{}{
		"target_user_id": user.ID.String(),
		"role_id":        roleID.String(),
	})
	return nil
}

func (s *ProjectService) UpdateMemberRole(ctx context.Context, actorUserID, projectID, targetUserID, newRoleID uuid.UUID) error {
	if _, err := s.authz.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.MemberUpdateRole); err != nil {
		return err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return err
	}
	newRole, err := s.rbac.GetRole(ctx, newRoleID)
	if err != nil {
		return apperrors.BadRequest("invalid role", nil)
	}
	if err := ValidateAssignableRole(projectID, newRole); err != nil {
		return err
	}
	targetMem, err := s.rbac.GetMembershipForUser(ctx, projectID, targetUserID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFound("member not found")
		}
		return err
	}
	oldRole, err := s.rbac.GetRole(ctx, targetMem.RoleID)
	if err != nil {
		return err
	}

	if newRole.IsPredefined && newRole.Slug == rbacmodel.SlugOwner {
		actorMem, err := s.rbac.GetMembershipForUser(ctx, projectID, actorUserID)
		if err != nil {
			return err
		}
		if err := MustBeOwner(actorMem); err != nil {
			return apperrors.Forbidden()
		}
	}

	if oldRole.IsPredefined && oldRole.Slug == rbacmodel.SlugOwner && !(newRole.IsPredefined && newRole.Slug == rbacmodel.SlugOwner) {
		ownerRole, err := s.rbac.GetPredefinedRoleBySlug(ctx, rbacmodel.SlugOwner)
		if err != nil {
			return err
		}
		n, err := s.rbac.CountOwners(ctx, projectID, ownerRole.ID)
		if err != nil {
			return err
		}
		if n <= 1 {
			return apperrors.New("last_owner", "cannot remove the last owner role", 409, nil)
		}
	}

	if err := s.rbac.UpdateMembershipRole(ctx, projectID, targetUserID, newRoleID); err != nil {
		return err
	}
	_ = s.audit(ctx, &actorUserID, "member.role_updated", "project", projectID.String(), map[string]interface{}{
		"target_user_id": targetUserID.String(),
		"new_role_id":    newRoleID.String(),
	})
	return nil
}

func (s *ProjectService) RemoveMember(ctx context.Context, actorUserID, projectID, targetUserID uuid.UUID) error {
	if _, err := s.authz.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.MemberRemove); err != nil {
		return err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return err
	}
	targetMem, err := s.rbac.GetMembershipForUser(ctx, projectID, targetUserID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFound("member not found")
		}
		return err
	}
	role, err := s.rbac.GetRole(ctx, targetMem.RoleID)
	if err != nil {
		return err
	}
	if role.IsPredefined && role.Slug == rbacmodel.SlugOwner {
		ownerRole, err := s.rbac.GetPredefinedRoleBySlug(ctx, rbacmodel.SlugOwner)
		if err != nil {
			return err
		}
		n, err := s.rbac.CountOwners(ctx, projectID, ownerRole.ID)
		if err != nil {
			return err
		}
		if n <= 1 {
			return apperrors.New("last_owner", "cannot remove the last owner", 409, nil)
		}
	}
	if err := s.rbac.DeleteMembership(ctx, projectID, targetUserID); err != nil {
		return err
	}
	_ = s.audit(ctx, &actorUserID, "member.removed", "project", projectID.String(), map[string]interface{}{
		"target_user_id": targetUserID.String(),
	})
	return nil
}

func (s *ProjectService) TransferOwnership(ctx context.Context, actorUserID, projectID, newOwnerUserID uuid.UUID) error {
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return err
	}
	actorMem, err := s.rbac.GetMembershipForUser(ctx, projectID, actorUserID)
	if err != nil {
		return err
	}
	if err := MustBeOwner(actorMem); err != nil {
		return apperrors.Forbidden()
	}
	ownerRole, err := s.rbac.GetPredefinedRoleBySlug(ctx, rbacmodel.SlugOwner)
	if err != nil {
		return err
	}
	adminRole, err := s.rbac.GetPredefinedRoleBySlug(ctx, rbacmodel.SlugAdmin)
	if err != nil {
		return err
	}
	if _, err := s.rbac.GetMembershipForUser(ctx, projectID, newOwnerUserID); err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.BadRequest("new owner must already be a member", nil)
		}
		return err
	}
	if err := s.rbac.TransferOwnership(ctx, projectID, actorUserID, newOwnerUserID, ownerRole.ID, adminRole.ID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.BadRequest("invalid transfer state", nil)
		}
		return err
	}
	_ = s.audit(ctx, &actorUserID, "project.ownership_transferred", "project", projectID.String(), map[string]interface{}{
		"new_owner_user_id": newOwnerUserID.String(),
	})
	return nil
}

func (s *ProjectService) ListMembers(ctx context.Context, actorUserID, projectID uuid.UUID) ([]rbacmodel.ProjectMembership, error) {
	if _, err := s.authz.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.MemberRead); err != nil {
		return nil, err
	}
	return s.rbac.ListMemberships(ctx, projectID)
}

func (s *ProjectService) CreateCustomRole(ctx context.Context, actorUserID, projectID uuid.UUID, name, description string, permissionKeys []string) (*ProjectRoleView, error) {
	if _, err := s.authz.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.RoleCreate); err != nil {
		return nil, err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return nil, err
	}
	if !hasNonEmptyPermissionKey(permissionKeys) {
		return nil, apperrors.BadRequest("at least one permission is required", nil)
	}
	role, err := s.rbac.CreateCustomRole(ctx, projectID, name, description, permissionKeys)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	_ = s.audit(ctx, &actorUserID, "role.created", "project", projectID.String(), map[string]interface{}{
		"role_id": role.ID.String(),
		"name":    name,
	})
	return s.roleViewByID(ctx, projectID, role.ID)
}

func (s *ProjectService) UpdateCustomRole(ctx context.Context, actorUserID, projectID, roleID uuid.UUID, name *string, description *string, permissionKeys []string) (*ProjectRoleView, error) {
	if _, err := s.authz.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.RoleUpdate); err != nil {
		return nil, err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return nil, err
	}
	if _, err := s.validateMutableCustomRole(ctx, projectID, roleID); err != nil {
		return nil, err
	}
	if _, err := s.rbac.UpdateCustomRole(ctx, projectID, roleID, name, description, permissionKeys); err != nil {
		return nil, mapRepositoryError(err)
	}
	_ = s.audit(ctx, &actorUserID, "role.updated", "project", projectID.String(), map[string]interface{}{
		"role_id": roleID.String(),
	})
	return s.roleViewByID(ctx, projectID, roleID)
}

func (s *ProjectService) DeleteCustomRole(ctx context.Context, actorUserID, projectID, roleID uuid.UUID) error {
	if _, err := s.authz.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.RoleDelete); err != nil {
		return err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return err
	}
	if _, err := s.validateMutableCustomRole(ctx, projectID, roleID); err != nil {
		return err
	}
	if err := s.rbac.DeleteCustomRole(ctx, projectID, roleID); err != nil {
		return mapRepositoryError(err)
	}
	_ = s.audit(ctx, &actorUserID, "role.deleted", "project", projectID.String(), map[string]interface{}{
		"role_id": roleID.String(),
	})
	return nil
}

func (s *ProjectService) ListAssignableRoles(ctx context.Context, actorUserID, projectID uuid.UUID) ([]rbacmodel.Role, error) {
	if _, err := s.authz.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.MemberRead); err != nil {
		return nil, err
	}
	custom, err := s.rbac.ListCustomRoles(ctx, projectID)
	if err != nil {
		return nil, err
	}
	var predefined []rbacmodel.Role
	for _, slug := range []string{rbacmodel.SlugOwner, rbacmodel.SlugAdmin, rbacmodel.SlugDeveloper, rbacmodel.SlugViewer} {
		r, err := s.rbac.GetPredefinedRoleBySlug(ctx, slug)
		if err != nil {
			return nil, err
		}
		predefined = append(predefined, *r)
	}
	out := append(predefined, custom...)
	return out, nil
}

func mapRepositoryError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.NotFound("role not found")
	}
	switch err.Error() {
	case "role is assigned to members":
		return apperrors.New("role_in_use", "cannot delete this role while users are assigned to it", 409, nil)
	case "unknown permission keys":
		return apperrors.BadRequest("one or more permission keys are invalid", nil)
	case "duplicate role name":
		return apperrors.New("duplicate_role_name", "a role with this name already exists in this project", 409, nil)
	case "invalid role name":
		return apperrors.BadRequest("invalid role name", nil)
	default:
		return err
	}
}

func trim(s string) string { return strings.TrimSpace(s) }
