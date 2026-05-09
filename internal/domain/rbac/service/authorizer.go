package service

import (
	"context"

	apperrors "firmflow/internal/common/errors"
	rbacmodel "firmflow/internal/domain/rbac/model"
	rbacrepo "firmflow/internal/domain/rbac/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Authorizer enforces project-scoped RBAC using membership + role permissions.
type Authorizer struct {
	repo *rbacrepo.Repository
}

func NewAuthorizer(repo *rbacrepo.Repository) *Authorizer {
	return &Authorizer{repo: repo}
}

func (a *Authorizer) Repo() *rbacrepo.Repository {
	return a.repo
}

// AuthorizeProject loads membership and ensures the role grants the permission key.
func (a *Authorizer) AuthorizeProject(ctx context.Context, projectID, userID uuid.UUID, permissionKey string) (*rbacmodel.ProjectMembership, error) {
	m, err := a.repo.GetMembershipForUser(ctx, projectID, userID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.Forbidden()
		}
		return nil, err
	}
	if RoleHasPermission(&m.Role, permissionKey) {
		return m, nil
	}
	return nil, apperrors.Forbidden()
}

// RoleHasPermission checks effective permission keys on the role (custom or predefined).
func RoleHasPermission(role *rbacmodel.Role, permissionKey string) bool {
	if role == nil {
		return false
	}
	for _, p := range role.Permissions {
		if p.Key == permissionKey {
			return true
		}
	}
	return false
}

// IsPredefinedOwner returns true if the membership uses the global Owner template role.
func IsPredefinedOwner(m *rbacmodel.ProjectMembership) bool {
	if m == nil {
		return false
	}
	return m.Role.IsPredefined && m.Role.Slug == rbacmodel.SlugOwner
}

// ValidateAssignableRole ensures role can be used in this project (global predefined or project custom).
func ValidateAssignableRole(projectID uuid.UUID, role *rbacmodel.Role) error {
	if role == nil {
		return apperrors.BadRequest("invalid role", nil)
	}
	if role.IsPredefined && role.ProjectID == nil {
		return nil
	}
	if !role.IsPredefined && role.ProjectID != nil && *role.ProjectID == projectID {
		return nil
	}
	return apperrors.BadRequest("role does not belong to this project", nil)
}

// MustBeOwner rejects if membership is not the predefined Owner role.
func MustBeOwner(m *rbacmodel.ProjectMembership) error {
	if IsPredefinedOwner(m) {
		return nil
	}
	return apperrors.Forbidden()
}

func ParseUUIDParam(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, apperrors.BadRequest("invalid id", nil)
	}
	return id, nil
}
