package handlers

import (
	apperrors "firmflow/internal/common/errors"
	"firmflow/internal/common/pagination"
	"firmflow/internal/common/response"
	"firmflow/internal/common/validator"
	devicesvc "firmflow/internal/domain/device/service"
	"firmflow/internal/transport/http/dto"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type DeviceHandler struct {
	svc *devicesvc.Service
}

func NewDeviceHandler(svc *devicesvc.Service) *DeviceHandler {
	return &DeviceHandler{svc: svc}
}

func (h *DeviceHandler) ListDeviceTypes(c *gin.Context) {
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
	types, err := h.svc.ListDeviceTypes(c.Request.Context(), uid, projectID)
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, types)
}

func (h *DeviceHandler) CreateCustomDeviceType(c *gin.Context) {
	var req dto.CreateCustomDeviceTypeRequest
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
	typ, err := h.svc.CreateCustomDeviceType(c.Request.Context(), uid, projectID, req.Name, req.ProcessorArchitecture, req.HardwareBoardVersion, req.FlashSizeBytes, req.MemoryNotes)
	if err != nil {
		c.Error(err)
		return
	}
	response.Created(c, typ)
}

func (h *DeviceHandler) UpdateCustomDeviceType(c *gin.Context) {
	var req dto.UpdateCustomDeviceTypeRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	deviceTypeID, err := uuid.Parse(c.Param("deviceTypeID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid device type id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	typ, err := h.svc.UpdateCustomDeviceType(c.Request.Context(), uid, projectID, deviceTypeID, req.Name, req.ProcessorArchitecture, req.HardwareBoardVersion, req.FlashSizeBytes, req.MemoryNotes)
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, typ)
}

func (h *DeviceHandler) DeleteCustomDeviceType(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	deviceTypeID, err := uuid.Parse(c.Param("deviceTypeID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid device type id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	if err := h.svc.DeleteCustomDeviceType(c.Request.Context(), uid, projectID, deviceTypeID); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "device type deleted"})
}

func (h *DeviceHandler) ListDevices(c *gin.Context) {
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
	f, err := devicesvc.ParseDeviceListFilter(
		c.Query("online"), c.Query("blocked"),
		c.Query("device_type_id"), c.Query("group_id"),
		c.Query("firmware_version"), c.Query("last_seen_from"), c.Query("last_seen_to"), c.Query("q"),
	)
	if err != nil {
		c.Error(err)
		return
	}
	params := pagination.FromQuery(c)
	items, total, err := h.svc.ListDevices(c.Request.Context(), uid, projectID, f, params.Page, params.PageSize, params.Sort)
	if err != nil {
		c.Error(err)
		return
	}
	meta := pagination.BuildMeta(params.Page, params.PageSize, total)
	response.WithMeta(c, 200, items, meta)
}

func (h *DeviceHandler) BulkDevices(c *gin.Context) {
	var req dto.BulkDevicesRequest
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
	f, err := devicesvc.ParseDeviceListFilterInput(devicesvc.DeviceListFilterInput{
		Online:          req.Filter.Online,
		Blocked:         req.Filter.Blocked,
		DeviceTypeID:    req.Filter.DeviceTypeID,
		GroupID:         req.Filter.GroupID,
		FirmwareVersion: req.Filter.FirmwareVersion,
		LastSeenFrom:    req.Filter.LastSeenFrom,
		LastSeenTo:      req.Filter.LastSeenTo,
		Search:          req.Filter.Search,
	})
	if err != nil {
		c.Error(err)
		return
	}
	var gid *uuid.UUID
	if req.GroupID != nil && *req.GroupID != "" {
		id, err := uuid.Parse(*req.GroupID)
		if err != nil {
			c.Error(apperrors.BadRequest("invalid group_id", nil))
			return
		}
		gid = &id
	}
	res, err := h.svc.BulkDevices(c.Request.Context(), uid, projectID, req.Action, req.ApplyToFilter, f, req.DeviceIDs, gid)
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, res)
}

func (h *DeviceHandler) ListDeviceGroups(c *gin.Context) {
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
	groups, err := h.svc.ListDeviceGroups(c.Request.Context(), uid, projectID)
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, groups)
}

func (h *DeviceHandler) CreateDeviceGroup(c *gin.Context) {
	var req dto.CreateDeviceGroupRequest
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
	desc := ""
	if req.Description != nil {
		desc = *req.Description
	}
	g, err := h.svc.CreateDeviceGroup(c.Request.Context(), uid, projectID, req.Name, desc)
	if err != nil {
		c.Error(err)
		return
	}
	response.Created(c, g)
}

func (h *DeviceHandler) UpdateDeviceGroup(c *gin.Context) {
	var req dto.UpdateDeviceGroupRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	groupID, err := uuid.Parse(c.Param("groupID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid group id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	g, err := h.svc.UpdateDeviceGroup(c.Request.Context(), uid, projectID, groupID, req.Name, req.Description)
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, g)
}

func (h *DeviceHandler) DeleteDeviceGroup(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	groupID, err := uuid.Parse(c.Param("groupID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid group id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	if err := h.svc.DeleteDeviceGroup(c.Request.Context(), uid, projectID, groupID); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "device group deleted"})
}

func (h *DeviceHandler) AddDevicesToGroup(c *gin.Context) {
	var req dto.DeviceGroupMembersRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	groupID, err := uuid.Parse(c.Param("groupID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid group id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	ids, err := parseUUIDList(req.DeviceIDs)
	if err != nil {
		c.Error(err)
		return
	}
	if err := h.svc.AddDevicesToGroup(c.Request.Context(), uid, projectID, groupID, ids); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "devices added to group"})
}

func (h *DeviceHandler) RemoveDevicesFromGroup(c *gin.Context) {
	var req dto.DeviceGroupMembersRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	groupID, err := uuid.Parse(c.Param("groupID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid group id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	ids, err := parseUUIDList(req.DeviceIDs)
	if err != nil {
		c.Error(err)
		return
	}
	if err := h.svc.RemoveDevicesFromGroup(c.Request.Context(), uid, projectID, groupID, ids); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "devices removed from group"})
}

func parseUUIDList(strs []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(strs))
	for _, s := range strs {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, apperrors.BadRequest("invalid device id", nil)
		}
		out = append(out, id)
	}
	return out, nil
}

func (h *DeviceHandler) RegisterDevice(c *gin.Context) {
	var req dto.RegisterDeviceRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	deviceTypeID, err := uuid.Parse(req.DeviceTypeID)
	if err != nil {
		c.Error(apperrors.BadRequest("invalid device type id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}

	twin, rawToken, err := h.svc.RegisterDevice(c.Request.Context(), uid, projectID, deviceTypeID, req.Name, req.HardwareIdentifier)
	if err != nil {
		c.Error(err)
		return
	}
	response.Created(c, gin.H{
		"device":      twin,
		"auth_token": rawToken,
	})
}

func (h *DeviceHandler) GetDeviceTwin(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	deviceID, err := uuid.Parse(c.Param("deviceID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid device id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	twin, err := h.svc.GetDeviceTwin(c.Request.Context(), uid, projectID, deviceID)
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, twin)
}

func (h *DeviceHandler) BlockDevice(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	deviceID, err := uuid.Parse(c.Param("deviceID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid device id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	if err := h.svc.BlockDevice(c.Request.Context(), uid, projectID, deviceID); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "device blocked"})
}

func (h *DeviceHandler) UnblockDevice(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	deviceID, err := uuid.Parse(c.Param("deviceID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid device id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	if err := h.svc.UnblockDevice(c.Request.Context(), uid, projectID, deviceID); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "device unblocked"})
}

func (h *DeviceHandler) RotateDeviceToken(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	deviceID, err := uuid.Parse(c.Param("deviceID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid device id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	twin, rawToken, err := h.svc.RotateDeviceToken(c.Request.Context(), uid, projectID, deviceID)
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{
		"device":      twin,
		"auth_token": rawToken,
	})
}

// Device poll/report endpoints are authenticated by device auth middleware.
func (h *DeviceHandler) DevicePoll(c *gin.Context) {
	projectID, err := uuid.Parse(c.GetString("device_project_id"))
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	deviceID, err := uuid.Parse(c.GetString("device_id"))
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	ip := c.ClientIP()
	ua := c.GetHeader("User-Agent")
	endpoint := c.Request.URL.Path
	if err := h.svc.DevicePoll(c.Request.Context(), projectID, deviceID, ip, ua, endpoint); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "poll received"})
}

func (h *DeviceHandler) DeviceReport(c *gin.Context) {
	projectID, err := uuid.Parse(c.GetString("device_project_id"))
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	deviceID, err := uuid.Parse(c.GetString("device_id"))
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	var req dto.ReportDeviceRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	ip := c.ClientIP()
	ua := c.GetHeader("User-Agent")
	endpoint := c.Request.URL.Path
	if err := h.svc.DeviceReport(c.Request.Context(), projectID, deviceID, req.CurrentFirmwareVersion, ip, ua, endpoint); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "report received"})
}

