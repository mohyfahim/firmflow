package service

import (
	"context"
	"testing"

	devicerepo "firmflow/internal/domain/device/repository"

	"github.com/google/uuid"
)

func TestListDevices_FilterBlockedAndGroup(t *testing.T) {
	db := setupDeviceTestDB(t)
	ownerID := seedUser(t, db, "filterowner@x.com")
	svc, projectSvc := setupServices(t, db)
	ctx := context.Background()

	proj, err := projectSvc.CreateProject(ctx, ownerID, "FProj", "")
	if err != nil {
		t.Fatal(err)
	}
	pre, err := svc.deviceRepo.ListPredefinedDeviceTypes(ctx)
	if err != nil || len(pre) == 0 {
		t.Fatal("need predefined device types")
	}

	d1, _, err := svc.RegisterDevice(ctx, ownerID, proj.ID, pre[0].ID, "A", "hw-a")
	if err != nil {
		t.Fatal(err)
	}
	d2, _, err := svc.RegisterDevice(ctx, ownerID, proj.ID, pre[0].ID, "B", "hw-b")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.BlockDevice(ctx, ownerID, proj.ID, d1.ID); err != nil {
		t.Fatal(err)
	}

	g, err := svc.CreateDeviceGroup(ctx, ownerID, proj.ID, "G1", "desc")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.AddDevicesToGroup(ctx, ownerID, proj.ID, g.ID, []uuid.UUID{d2.ID}); err != nil {
		t.Fatal(err)
	}

	b := true
	items, total, err := svc.ListDevices(ctx, ownerID, proj.ID, devicerepo.DeviceListFilter{Blocked: &b}, 1, 20, "name")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("blocked filter: total=%d len=%d", total, len(items))
	}
	if items[0].ID != d1.ID {
		t.Fatalf("expected blocked device A")
	}

	items2, total2, err := svc.ListDevices(ctx, ownerID, proj.ID, devicerepo.DeviceListFilter{GroupID: &g.ID}, 1, 20, "name")
	if err != nil {
		t.Fatal(err)
	}
	if total2 != 1 || len(items2) != 1 || items2[0].ID != d2.ID {
		t.Fatalf("group filter: total=%d want device B", total2)
	}
}

func TestAddDevicesToGroup_Idempotent(t *testing.T) {
	db := setupDeviceTestDB(t)
	ownerID := seedUser(t, db, "idem@x.com")
	svc, projectSvc := setupServices(t, db)
	ctx := context.Background()
	proj, err := projectSvc.CreateProject(ctx, ownerID, "Idem", "")
	if err != nil {
		t.Fatal(err)
	}
	pre, _ := svc.deviceRepo.ListPredefinedDeviceTypes(ctx)
	d, _, err := svc.RegisterDevice(ctx, ownerID, proj.ID, pre[0].ID, "D", "hw-idem")
	if err != nil {
		t.Fatal(err)
	}
	g, err := svc.CreateDeviceGroup(ctx, ownerID, proj.ID, "G2", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.AddDevicesToGroup(ctx, ownerID, proj.ID, g.ID, []uuid.UUID{d.ID}); err != nil {
		t.Fatal(err)
	}
	if err := svc.AddDevicesToGroup(ctx, ownerID, proj.ID, g.ID, []uuid.UUID{d.ID}); err != nil {
		t.Fatal("second add should be idempotent:", err)
	}
	cnt, err := svc.deviceRepo.CountGroupMembers(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cnt != 1 {
		t.Fatalf("membership rows: got %d want 1", cnt)
	}
}

func TestBulkAddToGroup_PartialFailures(t *testing.T) {
	db := setupDeviceTestDB(t)
	ownerID := seedUser(t, db, "bulk@x.com")
	svc, projectSvc := setupServices(t, db)
	ctx := context.Background()
	proj, err := projectSvc.CreateProject(ctx, ownerID, "BulkP", "")
	if err != nil {
		t.Fatal(err)
	}
	pre, _ := svc.deviceRepo.ListPredefinedDeviceTypes(ctx)
	d, _, err := svc.RegisterDevice(ctx, ownerID, proj.ID, pre[0].ID, "D", "hw-bulk")
	if err != nil {
		t.Fatal(err)
	}
	g, err := svc.CreateDeviceGroup(ctx, ownerID, proj.ID, "G3", "")
	if err != nil {
		t.Fatal(err)
	}
	badID := uuid.New()
	ids := []string{d.ID.String(), badID.String()}
	res, err := svc.BulkDevices(ctx, ownerID, proj.ID, "add_to_group", false, devicerepo.DeviceListFilter{}, ids, &g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Succeeded != 1 || len(res.Failed) != 1 {
		t.Fatalf("bulk: succeeded=%d failed=%d", res.Succeeded, len(res.Failed))
	}
}

func TestDuplicateGroupName(t *testing.T) {
	db := setupDeviceTestDB(t)
	ownerID := seedUser(t, db, "dupg@x.com")
	svc, projectSvc := setupServices(t, db)
	ctx := context.Background()
	proj, err := projectSvc.CreateProject(ctx, ownerID, "DupG", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateDeviceGroup(ctx, ownerID, proj.ID, "Same", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateDeviceGroup(ctx, ownerID, proj.ID, "  same  ", ""); err == nil {
		t.Fatal("expected duplicate group name")
	}
}

func TestDeleteDeviceGroup_ClearsMemberships(t *testing.T) {
	db := setupDeviceTestDB(t)
	ownerID := seedUser(t, db, "delg@x.com")
	svc, projectSvc := setupServices(t, db)
	ctx := context.Background()
	proj, err := projectSvc.CreateProject(ctx, ownerID, "DelG", "")
	if err != nil {
		t.Fatal(err)
	}
	pre, _ := svc.deviceRepo.ListPredefinedDeviceTypes(ctx)
	d, _, err := svc.RegisterDevice(ctx, ownerID, proj.ID, pre[0].ID, "D", "hw-delg")
	if err != nil {
		t.Fatal(err)
	}
	g, err := svc.CreateDeviceGroup(ctx, ownerID, proj.ID, "ToDelete", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = svc.AddDevicesToGroup(ctx, ownerID, proj.ID, g.ID, []uuid.UUID{d.ID})
	if err := svc.DeleteDeviceGroup(ctx, ownerID, proj.ID, g.ID); err != nil {
		t.Fatal(err)
	}
	cnt, err := svc.deviceRepo.CountGroupMembers(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cnt != 0 {
		t.Fatalf("expected memberships cleared, count=%d", cnt)
	}
}
