package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	apperrors "firmflow/internal/common/errors"
	authmodel "firmflow/internal/domain/auth/model"
	authrepo "firmflow/internal/domain/auth/repository"
	devicemodel "firmflow/internal/domain/device/model"
	devicerepo "firmflow/internal/domain/device/repository"
	firmwaremodel "firmflow/internal/domain/firmware/model"
	firmwarerepo "firmflow/internal/domain/firmware/repository"
	projectmodel "firmflow/internal/domain/project/model"
	rbacmodel "firmflow/internal/domain/rbac/model"
	rbacrepo "firmflow/internal/domain/rbac/repository"
	rbacsvc "firmflow/internal/domain/rbac/service"
	"firmflow/internal/platform/storage"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupFirmwareTestDB(t *testing.T) *gorm.DB {
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
	} {
		if err := m.Migrate(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func seedUserFirmware(t *testing.T, db *gorm.DB, email string) uuid.UUID {
	t.Helper()
	repo := authrepo.New(db)
	u := &authmodel.User{Email: email, PasswordHash: "x"}
	p := &authmodel.UserProfile{Timezone: "UTC", PreferredLanguage: "en"}
	if err := repo.CreateUserWithProfile(context.Background(), u, p); err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func newFirmwareTestService(t *testing.T, db *gorm.DB) (*Service, *rbacsvc.ProjectService, string) {
	t.Helper()
	storeDir := t.TempDir()
	obj, err := storage.NewLocalObjectStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	rbacR := rbacrepo.New(db)
	authR := authrepo.New(db)
	authz := rbacsvc.NewAuthorizer(rbacR)
	devR := devicerepo.New(db)
	fwR := firmwarerepo.New(db)
	svc := New(rbacR, authR, authz, fwR, devR, obj, 1<<20, "local")
	projectSvc := rbacsvc.NewProjectService(rbacR, authR, authz)
	return svc, projectSvc, storeDir
}

func TestUploadFirmware_ChecksumAndSize(t *testing.T) {
	db := setupFirmwareTestDB(t)
	owner := seedUserFirmware(t, db, "owner-fw@example.com")
	svc, projectSvc, _ := newFirmwareTestService(t, db)
	ctx := context.Background()

	proj, err := projectSvc.CreateProject(ctx, owner, "FW", "")
	if err != nil {
		t.Fatal(err)
	}
	pre, err := svc.deviceRepo.ListPredefinedDeviceTypes(ctx)
	if err != nil || len(pre) == 0 {
		t.Fatal("need predefined device types")
	}
	payload := []byte("hello-firmware")
	h := sha256.Sum256(payload)
	wantSum := hex.EncodeToString(h[:])

	detail, err := svc.UploadFirmware(ctx, owner, proj.ID, "1.0.0", "notes", "app.bin", []uuid.UUID{pre[0].ID}, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if detail.ChecksumSHA256 != wantSum {
		t.Fatalf("checksum: got %s want %s", detail.ChecksumSHA256, wantSum)
	}
	if detail.FileSizeBytes != int64(len(payload)) {
		t.Fatalf("size: got %d want %d", detail.FileSizeBytes, len(payload))
	}
	if !detail.IsSemver {
		t.Fatal("expected semver")
	}
}

func TestUploadFirmware_DuplicateVersionRejected(t *testing.T) {
	db := setupFirmwareTestDB(t)
	owner := seedUserFirmware(t, db, "owner-dup@example.com")
	svc, projectSvc, _ := newFirmwareTestService(t, db)
	ctx := context.Background()
	proj, err := projectSvc.CreateProject(ctx, owner, "P", "")
	if err != nil {
		t.Fatal(err)
	}
	pre, err := svc.deviceRepo.ListPredefinedDeviceTypes(ctx)
	if err != nil || len(pre) == 0 {
		t.Fatal(err)
	}
	body := bytes.NewReader([]byte("a"))
	_, err = svc.UploadFirmware(ctx, owner, proj.ID, "1.0.0", "", "x.bin", []uuid.UUID{pre[0].ID}, body)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.UploadFirmware(ctx, owner, proj.ID, "  1.0.0  ", "", "y.bin", []uuid.UUID{pre[0].ID}, bytes.NewReader([]byte("b")))
	if err == nil {
		t.Fatal("expected duplicate version error")
	}
	appErr, ok := err.(apperrors.AppError)
	if !ok || appErr.Code != "firmware_version_exists" {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestUploadFirmware_NonSemverStored(t *testing.T) {
	db := setupFirmwareTestDB(t)
	owner := seedUserFirmware(t, db, "owner-ns@example.com")
	svc, projectSvc, _ := newFirmwareTestService(t, db)
	ctx := context.Background()
	proj, err := projectSvc.CreateProject(ctx, owner, "P", "")
	if err != nil {
		t.Fatal(err)
	}
	pre, _ := svc.deviceRepo.ListPredefinedDeviceTypes(ctx)
	detail, err := svc.UploadFirmware(ctx, owner, proj.ID, "custom-REL", "", "x.bin", []uuid.UUID{pre[0].ID}, bytes.NewReader([]byte("z")))
	if err != nil {
		t.Fatal(err)
	}
	if detail.Version != "custom-REL" {
		t.Fatalf("version: %s", detail.Version)
	}
	if detail.IsSemver {
		t.Fatal("expected non-semver")
	}
}
