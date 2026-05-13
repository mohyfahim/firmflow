package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	apperrors "firmflow/internal/common/errors"
	authmodel "firmflow/internal/domain/auth/model"
	authrepo "firmflow/internal/domain/auth/repository"
	campaignmodel "firmflow/internal/domain/campaign/model"
	campaignrepo "firmflow/internal/domain/campaign/repository"
	devicerepo "firmflow/internal/domain/device/repository"
	firmwarerepo "firmflow/internal/domain/firmware/repository"
	rbacperm "firmflow/internal/domain/rbac/permission"
	rbacrepo "firmflow/internal/domain/rbac/repository"
	rbacsvc "firmflow/internal/domain/rbac/service"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Service struct {
	rbacRepo     *rbacrepo.Repository
	authRepo     *authrepo.Repository
	rbacAuth     *rbacsvc.Authorizer
	campaignRepo *campaignrepo.Repository
	firmwareRepo *firmwarerepo.Repository
	deviceRepo   *devicerepo.Repository
}

func New(
	rbacRepo *rbacrepo.Repository,
	authRepo *authrepo.Repository,
	rbacAuth *rbacsvc.Authorizer,
	campaignRepo *campaignrepo.Repository,
	firmwareRepo *firmwarerepo.Repository,
	deviceRepo *devicerepo.Repository,
) *Service {
	return &Service{
		rbacRepo:     rbacRepo,
		authRepo:     authRepo,
		rbacAuth:     rbacAuth,
		campaignRepo: campaignRepo,
		firmwareRepo: firmwareRepo,
		deviceRepo:   deviceRepo,
	}
}

func (s *Service) audit(ctx context.Context, actorUserID uuid.UUID, event, projectID string, metadata map[string]interface{}) error {
	b, _ := json.Marshal(metadata)
	return s.authRepo.AppendAuditLog(ctx, &authmodel.AuditLog{
		ActorUserID: &actorUserID,
		Event:       event,
		TargetType:  "project",
		TargetID:    projectID,
		Metadata:    b,
	})
}

func (s *Service) ensureProjectNotArchived(ctx context.Context, projectID uuid.UUID) error {
	p, err := s.rbacRepo.GetProject(ctx, projectID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFound("project not found")
		}
		return err
	}
	if p.ArchivedAt != nil {
		return apperrors.New("project_archived", "project is archived", 409, nil)
	}
	return nil
}

// CreateCampaignInput defines a new rollout.
type CreateCampaignInput struct {
	Name              string
	FirmwareID        uuid.UUID
	RolloutKind       string
	RolloutPercent    *int
	ScheduledStartAt  *time.Time
	DeviceGroupIDs    []uuid.UUID
	ExplicitDeviceIDs []uuid.UUID
}

