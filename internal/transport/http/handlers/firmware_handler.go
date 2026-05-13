package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	apperrors "firmflow/internal/common/errors"
	"firmflow/internal/common/pagination"
	"firmflow/internal/common/response"
	firmwaresvc "firmflow/internal/domain/firmware/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type FirmwareHandler struct {
	svc *firmwaresvc.Service
}

func NewFirmwareHandler(svc *firmwaresvc.Service) *FirmwareHandler {
	return &FirmwareHandler{svc: svc}
}

func (h *FirmwareHandler) ListFirmware(c *gin.Context) {
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
	list, total, err := h.svc.ListFirmware(c.Request.Context(), uid, projectID, params.Page, params.PageSize, params.Sort)
	if err != nil {
		c.Error(err)
		return
	}
	meta := pagination.BuildMeta(params.Page, params.PageSize, total)
	response.WithMeta(c, http.StatusOK, list, meta)
}

func (h *FirmwareHandler) GetFirmware(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	firmwareID, err := uuid.Parse(c.Param("firmwareID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid firmware id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	detail, err := h.svc.GetFirmware(c.Request.Context(), uid, projectID, firmwareID)
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, detail)
}

func (h *FirmwareHandler) UploadFirmware(c *gin.Context) {
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

	maxBytes := h.svc.MaxUploadBytes()
	if maxBytes <= 0 {
		maxBytes = 64 << 20
	}
	// Total multipart body cap (file + fields); slight headroom for metadata.
	if err := c.Request.ParseMultipartForm(maxBytes + 8<<20); err != nil {
		c.Error(apperrors.BadRequest("invalid multipart form", nil))
		return
	}

	version := strings.TrimSpace(c.PostForm("version"))
	changelog := c.PostForm("changelog")

	rawTypes := strings.TrimSpace(c.PostForm("device_type_ids"))
	if rawTypes == "" {
		c.Error(apperrors.BadRequest("device_type_ids is required (JSON array of UUID strings)", nil))
		return
	}
	var deviceTypeIDs []uuid.UUID
	if err := json.Unmarshal([]byte(rawTypes), &deviceTypeIDs); err != nil {
		c.Error(apperrors.BadRequest("device_type_ids must be a JSON array of UUID strings", nil))
		return
	}

	fh, err := c.FormFile("file")
	if err != nil {
		c.Error(apperrors.BadRequest("file is required", nil))
		return
	}
	if fh.Size > maxBytes {
		c.Error(apperrors.BadRequest("firmware file exceeds maximum upload size", map[string]int64{"max_bytes": maxBytes, "file_size": fh.Size}))
		return
	}

	src, err := fh.Open()
	if err != nil {
		c.Error(apperrors.BadRequest("could not read uploaded file", nil))
		return
	}
	defer src.Close()

	detail, err := h.svc.UploadFirmware(c.Request.Context(), uid, projectID, version, changelog, fh.Filename, deviceTypeIDs, src)
	if err != nil {
		c.Error(err)
		return
	}
	response.Created(c, detail)
}

func (h *FirmwareHandler) DownloadFirmware(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	firmwareID, err := uuid.Parse(c.Param("firmwareID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid firmware id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}

	rc, fw, err := h.svc.OpenFirmwareBinary(c.Request.Context(), uid, projectID, firmwareID)
	if err != nil {
		c.Error(err)
		return
	}
	defer rc.Close()

	filename := safeDownloadFilename(fw.OriginalFilename)
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("X-Checksum-Sha256", fw.ChecksumSHA256)
	c.Header("Content-Length", strconv.FormatInt(fw.FileSizeBytes, 10))

	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, rc); err != nil {
		return
	}
}

// DeviceFirmwareDownloadByToken streams firmware bytes using a short-lived OTA download token (no user JWT).
func (h *FirmwareHandler) DeviceFirmwareDownloadByToken(c *gin.Context) {
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		c.Error(apperrors.BadRequest("token is required", nil))
		return
	}
	rc, fw, err := h.svc.OpenFirmwareWithOtaDownloadToken(c.Request.Context(), token)
	if err != nil {
		c.Error(err)
		return
	}
	defer rc.Close()

	filename := safeDownloadFilename(fw.OriginalFilename)
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("X-Checksum-Sha256", fw.ChecksumSHA256)
	c.Header("Content-Length", strconv.FormatInt(fw.FileSizeBytes, 10))

	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, rc); err != nil {
		return
	}
}

func (h *FirmwareHandler) DeleteFirmware(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	firmwareID, err := uuid.Parse(c.Param("firmwareID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid firmware id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	if err := h.svc.DeleteFirmware(c.Request.Context(), uid, projectID, firmwareID); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "firmware deleted"})
}

func safeDownloadFilename(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "firmware.bin"
	}
	return strings.ReplaceAll(base, `"`, "'")
}
