package service

import (
	"context"
	"strings"
	"time"

	apperrors "firmflow/internal/common/errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OtaDeviceReportStatus is a coarse device-reported OTA milestone.
type OtaDeviceReportStatus string

const (
	OtaReportDownloaded OtaDeviceReportStatus = "downloaded"
	OtaReportInstalled  OtaDeviceReportStatus = "installed"
	OtaReportFailed     OtaDeviceReportStatus = "failed"
)

// ApplyOtaDeviceReport updates campaign assignment progress from a device OTA report.
// It returns the campaign firmware version string when an install transition was applied (for callers that log device state).
func (s *Service) ApplyOtaDeviceReport(ctx context.Context, projectID, deviceID, campaignID uuid.UUID, st OtaDeviceReportStatus, errCode *uint16, errMsg string) (installedFirmwareVersion string, err error) {
	msg := strings.TrimSpace(errMsg)
	if len(msg) > 256 {
		msg = msg[:256]
	}

	if _, err := s.campaignRepo.GetDeviceAssignment(ctx, campaignID, deviceID); err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", apperrors.BadRequest("unknown campaign assignment", nil)
		}
		return "", err
	}

	var rows int64
	switch st {
	case OtaReportDownloaded:
		rows, err = s.campaignRepo.ApplyOtaDownloaded(ctx, projectID, campaignID, deviceID)
	case OtaReportInstalled:
		rows, err = s.campaignRepo.ApplyOtaInstalled(ctx, projectID, campaignID, deviceID)
		if err != nil {
			return "", err
		}
		if rows > 0 {
			camp, err2 := s.campaignRepo.GetCampaign(ctx, projectID, campaignID)
			if err2 != nil {
				return "", err2
			}
			fw, err2 := s.firmwareRepo.GetFirmware(ctx, projectID, camp.FirmwareID)
			if err2 != nil {
				if err2 == gorm.ErrRecordNotFound {
					return "", apperrors.NotFound("firmware not found")
				}
				return "", err2
			}
			ver := strings.TrimSpace(fw.Version)
			installedFirmwareVersion = ver
			now := time.Now().UTC()
			if err2 := s.deviceRepo.UpdateDeviceSeenAndFirmware(ctx, projectID, deviceID, &ver, now, "online"); err2 != nil {
				return "", err2
			}
		}
	case OtaReportFailed:
		rows, err = s.campaignRepo.ApplyOtaFailed(ctx, projectID, campaignID, deviceID, errCode, msg)
	default:
		return "", apperrors.BadRequest("invalid ota report status", nil)
	}
	if err != nil {
		return "", err
	}
	_ = rows
	_ = s.tryAutoComplete(ctx, projectID, campaignID)
	return installedFirmwareVersion, nil
}
