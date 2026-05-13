package repository

import (
	"context"
	"time"

	campaignmodel "firmflow/internal/domain/campaign/model"

	"github.com/google/uuid"
)

func (r *Repository) GetDeviceAssignment(ctx context.Context, campaignID, deviceID uuid.UUID) (*campaignmodel.CampaignDeviceAssignment, error) {
	var a campaignmodel.CampaignDeviceAssignment
	err := r.db.WithContext(ctx).
		Where("campaign_id = ? AND device_id = ?", campaignID, deviceID).
		First(&a).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *Repository) isCampaignActiveInProject(ctx context.Context, projectID, campaignID uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&campaignmodel.Campaign{}).
		Where("id = ? AND project_id = ? AND status = ? AND deleted_at IS NULL", campaignID, projectID, campaignmodel.StatusActive).
		Count(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *Repository) applyAssignmentStatus(ctx context.Context, projectID, campaignID, deviceID uuid.UUID, fromStatuses []string, to string, reportCode *uint16, reportMsg string) (int64, error) {
	ok, err := r.isCampaignActiveInProject(ctx, projectID, campaignID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, nil
	}

	now := time.Now().UTC()
	updates := map[string]interface{}{
		"status":     to,
		"updated_at": now,
	}
	if reportCode != nil {
		updates["last_report_code"] = *reportCode
	} else {
		updates["last_report_code"] = nil
	}
	updates["last_report_msg"] = reportMsg

	res := r.db.WithContext(ctx).Model(&campaignmodel.CampaignDeviceAssignment{}).
		Where("campaign_id = ? AND device_id = ? AND status IN ?", campaignID, deviceID, fromStatuses).
		Updates(updates)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

func (r *Repository) ApplyOtaDownloaded(ctx context.Context, projectID, campaignID, deviceID uuid.UUID) (int64, error) {
	return r.applyAssignmentStatus(ctx, projectID, campaignID, deviceID, []string{
		campaignmodel.AssignmentOffered,
		campaignmodel.AssignmentDownloaded,
	}, campaignmodel.AssignmentDownloaded, nil, "")
}

func (r *Repository) ApplyOtaInstalled(ctx context.Context, projectID, campaignID, deviceID uuid.UUID) (int64, error) {
	return r.applyAssignmentStatus(ctx, projectID, campaignID, deviceID, []string{
		campaignmodel.AssignmentOffered,
		campaignmodel.AssignmentDownloaded,
		campaignmodel.AssignmentInstalled,
	}, campaignmodel.AssignmentInstalled, nil, "")
}

func (r *Repository) ApplyOtaFailed(ctx context.Context, projectID, campaignID, deviceID uuid.UUID, reportCode *uint16, reportMsg string) (int64, error) {
	return r.applyAssignmentStatus(ctx, projectID, campaignID, deviceID, []string{
		campaignmodel.AssignmentPending,
		campaignmodel.AssignmentOffered,
		campaignmodel.AssignmentDownloaded,
		campaignmodel.AssignmentFailed,
	}, campaignmodel.AssignmentFailed, reportCode, reportMsg)
}
