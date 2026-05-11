package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	apperrors "firmflow/internal/common/errors"
	authmodel "firmflow/internal/domain/auth/model"
	authrepo "firmflow/internal/domain/auth/repository"
	security "firmflow/internal/domain/auth/security"
	rbacperm "firmflow/internal/domain/rbac/permission"
	rbacsvc "firmflow/internal/domain/rbac/service"
	rbacrepo "firmflow/internal/domain/rbac/repository"
	projectmodel "firmflow/internal/domain/project/model"
	deviceModel "firmflow/internal/domain/device/model"
	devicerepo "firmflow/internal/domain/device/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const onlineThreshold = 5 * time.Minute

type Service struct {
	rbacRepo  *rbacrepo.Repository
	rbacAuth  *rbacsvc.Authorizer
	authRepo  *authrepo.Repository
	deviceRepo *devicerepo.Repository
}

func New(rbacRepo *rbacrepo.Repository, authRepo *authrepo.Repository, rbacAuth *rbacsvc.Authorizer, deviceRepo *devicerepo.Repository) *Service {
	return &Service{
		rbacRepo:   rbacRepo,
		rbacAuth:   rbacAuth,
		authRepo:   authRepo,
		deviceRepo: deviceRepo,
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
		return apperrors.New("project_archived", "project is archived; unarchive to modify", 409, nil)
	}
	return nil
}

func isOnline(lastSeenAt *time.Time, now time.Time) bool {
	if lastSeenAt == nil {
		return false
	}
	return !lastSeenAt.Before(now.Add(-onlineThreshold))
}

func normalizeHardwareIdentifier(raw string) string {
	raw = strings.TrimSpace(raw)
	return strings.ToLower(raw)
}

// ===== Device type APIs =====

