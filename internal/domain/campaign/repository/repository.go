package repository

import (
	"context"
	"time"

	campaignmodel "firmflow/internal/domain/campaign/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) DB() *gorm.DB { return r.db }

func (r *Repository) CreateCampaignBundle(ctx context.Context, c *campaignmodel.Campaign, groupIDs, deviceIDs []uuid.UUID, assignmentDeviceIDs []uuid.UUID) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(c).Error; err != nil {
			return err
		}
		for _, gid := range groupIDs {
			row := campaignmodel.CampaignTargetGroup{
				CampaignID:    c.ID,
				DeviceGroupID: gid,
				CreatedAt:     now,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		for _, did := range deviceIDs {
			row := campaignmodel.CampaignTargetDevice{
				CampaignID: c.ID,
				DeviceID:   did,
				CreatedAt:  now,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		const batch = 200
		for i := 0; i < len(assignmentDeviceIDs); i += batch {
			j := i + batch
			if j > len(assignmentDeviceIDs) {
				j = len(assignmentDeviceIDs)
			}
			chunk := assignmentDeviceIDs[i:j]
			rows := make([]campaignmodel.CampaignDeviceAssignment, 0, len(chunk))
			for _, did := range chunk {
				rows = append(rows, campaignmodel.CampaignDeviceAssignment{
					CampaignID: c.ID,
					DeviceID:   did,
					Status:     campaignmodel.AssignmentPending,
					CreatedAt:  now,
					UpdatedAt:  now,
				})
			}
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) GetCampaign(ctx context.Context, projectID, campaignID uuid.UUID) (*campaignmodel.Campaign, error) {
	var c campaignmodel.Campaign
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND id = ?", projectID, campaignID).
		First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) ListCampaigns(ctx context.Context, projectID uuid.UUID, page, pageSize int) ([]campaignmodel.Campaign, int64, error) {
	var total int64
	q := r.db.WithContext(ctx).Model(&campaignmodel.Campaign{}).Where("project_id = ?", projectID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	var list []campaignmodel.Campaign
	err := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("created_at DESC, id DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&list).Error
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// TransitionCampaignStatus updates status when current matches expectFrom (optimistic lock).
func (r *Repository) TransitionCampaignStatus(ctx context.Context, projectID, campaignID uuid.UUID, expectFrom, to string, patch map[string]interface{}) error {
	patch["updated_at"] = time.Now().UTC()
	patch["status"] = to
	res := r.db.WithContext(ctx).Model(&campaignmodel.Campaign{}).
		Where("project_id = ? AND id = ? AND status = ?", projectID, campaignID, expectFrom).
		Updates(patch)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) CountAssignmentsByStatus(ctx context.Context, campaignID uuid.UUID) (map[string]int64, error) {
	type row struct {
		Status string
		Cnt    int64
	}
	var rows []row
	err := r.db.WithContext(ctx).Model(&campaignmodel.CampaignDeviceAssignment{}).
		Select("status, count(*) as cnt").
		Where("campaign_id = ?", campaignID).
		Group("status").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64)
	for _, rw := range rows {
		out[rw.Status] = rw.Cnt
	}
	return out, nil
}

func (r *Repository) CountAssignments(ctx context.Context, campaignID uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&campaignmodel.CampaignDeviceAssignment{}).
		Where("campaign_id = ?", campaignID).
		Count(&n).Error
	return n, err
}

func (r *Repository) UpdateAssignmentStatus(ctx context.Context, campaignID, deviceID uuid.UUID, expectFrom, to string) error {
	res := r.db.WithContext(ctx).Model(&campaignmodel.CampaignDeviceAssignment{}).
		Where("campaign_id = ? AND device_id = ? AND status = ?", campaignID, deviceID, expectFrom).
		Updates(map[string]interface{}{
			"status":     to,
			"updated_at": time.Now().UTC(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) SetAssignmentStatus(ctx context.Context, campaignID, deviceID uuid.UUID, to string) error {
	res := r.db.WithContext(ctx).Model(&campaignmodel.CampaignDeviceAssignment{}).
		Where("campaign_id = ? AND device_id = ?", campaignID, deviceID).
		Updates(map[string]interface{}{
			"status":     to,
			"updated_at": time.Now().UTC(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// OfferRow is an active OTA offer for a device (FIFO by campaign creation).
type OfferRow struct {
	CampaignID       uuid.UUID
	FirmwareID       uuid.UUID
	FirmwareVersion  string
	ChecksumSHA256   string
	AssignmentStatus string
}

func (r *Repository) FindActivePendingOffer(ctx context.Context, projectID, deviceID uuid.UUID) (*OfferRow, error) {
	var row OfferRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT c.id AS campaign_id, f.id AS firmware_id, f.version AS firmware_version, f.checksum_sha256 AS checksum_sha256,
		       a.status AS assignment_status
		FROM campaign_device_assignments AS a
		INNER JOIN campaigns AS c ON c.id = a.campaign_id AND c.deleted_at IS NULL
		INNER JOIN firmwares AS f ON f.id = c.firmware_id AND f.deleted_at IS NULL
		WHERE c.project_id = ? AND a.device_id = ?
		  AND c.status = ? AND a.status IN (?, ?)
		ORDER BY CASE WHEN a.status = ? THEN 0 ELSE 1 END, c.created_at ASC, c.id ASC
		LIMIT 1
	`, projectID, deviceID, campaignmodel.StatusActive,
		campaignmodel.AssignmentPending, campaignmodel.AssignmentOffered,
		campaignmodel.AssignmentPending).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.CampaignID == uuid.Nil {
		return nil, nil
	}
	return &row, nil
}

func (r *Repository) ActivateDueScheduledCampaigns(ctx context.Context, now time.Time) ([]uuid.UUID, error) {
	var activated []uuid.UUID
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []uuid.UUID
		if err := tx.Model(&campaignmodel.Campaign{}).
			Where("status = ? AND scheduled_start_at IS NOT NULL AND scheduled_start_at <= ? AND deleted_at IS NULL",
				campaignmodel.StatusScheduled, now).
			Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		res := tx.Model(&campaignmodel.Campaign{}).
			Where("id IN ? AND status = ?", ids, campaignmodel.StatusScheduled).
			Updates(map[string]interface{}{
				"status":       campaignmodel.StatusActive,
				"activated_at": now,
				"updated_at":   now,
			})
		if res.Error != nil {
			return res.Error
		}
		activated = ids
		return nil
	})
	return activated, err
}

func (r *Repository) CountNonTerminalAssignments(ctx context.Context, campaignID uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&campaignmodel.CampaignDeviceAssignment{}).
		Where("campaign_id = ? AND status NOT IN ?", campaignID, []string{
			campaignmodel.AssignmentInstalled,
			campaignmodel.AssignmentFailed,
		}).
		Count(&n).Error
	return n, err
}

func (r *Repository) MarkCampaignCompleted(ctx context.Context, projectID, campaignID uuid.UUID) error {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).Model(&campaignmodel.Campaign{}).
		Where("project_id = ? AND id = ? AND status = ?", projectID, campaignID, campaignmodel.StatusActive).
		Updates(map[string]interface{}{
			"status":       campaignmodel.StatusCompleted,
			"completed_at": now,
			"updated_at":   now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) PauseCampaign(ctx context.Context, projectID, campaignID uuid.UUID) error {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).Model(&campaignmodel.Campaign{}).
		Where("project_id = ? AND id = ? AND status = ?", projectID, campaignID, campaignmodel.StatusActive).
		Updates(map[string]interface{}{
			"status":     campaignmodel.StatusPaused,
			"paused_at":  now,
			"updated_at": now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) ResumeCampaign(ctx context.Context, projectID, campaignID uuid.UUID) error {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).Model(&campaignmodel.Campaign{}).
		Where("project_id = ? AND id = ? AND status = ?", projectID, campaignID, campaignmodel.StatusPaused).
		Updates(map[string]interface{}{
			"status":     campaignmodel.StatusActive,
			"paused_at":  gorm.Expr("NULL"),
			"updated_at": now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) CancelCampaign(ctx context.Context, projectID, campaignID uuid.UUID) error {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).Model(&campaignmodel.Campaign{}).
		Where("project_id = ? AND id = ? AND status IN ?", projectID, campaignID, []string{
			campaignmodel.StatusDraft,
			campaignmodel.StatusScheduled,
			campaignmodel.StatusActive,
			campaignmodel.StatusPaused,
		}).
		Updates(map[string]interface{}{
			"status":       campaignmodel.StatusCancelled,
			"cancelled_at": now,
			"updated_at":   now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