type CampaignView struct {
	ID                uuid.UUID  `json:"id"`
	Name              string     `json:"name"`
	FirmwareID        uuid.UUID  `json:"firmware_id"`
	RolloutKind       string     `json:"rollout_kind"`
	RolloutPercent    *int       `json:"rollout_percent,omitempty"`
	ScheduledStartAt  *time.Time `json:"scheduled_start_at,omitempty"`
	Status            string     `json:"status"`
	TargetDeviceCount int64      `json:"target_device_count"`
	ActivatedAt       *time.Time `json:"activated_at,omitempty"`
	PausedAt          *time.Time `json:"paused_at,omitempty"`
	CancelledAt       *time.Time `json:"cancelled_at,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	CreatedByUserID   uuid.UUID  `json:"created_by_user_id"`
	CreatedAt         time.Time  `json:"created_at"`
}

type ProgressView struct {
	TargetCount       int64   `json:"target_count"`
	Pending           int64   `json:"pending"`
	Offered           int64   `json:"offered"`
	Downloaded        int64   `json:"downloaded"`
	Installed         int64   `json:"installed"`
	Failed            int64   `json:"failed"`
	CompletionPercent float64 `json:"completion_percent"`
	SuccessPercent    float64 `json:"success_percent"`
}

type CampaignDetailView struct {
	CampaignView
	Progress ProgressView `json:"progress"`
}

type PollOfferView struct {
	CampaignID     uuid.UUID `json:"campaign_id"`
	FirmwareID     uuid.UUID `json:"firmware_id"`
	Version        string    `json:"version"`
	ChecksumSHA256 string    `json:"checksum_sha256"`
}

func (s *Service) CreateCampaign(ctx context.Context, actorUserID, projectID uuid.UUID, in CreateCampaignInput) (*CampaignDetailView, error) {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.CampaignCreate); err != nil {
		return nil, err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, apperrors.BadRequest("name is required", nil)
	}
	if in.FirmwareID == uuid.Nil {
		return nil, apperrors.BadRequest("firmware_id is required", nil)
	}
	if len(in.DeviceGroupIDs) == 0 && len(in.ExplicitDeviceIDs) == 0 {
		return nil, apperrors.BadRequest("at least one device_group_id or device_id is required", nil)
	}

	switch in.RolloutKind {
	case campaignmodel.RolloutKindImmediate, campaignmodel.RolloutKindPercentage:
		if in.RolloutKind == campaignmodel.RolloutKindPercentage && (in.RolloutPercent == nil || *in.RolloutPercent < 1 || *in.RolloutPercent > 100) {
			return nil, apperrors.BadRequest("rollout_percent must be between 1 and 100 for percentage rollout", nil)
		}
	case campaignmodel.RolloutKindTimeScheduled:
		if in.ScheduledStartAt == nil {
			return nil, apperrors.BadRequest("scheduled_start_at is required for time_scheduled rollout", nil)
		}
	default:
		return nil, apperrors.BadRequest("invalid rollout_kind", nil)
	}

	for _, gid := range in.DeviceGroupIDs {
		ok, err := s.deviceRepo.DeviceGroupExistsInProject(ctx, projectID, gid)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, apperrors.BadRequest("unknown device_group_id", map[string]string{"device_group_id": gid.String()})
		}
	}

	fw, err := s.firmwareRepo.GetFirmware(ctx, projectID, in.FirmwareID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("firmware not found")
		}
		return nil, err
	}
	compatTypes, err := s.firmwareRepo.ListDeviceTypeIDsForFirmware(ctx, fw.ID)
	if err != nil {
		return nil, err
	}
	allowed := make(map[uuid.UUID]struct{}, len(compatTypes))
	for _, t := range compatTypes {
		allowed[t] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil, apperrors.BadRequest("firmware has no compatible device types", nil)
	}

	fromGroups, err := s.deviceRepo.ListDeviceIDsInGroupsForProject(ctx, projectID, in.DeviceGroupIDs)
	if err != nil {
		return nil, err
	}
	fromExplicit, err := s.deviceRepo.ListDeviceIDsExplicitInProject(ctx, projectID, in.ExplicitDeviceIDs)
	if err != nil {
		return nil, err
	}
	union := make(map[uuid.UUID]struct{})
	for _, id := range fromGroups {
		union[id] = struct{}{}
	}
	for _, id := range fromExplicit {
		union[id] = struct{}{}
	}
	allIDs := make([]uuid.UUID, 0, len(union))
	for id := range union {
		allIDs = append(allIDs, id)
	}
	compatible, err := s.deviceRepo.FilterDeviceIDsByCompatibleTypes(ctx, projectID, allIDs, allowed)
	if err != nil {
		return nil, err
	}
	compatible = sortUUIDs(compatible)
	if len(compatible) == 0 {
		return nil, apperrors.BadRequest("no compatible devices for this firmware in the selected scope", nil)
	}

	assignIDs := compatible
	if in.RolloutKind == campaignmodel.RolloutKindPercentage {
		assignIDs = takeStablePercentage(compatible, *in.RolloutPercent)
	}

	now := time.Now().UTC()
	status := campaignmodel.StatusActive
	if in.ScheduledStartAt != nil && in.ScheduledStartAt.After(now) {
		status = campaignmodel.StatusScheduled
	}

	c := &campaignmodel.Campaign{
		ProjectID:         projectID,
		Name:              name,
		FirmwareID:        fw.ID,
		RolloutKind:       in.RolloutKind,
		RolloutPercent:    in.RolloutPercent,
		ScheduledStartAt:  in.ScheduledStartAt,
		Status:            status,
		TargetDeviceCount: int64(len(assignIDs)),
		CreatedByUserID:   actorUserID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if status == campaignmodel.StatusActive {
		c.ActivatedAt = &now
	}

	if err := s.campaignRepo.CreateCampaignBundle(ctx, c, in.DeviceGroupIDs, in.ExplicitDeviceIDs, assignIDs); err != nil {
		return nil, err
	}
	_ = s.audit(ctx, actorUserID, "campaign_created", projectID.String(), map[string]interface{}{
		"campaign_id": c.ID.String(),
		"firmware_id": fw.ID.String(),
		"rollout":     in.RolloutKind,
		"targets":     len(assignIDs),
	})
	return s.GetCampaignDetail(ctx, actorUserID, projectID, c.ID)
}

func (s *Service) ListCampaigns(ctx context.Context, actorUserID, projectID uuid.UUID, page, pageSize int) ([]CampaignView, int64, error) {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.CampaignRead); err != nil {
		return nil, 0, err
	}
	list, total, err := s.campaignRepo.ListCampaigns(ctx, projectID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	out := make([]CampaignView, 0, len(list))
	for i := range list {
		out = append(out, mapCampaign(&list[i]))
	}
	return out, total, nil
}

func (s *Service) GetCampaignDetail(ctx context.Context, actorUserID, projectID, campaignID uuid.UUID) (*CampaignDetailView, error) {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.CampaignRead); err != nil {
		return nil, err
	}
	c, err := s.campaignRepo.GetCampaign(ctx, projectID, campaignID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("campaign not found")
		}
		return nil, err
	}
	pv, err := s.buildProgress(ctx, campaignID, c.TargetDeviceCount)
	if err != nil {
		return nil, err
	}
	v := mapCampaign(c)
	return &CampaignDetailView{CampaignView: v, Progress: pv}, nil
}

func mapCampaign(c *campaignmodel.Campaign) CampaignView {
	return CampaignView{
		ID:                c.ID,
		Name:              c.Name,
		FirmwareID:        c.FirmwareID,
		RolloutKind:       c.RolloutKind,
		RolloutPercent:    c.RolloutPercent,
		ScheduledStartAt:  c.ScheduledStartAt,
		Status:            c.Status,
		TargetDeviceCount: c.TargetDeviceCount,
		ActivatedAt:       c.ActivatedAt,
		PausedAt:          c.PausedAt,
		CancelledAt:       c.CancelledAt,
		CompletedAt:       c.CompletedAt,
		CreatedByUserID:   c.CreatedByUserID,
		CreatedAt:         c.CreatedAt,
	}
}

func (s *Service) buildProgress(ctx context.Context, campaignID uuid.UUID, target int64) (ProgressView, error) {
	counts, err := s.campaignRepo.CountAssignmentsByStatus(ctx, campaignID)
	if err != nil {
		return ProgressView{}, err
	}
	p := ProgressView{TargetCount: target}
	p.Pending = counts[campaignmodel.AssignmentPending]
	p.Offered = counts[campaignmodel.AssignmentOffered]
	p.Downloaded = counts[campaignmodel.AssignmentDownloaded]
	p.Installed = counts[campaignmodel.AssignmentInstalled]
	p.Failed = counts[campaignmodel.AssignmentFailed]
	if target > 0 {
		term := p.Installed + p.Failed
		p.CompletionPercent = float64(term) * 100.0 / float64(target)
		p.SuccessPercent = float64(p.Installed) * 100.0 / float64(target)
	}
	return p, nil
}

func (s *Service) PauseCampaign(ctx context.Context, actorUserID, projectID, campaignID uuid.UUID) error {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.CampaignPause); err != nil {
		return err
	}
	if err := s.campaignRepo.PauseCampaign(ctx, projectID, campaignID); err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.BadRequest("campaign cannot be paused from its current state", nil)
		}
		return err
	}
	_ = s.audit(ctx, actorUserID, "campaign_paused", projectID.String(), map[string]interface{}{"campaign_id": campaignID.String()})
	return nil
}

func (s *Service) ResumeCampaign(ctx context.Context, actorUserID, projectID, campaignID uuid.UUID) error {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.CampaignUpdate); err != nil {
		return err
	}
	if err := s.campaignRepo.ResumeCampaign(ctx, projectID, campaignID); err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.BadRequest("campaign cannot be resumed from its current state", nil)
		}
		return err
	}
	_ = s.audit(ctx, actorUserID, "campaign_resumed", projectID.String(), map[string]interface{}{"campaign_id": campaignID.String()})
	return nil
}

func (s *Service) CancelCampaign(ctx context.Context, actorUserID, projectID, campaignID uuid.UUID) error {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.CampaignCancel); err != nil {
		return err
	}
	if err := s.campaignRepo.CancelCampaign(ctx, projectID, campaignID); err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.BadRequest("campaign cannot be cancelled from its current state", nil)
		}
		return err
	}
	_ = s.audit(ctx, actorUserID, "campaign_cancelled", projectID.String(), map[string]interface{}{"campaign_id": campaignID.String()})
	return nil
}

// ActivateDueCampaigns activates scheduled campaigns whose start time has passed (idempotent).
func (s *Service) ActivateDueCampaigns(ctx context.Context) ([]uuid.UUID, error) {
	now := time.Now().UTC()
	return s.campaignRepo.ActivateDueScheduledCampaigns(ctx, now)
}

func (s *Service) tryAutoComplete(ctx context.Context, projectID, campaignID uuid.UUID) error {
	c, err := s.campaignRepo.GetCampaign(ctx, projectID, campaignID)
	if err != nil {
		return err
	}
	if c.Status != campaignmodel.StatusActive {
		return nil
	}
	n, err := s.campaignRepo.CountNonTerminalAssignments(ctx, campaignID)
	if err != nil {
		return err
	}
	if n == 0 && c.TargetDeviceCount > 0 {
		_ = s.campaignRepo.MarkCampaignCompleted(ctx, projectID, campaignID)
	}
	return nil
}

// BuildPollOffer returns an OTA payload when an active campaign has a pending or offered assignment for the device.
func (s *Service) BuildPollOffer(ctx context.Context, projectID, deviceID uuid.UUID) (*PollOfferView, error) {
	row, err := s.campaignRepo.FindActivePendingOffer(ctx, projectID, deviceID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	if row.AssignmentStatus == campaignmodel.AssignmentPending {
		if err := s.campaignRepo.UpdateAssignmentStatus(ctx, row.CampaignID, deviceID, campaignmodel.AssignmentPending, campaignmodel.AssignmentOffered); err != nil {
			return nil, err
		}
	}
	_ = s.tryAutoComplete(ctx, projectID, row.CampaignID)
	return &PollOfferView{
		CampaignID:     row.CampaignID,
		FirmwareID:     row.FirmwareID,
		Version:        row.FirmwareVersion,
		ChecksumSHA256: row.ChecksumSHA256,
	}, nil
}

// OnDeviceReportFirmware marks assignment installed when the reported version matches the campaign firmware version string.
func (s *Service) OnDeviceReportFirmware(ctx context.Context, projectID, deviceID uuid.UUID, reportedVersion string) error {
	type row2 struct {
		CampaignID      uuid.UUID
		FirmwareVersion string
	}
	var r2 row2
	err := s.campaignRepo.DB().WithContext(ctx).Raw(`
		SELECT c.id AS campaign_id, f.version AS firmware_version
		FROM campaign_device_assignments AS a
		INNER JOIN campaigns AS c ON c.id = a.campaign_id AND c.deleted_at IS NULL
		INNER JOIN firmwares AS f ON f.id = c.firmware_id AND f.deleted_at IS NULL
		WHERE c.project_id = ? AND a.device_id = ?
		  AND c.status = ? AND a.status IN (?, ?)
		ORDER BY c.created_at ASC, c.id ASC
		LIMIT 1
	`, projectID, deviceID, campaignmodel.StatusActive, campaignmodel.AssignmentOffered, campaignmodel.AssignmentDownloaded).Scan(&r2).Error
	if err != nil {
		return err
	}
	if r2.CampaignID == uuid.Nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(reportedVersion), strings.TrimSpace(r2.FirmwareVersion)) {
		return nil
	}
	_ = s.campaignRepo.SetAssignmentStatus(ctx, r2.CampaignID, deviceID, campaignmodel.AssignmentInstalled)
	_ = s.tryAutoComplete(ctx, projectID, r2.CampaignID)
	return nil
}
