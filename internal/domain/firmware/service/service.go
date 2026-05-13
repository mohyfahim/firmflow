package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	apperrors "firmflow/internal/common/errors"
	authmodel "firmflow/internal/domain/auth/model"
	authrepo "firmflow/internal/domain/auth/repository"
	devicerepo "firmflow/internal/domain/device/repository"
	firmwaremodel "firmflow/internal/domain/firmware/model"
	firmwarerepo "firmflow/internal/domain/firmware/repository"
	rbacperm "firmflow/internal/domain/rbac/permission"
	rbacrepo "firmflow/internal/domain/rbac/repository"
	rbacsvc "firmflow/internal/domain/rbac/service"
	"firmflow/internal/platform/storage"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const maxChangelogRunes = 65535

type Service struct {
	rbacRepo        *rbacrepo.Repository
	authRepo        *authrepo.Repository
	rbacAuth        *rbacsvc.Authorizer
	firmwareRepo    *firmwarerepo.Repository
	deviceRepo      *devicerepo.Repository
	objects         storage.ObjectStore
	maxUploadBytes  int64
	storageProvider string
}

func New(
	rbacRepo *rbacrepo.Repository,
	authRepo *authrepo.Repository,
	rbacAuth *rbacsvc.Authorizer,
	firmwareRepo *firmwarerepo.Repository,
	deviceRepo *devicerepo.Repository,
	objects storage.ObjectStore,
	maxUploadBytes int64,
	storageProvider string,
) *Service {
	if maxUploadBytes <= 0 {
		maxUploadBytes = 64 << 20
	}
	if storageProvider == "" {
		storageProvider = "local"
	}
	return &Service{
		rbacRepo:        rbacRepo,
		authRepo:        authRepo,
		rbacAuth:        rbacAuth,
		firmwareRepo:    firmwareRepo,
		deviceRepo:      deviceRepo,
		objects:         objects,
		maxUploadBytes:  maxUploadBytes,
		storageProvider: storageProvider,
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

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "duplicate key")
}

// FirmwareListItem is a safe list row (no storage keys).
type FirmwareListItem struct {
	ID               uuid.UUID `json:"id"`
	Version          string    `json:"version"`
	FileSizeBytes    int64     `json:"file_size_bytes"`
	ChecksumSHA256   string    `json:"checksum_sha256"`
	OriginalFilename string    `json:"original_filename"`
	DeviceTypeCount  int64     `json:"device_type_count"`
	IsSemver         bool      `json:"is_semver"`
	UploadedByUserID uuid.UUID `json:"uploaded_by_user_id"`
	CreatedAt        time.Time `json:"created_at"`
}

type FirmwareDetailView struct {
	FirmwareListItem
	Changelog               string      `json:"changelog"`
	CompatibleDeviceTypeIDs []uuid.UUID `json:"compatible_device_type_ids"`
}

func (s *Service) ListFirmware(ctx context.Context, actorUserID, projectID uuid.UUID, page, pageSize int, sort string) ([]FirmwareListItem, int64, error) {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.FirmwareRead); err != nil {
		return nil, 0, err
	}
	list, total, err := s.firmwareRepo.ListFirmware(ctx, projectID, page, pageSize, sort)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]uuid.UUID, 0, len(list))
	for _, f := range list {
		ids = append(ids, f.ID)
	}
	counts, err := s.firmwareRepo.CountDeviceTypesByFirmwareIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	out := make([]FirmwareListItem, 0, len(list))
	for _, f := range list {
		out = append(out, FirmwareListItem{
			ID:               f.ID,
			Version:          f.Version,
			FileSizeBytes:    f.FileSizeBytes,
			ChecksumSHA256:   f.ChecksumSHA256,
			OriginalFilename: f.OriginalFilename,
			DeviceTypeCount:  counts[f.ID],
			IsSemver:         f.SemverMajor != nil,
			UploadedByUserID: f.UploadedByUserID,
			CreatedAt:        f.CreatedAt,
		})
	}
	return out, total, nil
}

