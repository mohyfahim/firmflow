package handlers

import (
	"context"
	"net/http"
	"time"

	apperrors "firmflow/internal/common/errors"
	"firmflow/internal/common/pagination"
	"firmflow/internal/common/response"
	"firmflow/internal/common/validator"
	campaignsvc "firmflow/internal/domain/campaign/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type CampaignHandler struct {
	svc *campaignsvc.Service
}

func NewCampaignHandler(svc *campaignsvc.Service) *CampaignHandler {
	return &CampaignHandler{svc: svc}
}

type createCampaignRequest struct {
	Name              string      `json:"name"`
	FirmwareID        uuid.UUID   `json:"firmware_id"`
	RolloutKind       string      `json:"rollout_kind"`
	RolloutPercent    *int        `json:"rollout_percent,omitempty"`
	ScheduledStartAt  *time.Time  `json:"scheduled_start_at,omitempty"`
	DeviceGroupIDs    []uuid.UUID `json:"device_group_ids,omitempty"`
	ExplicitDeviceIDs []uuid.UUID `json:"explicit_device_ids,omitempty"`
}

func (h *CampaignHandler) CreateCampaign(c *gin.Context) {
	var req createCampaignRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	detail, err := h.svc.CreateCampaign(c.Request.Context(), uid, projectID, campaignsvc.CreateCampaignInput{
		Name:              req.Name,
		FirmwareID:        req.FirmwareID,
		RolloutKind:       req.RolloutKind,
		RolloutPercent:    req.RolloutPercent,
		ScheduledStartAt:  req.ScheduledStartAt,
		DeviceGroupIDs:    req.DeviceGroupIDs,
		ExplicitDeviceIDs: req.ExplicitDeviceIDs,
	})
	if err != nil {
		c.Error(err)
		return
	}
	response.Created(c, detail)
}

func (h *CampaignHandler) ListCampaigns(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	params := pagination.FromQuery(c)
	list, total, err := h.svc.ListCampaigns(c.Request.Context(), uid, projectID, params.Page, params.PageSize)
	if err != nil {
		c.Error(err)
		return
	}
	meta := pagination.BuildMeta(params.Page, params.PageSize, total)
	response.WithMeta(c, http.StatusOK, list, meta)
}

func (h *CampaignHandler) GetCampaign(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	campaignID, err := uuid.Parse(c.Param("campaignID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid campaign id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	detail, err := h.svc.GetCampaignDetail(c.Request.Context(), uid, projectID, campaignID)
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, detail)
}

func (h *CampaignHandler) PauseCampaign(c *gin.Context) {
	h.transition(c, h.svc.PauseCampaign)
}

func (h *CampaignHandler) ResumeCampaign(c *gin.Context) {
	h.transition(c, func(ctx context.Context, actor, projectID, campaignID uuid.UUID) error {
		return h.svc.ResumeCampaign(ctx, actor, projectID, campaignID)
	})
}

func (h *CampaignHandler) CancelCampaign(c *gin.Context) {
	h.transition(c, h.svc.CancelCampaign)
}

type transitionFn func(ctx context.Context, actor, projectID, campaignID uuid.UUID) error

func (h *CampaignHandler) transition(c *gin.Context, fn transitionFn) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	campaignID, err := uuid.Parse(c.Param("campaignID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid campaign id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	if err := fn(c.Request.Context(), uid, projectID, campaignID); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "ok"})
}
