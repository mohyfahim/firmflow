package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	apperrors "firmflow/internal/common/errors"
	rbacmodel "firmflow/internal/domain/rbac/model"
	rbacperm "firmflow/internal/domain/rbac/permission"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RoleType values match JSON API contract ("system" | "custom").
const (
	RoleTypeSystem = "system"
	RoleTypeCustom = "custom"
)

// PermissionGrant is one assigned permission on a role for API responses and checklist UIs.
type PermissionGrant struct {
	Key         string `json:"key"`
	Description string `json:"description,omitempty"`
}

// PermissionCatalogItem describes a registry permission for building checkbox lists (create/update forms).
type PermissionCatalogItem struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Group       string `json:"group"`
}

// ProjectRoleView is the canonical role payload for list/create/update responses.
type ProjectRoleView struct {
	ID                uuid.UUID         `json:"id"`
	Name              string            `json:"name"`
	Type              string            `json:"type"`
	Slug              string            `json:"slug,omitempty"`
	Description       string            `json:"description,omitempty"`
	Permissions       []PermissionGrant `json:"permissions"`
	AssignedUserCount int64             `json:"assigned_user_count"`
}

// ProjectRolesPayload is returned by GET .../roles with checklist metadata for the dashboard.
type ProjectRolesPayload struct {
	Roles             []ProjectRoleView       `json:"roles"`
	PermissionCatalog []PermissionCatalogItem `json:"permission_catalog"`
}

func hasNonEmptyPermissionKey(keys []string) bool {
	for _, k := range keys {
		if strings.TrimSpace(k) != "" {
			return true
		}
	}
	return false
}

func permissionGroupFromKey(key string) string {
	if i := strings.Index(key, "."); i > 0 {
		return key[:i]
	}
	return "other"
}

func sortPredefinedRoles(roles []rbacmodel.Role) {
	order := []string{
		rbacmodel.SlugOwner,
		rbacmodel.SlugAdmin,
		rbacmodel.SlugDeveloper,
		rbacmodel.SlugViewer,
	}
	rank := func(slug string) int {
		for i, s := range order {
			if s == slug {
				return i
			}
		}
		return len(order)
	}
	sort.SliceStable(roles, func(i, j int) bool {
		return rank(roles[i].Slug) < rank(roles[j].Slug)
	})
}

func roleToGrants(role *rbacmodel.Role) []PermissionGrant {
	if role == nil {
		return nil
	}
	out := make([]PermissionGrant, 0, len(role.Permissions))
	for _, p := range role.Permissions {
		out = append(out, PermissionGrant{Key: p.Key, Description: p.Description})
	}
	return out
}

func (s *ProjectService) buildPermissionCatalog(ctx context.Context) ([]PermissionCatalogItem, error) {
	rows, err := s.rbac.ListPermissionRegistry(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PermissionCatalogItem, 0, len(rows))
	for _, p := range rows {
		out = append(out, PermissionCatalogItem{
			Key:         p.Key,
			Description: p.Description,
			Group:       permissionGroupFromKey(p.Key),
		})
	}
	return out, nil
}

func (s *ProjectService) projectRoleView(role *rbacmodel.Role, counts map[uuid.UUID]int64) ProjectRoleView {
	if role == nil {
		return ProjectRoleView{}
	}
	t := RoleTypeCustom
	if role.IsPredefined {
		t = RoleTypeSystem
	}
	n := counts[role.ID]
	return ProjectRoleView{
		ID:                role.ID,
		Name:              role.Name,
		Type:              t,
		Slug:              role.Slug,
		Description:       role.Description,
		Permissions:       roleToGrants(role),
		AssignedUserCount: n,
	}
}

func (s *ProjectService) roleViewByID(ctx context.Context, projectID, roleID uuid.UUID) (*ProjectRoleView, error) {
	role, err := s.rbac.GetRole(ctx, roleID)
	if err != nil {
		return nil, err
	}
	counts, err := s.rbac.CountMembersByRoleIDsInProject(ctx, projectID, []uuid.UUID{roleID})
	if err != nil {
		return nil, err
	}
	v := s.projectRoleView(role, counts)
	return &v, nil
}

// ListProjectRoles returns predefined and custom roles with permission grants and member counts per role.
func (s *ProjectService) ListProjectRoles(ctx context.Context, actorUserID, projectID uuid.UUID) (*ProjectRolesPayload, error) {
	if _, err := s.authz.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.RoleRead); err != nil {
		return nil, err
	}
	pre, err := s.rbac.ListPredefinedRoles(ctx)
	if err != nil {
		return nil, err
	}
	sortPredefinedRoles(pre)

	custom, err := s.rbac.ListCustomRoles(ctx, projectID)
	if err != nil {
		return nil, err
	}

	roleIDs := make([]uuid.UUID, 0, len(pre)+len(custom))
	for i := range pre {
		roleIDs = append(roleIDs, pre[i].ID)
	}
	for i := range custom {
		roleIDs = append(roleIDs, custom[i].ID)
	}

	counts, err := s.rbac.CountMembersByRoleIDsInProject(ctx, projectID, roleIDs)
	if err != nil {
		return nil, err
	}

	catalog, err := s.buildPermissionCatalog(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]ProjectRoleView, 0, len(pre)+len(custom))
	for i := range pre {
		out = append(out, s.projectRoleView(&pre[i], counts))
	}
	for i := range custom {
		out = append(out, s.projectRoleView(&custom[i], counts))
	}

	return &ProjectRolesPayload{Roles: out, PermissionCatalog: catalog}, nil
}

func (s *ProjectService) validateMutableCustomRole(ctx context.Context, projectID, roleID uuid.UUID) (*rbacmodel.Role, error) {
	role, err := s.rbac.GetRole(ctx, roleID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("role not found")
		}
		return nil, err
	}
	if role.IsPredefined {
		return nil, apperrors.BadRequest("system roles cannot be modified", nil)
	}
	if role.ProjectID == nil || *role.ProjectID != projectID {
		return nil, apperrors.BadRequest("role does not belong to this project", nil)
	}
	return role, nil
}
