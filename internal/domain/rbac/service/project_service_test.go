package service

import (
	"context"
	"testing"
	"time"

	authmodel "firmflow/internal/domain/auth/model"
	authrepo "firmflow/internal/domain/auth/repository"
	campaignmodel "firmflow/internal/domain/campaign/model"
	projectmodel "firmflow/internal/domain/project/model"
	rbacmodel "firmflow/internal/domain/rbac/model"
	rbacperm "firmflow/internal/domain/rbac/permission"
	rbacrepo "firmflow/internal/domain/rbac/repository"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupRBACTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	am := authmodel.Migrator{}
	if err := am.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	rm := rbacmodel.Migrator{}
	if err := rm.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	pm := projectmodel.Migrator{}
	if err := pm.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	cm := campaignmodel.Migrator{}
	if err := cm.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	return db
}

func seedUser(t *testing.T, db *gorm.DB, email string) uuid.UUID {
	t.Helper()
	repo := authrepo.New(db)
	u := &authmodel.User{Email: email, PasswordHash: "x"}
	p := &authmodel.UserProfile{Timezone: "UTC", PreferredLanguage: "en"}
	if err := repo.CreateUserWithProfile(context.Background(), u, p); err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func TestCreateCustomRole_UpdatePermissions_DeleteBlockedWhenAssigned(t *testing.T) {
	db := setupRBACTestDB(t)
	ownerID := seedUser(t, db, "owner@example.com")
	_ = seedUser(t, db, "member@example.com")

	rbacR := rbacrepo.New(db)
	authR := authrepo.New(db)
	authz := NewAuthorizer(rbacR)
	svc := NewProjectService(rbacR, authR, authz)

	ctx := context.Background()
	proj, err := svc.CreateProject(ctx, ownerID, "Acme", "")
	if err != nil {
		t.Fatal(err)
	}

	perms := []string{rbacperm.DeviceRead, rbacperm.DeviceCreate}
	view, err := svc.CreateCustomRole(ctx, ownerID, proj.ID, "Operators", "ops", perms)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Permissions) != len(perms) {
		t.Fatalf("permissions: got %d want %d", len(view.Permissions), len(perms))
	}

	more := append(perms, rbacperm.FirmwareRead)
	updated, err := svc.UpdateCustomRole(ctx, ownerID, proj.ID, view.ID, nil, nil, more)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Permissions) != len(more) {
		t.Fatalf("updated permissions: got %d want %d", len(updated.Permissions), len(more))
	}

	if err := svc.InviteMember(ctx, ownerID, proj.ID, "member@example.com", view.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteCustomRole(ctx, ownerID, proj.ID, view.ID); err == nil {
		t.Fatal("expected delete to fail when role is assigned")
	}
}

func TestNonMemberCannotInvite(t *testing.T) {
	db := setupRBACTestDB(t)
	ownerID := seedUser(t, db, "o@example.com")
	outsiderID := seedUser(t, db, "out@example.com")

	rbacR := rbacrepo.New(db)
	authR := authrepo.New(db)
	authz := NewAuthorizer(rbacR)
	svc := NewProjectService(rbacR, authR, authz)

	ctx := context.Background()
	proj, err := svc.CreateProject(ctx, ownerID, "P", "d")
	if err != nil {
		t.Fatal(err)
	}
	vr, err := rbacR.GetPredefinedRoleBySlug(ctx, rbacmodel.SlugViewer)
	if err != nil {
		t.Fatal(err)
	}
	err = svc.InviteMember(ctx, outsiderID, proj.ID, "o@example.com", vr.ID)
	if err == nil {
		t.Fatal("expected forbidden for non-member")
	}
}

