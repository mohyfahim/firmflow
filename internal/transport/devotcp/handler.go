package devotcp

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	security "firmflow/internal/domain/auth/security"
	campaignsvc "firmflow/internal/domain/campaign/service"
	devicerepo "firmflow/internal/domain/device/repository"
	devicesvc "firmflow/internal/domain/device/service"
	firmwaresvc "firmflow/internal/domain/firmware/service"
	projectmodel "firmflow/internal/domain/project/model"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// Handler serves the proprietary binary OTA protocol over a TCP connection.
type Handler struct {
	Log                   *logrus.Logger
	DeviceRepo            *devicerepo.Repository
	DeviceSvc             *devicesvc.Service
	CampaignSvc           *campaignsvc.Service
	FirmwareSvc           *firmwaresvc.Service
	PublicDownloadBaseURL string
	TokenTTL              time.Duration
}

func (h *Handler) ServeConn(ctx context.Context, c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(90 * time.Second))

	fr, err := readFrame(c)
	if err != nil {
		return
	}
	if fr.MsgType != MsgAuthReq {
		_ = writeErr(c, fr.ReqID, ErrCodeBadPayload, "auth frame required")
		return
	}
	rawToken, err := decodeAuthPayload(fr.Payload)
	if err != nil || rawToken == "" {
		_ = writeErr(c, fr.ReqID, ErrCodeBadPayload, "invalid auth payload")
		return
	}

	device, err := h.authenticateDevice(ctx, rawToken)
	if err != nil {
		code, msg := mapDeviceAuthErr(err)
		_ = writeErr(c, fr.ReqID, code, msg)
		return
	}
	if err := writeFrame(c, MsgAuthOK, fr.ReqID, nil); err != nil {
		return
	}

	projectID := device.ProjectID
	deviceID := device.ID
	remote := c.RemoteAddr().String()
	ip := remote
	if host, _, err := net.SplitHostPort(remote); err == nil {
		ip = host
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_ = c.SetDeadline(time.Now().Add(60 * time.Second))
		fr, err := readFrame(c)
		if err != nil {
			return
		}
		switch fr.MsgType {
		case MsgPollReq:
			h.handlePoll(ctx, c, fr.ReqID, projectID, deviceID, ip, fr.Payload)
		case MsgReportReq:
			h.handleReport(ctx, c, fr.ReqID, projectID, deviceID, ip, fr.Payload)
		default:
			_ = writeErr(c, fr.ReqID, ErrCodeBadPayload, "unknown message type")
		}
	}
}

func (h *Handler) authenticateDevice(ctx context.Context, rawToken string) (*projectmodel.Device, error) {
	hash := security.HashToken(rawToken)
	device, _, err := h.DeviceRepo.GetDeviceByActiveTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return device, nil
}

func mapDeviceAuthErr(err error) (uint32, string) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrCodeUnauthorized, "unauthorized"
	}
	switch err.Error() {
	case "device_blocked":
		return ErrCodeBlocked, "device blocked"
	case "device_token_revoked":
		return ErrCodeRevokedOrDisabled, "token revoked"
	case "device_polling_disabled":
		return ErrCodeRevokedOrDisabled, "polling disabled"
	default:
		return ErrCodeUnauthorized, "unauthorized"
	}
}

