package service

import (
	"context"
	"testing"
	"time"

	apperrors "firmflow/internal/common/errors"
	authmodel "firmflow/internal/domain/auth/model"
	authrepo "firmflow/internal/domain/auth/repository"
	authsecurity "firmflow/internal/domain/auth/security"
	devicemodel "firmflow/internal/domain/device/model"
	devicerepo "firmflow/internal/domain/device/repository"
	projectmodel "firmflow/internal/domain/project/model"
	rbacmodel "firmflow/internal/domain/rbac/model"
	rbacrepo "firmflow/internal/domain/rbac/repository"
	rbacsvc "firmflow/internal/domain/rbac/service"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupDeviceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := (authmodel.Migrator{}).Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := (rbacmodel.Migrator{}).Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := (projectmodel.Migrator{}).Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := (devicemodel.Migrator{}).Migrate(ctx, db); err != nil {
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

func setupServices(t *testing.T, db *gorm.DB) (*Service, *rbacsvc.ProjectService) {
	t.Helper()
	rbacR := rbacrepo.New(db)
	authR := authrepo.New(db)
	authz := rbacsvc.NewAuthorizer(rbacR)
	devR := devicerepo.New(db)
	svc := New(rbacR, authR, authz, devR)
	projectSvc := rbacsvc.NewProjectService(rbacR, authR, authz)
	return svc, projectSvc
}

func TestDeviceRegistration_DuplicateHardwareIdentifierRejected(t *testing.T) {
	db := setupDeviceTestDB(t)
	ownerID := seedUser(t, db, "owner@example.com")
	svc, projectSvc := setupServices(t, db)

	ctx := context.Background()
	proj, err := projectSvc.CreateProject(ctx, ownerID, "P", "")
	if err != nil {
		t.Fatal(err)
	}
	pre, err := svc.deviceRepo.ListPredefinedDeviceTypes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pre) == 0 {
		t.Fatal("expected predefined device types seeded")
	}

	d1, raw1, err := svc.RegisterDevice(ctx, ownerID, proj.ID, pre[0].ID, "Dev1", "AA:BB:CC:DD")
	if err != nil {
		t.Fatal(err)
	}
	if d1.ID == uuid.Nil || raw1 == "" {
		t.Fatal("expected device + raw token")
	}

	_, _, err = svc.RegisterDevice(ctx, ownerID, proj.ID, pre[0].ID, "Dev2", "aa:bb:cc:dd")
	if err == nil {
		t.Fatal("expected duplicate hardware id error")
	}
	if appErr, ok := err.(apperrors.AppError); ok {
		if appErr.Code != "hardware_id_in_use" {
			t.Fatalf("unexpected error code: got %s want %s", appErr.Code, "hardware_id_in_use")
		}
	} else {
		t.Fatalf("expected AppError, got %T", err)
	}
}

func TestDeviceBlock_UnblockAffectsPollReport(t *testing.T) {
	db := setupDeviceTestDB(t)
	ownerID := seedUser(t, db, "owner2@example.com")
	svc, projectSvc := setupServices(t, db)

	ctx := context.Background()
	proj, err := projectSvc.CreateProject(ctx, ownerID, "P2", "")
	if err != nil {
		t.Fatal(err)
	}
	pre, err := svc.deviceRepo.ListPredefinedDeviceTypes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	device, raw, err := svc.RegisterDevice(ctx, ownerID, proj.ID, pre[0].ID, "D", "SERIAL-1")
	if err != nil {
		t.Fatal(err)
	}
	_ = raw

	if err := svc.BlockDevice(ctx, ownerID, proj.ID, device.ID); err != nil {
		t.Fatal(err)
	}
	err = svc.DevicePoll(ctx, proj.ID, device.ID, "127.0.0.1", "ua", "/api/v1/device/poll")
	if err == nil {
		t.Fatal("expected poll to be rejected when blocked")
	}

	if err := svc.UnblockDevice(ctx, ownerID, proj.ID, device.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DevicePoll(ctx, proj.ID, device.ID, "127.0.0.1", "ua", "/api/v1/device/poll"); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceTokenRotation_RevokesOldTokenImmediately(t *testing.T) {
	db := setupDeviceTestDB(t)
	ownerID := seedUser(t, db, "owner3@example.com")
	svc, projectSvc := setupServices(t, db)

	ctx := context.Background()
	proj, err := projectSvc.CreateProject(ctx, ownerID, "P3", "")
	if err != nil {
		t.Fatal(err)
	}
	pre, err := svc.deviceRepo.ListPredefinedDeviceTypes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	device, raw1, err := svc.RegisterDevice(ctx, ownerID, proj.ID, pre[0].ID, "D1", "SERIAL-ROT")
	if err != nil {
		t.Fatal(err)
	}

	_, raw2, err := svc.RotateDeviceToken(ctx, ownerID, proj.ID, device.ID)
	if err != nil {
		t.Fatal(err)
	}

	oldHash := authsecurity.HashToken(raw1)
	newHash := authsecurity.HashToken(raw2)

	if _, _, err := svc.deviceRepo.GetDeviceByActiveTokenHash(ctx, oldHash); err == nil {
		t.Fatal("expected old token to be revoked")
	}
	if _, _, err := svc.deviceRepo.GetDeviceByActiveTokenHash(ctx, newHash); err != nil {
		t.Fatal("expected new token to be active:", err)
	}
}

func TestDeviceTwin_IncludesRecentConnectionLogsAndOnlineState(t *testing.T) {
	db := setupDeviceTestDB(t)
	ownerID := seedUser(t, db, "owner4@example.com")
	svc, projectSvc := setupServices(t, db)

	ctx := context.Background()
	proj, err := projectSvc.CreateProject(ctx, ownerID, "P4", "")
	if err != nil {
		t.Fatal(err)
	}
	pre, err := svc.deviceRepo.ListPredefinedDeviceTypes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	device, _, err := svc.RegisterDevice(ctx, ownerID, proj.ID, pre[0].ID, "D2", "SERIAL-LOGS")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.DeviceReport(ctx, proj.ID, device.ID, "1.2.3", "203.0.113.1", "ua", "/api/v1/device/report"); err != nil {
		t.Fatal(err)
	}

	twin, err := svc.GetDeviceTwin(ctx, ownerID, proj.ID, device.ID)
	if err != nil {
		t.Fatal(err)
	}
	if twin.LastSeenAt == nil {
		t.Fatal("expected last_seen_at to be set")
	}
	if twin.Online == false {
		t.Fatal("expected device to be online after report")
	}
	if len(twin.RecentConnectionLogs) == 0 {
		t.Fatal("expected recent connection logs")
	}
	foundReport := false
	for _, l := range twin.RecentConnectionLogs {
		if l.Action == "report" {
			foundReport = true
			break
		}
	}
	if !foundReport {
		t.Fatal("expected a report connection log")
	}

	// Force offline by setting last_seen_at beyond threshold.
	old := time.Now().UTC().Add(-10 * time.Minute)
	if err := svc.deviceRepo.UpdateDeviceSeenAndFirmware(ctx, proj.ID, device.ID, nil, old, "offline"); err != nil {
		t.Fatal(err)
	}
	twin2, err := svc.GetDeviceTwin(ctx, ownerID, proj.ID, device.ID)
	if err != nil {
		t.Fatal(err)
	}
	if twin2.Online {
		t.Fatal("expected device to be offline after threshold")
	}
}