type DeviceTypeView struct {
	ID uuid.UUID `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // system/custom

	ProcessorArchitecture string `json:"processor_architecture"`
	HardwareBoardVersion  string `json:"hardware_board_version"`
	FlashSizeBytes        int64  `json:"flash_size_bytes"`
	MemoryNotes           string `json:"memory_notes,omitempty"`
}

func (v DeviceTypeView) isSystem() bool { return v.Type == "system" }

func (s *Service) ListDeviceTypes(ctx context.Context, actorUserID, projectID uuid.UUID) ([]DeviceTypeView, error) {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.DeviceRead); err != nil {
		return nil, err
	}
	pre, err := s.deviceRepo.ListPredefinedDeviceTypes(ctx)
	if err != nil {
		return nil, err
	}
	cust, err := s.deviceRepo.ListCustomDeviceTypes(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]DeviceTypeView, 0, len(pre)+len(cust))
	for i := range pre {
		out = append(out, mapDeviceTypeView(&pre[i]))
	}
	for i := range cust {
		out = append(out, mapDeviceTypeView(&cust[i]))
	}
	return out, nil
}

func mapDeviceTypeView(dt *deviceModel.DeviceType) DeviceTypeView {
	t := "custom"
	if dt.IsPredefined {
		t = "system"
	}
	return DeviceTypeView{
		ID:                   dt.ID,
		Name:                 dt.Name,
		Type:                 t,
		ProcessorArchitecture: dt.ProcessorArchitecture,
		HardwareBoardVersion:  dt.HardwareBoardVersion,
		FlashSizeBytes:        dt.FlashSizeBytes,
		MemoryNotes:           dt.MemoryNotes,
	}
}

func (s *Service) CreateCustomDeviceType(ctx context.Context, actorUserID, projectID uuid.UUID, name, processorArchitecture, hardwareBoardVersion string, flashSizeBytes int64, memoryNotes *string) (*DeviceTypeView, error) {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.DeviceCreate); err != nil {
		return nil, err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return nil, err
	}
	norm := strings.ToLower(strings.TrimSpace(name))
	if norm == "" {
		return nil, apperrors.BadRequest("device type name is required", nil)
	}
	taken, err := s.deviceRepo.CustomDeviceTypeNameTaken(ctx, projectID, norm, nil)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, apperrors.New("duplicate_device_type_name", "a device type with this name already exists in this project", 409, nil)
	}

	mem := ""
	if memoryNotes != nil {
		mem = *memoryNotes
	}
	dt, err := s.deviceRepo.CreateCustomDeviceType(ctx, projectID, name, processorArchitecture, hardwareBoardVersion, flashSizeBytes, mem)
	if err != nil {
		return nil, err
	}
	_ = s.audit(ctx, actorUserID, "device_type.created", projectID.String(), map[string]interface{}{
		"device_type_id": dt.ID.String(),
		"name":            name,
	})
	v := mapDeviceTypeView(dt)
	return &v, nil
}

func (s *Service) UpdateCustomDeviceType(ctx context.Context, actorUserID, projectID, deviceTypeID uuid.UUID, name *string, processorArchitecture *string, hardwareBoardVersion *string, flashSizeBytes *int64, memoryNotes *string) (*DeviceTypeView, error) {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.DeviceUpdate); err != nil {
		return nil, err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return nil, err
	}
	dt, err := s.deviceRepo.GetDeviceType(ctx, deviceTypeID)
	if err != nil {
		return nil, err
	}
	if dt.IsPredefined || dt.ProjectID == nil || *dt.ProjectID != projectID {
		return nil, apperrors.BadRequest("system device types cannot be modified", nil)
	}
	if name != nil {
		norm := strings.ToLower(strings.TrimSpace(*name))
		taken, err := s.deviceRepo.CustomDeviceTypeNameTaken(ctx, projectID, norm, &deviceTypeID)
		if err != nil {
			return nil, err
		}
		if taken {
			return nil, apperrors.New("duplicate_device_type_name", "a device type with this name already exists in this project", 409, nil)
		}
	}
	updated, err := s.deviceRepo.UpdateCustomDeviceType(ctx, projectID, deviceTypeID, name, processorArchitecture, hardwareBoardVersion, flashSizeBytes, memoryNotes)
	if err != nil {
		return nil, err
	}
	_ = s.audit(ctx, actorUserID, "device_type.updated", projectID.String(), map[string]interface{}{
		"device_type_id": deviceTypeID.String(),
	})
	v := mapDeviceTypeView(updated)
	return &v, nil
}

func (s *Service) DeleteCustomDeviceType(ctx context.Context, actorUserID, projectID, deviceTypeID uuid.UUID) error {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.DeviceUpdate); err != nil {
		return err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return err
	}
	dt, err := s.deviceRepo.GetDeviceType(ctx, deviceTypeID)
	if err != nil {
		return err
	}
	if dt.IsPredefined || dt.ProjectID == nil || *dt.ProjectID != projectID {
		return apperrors.BadRequest("system device types cannot be deleted", nil)
	}
	if err := s.deviceRepo.DeleteCustomDeviceType(ctx, projectID, deviceTypeID); err != nil {
		// Map repository-level “in use” to a readable API error.
		if err.Error() == "device_type_in_use" {
			return apperrors.New("device_type_in_use", "cannot delete device type while devices are registered with it", 409, nil)
		}
		return err
	}
	_ = s.audit(ctx, actorUserID, "device_type.deleted", projectID.String(), map[string]interface{}{
		"device_type_id": deviceTypeID.String(),
	})
	return nil
}

// ===== Device APIs =====

type ConnectionLogView struct {
	ID uuid.UUID `json:"id"`
	DeviceID uuid.UUID `json:"device_id"`
	IP string `json:"ip"`
	UserAgent string `json:"user_agent,omitempty"`
	Action string `json:"action"`
	Endpoint string `json:"endpoint"`
	At time.Time `json:"timestamp"`
}

type DeviceTwinView struct {
	ID uuid.UUID `json:"id"`
	Name string `json:"name"`
	HardwareIdentifier string `json:"hardware_identifier"`

	DeviceType DeviceTypeView `json:"device_type"`
	Blocked bool `json:"blocked_state"`

	Online bool `json:"online"`
	CurrentFirmwareVersion string `json:"current_firmware_version"`
	LastSeenAt *time.Time `json:"last_seen_at"`

	RecentConnectionLogs []ConnectionLogView `json:"recent_connection_logs"`

	AuthTokenMetadata *struct {
		ID uuid.UUID `json:"id"`
		IssuedAt *time.Time `json:"issued_at,omitempty"`
	} `json:"token_metadata,omitempty"`
}

func (s *Service) RegisterDevice(ctx context.Context, actorUserID, projectID, deviceTypeID uuid.UUID, name, hardwareIdentifier string) (*DeviceTwinView, string, error) {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.DeviceCreate); err != nil {
		return nil, "", err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return nil, "", err
	}

	dt, err := s.deviceRepo.GetDeviceType(ctx, deviceTypeID)
	if err != nil {
		return nil, "", err
	}
	if !dt.IsPredefined {
		if dt.ProjectID == nil || *dt.ProjectID != projectID {
			return nil, "", apperrors.BadRequest("device type does not belong to this project", nil)
		}
	}

	hwNorm := normalizeHardwareIdentifier(hardwareIdentifier)
	if hwNorm == "" {
		return nil, "", apperrors.BadRequest("hardware identifier is required", nil)
	}
	// Uniqueness within project (service precheck; DB constraints still apply).
	if _, err := s.deviceRepo.GetDeviceByHardwareIdentifier(ctx, projectID, hwNorm); err == nil {
		return nil, "", apperrors.New("hardware_id_in_use", "a device with this hardware identifier already exists in this project", 409, nil)
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		// If DB returned a different error, surface it.
		return nil, "", err
	}

	rawToken, err := security.GenerateSecureToken(48)
	if err != nil {
		return nil, "", err
	}
	now := time.Now().UTC()
	tokenHash := security.HashToken(rawToken)

	device, _, err := s.deviceRepo.CreateDeviceAndAuthTokenTx(ctx, projectID, deviceTypeID, name, hardwareIdentifier, hwNorm, tokenHash, now, &actorUserID)
	if err != nil {
		return nil, "", err
	}

	_ = s.audit(ctx, actorUserID, "device.created", projectID.String(), map[string]interface{}{
		"device_id":        device.ID.String(),
		"device_type_id":  deviceTypeID.String(),
		"hardware_id_norm": hwNorm,
	})

	twin, err := s.GetDeviceTwin(ctx, actorUserID, projectID, device.ID)
	if err != nil {
		return nil, "", err
	}
	return twin, rawToken, nil
}

func (s *Service) RotateDeviceToken(ctx context.Context, actorUserID, projectID, deviceID uuid.UUID) (*DeviceTwinView, string, error) {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.DeviceTokenRotate); err != nil {
		return nil, "", err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return nil, "", err
	}

	// Ensure device exists.
	if _, err := s.deviceRepo.GetDevice(ctx, projectID, deviceID); err != nil {
		return nil, "", err
	}

	rawToken, err := security.GenerateSecureToken(48)
	if err != nil {
		return nil, "", err
	}
	now := time.Now().UTC()
	tokenHash := security.HashToken(rawToken)
	newTokenID := uuid.New()

	if _, _, err := s.deviceRepo.RotateDeviceAuthTokenTx(ctx, projectID, deviceID, actorUserID, tokenHash, newTokenID, now); err != nil {
		return nil, "", err
	}

	_ = s.audit(ctx, actorUserID, "device.token_rotated", projectID.String(), map[string]interface{}{
		"device_id":   deviceID.String(),
		"token_issued": now,
	})

	twin, err := s.GetDeviceTwin(ctx, actorUserID, projectID, deviceID)
	if err != nil {
		return nil, "", err
	}
	return twin, rawToken, nil
}

func (s *Service) BlockDevice(ctx context.Context, actorUserID, projectID, deviceID uuid.UUID) error {
	return s.setDeviceBlocked(ctx, actorUserID, projectID, deviceID, true)
}
func (s *Service) UnblockDevice(ctx context.Context, actorUserID, projectID, deviceID uuid.UUID) error {
	return s.setDeviceBlocked(ctx, actorUserID, projectID, deviceID, false)
}

func (s *Service) setDeviceBlocked(ctx context.Context, actorUserID, projectID, deviceID uuid.UUID, blocked bool) error {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.DeviceBlock); err != nil {
		return err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return err
	}
	if err := s.deviceRepo.SetDeviceBlockedTx(ctx, projectID, deviceID, blocked, actorUserID); err != nil {
		return err
	}
	ev := "device.blocked"
	if !blocked {
		ev = "device.unblocked"
	}
	_ = s.audit(ctx, actorUserID, ev, projectID.String(), map[string]interface{}{
		"device_id": deviceID.String(),
	})
	return nil
}

func (s *Service) GetDeviceTwin(ctx context.Context, actorUserID, projectID, deviceID uuid.UUID) (*DeviceTwinView, error) {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.DeviceRead); err != nil {
		return nil, err
	}
	d, err := s.deviceRepo.GetDevice(ctx, projectID, deviceID)
	if err != nil {
		return nil, err
	}

	dt, err := s.deviceRepo.GetDeviceType(ctx, d.DeviceTypeID)
	if err != nil {
		return nil, err
	}

	logs, err := s.deviceRepo.ListRecentDeviceConnectionLogs(ctx, projectID, deviceID, 20)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	online := isOnline(d.LastSeenAt, now)

	view := &DeviceTwinView{
		ID:                     d.ID,
		Name:                   d.Name,
		HardwareIdentifier:     d.HardwareIdentifier,
		DeviceType:            mapDeviceTypeView(dt),
		Blocked:               d.Blocked,
		Online:                online,
		CurrentFirmwareVersion: d.CurrentFirmwareVersion,
		LastSeenAt:            d.LastSeenAt,
		RecentConnectionLogs:  make([]ConnectionLogView, 0, len(logs)),
	}
	for i := range logs {
		l := logs[i]
		view.RecentConnectionLogs = append(view.RecentConnectionLogs, ConnectionLogView{
			ID:        l.ID,
			DeviceID:  l.DeviceID,
			IP:        l.IP,
			UserAgent: l.UserAgent,
			Action:    l.Action,
			Endpoint:  l.Endpoint,
			At:        l.CreatedAt,
		})
	}

	if d.ActiveAuthTokenID != nil {
		view.AuthTokenMetadata = &struct {
			ID       uuid.UUID  `json:"id"`
			IssuedAt *time.Time `json:"issued_at,omitempty"`
		}{ID: *d.ActiveAuthTokenID, IssuedAt: d.ActiveAuthTokenIssuedAt}
	}

	return view, nil
}

// ===== Device auth endpoints (poll/report) =====

// DevicePoll records a poll connection and updates last_seen_at for online/offline.
func (s *Service) DevicePoll(ctx context.Context, projectID, deviceID uuid.UUID, ip, userAgent, endpoint string) error {
	d, err := s.deviceRepo.GetDevice(ctx, projectID, deviceID)
	if err != nil {
		return err
	}
	if d.Blocked {
		return apperrors.New("device_blocked", "device is blocked", 403, nil)
	}
	now := time.Now().UTC()
	online := isOnline(&now, now) // always true for just-updated last_seen_at
	connStatus := "offline"
	if online {
		connStatus = "online"
	}
	if err := s.deviceRepo.UpdateDeviceSeenAndFirmware(ctx, projectID, deviceID, nil, now, connStatus); err != nil {
		return err
	}
	return s.deviceRepo.CreateDeviceConnectionLog(ctx, &deviceModel.DeviceConnectionLog{
		ProjectID: projectID,
		DeviceID:  deviceID,
		IP:         ip,
		UserAgent:  userAgent,
		Action:     "poll",
		Endpoint:   endpoint,
		CreatedAt:  now,
	})
}

// DeviceReport records a report connection and updates firmware version + last_seen_at.
func (s *Service) DeviceReport(ctx context.Context, projectID, deviceID uuid.UUID, firmwareVersion string, ip, userAgent, endpoint string) error {
	d, err := s.deviceRepo.GetDevice(ctx, projectID, deviceID)
	if err != nil {
		return err
	}
	if d.Blocked {
		return apperrors.New("device_blocked", "device is blocked", 403, nil)
	}
	now := time.Now().UTC()
	online := isOnline(&now, now)
	connStatus := "offline"
	if online {
		connStatus = "online"
	}
	fw := strings.TrimSpace(firmwareVersion)
	if fw == "" {
		return apperrors.BadRequest("current_firmware_version is required", nil)
	}
	if err := s.deviceRepo.UpdateDeviceSeenAndFirmware(ctx, projectID, deviceID, &fw, now, connStatus); err != nil {
		return err
	}
	return s.deviceRepo.CreateDeviceConnectionLog(ctx, &deviceModel.DeviceConnectionLog{
		ProjectID: projectID,
		DeviceID:  deviceID,
		IP:         ip,
		UserAgent:  userAgent,
		Action:     "report",
		Endpoint:   endpoint,
		CreatedAt:  now,
	})
}

// ensure usage of imported packages
var _ = projectmodel.Device{}