func (s *Service) GetFirmware(ctx context.Context, actorUserID, projectID, firmwareID uuid.UUID) (*FirmwareDetailView, error) {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.FirmwareRead); err != nil {
		return nil, err
	}
	fw, err := s.firmwareRepo.GetFirmware(ctx, projectID, firmwareID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("firmware not found")
		}
		return nil, err
	}
	dtIDs, err := s.firmwareRepo.ListDeviceTypeIDsForFirmware(ctx, firmwareID)
	if err != nil {
		return nil, err
	}
	item := FirmwareListItem{
		ID:               fw.ID,
		Version:          fw.Version,
		FileSizeBytes:    fw.FileSizeBytes,
		ChecksumSHA256:   fw.ChecksumSHA256,
		OriginalFilename: fw.OriginalFilename,
		DeviceTypeCount:  int64(len(dtIDs)),
		IsSemver:         fw.SemverMajor != nil,
		UploadedByUserID: fw.UploadedByUserID,
		CreatedAt:        fw.CreatedAt,
	}
	return &FirmwareDetailView{
		FirmwareListItem:        item,
		Changelog:               fw.Changelog,
		CompatibleDeviceTypeIDs: dtIDs,
	}, nil
}

// UploadFirmware streams body to object storage, computes SHA-256, and persists metadata.
func (s *Service) UploadFirmware(
	ctx context.Context,
	actorUserID, projectID uuid.UUID,
	version, changelog, originalFilename string,
	deviceTypeIDs []uuid.UUID,
	body io.Reader,
) (*FirmwareDetailView, error) {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.FirmwareUpload); err != nil {
		return nil, err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return nil, err
	}

	version = strings.TrimSpace(version)
	if version == "" {
		return nil, apperrors.BadRequest("version is required", nil)
	}
	if len(version) > 128 {
		return nil, apperrors.BadRequest("version is too long", nil)
	}
	if utf8.RuneCountInString(changelog) > maxChangelogRunes {
		return nil, apperrors.BadRequest("changelog is too long", nil)
	}
	if len(deviceTypeIDs) == 0 {
		return nil, apperrors.BadRequest("at least one device_type_id is required", nil)
	}

	seen := make(map[uuid.UUID]struct{})
	dedup := make([]uuid.UUID, 0, len(deviceTypeIDs))
	for _, id := range deviceTypeIDs {
		if id == uuid.Nil {
			return nil, apperrors.BadRequest("invalid device_type_id", nil)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		dedup = append(dedup, id)
	}
	if err := s.validateDeviceTypesForProject(ctx, projectID, dedup); err != nil {
		return nil, err
	}

	vNorm := normalizeVersion(version)
	exists, err := s.firmwareRepo.FirmwareVersionExists(ctx, projectID, vNorm)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, apperrors.Conflict("firmware_version_exists", "this firmware version already exists in the project", map[string]string{"version": version})
	}

	fwID := uuid.New()
	storageKey := firmwarerepo.FirmwareStorageKey(projectID, fwID)

	tmp, err := writeLimitedTemp(body, s.maxUploadBytes)
	if err != nil {
		if err == errUploadTooLarge {
			return nil, apperrors.BadRequest("firmware file exceeds maximum upload size", map[string]int64{"max_bytes": s.maxUploadBytes})
		}
		return nil, err
	}
	defer tmp.cleanup()

	sum, err := tmp.sha256Hex()
	if err != nil {
		return nil, err
	}

	putFile, err := os.Open(tmp.path)
	if err != nil {
		return nil, err
	}
	defer putFile.Close()

	n, err := s.objects.Put(ctx, storageKey, putFile)
	if err != nil {
		_ = s.objects.Delete(ctx, storageKey)
		return nil, err
	}
	if n == 0 {
		_ = s.objects.Delete(ctx, storageKey)
		return nil, apperrors.BadRequest("empty firmware file", nil)
	}

	origName := strings.TrimSpace(originalFilename)
	if origName == "" {
		origName = "firmware.bin"
	}
	origName = filepath.Base(origName)
	if len(origName) > 255 {
		origName = origName[:255]
	}

	sp := parseSemverParts(version)
	now := time.Now().UTC()
	fw := &firmwaremodel.Firmware{
		ID:                fwID,
		ProjectID:         projectID,
		Version:           version,
		VersionNormalized: vNorm,
		Changelog:         changelog,
		FileSizeBytes:     n,
		ChecksumSHA256:    sum,
		OriginalFilename:  origName,
		StorageProvider:   s.storageProvider,
		StorageKey:        storageKey,
		SemverMajor:       sp.Major,
		SemverMinor:       sp.Minor,
		SemverPatch:       sp.Patch,
		SemverPrerelease:  sp.Prerelease,
		UploadedByUserID:  actorUserID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := s.firmwareRepo.CreateFirmwareWithTypes(ctx, fw, dedup); err != nil {
		_ = s.objects.Delete(ctx, storageKey)
		if isUniqueViolation(err) {
			return nil, apperrors.Conflict("firmware_version_exists", "this firmware version already exists in the project", map[string]string{"version": version})
		}
		return nil, err
	}

	_ = s.audit(ctx, actorUserID, "firmware_uploaded", projectID.String(), map[string]interface{}{
		"firmware_id":       fw.ID.String(),
		"version":           fw.Version,
		"checksum_sha256":   fw.ChecksumSHA256,
		"file_size_bytes":   fw.FileSizeBytes,
		"device_type_count": len(dedup),
	})

	return s.GetFirmware(ctx, actorUserID, projectID, fw.ID)
}