func (h *Handler) handlePoll(ctx context.Context, c net.Conn, reqID uint32, projectID, deviceID uuid.UUID, ip string, payload []byte) {
	_, _, err := decodePollPayload(payload)
	if err != nil {
		_ = writeErr(c, reqID, ErrCodeBadPayload, "invalid poll payload")
		return
	}
	if err := h.DeviceSvc.DeviceOtaPoll(ctx, projectID, deviceID, ip, "tcp/ota"); err != nil {
		_ = writeErr(c, reqID, ErrCodeInternal, "poll failed")
		return
	}
	if h.CampaignSvc == nil {
		_ = writeFrame(c, MsgPollResp, reqID, []byte{0})
		return
	}
	offer, err := h.CampaignSvc.BuildPollOffer(ctx, projectID, deviceID)
	if err != nil {
		_ = writeErr(c, reqID, ErrCodeInternal, "campaign poll failed")
		return
	}
	if offer == nil {
		_ = writeFrame(c, MsgPollResp, reqID, []byte{0})
		return
	}
	if h.FirmwareSvc == nil {
		_ = writeFrame(c, MsgPollResp, reqID, []byte{0})
		return
	}
	ttl := h.TokenTTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	rawDL, exp, err := h.FirmwareSvc.IssueOtaDownloadToken(ctx, projectID, deviceID, offer.CampaignID, offer.FirmwareID, ttl)
	if err != nil {
		_ = writeErr(c, reqID, ErrCodeInternal, "token issue failed")
		return
	}
	sum, err := sha256HexTo32(offer.ChecksumSHA256)
	if err != nil {
		_ = writeErr(c, reqID, ErrCodeInternal, "bad firmware checksum")
		return
	}
	base := strings.TrimRight(strings.TrimSpace(h.PublicDownloadBaseURL), "/")
	path := "/api/v1/device/firmware-download?token=" + rawDL
	url := path
	if base != "" {
		url = base + path
	}
	body := encodePollResponse(true, offer.Version, url, sum, offer.CampaignID, offer.FirmwareID, uint32(exp.Unix()))
	if body == nil {
		_ = writeErr(c, reqID, ErrCodeInternal, "encode failed")
		return
	}
	_ = writeFrame(c, MsgPollResp, reqID, body)
}

func (h *Handler) handleReport(ctx context.Context, c net.Conn, reqID uint32, projectID, deviceID uuid.UUID, ip string, payload []byte) {
	campaignID, st, errCode, errMsg, curVer, err := decodeReportPayload(payload)
	if err != nil {
		_ = writeErr(c, reqID, ErrCodeBadPayload, "invalid report payload")
		return
	}
	var stEnum campaignsvc.OtaDeviceReportStatus
	switch st {
	case 1:
		stEnum = campaignsvc.OtaReportDownloaded
	case 2:
		stEnum = campaignsvc.OtaReportInstalled
	case 3:
		stEnum = campaignsvc.OtaReportFailed
	default:
		_ = writeErr(c, reqID, ErrCodeBadPayload, "invalid status")
		return
	}
	var codePtr *uint16
	if errCode != 0 {
		codePtr = &errCode
	}

	if h.CampaignSvc != nil {
		installedVer, err := h.CampaignSvc.ApplyOtaDeviceReport(ctx, projectID, deviceID, campaignID, stEnum, codePtr, errMsg)
		if err != nil {
			_ = writeErr(c, reqID, ErrCodeInternal, "report apply failed")
			return
		}
		var fwPtr *string
		switch stEnum {
		case campaignsvc.OtaReportInstalled:
			if installedVer != "" {
				fwPtr = &installedVer
			} else if curVer != "" {
				fwPtr = &curVer
			}
		default:
			if curVer != "" {
				fwPtr = &curVer
			}
		}
		if err := h.DeviceSvc.DeviceOtaReportTouch(ctx, projectID, deviceID, ip, "tcp/ota", fwPtr); err != nil {
			_ = writeErr(c, reqID, ErrCodeInternal, "report touch failed")
			return
		}
	} else {
		var fwPtr *string
		if curVer != "" {
			fwPtr = &curVer
		}
		if err := h.DeviceSvc.DeviceOtaReportTouch(ctx, projectID, deviceID, ip, "tcp/ota", fwPtr); err != nil {
			_ = writeErr(c, reqID, ErrCodeInternal, "report touch failed")
			return
		}
	}

	_ = writeFrame(c, MsgReportResp, reqID, []byte{1})
}