func TestTransferOwnership(t *testing.T) {
	db := setupRBACTestDB(t)
	ownerID := seedUser(t, db, "owner@t.com")
	nextID := seedUser(t, db, "next@t.com")

	rbacR := rbacrepo.New(db)
	authR := authrepo.New(db)
	authz := NewAuthorizer(rbacR)
	svc := NewProjectService(rbacR, authR, authz)
	ctx := context.Background()

	proj, err := svc.CreateProject(ctx, ownerID, "T", "")
	if err != nil {
		t.Fatal(err)
	}
	vr, err := rbacR.GetPredefinedRoleBySlug(ctx, rbacmodel.SlugViewer)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.InviteMember(ctx, ownerID, proj.ID, "next@t.com", vr.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.TransferOwnership(ctx, ownerID, proj.ID, nextID); err != nil {
		t.Fatal(err)
	}

	ownerMem, err := rbacR.GetMembershipForUser(ctx, proj.ID, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	adminRole, err := rbacR.GetPredefinedRoleBySlug(ctx, rbacmodel.SlugAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if ownerMem.RoleID != adminRole.ID {
		t.Fatalf("previous owner should be admin")
	}

	nextMem, err := rbacR.GetMembershipForUser(ctx, proj.ID, nextID)
	if err != nil {
		t.Fatal(err)
	}
	or, err := rbacR.GetPredefinedRoleBySlug(ctx, rbacmodel.SlugOwner)
	if err != nil {
		t.Fatal(err)
	}
	if nextMem.RoleID != or.ID {
		t.Fatalf("new owner should have owner role")
	}
}

func TestArchivedProjectBlocksInvite(t *testing.T) {
	db := setupRBACTestDB(t)
	ownerID := seedUser(t, db, "archowner@x.com")
	_ = seedUser(t, db, "archmem@x.com")

	rbacR := rbacrepo.New(db)
	authR := authrepo.New(db)
	authz := NewAuthorizer(rbacR)
	svc := NewProjectService(rbacR, authR, authz)
	ctx := context.Background()

	proj, err := svc.CreateProject(ctx, ownerID, "Archived", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ArchiveProject(ctx, ownerID, proj.ID, true); err != nil {
		t.Fatal(err)
	}
	vr, err := rbacR.GetPredefinedRoleBySlug(ctx, rbacmodel.SlugViewer)
	if err != nil {
		t.Fatal(err)
	}
	err = svc.InviteMember(ctx, ownerID, proj.ID, "archmem@x.com", vr.ID)
	if err == nil {
		t.Fatal("expected invite to fail for archived project")
	}
}

func TestDeleteBlockedByActiveCampaign(t *testing.T) {
	db := setupRBACTestDB(t)
	ownerID := seedUser(t, db, "del@x.com")

	rbacR := rbacrepo.New(db)
	authR := authrepo.New(db)
	authz := NewAuthorizer(rbacR)
	svc := NewProjectService(rbacR, authR, authz)
	ctx := context.Background()

	proj, err := svc.CreateProject(ctx, ownerID, "DelProj", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := rbacR.DB().WithContext(ctx).Create(&campaignmodel.Campaign{
		ProjectID:         proj.ID,
		Name:              "blocking",
		FirmwareID:        uuid.New(),
		RolloutKind:       campaignmodel.RolloutKindImmediate,
		Status:            campaignmodel.StatusActive,
		TargetDeviceCount: 0,
		CreatedByUserID:   ownerID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteProject(ctx, ownerID, proj.ID); err == nil {
		t.Fatal("expected delete to fail with active campaign")
	}
}

func TestListProjectRoles_IncludesPredefinedCatalogAndCounts(t *testing.T) {
	db := setupRBACTestDB(t)
	ownerID := seedUser(t, db, "listroles@x.com")

	rbacR := rbacrepo.New(db)
	authR := authrepo.New(db)
	authz := NewAuthorizer(rbacR)
	svc := NewProjectService(rbacR, authR, authz)
	ctx := context.Background()

	proj, err := svc.CreateProject(ctx, ownerID, "RolesProj", "")
	if err != nil {
		t.Fatal(err)
	}
	perms := []string{rbacperm.DeviceRead}
	cr, err := svc.CreateCustomRole(ctx, ownerID, proj.ID, "Ops", "", perms)
	if err != nil {
		t.Fatal(err)
	}

	payload, err := svc.ListProjectRoles(ctx, ownerID, proj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.PermissionCatalog) < len(rbacperm.All()) {
		t.Fatalf("catalog short: got %d", len(payload.PermissionCatalog))
	}
	if len(payload.Roles) < 5 {
		t.Fatalf("expected 4 predefined + 1 custom, got %d", len(payload.Roles))
	}
	var ownerHit, customHit bool
	for _, r := range payload.Roles {
		if r.Type == RoleTypeSystem && r.Slug == rbacmodel.SlugOwner && r.AssignedUserCount == 1 {
			ownerHit = true
		}
		if r.ID == cr.ID && r.Type == RoleTypeCustom && r.AssignedUserCount == 0 {
			customHit = true
		}
	}
	if !ownerHit || !customHit {
		t.Fatalf("expected owner membership count and zero-count custom: owner=%v custom=%v", ownerHit, customHit)
	}
}

func TestDuplicateCustomRoleName(t *testing.T) {
	db := setupRBACTestDB(t)
	ownerID := seedUser(t, db, "duprole@x.com")

	rbacR := rbacrepo.New(db)
	authR := authrepo.New(db)
	authz := NewAuthorizer(rbacR)
	svc := NewProjectService(rbacR, authR, authz)
	ctx := context.Background()

	proj, err := svc.CreateProject(ctx, ownerID, "Dup", "")
	if err != nil {
		t.Fatal(err)
	}
	p := []string{rbacperm.DeviceRead}
	if _, err := svc.CreateCustomRole(ctx, ownerID, proj.ID, "Same Name", "", p); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateCustomRole(ctx, ownerID, proj.ID, "  same name  ", "", p); err == nil {
		t.Fatal("expected duplicate name error")
	}
}

func TestCreateCustomRole_UnknownPermissionKey(t *testing.T) {
	db := setupRBACTestDB(t)
	ownerID := seedUser(t, db, "badperm@x.com")

	rbacR := rbacrepo.New(db)
	authR := authrepo.New(db)
	authz := NewAuthorizer(rbacR)
	svc := NewProjectService(rbacR, authR, authz)
	ctx := context.Background()

	proj, err := svc.CreateProject(ctx, ownerID, "BadPerm", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateCustomRole(ctx, ownerID, proj.ID, "R", "", []string{"not.a.real.permission"}); err == nil {
		t.Fatal("expected invalid permission error")
	}
}

func TestCannotMutateOrDeleteSystemRole(t *testing.T) {
	db := setupRBACTestDB(t)
	ownerID := seedUser(t, db, "sysrole@x.com")

	rbacR := rbacrepo.New(db)
	authR := authrepo.New(db)
	authz := NewAuthorizer(rbacR)
	svc := NewProjectService(rbacR, authR, authz)
	ctx := context.Background()

	proj, err := svc.CreateProject(ctx, ownerID, "Sys", "")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := rbacR.GetPredefinedRoleBySlug(ctx, rbacmodel.SlugAdmin)
	if err != nil {
		t.Fatal(err)
	}
	name := "x"
	if _, err := svc.UpdateCustomRole(ctx, ownerID, proj.ID, admin.ID, &name, nil, nil); err == nil {
		t.Fatal("expected rejection updating system role")
	}
	if err := svc.DeleteCustomRole(ctx, ownerID, proj.ID, admin.ID); err == nil {
		t.Fatal("expected rejection deleting system role")
	}
}

func TestTransferOwnershipNonMemberFails(t *testing.T) {
	db := setupRBACTestDB(t)
	ownerID := seedUser(t, db, "a@t.com")
	strangerID := seedUser(t, db, "stranger@t.com")

	rbacR := rbacrepo.New(db)
	authR := authrepo.New(db)
	authz := NewAuthorizer(rbacR)
	svc := NewProjectService(rbacR, authR, authz)
	ctx := context.Background()

	proj, err := svc.CreateProject(ctx, ownerID, "X", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.TransferOwnership(ctx, ownerID, proj.ID, strangerID); err == nil {
		t.Fatal("expected transfer to non-member to fail")
	}
}
