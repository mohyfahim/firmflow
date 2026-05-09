package repository

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	rbacmodel "firmflow/internal/domain/rbac/model"

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

func normalizeRoleName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (r *Repository) GetProject(ctx context.Context, projectID uuid.UUID) (*rbacmodel.Project, error) {
	var p rbacmodel.Project
	if err := r.db.WithContext(ctx).First(&p, "id = ?", projectID).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repository) CreateProject(ctx context.Context, p *rbacmodel.Project) error {
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *Repository) GetPredefinedRoleBySlug(ctx context.Context, slug string) (*rbacmodel.Role, error) {
	var role rbacmodel.Role
	err := r.db.WithContext(ctx).
		Preload("Permissions").
		Where("is_predefined = ? AND project_id IS NULL AND slug = ?", true, slug).
		First(&role).Error
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *Repository) GetMembershipForUser(ctx context.Context, projectID, userID uuid.UUID) (*rbacmodel.ProjectMembership, error) {
	var m rbacmodel.ProjectMembership
	err := r.db.WithContext(ctx).
		Preload("Role", func(db *gorm.DB) *gorm.DB {
			return db.Preload("Permissions")
		}).
		Where("project_id = ? AND user_id = ?", projectID, userID).
		First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repository) GetMembershipByID(ctx context.Context, projectID, membershipID uuid.UUID) (*rbacmodel.ProjectMembership, error) {
	var m rbacmodel.ProjectMembership
	err := r.db.WithContext(ctx).
		Preload("Role", func(db *gorm.DB) *gorm.DB {
			return db.Preload("Permissions")
		}).
		Where("project_id = ? AND id = ?", projectID, membershipID).
		First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repository) CreateMembership(ctx context.Context, m *rbacmodel.ProjectMembership) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *Repository) UpdateMembershipRole(ctx context.Context, projectID, userID, newRoleID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&rbacmodel.ProjectMembership{}).
		Where("project_id = ? AND user_id = ?", projectID, userID).
		Updates(map[string]interface{}{
			"role_id":    newRoleID,
			"updated_at": time.Now().UTC(),
		}).Error
}

func (r *Repository) DeleteMembership(ctx context.Context, projectID, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("project_id = ? AND user_id = ?", projectID, userID).
		Delete(&rbacmodel.ProjectMembership{}).Error
}

func (r *Repository) ListMemberships(ctx context.Context, projectID uuid.UUID) ([]rbacmodel.ProjectMembership, error) {
	var list []rbacmodel.ProjectMembership
	err := r.db.WithContext(ctx).
		Preload("Role").
		Where("project_id = ?", projectID).
		Order("created_at ASC").
		Find(&list).Error
	return list, err
}

func (r *Repository) CountMembersWithRole(ctx context.Context, roleID uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&rbacmodel.ProjectMembership{}).Where("role_id = ?", roleID).Count(&n).Error
	return n, err
}

// CountMembersByRoleIDsInProject returns assigned member counts per role, scoped to one project (single query).
func (r *Repository) CountMembersByRoleIDsInProject(ctx context.Context, projectID uuid.UUID, roleIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	out := make(map[uuid.UUID]int64)
	if len(roleIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		RoleID uuid.UUID `gorm:"column:role_id"`
		Cnt    int64     `gorm:"column:cnt"`
	}
	err := r.db.WithContext(ctx).Model(&rbacmodel.ProjectMembership{}).
		Select("role_id, COUNT(*) as cnt").
		Where("project_id = ? AND role_id IN ?", projectID, roleIDs).
		Group("role_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.RoleID] = row.Cnt
	}
	return out, nil
}

func (r *Repository) CountOwners(ctx context.Context, projectID uuid.UUID, ownerRoleID uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&rbacmodel.ProjectMembership{}).
		Where("project_id = ? AND role_id = ?", projectID, ownerRoleID).
		Count(&n).Error
	return n, err
}

func (r *Repository) GetRole(ctx context.Context, roleID uuid.UUID) (*rbacmodel.Role, error) {
	var role rbacmodel.Role
	err := r.db.WithContext(ctx).Preload("Permissions").First(&role, "id = ?", roleID).Error
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *Repository) ListCustomRoles(ctx context.Context, projectID uuid.UUID) ([]rbacmodel.Role, error) {
	var roles []rbacmodel.Role
	err := r.db.WithContext(ctx).Preload("Permissions").
		Where("project_id = ? AND is_predefined = ?", projectID, false).
		Order("name ASC").
		Find(&roles).Error
	return roles, err
}

// ListPredefinedRoles returns global role templates (Owner, Admin, …) with permissions preloaded.
func (r *Repository) ListPredefinedRoles(ctx context.Context) ([]rbacmodel.Role, error) {
	var roles []rbacmodel.Role
	err := r.db.WithContext(ctx).Preload("Permissions").
		Where("is_predefined = ? AND project_id IS NULL", true).
		Find(&roles).Error
	return roles, err
}

// ListPermissionRegistry returns all permission rows for catalog / validation surfaces.
func (r *Repository) ListPermissionRegistry(ctx context.Context) ([]rbacmodel.Permission, error) {
	var rows []rbacmodel.Permission
	err := r.db.WithContext(ctx).Order("key ASC").Find(&rows).Error
	return rows, err
}

