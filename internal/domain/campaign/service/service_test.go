package service

import (
	"context"
	"testing"
	"time"

	authmodel "firmflow/internal/domain/auth/model"
	authrepo "firmflow/internal/domain/auth/repository"
	campaignmodel "firmflow/internal/domain/campaign/model"
	campaignrepo "firmflow/internal/domain/campaign/repository"
	devicemodel "firmflow/internal/domain/device/model"
	devicerepo "firmflow/internal/domain/device/repository"
	firmwaremodel "firmflow/internal/domain/firmware/model"
	firmwarerepo "firmflow/internal/domain/firmware/repository"
	projectmodel "firmflow/internal/domain/project/model"
	rbacmodel "firmflow/internal/domain/rbac/model"
	rbacrepo "firmflow/internal/domain/rbac/repository"
	rbacsvc "firmflow/internal/domain/rbac/service"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupCampaignTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, m := range []interface {
		Migrate(context.Context, *gorm.DB) error
	}{
		authmodel.Migrator{},
		rbacmodel.Migrator{},
		projectmodel.Migrator{},
		devicemodel.Migrator{},
		firmwaremodel.Migrator{},
		campaignmodel.Migrator{},
	} {
		if err := m.Migrate(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func seedUserCamp(t *testing.T, db *gorm.DB, email string) uuid.UUID {
	t.Helper()
	repo := authrepo.New(db)
	u := &authmodel.User{Email: email, PasswordHash: "x"}
	p := &authmodel.UserProfile{Timezone: "UTC", PreferredLanguage: "en"}
	if err := repo.CreateUserWithProfile(context.Background(), u, p); err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func newCampaignSvc(t *testing.T, db *gorm.DB) (*Service, *rbacsvc.ProjectService) {
	t.Helper()
	rbacR := rbacrepo.New(db)
	authR := authrepo.New(db)
	authz := rbacsvc.NewAuthorizer(rbacR)
	cr := campaignrepo.New(db)
	fwR := firmwarerepo.New(db)
	devR := devicerepo.New(db)
	svc := New(rbacR, authR, authz, cr, fwR, devR)
	ps := rbacsvc.NewProjectService(rbacR, authR, authz)
	return svc, ps
}

func TestCreateCampaign_StateAndProgress(t *testing.T) {
	db := setupCampaignTestDB(t)
	owner := seedUserCamp(t, db, "owner-camp@example.com")
	svc, ps := newCampaignSvc(t, db)
	ctx := context.Background()
	proj, err := ps.CreateProject(ctx, owner, "CampProj", "")
	if err != nil {
		t.Fatal(err)
	}
	devR := devicerepo.New(db)
	fwR := firmwarerepo.New(db)
	pre, err := devR.ListPredefinedDeviceTypes(ctx)
	if err != nil || len(pre) == 0 {
		t.Fatal("need predefined types")
	}
	now := time.Now().UTC()
	fw := &firmwaremodel.Firmware{
		ProjectID: proj.ID, Version: "1.0.0", VersionNormalized: "1.0.0",
		FileSizeBytes: 1, ChecksumSHA256: "ab", OriginalFilename: "x.bin",
		StorageProvider: "local", StorageKey: "k1", UploadedByUserID: owner,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := fwR.CreateFirmwareWithTypes(ctx, fw, []uuid.UUID{pre[0].ID}); err != nil {
		t.Fatal(err)
	}
	d := &projectmodel.Device{
		ProjectID: proj.ID, Name: "D1", DeviceTypeID: pre[0].ID,
		HardwareIdentifier: "AA:01", CurrentFirmwareVersion: "0.9.0",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.WithContext(ctx).Create(d).Error; err != nil {
		t.Fatal(err)
	}

	detail, err := svc.CreateCampaign(ctx, owner, proj.ID, CreateCampaignInput{
		Name:              "Roll",
		FirmwareID:        fw.ID,
		RolloutKind:       campaignmodel.RolloutKindImmediate,
		ExplicitDeviceIDs: []uuid.UUID{d.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status != campaignmodel.StatusActive {
		t.Fatalf("status %s", detail.Status)
	}
	if detail.Progress.TargetCount != 1 || detail.Progress.Pending != 1 {
		t.Fatalf("progress %+v", detail.Progress)
	}
}

func TestPauseCancelTransitions(t *testing.T) {
	db := setupCampaignTestDB(t)
	owner := seedUserCamp(t, db, "owner-pc@example.com")
	svc, ps := newCampaignSvc(t, db)
	ctx := context.Background()
	proj, _ := ps.CreateProject(ctx, owner, "P", "")
	devR := devicerepo.New(db)
	fwR := firmwarerepo.New(db)
	pre, _ := devR.ListPredefinedDeviceTypes(ctx)
	now := time.Now().UTC()
	fw := &firmwaremodel.Firmware{
		ProjectID: proj.ID, Version: "2.0.0", VersionNormalized: "2.0.0",
		FileSizeBytes: 1, ChecksumSHA256: "cd", OriginalFilename: "y.bin",
		StorageProvider: "local", StorageKey: "k2", UploadedByUserID: owner,
		CreatedAt: now, UpdatedAt: now,
	}
	_ = fwR.CreateFirmwareWithTypes(ctx, fw, []uuid.UUID{pre[0].ID})
	d := &projectmodel.Device{
		ProjectID: proj.ID, Name: "D2", DeviceTypeID: pre[0].ID,
		HardwareIdentifier: "AA:02", CurrentFirmwareVersion: "0.9.0",
		CreatedAt: now, UpdatedAt: now,
	}
	_ = db.WithContext(ctx).Create(d).Error

	detail, _ := svc.CreateCampaign(ctx, owner, proj.ID, CreateCampaignInput{
		Name: "C", FirmwareID: fw.ID, RolloutKind: campaignmodel.RolloutKindImmediate,
		ExplicitDeviceIDs: []uuid.UUID{d.ID},
	})
	if err := svc.PauseCampaign(ctx, owner, proj.ID, detail.ID); err != nil {
		t.Fatal(err)
	}
	d2, _ := svc.GetCampaignDetail(ctx, owner, proj.ID, detail.ID)
	if d2.Status != campaignmodel.StatusPaused {
		t.Fatalf("want paused got %s", d2.Status)
	}
	if err := svc.ResumeCampaign(ctx, owner, proj.ID, detail.ID); err != nil {
		t.Fatal(err)
	}
	d3, _ := svc.GetCampaignDetail(ctx, owner, proj.ID, detail.ID)
	if d3.Status != campaignmodel.StatusActive {
		t.Fatalf("want active got %s", d3.Status)
	}
	if err := svc.CancelCampaign(ctx, owner, proj.ID, detail.ID); err != nil {
		t.Fatal(err)
	}
	d4, _ := svc.GetCampaignDetail(ctx, owner, proj.ID, detail.ID)
	if d4.Status != campaignmodel.StatusCancelled {
		t.Fatalf("want cancelled got %s", d4.Status)
	}
}

func TestStablePercentageSubset(t *testing.T) {
	ids := []uuid.UUID{
		uuid.MustParse("00000000-0000-0000-0000-000000000003"),
		uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		uuid.MustParse("00000000-0000-0000-0000-000000000002"),
	}
	sorted := sortUUIDs(ids)
	sub := takeStablePercentage(sorted, 34)
	if len(sub) != 2 {
		t.Fatalf("len %d", len(sub))
	}
	if sub[0] != sorted[0] || sub[1] != sorted[1] {
		t.Fatalf("unexpected subset %v from %v", sub, sorted)
	}
}

func TestActivateDueCampaigns(t *testing.T) {
	db := setupCampaignTestDB(t)
	owner := seedUserCamp(t, db, "owner-act@example.com")
	svc, ps := newCampaignSvc(t, db)
	ctx := context.Background()
	proj, _ := ps.CreateProject(ctx, owner, "P", "")
	devR := devicerepo.New(db)
	fwR := firmwarerepo.New(db)
	pre, _ := devR.ListPredefinedDeviceTypes(ctx)
	now := time.Now().UTC()
	fw := &firmwaremodel.Firmware{
		ProjectID: proj.ID, Version: "3.0.0", VersionNormalized: "3.0.0",
		FileSizeBytes: 1, ChecksumSHA256: "ef", OriginalFilename: "z.bin",
		StorageProvider: "local", StorageKey: "k3", UploadedByUserID: owner,
		CreatedAt: now, UpdatedAt: now,
	}
	_ = fwR.CreateFirmwareWithTypes(ctx, fw, []uuid.UUID{pre[0].ID})
	d := &projectmodel.Device{
		ProjectID: proj.ID, Name: "D3", DeviceTypeID: pre[0].ID,
		HardwareIdentifier: "AA:03", CurrentFirmwareVersion: "0.9.0",
		CreatedAt: now, UpdatedAt: now,
	}
	_ = db.WithContext(ctx).Create(d).Error
	start := now.Add(10 * time.Minute)
	detail, err := svc.CreateCampaign(ctx, owner, proj.ID, CreateCampaignInput{
		Name: "Sch", FirmwareID: fw.ID, RolloutKind: campaignmodel.RolloutKindTimeScheduled,
		ScheduledStartAt:  &start,
		ExplicitDeviceIDs: []uuid.UUID{d.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status != campaignmodel.StatusScheduled {
		t.Fatalf("want scheduled got %s", detail.Status)
	}
	past := now.Add(-2 * time.Minute)
	if err := db.Model(&campaignmodel.Campaign{}).Where("id = ?", detail.ID).Update("scheduled_start_at", past).Error; err != nil {
		t.Fatal(err)
	}
	ids, err := svc.ActivateDueCampaigns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("activated %v", ids)
	}
	d2, _ := svc.GetCampaignDetail(ctx, owner, proj.ID, detail.ID)
	if d2.Status != campaignmodel.StatusActive {
		t.Fatalf("want active got %s", d2.Status)
	}
}
