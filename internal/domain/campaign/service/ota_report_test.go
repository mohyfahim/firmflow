package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	campaignmodel "firmflow/internal/domain/campaign/model"
	devicerepo "firmflow/internal/domain/device/repository"
	firmwaremodel "firmflow/internal/domain/firmware/model"
	firmwarerepo "firmflow/internal/domain/firmware/repository"
	projectmodel "firmflow/internal/domain/project/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func seedOtaCampaign(t *testing.T, db *gorm.DB) (*Service, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, string) {
	t.Helper()
	owner := seedUserCamp(t, db, t.Name()+"-owner-ota@example.com")
	svc, ps := newCampaignSvc(t, db)
	ctx := context.Background()
	proj, err := ps.CreateProject(ctx, owner, t.Name()+" OtaProj", "")
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
	sum := strings.Repeat("a", 64)
	sk := fmt.Sprintf("kz-%s", uuid.NewString())
	ver := fmt.Sprintf("9.9.%s", uuid.NewString()[:8])
	fw := &firmwaremodel.Firmware{
		ProjectID: proj.ID, Version: ver, VersionNormalized: ver,
		FileSizeBytes: 1, ChecksumSHA256: sum, OriginalFilename: "z.bin",
		StorageProvider: "local", StorageKey: sk, UploadedByUserID: owner,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := fwR.CreateFirmwareWithTypes(ctx, fw, []uuid.UUID{pre[0].ID}); err != nil {
		t.Fatal(err)
	}
	d := &projectmodel.Device{
		ProjectID: proj.ID, Name: "OtaDev", DeviceTypeID: pre[0].ID,
		HardwareIdentifier: fmt.Sprintf("hw-%s-%s", t.Name(), uuid.NewString()[:8]), CurrentFirmwareVersion: "0.1.0",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.WithContext(ctx).Create(d).Error; err != nil {
		t.Fatal(err)
	}
	detail, err := svc.CreateCampaign(ctx, owner, proj.ID, CreateCampaignInput{
		Name:              t.Name() + " Ota",
		FirmwareID:        fw.ID,
		RolloutKind:       campaignmodel.RolloutKindImmediate,
		ExplicitDeviceIDs: []uuid.UUID{d.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, proj.ID, d.ID, detail.ID, fw.ID, ver
}

func TestBuildPollOffer_RepeatedWhileOffered(t *testing.T) {
	db := setupCampaignTestDB(t)
	svc, projID, devID, _, _, wantVer := seedOtaCampaign(t, db)
	ctx := context.Background()

	offer, err := svc.BuildPollOffer(ctx, projID, devID)
	if err != nil {
		t.Fatal(err)
	}
	if offer == nil || offer.Version != wantVer {
		t.Fatalf("offer %+v want %q", offer, wantVer)
	}
	offer2, err := svc.BuildPollOffer(ctx, projID, devID)
	if err != nil {
		t.Fatal(err)
	}
	if offer2 == nil || offer2.Version != wantVer {
		t.Fatalf("re-offer missing %+v want %q", offer2, wantVer)
	}
}

func TestApplyOtaDeviceReport_DownloadedInstalledFailedRepeated(t *testing.T) {
	db := setupCampaignTestDB(t)
	svc, projID, devID, campID, _, wantVer := seedOtaCampaign(t, db)
	ctx := context.Background()

	_, err := svc.BuildPollOffer(ctx, projID, devID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.ApplyOtaDeviceReport(ctx, projID, devID, campID, OtaReportDownloaded, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	var st string
	if err := db.Raw(`SELECT status FROM campaign_device_assignments WHERE campaign_id = ? AND device_id = ?`, campID, devID).Scan(&st).Error; err != nil {
		t.Fatal(err)
	}
	if st != campaignmodel.AssignmentDownloaded {
		t.Fatalf("want downloaded got %s", st)
	}

	_, err = svc.ApplyOtaDeviceReport(ctx, projID, devID, campID, OtaReportDownloaded, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	ver, err := svc.ApplyOtaDeviceReport(ctx, projID, devID, campID, OtaReportInstalled, nil, "")
	if err != nil || ver != wantVer {
		t.Fatalf("installed err=%v ver=%q want %q", err, ver, wantVer)
	}
	var d projectmodel.Device
	if err := db.First(&d, "id = ?", devID).Error; err != nil {
		t.Fatal(err)
	}
	if d.CurrentFirmwareVersion != wantVer {
		t.Fatalf("device fw %q want %q", d.CurrentFirmwareVersion, wantVer)
	}

	_, err = svc.ApplyOtaDeviceReport(ctx, projID, devID, campID, OtaReportInstalled, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	code := uint16(7)
	_, err = svc.ApplyOtaDeviceReport(ctx, projID, devID, campID, OtaReportFailed, &code, "boom")
	if err != nil {
		t.Fatal(err)
	}
	var st2 string
	if err := db.Raw(`SELECT status FROM campaign_device_assignments WHERE campaign_id = ? AND device_id = ?`, campID, devID).Scan(&st2).Error; err != nil {
		t.Fatal(err)
	}
	if st2 != campaignmodel.AssignmentInstalled {
		t.Fatalf("terminal install should not regress to failed; got %s", st2)
	}
}