// MaxUploadBytes returns the configured firmware upload limit (for multipart sizing).
func (s *Service) MaxUploadBytes() int64 {
	return s.maxUploadBytes
}

func (s *Service) validateDeviceTypesForProject(ctx context.Context, projectID uuid.UUID, deviceTypeIDs []uuid.UUID) error {
	for _, id := range deviceTypeIDs {
		dt, err := s.deviceRepo.GetDeviceType(ctx, id)
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.BadRequest("unknown device_type_id", map[string]string{"device_type_id": id.String()})
			}
			return err
		}
		if dt.IsPredefined && dt.ProjectID == nil {
			continue
		}
		if !dt.IsPredefined && dt.ProjectID != nil && *dt.ProjectID == projectID {
			continue
		}
		return apperrors.BadRequest("device type is not compatible with this project", map[string]string{"device_type_id": id.String()})
	}
	return nil
}

// OpenFirmwareBinary returns a ReadCloser for the stored object after authorization.
func (s *Service) OpenFirmwareBinary(ctx context.Context, actorUserID, projectID, firmwareID uuid.UUID) (io.ReadCloser, *firmwaremodel.Firmware, error) {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.FirmwareRead); err != nil {
		return nil, nil, err
	}
	fw, err := s.firmwareRepo.GetFirmware(ctx, projectID, firmwareID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil, apperrors.NotFound("firmware not found")
		}
		return nil, nil, err
	}
	rc, err := s.objects.Open(ctx, fw.StorageKey)
	if err != nil {
		return nil, nil, apperrors.New("firmware_storage_unavailable", "firmware binary could not be read", 500, nil)
	}
	_ = s.audit(ctx, actorUserID, "firmware_downloaded", projectID.String(), map[string]interface{}{
		"firmware_id":     fw.ID.String(),
		"version":         fw.Version,
		"checksum_sha256": fw.ChecksumSHA256,
	})
	return rc, fw, nil
}

func (s *Service) DeleteFirmware(ctx context.Context, actorUserID, projectID, firmwareID uuid.UUID) error {
	if _, err := s.rbacAuth.AuthorizeProject(ctx, projectID, actorUserID, rbacperm.FirmwareUpload); err != nil {
		return err
	}
	if err := s.ensureProjectNotArchived(ctx, projectID); err != nil {
		return err
	}
	key, err := s.firmwareRepo.SoftDeleteFirmwareAndReturnKey(ctx, projectID, firmwareID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFound("firmware not found")
		}
		return err
	}
	if err := s.objects.Delete(ctx, key); err != nil {
		// Object leak is acceptable vs inconsistent state; operator can GC storage.
		_ = err
	}
	_ = s.audit(ctx, actorUserID, "firmware_deleted", projectID.String(), map[string]interface{}{
		"firmware_id": firmwareID.String(),
	})
	return nil
}

// --- temp + checksum (upload pipeline) ---

var errUploadTooLarge = errors.New("upload too large")

type limitedTemp struct {
	path string
}

func writeLimitedTemp(body io.Reader, max int64) (*limitedTemp, error) {
	f, err := os.CreateTemp("", "fw-up-*")
	if err != nil {
		return nil, err
	}
	path := f.Name()
	lr := io.LimitReader(body, max+1)
	n, copyErr := io.Copy(f, lr)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(path)
		return nil, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return nil, closeErr
	}
	if n > max {
		_ = os.Remove(path)
		return nil, errUploadTooLarge
	}
	return &limitedTemp{path: path}, nil
}

func (t *limitedTemp) cleanup() { _ = os.Remove(t.path) }

func (t *limitedTemp) sha256Hex() (string, error) {
	f, err := os.Open(t.path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
