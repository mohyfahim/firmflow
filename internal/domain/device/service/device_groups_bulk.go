package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	apperrors "firmflow/internal/common/errors"
	security "firmflow/internal/domain/auth/security"
	deviceModel "firmflow/internal/domain/device/model"
	rbacperm "firmflow/internal/domain/rbac/permission"
	devicerepo "firmflow/internal/domain/device/repository"
	projectmodel "firmflow/internal/domain/project/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DeviceGroupView is returned by group list/create/update APIs.
type DeviceGroupView struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	MemberCount int64     `json:"member_count"`
}

type DeviceListItemView struct {
	ID                     uuid.UUID       `json:"id"`
	Name                   string          `json:"name"`
	HardwareIdentifier     string          `json:"hardware_identifier"`
	DeviceType             DeviceTypeView  `json:"device_type"`
	Blocked                bool            `json:"blocked"`
	Online                 bool            `json:"online"`
	CurrentFirmwareVersion string          `json:"current_firmware_version"`
	LastSeenAt             *time.Time      `json:"last_seen_at"`
	GroupIDs               []uuid.UUID     `json:"group_ids"`
}

type BulkFailure struct {
	DeviceID string `json:"device_id"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type BulkDevicesResult struct {
	Action            string            `json:"action"`
	Matched           int64             `json:"matched,omitempty"`
	Processed         int               `json:"processed"`
	Succeeded         int               `json:"succeeded"`
	Failed            []BulkFailure     `json:"failed,omitempty"`
	TokensByDeviceID  map[string]string `json:"tokens_by_device_id,omitempty"`
	SyncCap           int               `json:"sync_cap"`
	AsyncRecommended  bool              `json:"async_recommended"`
}

func (s *Service) ListDeviceGroups(ctx context.Context, actorUserID, projectID uuid.UUID) ([]DeviceGroupView, error) {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.DeviceRead); err != nil {
		return nil, err
	}
	groups, err := s.deviceRepo.ListDeviceGroups(ctx, projectID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(groups))
	for i := range groups {
		ids = append(ids, groups[i].ID)
	}
	counts, err := s.deviceRepo.CountMembersByGroupIDs(ctx, projectID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]DeviceGroupView, 0, len(groups))
	for i := range groups {
		g := groups[i]
		out = append(out, DeviceGroupView{
			ID:          g.ID,
			Name:        g.Name,
			Description: g.Description,
			MemberCount: counts[g.ID],
		})
	}
	return out, nil
}

func (s *Service) CreateDeviceGroup(ctx context.Context, actorUserID, projectID uuid.UUID, name, description string) (*DeviceGroupView, error) {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.DeviceUpdate); err != nil {
		return nil, err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return nil, err
	}
	norm := strings.ToLower(strings.TrimSpace(name))
	if norm == "" {
		return nil, apperrors.BadRequest("group name is required", nil)
	}
	taken, err := s.deviceRepo.DeviceGroupNameTaken(ctx, projectID, norm, nil)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, apperrors.New("duplicate_group_name", "a device group with this name already exists in this project", 409, nil)
	}
	g, err := s.deviceRepo.CreateDeviceGroup(ctx, projectID, name, description)
	if err != nil {
		return nil, err
	}
	_ = s.audit(ctx, actorUserID, "device_group.created", projectID.String(), map[string]interface{}{
		"group_id": g.ID.String(),
		"name":     name,
	})
	return &DeviceGroupView{ID: g.ID, Name: g.Name, Description: g.Description, MemberCount: 0}, nil
}

func (s *Service) UpdateDeviceGroup(ctx context.Context, actorUserID, projectID, groupID uuid.UUID, name *string, description *string) (*DeviceGroupView, error) {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.DeviceUpdate); err != nil {
		return nil, err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return nil, err
	}
	if name != nil {
		norm := strings.ToLower(strings.TrimSpace(*name))
		taken, err := s.deviceRepo.DeviceGroupNameTaken(ctx, projectID, norm, &groupID)
		if err != nil {
			return nil, err
		}
		if taken {
			return nil, apperrors.New("duplicate_group_name", "a device group with this name already exists in this project", 409, nil)
		}
	}
	g, err := s.deviceRepo.UpdateDeviceGroup(ctx, projectID, groupID, name, description)
	if err != nil {
		return nil, err
	}
	_ = s.audit(ctx, actorUserID, "device_group.updated", projectID.String(), map[string]interface{}{
		"group_id": groupID.String(),
	})
	n, _ := s.deviceRepo.CountGroupMembers(ctx, groupID)
	return &DeviceGroupView{ID: g.ID, Name: g.Name, Description: g.Description, MemberCount: n}, nil
}

func (s *Service) DeleteDeviceGroup(ctx context.Context, actorUserID, projectID, groupID uuid.UUID) error {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.DeviceUpdate); err != nil {
		return err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return err
	}
	if err := s.deviceRepo.DeleteDeviceGroup(ctx, projectID, groupID); err != nil {
		return err
	}
	_ = s.audit(ctx, actorUserID, "device_group.deleted", projectID.String(), map[string]interface{}{
		"group_id": groupID.String(),
	})
	return nil
}

func (s *Service) AddDevicesToGroup(ctx context.Context, actorUserID, projectID, groupID uuid.UUID, deviceIDs []uuid.UUID) error {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.DeviceAssignGroup); err != nil {
		return err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return err
	}
	if _, err := s.deviceRepo.GetDeviceGroup(ctx, projectID, groupID); err != nil {
		return err
	}
	ok, err := s.deviceRepo.DevicesBelongToProject(ctx, projectID, deviceIDs)
	if err != nil {
		return err
	}
	if !ok {
		return apperrors.BadRequest("one or more devices are not in this project", nil)
	}
	if _, err := s.deviceRepo.AddDevicesToGroup(ctx, groupID, deviceIDs); err != nil {
		return err
	}
	_ = s.audit(ctx, actorUserID, "device_group.members_added", projectID.String(), map[string]interface{}{
		"group_id":    groupID.String(),
		"device_ids":  deviceIDsToStrings(deviceIDs),
		"count":       len(deviceIDs),
	})
	return nil
}

func (s *Service) RemoveDevicesFromGroup(ctx context.Context, actorUserID, projectID, groupID uuid.UUID, deviceIDs []uuid.UUID) error {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.DeviceAssignGroup); err != nil {
		return err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return err
	}
	if _, err := s.deviceRepo.GetDeviceGroup(ctx, projectID, groupID); err != nil {
		return err
	}
	if _, err := s.deviceRepo.RemoveDevicesFromGroup(ctx, groupID, deviceIDs); err != nil {
		return err
	}
	_ = s.audit(ctx, actorUserID, "device_group.members_removed", projectID.String(), map[string]interface{}{
		"group_id":   groupID.String(),
		"device_ids": deviceIDsToStrings(deviceIDs),
	})
	return nil
}

func deviceIDsToStrings(ids []uuid.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}

func (s *Service) ListDevices(ctx context.Context, actorUserID, projectID uuid.UUID, f devicerepo.DeviceListFilter, page, pageSize int, sort string) ([]DeviceListItemView, int64, error) {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.DeviceRead); err != nil {
		return nil, 0, err
	}
	total, err := s.deviceRepo.CountDevicesWithFilter(ctx, projectID, f)
	if err != nil {
		return nil, 0, err
	}
	devices, err := s.deviceRepo.ListDevicesWithFilter(ctx, projectID, f, page, pageSize, sort)
	if err != nil {
		return nil, 0, err
	}
	return s.buildDeviceListViews(ctx, projectID, devices), total, nil
}

func (s *Service) buildDeviceListViews(ctx context.Context, projectID uuid.UUID, devices []projectmodel.Device) []DeviceListItemView {
	if len(devices) == 0 {
		return nil
	}
	typeIDs := make(map[uuid.UUID]struct{})
	deviceIDs := make([]uuid.UUID, 0, len(devices))
	for _, d := range devices {
		typeIDs[d.DeviceTypeID] = struct{}{}
		deviceIDs = append(deviceIDs, d.ID)
	}
	tidList := make([]uuid.UUID, 0, len(typeIDs))
	for id := range typeIDs {
		tidList = append(tidList, id)
	}
	typeByID := make(map[uuid.UUID]deviceModel.DeviceType, len(tidList))
	for _, tid := range tidList {
		dt, err := s.deviceRepo.GetDeviceType(ctx, tid)
		if err == nil && dt != nil {
			typeByID[tid] = *dt
		}
	}

	groupsByDevice := s.loadGroupIDsForDevices(ctx, projectID, deviceIDs)
	now := time.Now().UTC()
	out := make([]DeviceListItemView, 0, len(devices))
	for _, d := range devices {
		dt := typeByID[d.DeviceTypeID]
		out = append(out, DeviceListItemView{
			ID:                     d.ID,
			Name:                   d.Name,
			HardwareIdentifier:     d.HardwareIdentifier,
			DeviceType:             mapDeviceTypeView(&dt),
			Blocked:                d.Blocked,
			Online:                 isOnline(d.LastSeenAt, now),
			CurrentFirmwareVersion: d.CurrentFirmwareVersion,
			LastSeenAt:             d.LastSeenAt,
			GroupIDs:               groupsByDevice[d.ID],
		})
	}
	return out
}

func (s *Service) loadGroupIDsForDevices(ctx context.Context, projectID uuid.UUID, deviceIDs []uuid.UUID) map[uuid.UUID][]uuid.UUID {
	out := make(map[uuid.UUID][]uuid.UUID, len(deviceIDs))
	for _, id := range deviceIDs {
		out[id] = nil
	}
	if len(deviceIDs) == 0 {
		return out
	}
	type row struct {
		DeviceID uuid.UUID `gorm:"column:device_id"`
		GroupID  uuid.UUID `gorm:"column:device_group_id"`
	}
	var rows []row
	_ = s.deviceRepo.DB().WithContext(ctx).Raw(`
		SELECT m.device_id, m.device_group_id
		FROM device_group_memberships m
		INNER JOIN device_groups g ON g.id = m.device_group_id AND g.project_id = ? AND g.deleted_at IS NULL
		WHERE m.device_id IN ?
		ORDER BY m.device_group_id
	`, projectID, deviceIDs).Scan(&rows)
	for _, r := range rows {
		out[r.DeviceID] = append(out[r.DeviceID], r.GroupID)
	}
	return out
}

// DeviceListFilterInput is used for bulk JSON filters (same semantics as list query params).
type DeviceListFilterInput struct {
	Online          *bool
	Blocked         *bool
	DeviceTypeID    *string
	GroupID         *string
	FirmwareVersion *string
	LastSeenFrom    *string
	LastSeenTo      *string
	Search          *string
}

func ParseDeviceListFilterInput(in DeviceListFilterInput) (devicerepo.DeviceListFilter, error) {
	var f devicerepo.DeviceListFilter
	f.Online = in.Online
	f.Blocked = in.Blocked
	f.FirmwareVersion = in.FirmwareVersion
	if in.DeviceTypeID != nil && strings.TrimSpace(*in.DeviceTypeID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(*in.DeviceTypeID))
		if err != nil {
			return f, apperrors.BadRequest("invalid filter.device_type_id", nil)
		}
		f.DeviceTypeID = &id
	}
	if in.GroupID != nil && strings.TrimSpace(*in.GroupID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(*in.GroupID))
		if err != nil {
			return f, apperrors.BadRequest("invalid filter.group_id", nil)
		}
		f.GroupID = &id
	}
	if in.LastSeenFrom != nil && strings.TrimSpace(*in.LastSeenFrom) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*in.LastSeenFrom))
		if err != nil {
			return f, apperrors.BadRequest("invalid filter.last_seen_from (RFC3339)", nil)
		}
		f.LastSeenFrom = &t
	}
	if in.LastSeenTo != nil && strings.TrimSpace(*in.LastSeenTo) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*in.LastSeenTo))
		if err != nil {
			return f, apperrors.BadRequest("invalid filter.last_seen_to (RFC3339)", nil)
		}
		f.LastSeenTo = &t
	}
	if in.Search != nil && strings.TrimSpace(*in.Search) != "" {
		s := strings.TrimSpace(*in.Search)
		f.Search = &s
	}
	return f, nil
}

// ParseDeviceListFilter builds a filter from typical query-string parameters.
func ParseDeviceListFilter(online, blocked, deviceTypeID, groupID, firmware, lastFrom, lastTo, q string) (devicerepo.DeviceListFilter, error) {
	var f devicerepo.DeviceListFilter
	if online != "" {
		v := online == "true" || online == "1"
		f.Online = &v
	}
	if blocked != "" {
		v := blocked == "true" || blocked == "1"
		f.Blocked = &v
	}
	if deviceTypeID != "" {
		id, err := uuid.Parse(deviceTypeID)
		if err != nil {
			return f, apperrors.BadRequest("invalid device_type_id", nil)
		}
		f.DeviceTypeID = &id
	}
	if groupID != "" {
		id, err := uuid.Parse(groupID)
		if err != nil {
			return f, apperrors.BadRequest("invalid group_id", nil)
		}
		f.GroupID = &id
	}
	if strings.TrimSpace(firmware) != "" {
		fw := strings.TrimSpace(firmware)
		f.FirmwareVersion = &fw
	}
	if lastFrom != "" {
		t, err := time.Parse(time.RFC3339, lastFrom)
		if err != nil {
			return f, apperrors.BadRequest("invalid last_seen_from (RFC3339)", nil)
		}
		f.LastSeenFrom = &t
	}
	if lastTo != "" {
		t, err := time.Parse(time.RFC3339, lastTo)
		if err != nil {
			return f, apperrors.BadRequest("invalid last_seen_to (RFC3339)", nil)
		}
		f.LastSeenTo = &t
	}
	if strings.TrimSpace(q) != "" {
		qs := strings.TrimSpace(q)
		f.Search = &qs
	}
	return f, nil
}

func parseUUIDSlice(in []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, apperrors.BadRequest("invalid device id in list", nil)
		}
		out = append(out, id)
	}
	return out, nil
}

func (s *Service) BulkDevices(ctx context.Context, actorUserID, projectID uuid.UUID, action string, applyToFilter bool, f devicerepo.DeviceListFilter, deviceIDStrs []string, groupID *uuid.UUID) (*BulkDevicesResult, error) {
	res := &BulkDevicesResult{
		Action:           action,
		SyncCap:          devicerepo.MaxBulkDevicesSync,
		TokensByDeviceID: map[string]string{},
	}

	var perm string
	switch action {
	case "add_to_group", "remove_from_group":
		perm = rbacperm.DeviceAssignGroup
	case "block", "unblock":
		perm = rbacperm.DeviceBlock
	case "rotate_tokens":
		perm = rbacperm.DeviceTokenRotate
	default:
		return nil, apperrors.BadRequest("unsupported bulk action", nil)
	}
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, perm); err != nil {
		return nil, err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return nil, err
	}

	if (action == "add_to_group" || action == "remove_from_group") && (groupID == nil || *groupID == uuid.Nil) {
		return nil, apperrors.BadRequest("group_id is required for this action", nil)
	}
	if groupID != nil && *groupID != uuid.Nil {
		if _, err := s.deviceRepo.GetDeviceGroup(ctx, projectID, *groupID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, apperrors.NotFound("device group not found")
			}
			return nil, err
		}
	}

	var deviceIDs []uuid.UUID
	var err error
	if applyToFilter {
		n, err := s.deviceRepo.CountDeviceIDsWithFilter(ctx, projectID, f)
		if err != nil {
			return nil, err
		}
		res.Matched = n
		if n > int64(devicerepo.MaxBulkDevicesSync) {
			res.AsyncRecommended = true
			return nil, apperrors.New(
				"bulk_too_large",
				fmt.Sprintf("filter matches %d devices; synchronous bulk is limited to %d. Narrow the filter or use an async job (coming soon).", n, devicerepo.MaxBulkDevicesSync),
				400,
				map[string]interface{}{"matched": n, "sync_cap": devicerepo.MaxBulkDevicesSync},
			)
		}
		deviceIDs, err = s.deviceRepo.ListDeviceIDsWithFilter(ctx, projectID, f, int(n))
		if err != nil {
			return nil, err
		}
	} else {
		deviceIDs, err = parseUUIDSlice(deviceIDStrs)
		if err != nil {
			return nil, err
		}
		if len(deviceIDs) == 0 {
			return nil, apperrors.BadRequest("device_ids is required when apply_to_filter is false", nil)
		}
		if len(deviceIDs) > devicerepo.MaxBulkDevicesSync {
			res.AsyncRecommended = true
			return nil, apperrors.New("bulk_too_large", fmt.Sprintf("too many device_ids (max %d)", devicerepo.MaxBulkDevicesSync), 400, map[string]interface{}{"sync_cap": devicerepo.MaxBulkDevicesSync})
		}
	}

	res.Processed = len(deviceIDs)

	switch action {
	case "add_to_group":
		valid, missing, err := s.deviceRepo.FilterDevicesInProject(ctx, projectID, deviceIDs)
		if err != nil {
			return nil, err
		}
		for _, id := range missing {
			res.Failed = append(res.Failed, BulkFailure{DeviceID: id.String(), Code: "not_found", Message: "device not in project"})
		}
		if len(valid) > 0 {
			if _, err := s.deviceRepo.AddDevicesToGroup(ctx, *groupID, valid); err != nil {
				return nil, err
			}
		}
		res.Succeeded = len(valid)
		_ = s.audit(ctx, actorUserID, "device_group.bulk_members_added", projectID.String(), map[string]interface{}{
			"group_id": *groupID, "requested": len(deviceIDs), "added": len(valid), "missing": len(missing),
		})
	case "remove_from_group":
		valid, missing, err := s.deviceRepo.FilterDevicesInProject(ctx, projectID, deviceIDs)
		if err != nil {
			return nil, err
		}
		for _, id := range missing {
			res.Failed = append(res.Failed, BulkFailure{DeviceID: id.String(), Code: "not_found", Message: "device not in project"})
		}
		if len(valid) > 0 {
			if _, err := s.deviceRepo.RemoveDevicesFromGroup(ctx, *groupID, valid); err != nil {
				return nil, err
			}
		}
		res.Succeeded = len(valid)
		_ = s.audit(ctx, actorUserID, "device_group.bulk_members_removed", projectID.String(), map[string]interface{}{
			"group_id": *groupID, "requested": len(deviceIDs), "removed": len(valid), "missing": len(missing),
		})
	case "block", "unblock", "rotate_tokens":
		for _, did := range deviceIDs {
			switch action {
			case "block":
				if err := s.deviceRepo.SetDeviceBlockedTx(ctx, projectID, did, true, actorUserID); err != nil {
					res.Failed = append(res.Failed, BulkFailure{DeviceID: did.String(), Code: "error", Message: err.Error()})
					continue
				}
				res.Succeeded++
			case "unblock":
				if err := s.deviceRepo.SetDeviceBlockedTx(ctx, projectID, did, false, actorUserID); err != nil {
					res.Failed = append(res.Failed, BulkFailure{DeviceID: did.String(), Code: "error", Message: err.Error()})
					continue
				}
				res.Succeeded++
			case "rotate_tokens":
				raw, err := s.rotateDeviceTokenRaw(ctx, actorUserID, projectID, did)
				if err != nil {
					code := "error"
					if errors.Is(err, gorm.ErrRecordNotFound) {
						code = "not_found"
					}
					res.Failed = append(res.Failed, BulkFailure{DeviceID: did.String(), Code: code, Message: err.Error()})
					continue
				}
				res.TokensByDeviceID[did.String()] = raw
				res.Succeeded++
			}
		}
		if action == "block" {
			_ = s.audit(ctx, actorUserID, "device.bulk_blocked", projectID.String(), map[string]interface{}{"count": res.Succeeded})
		} else if action == "unblock" {
			_ = s.audit(ctx, actorUserID, "device.bulk_unblocked", projectID.String(), map[string]interface{}{"count": res.Succeeded})
		} else if action == "rotate_tokens" {
			_ = s.audit(ctx, actorUserID, "device.bulk_tokens_rotated", projectID.String(), map[string]interface{}{"count": res.Succeeded})
		}
	}

	if len(res.TokensByDeviceID) == 0 {
		res.TokensByDeviceID = nil
	}
	return res, nil
}

func (s *Service) rotateDeviceTokenRaw(ctx context.Context, actorUserID, projectID, deviceID uuid.UUID) (string, error) {
	if _, err := s.deviceRepo.GetDevice(ctx, projectID, deviceID); err != nil {
		return "", err
	}
	rawToken, err := security.GenerateSecureToken(48)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	tokenHash := security.HashToken(rawToken)
	newTokenID := uuid.New()
	if _, _, err := s.deviceRepo.RotateDeviceAuthTokenTx(ctx, projectID, deviceID, actorUserID, tokenHash, newTokenID, now); err != nil {
		return "", err
	}
	_ = s.audit(ctx, actorUserID, "device.token_rotated", projectID.String(), map[string]interface{}{
		"device_id": deviceID.String(),
		"bulk":      true,
	})
	return rawToken, nil
}