func (r *Repository) customRoleNameTaken(ctx context.Context, tx *gorm.DB, projectID uuid.UUID, normalized string, excludeRoleID *uuid.UUID) (bool, error) {
	q := tx.WithContext(ctx).Model(&rbacmodel.Role{}).
		Where("project_id = ? AND is_predefined = ? AND name_normalized = ?", projectID, false, normalized)
	if excludeRoleID != nil {
		q = q.Where("id <> ?", *excludeRoleID)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *Repository) permissionsByKeys(ctx context.Context, keys []string) ([]rbacmodel.Permission, error) {
	return permissionsByKeysDB(r.db.WithContext(ctx), keys)
}

func dedupePermissionKeys(keys []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func permissionsByKeysDB(db *gorm.DB, keys []string) ([]rbacmodel.Permission, error) {
	keys = dedupePermissionKeys(keys)
	if len(keys) == 0 {
		return nil, nil
	}
	var perms []rbacmodel.Permission
	if err := db.Where("key IN ?", keys).Find(&perms).Error; err != nil {
		return nil, err
	}
	if len(perms) != len(keys) {
		return nil, errors.New("unknown permission keys")
	}
	return perms, nil
}

func (r *Repository) CreateCustomRole(ctx context.Context, projectID uuid.UUID, name, description string, permissionKeys []string) (*rbacmodel.Role, error) {
	perms, err := r.permissionsByKeys(ctx, permissionKeys)
	if err != nil {
		return nil, err
	}
	pid := projectID
	norm := normalizeRoleName(name)
	if norm == "" {
		return nil, errors.New("invalid role name")
	}
	role := &rbacmodel.Role{
		ProjectID:      &pid,
		Slug:           "",
		Name:           strings.TrimSpace(name),
		NameNormalized: norm,
		IsPredefined:   false,
		Description:    strings.TrimSpace(description),
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		taken, err := r.customRoleNameTaken(ctx, tx, projectID, norm, nil)
		if err != nil {
			return err
		}
		if taken {
			return errors.New("duplicate role name")
		}
		if err := tx.Create(role).Error; err != nil {
			return err
		}
		return tx.Model(role).Association("Permissions").Replace(perms)
	})
	if err != nil {
		return nil, err
	}
	return r.GetRole(ctx, role.ID)
}

func (r *Repository) UpdateCustomRole(ctx context.Context, projectID, roleID uuid.UUID, name *string, description *string, permissionKeys []string) (*rbacmodel.Role, error) {
	var role rbacmodel.Role
	if err := r.db.WithContext(ctx).First(&role, "id = ? AND project_id = ? AND is_predefined = ?", roleID, projectID, false).Error; err != nil {
		return nil, err
	}
	updates := map[string]interface{}{"updated_at": time.Now().UTC()}
	if name != nil {
		updates["name"] = strings.TrimSpace(*name)
		updates["name_normalized"] = normalizeRoleName(*name)
	}
	if description != nil {
		updates["description"] = strings.TrimSpace(*description)
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if name != nil {
			norm := normalizeRoleName(*name)
			if norm == "" {
				return errors.New("invalid role name")
			}
			taken, err := r.customRoleNameTaken(ctx, tx, projectID, norm, &roleID)
			if err != nil {
				return err
			}
			if taken {
				return errors.New("duplicate role name")
			}
		}
		if len(updates) > 1 {
			if err := tx.Model(&role).Updates(updates).Error; err != nil {
				return err
			}
		}
		if permissionKeys != nil {
			perms, err := permissionsByKeysDB(tx, permissionKeys)
			if err != nil {
				return err
			}
			if err := tx.First(&role, "id = ?", roleID).Error; err != nil {
				return err
			}
			if err := tx.Model(&role).Association("Permissions").Replace(perms); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.GetRole(ctx, roleID)
}

func (r *Repository) DeleteCustomRole(ctx context.Context, projectID, roleID uuid.UUID) error {
	var role rbacmodel.Role
	if err := r.db.WithContext(ctx).First(&role, "id = ? AND project_id = ? AND is_predefined = ?", roleID, projectID, false).Error; err != nil {
		return err
	}
	n, err := r.CountMembersWithRole(ctx, roleID)
	if err != nil {
		return err
	}
	if n > 0 {
		return errors.New("role is assigned to members")
	}
	return r.db.WithContext(ctx).Delete(&role).Error
}

func (r *Repository) TransferOwnership(ctx context.Context, projectID, fromUserID, toUserID uuid.UUID, ownerRoleID, adminRoleID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var fromMember rbacmodel.ProjectMembership
		if err := tx.Where("project_id = ? AND user_id = ?", projectID, fromUserID).First(&fromMember).Error; err != nil {
			return err
		}
		var fromRole rbacmodel.Role
		if err := tx.First(&fromRole, "id = ?", fromMember.RoleID).Error; err != nil {
			return err
		}
		if !fromRole.IsPredefined || fromRole.Slug != rbacmodel.SlugOwner {
			return errors.New("only owner can transfer ownership")
		}
		var toMember rbacmodel.ProjectMembership
		if err := tx.Where("project_id = ? AND user_id = ?", projectID, toUserID).First(&toMember).Error; err != nil {
			return err
		}
		if fromUserID == toUserID {
			return errors.New("cannot transfer to self")
		}
		now := time.Now().UTC()
		if err := tx.Model(&rbacmodel.ProjectMembership{}).Where("id = ?", fromMember.ID).
			Updates(map[string]interface{}{"role_id": adminRoleID, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&rbacmodel.ProjectMembership{}).Where("id = ?", toMember.ID).
			Updates(map[string]interface{}{"role_id": ownerRoleID, "updated_at": now}).Error; err != nil {
			return err
		}
		return nil
	})
}
