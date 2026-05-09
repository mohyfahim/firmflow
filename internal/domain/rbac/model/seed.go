package model

import (
	"fmt"
	"time"

	"firmflow/internal/domain/rbac/permission"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SeedRegistry inserts all permissions and predefined roles with their permission sets.
func SeedRegistry(db *gorm.DB) error {
	now := time.Now().UTC()
	for _, key := range permission.All() {
		var existing Permission
		err := db.Where("key = ?", key).First(&existing).Error
		if err == nil {
			continue
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		p := &Permission{
			Key: key, Description: key,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := db.Create(p).Error; err != nil {
			return fmt.Errorf("seed permission %s: %w", key, err)
		}
	}

	var perms []Permission
	if err := db.Find(&perms).Error; err != nil {
		return err
	}
	byKey := make(map[string]uuid.UUID, len(perms))
	for _, p := range perms {
		byKey[p.Key] = p.ID
	}

	viewerKeys := []string{
		permission.ProjectRead, permission.DashboardRead,
		permission.DeviceRead, permission.FirmwareRead, permission.CampaignRead,
	}
	developerKeys := append(viewerKeys,
		permission.DeviceCreate, permission.DeviceUpdate, permission.DeviceBlock,
		permission.DeviceTokenRotate, permission.DeviceAssignGroup,
		permission.FirmwareUpload,
		permission.CampaignCreate, permission.CampaignUpdate, permission.CampaignPause,
	)
	adminKeys := append(developerKeys,
		permission.MemberRead, permission.MemberInvite, permission.MemberUpdateRole, permission.MemberRemove,
		permission.RoleRead, permission.RoleCreate, permission.RoleUpdate, permission.RoleDelete,
		permission.ProjectUpdate,
		permission.AuditRead,
	)
	ownerKeys := append(adminKeys,
		permission.ProjectDelete,
		permission.CampaignCancel,
	)

	defs := []struct {
		slug, name string
		keys       []string
	}{
		{SlugOwner, "Owner", ownerKeys},
		{SlugAdmin, "Admin", adminKeys},
		{SlugDeveloper, "Developer", developerKeys},
		{SlugViewer, "Viewer", viewerKeys},
	}

	for _, d := range defs {
		var existing Role
		err := db.Where("is_predefined = ? AND project_id IS NULL AND slug = ?", true, d.slug).First(&existing).Error
		if err == nil {
			ids := keysToIDs(byKey, d.keys)
			if err := replaceRolePermissions(db, existing.ID, ids); err != nil {
				return err
			}
			continue
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		r := &Role{
			ProjectID:      nil,
			Slug:           d.slug,
			Name:           d.name,
			NameNormalized: d.slug,
			IsPredefined:   true,
			Description:    "Predefined role",
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := db.Create(r).Error; err != nil {
			return fmt.Errorf("seed role %s: %w", d.slug, err)
		}
		ids := keysToIDs(byKey, d.keys)
		if err := replaceRolePermissions(db, r.ID, ids); err != nil {
			return err
		}
	}
	return nil
}

func keysToIDs(byKey map[string]uuid.UUID, keys []string) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(keys))
	for _, k := range keys {
		if id, ok := byKey[k]; ok {
			out = append(out, id)
		}
	}
	return out
}

func replaceRolePermissions(db *gorm.DB, roleID uuid.UUID, permIDs []uuid.UUID) error {
	var role Role
	if err := db.First(&role, "id = ?", roleID).Error; err != nil {
		return err
	}
	if len(permIDs) == 0 {
		return db.Model(&role).Association("Permissions").Clear()
	}
	var plist []Permission
	if err := db.Where("id IN ?", permIDs).Find(&plist).Error; err != nil {
		return err
	}
	return db.Model(&role).Association("Permissions").Replace(plist)
}
