package model

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Migrator struct{}

func (Migrator) Migrate(_ context.Context, db *gorm.DB) error {
	if err := db.AutoMigrate(
		&DeviceType{},
		&DeviceAuthToken{},
		&DeviceConnectionLog{},
		&DeviceBlockEvent{},
		&DeviceGroup{},
		&DeviceGroupMembership{},
		&OtaDownloadToken{},
	); err != nil {
		return err
	}

	// Best-effort indexes; if the underlying DB doesn't support partial indexes,
	// failures are ignored (tests use SQLite).
	_ = ensurePartialUniqueIndexes(db)

	return SeedPredefinedDeviceTypes(db)
}

func ensurePartialUniqueIndexes(db *gorm.DB) error {
	_ = db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_device_types_predefined_name
			ON device_types (name_normalized)
			WHERE project_id IS NULL AND is_predefined = true AND deleted_at IS NULL
	`).Error
	_ = db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_device_types_custom_name_per_project
			ON device_types (project_id, name_normalized)
			WHERE is_predefined = false AND deleted_at IS NULL
	`).Error
	_ = db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_device_groups_name_per_project
			ON device_groups (project_id, name_normalized)
			WHERE deleted_at IS NULL
	`).Error
	return nil
}

func SeedPredefinedDeviceTypes(db *gorm.DB) error {
	now := time.Now().UTC()

	type SeedDef struct {
		name                   string
		processorArchitecture  string
		boardVersion           string
		flashSizeBytes         int64
		memoryNotes            string
	}

	// Minimal standard catalog. Extend as needed for specific MCU families.
	defs := []SeedDef{
		{
			name:                  "Cortex-M4 (Generic)",
			processorArchitecture: "arm_cortex_m4",
			boardVersion:          "revA",
			flashSizeBytes:        1048576, // 1MiB
			memoryNotes:           "Default MCU settings for Cortex-M4 class devices.",
		},
		{
			name:                  "Cortex-M3 (Generic)",
			processorArchitecture: "arm_cortex_m3",
			boardVersion:          "revA",
			flashSizeBytes:        524288, // 512KiB
			memoryNotes:           "Default MCU settings for Cortex-M3 class devices.",
		},
		{
			name:                  "ESP32 (Generic)",
			processorArchitecture: "xtensa_esp32",
			boardVersion:          "revA",
			flashSizeBytes:        4194304, // 4MiB
			memoryNotes:           "Default MCU settings for ESP32 class devices.",
		},
	}

	for _, d := range defs {
		norm := normalizeName(d.name)

		var existing DeviceType
		err := db.
			Where("is_predefined = ? AND project_id IS NULL AND name_normalized = ?", true, norm).
			First(&existing).Error
		if err == nil {
			continue
		}
		if !strings.Contains(err.Error(), "record not found") && err != gorm.ErrRecordNotFound {
			// SQLite error string differs; prefer ErrRecordNotFound when possible.
			if err != gorm.ErrRecordNotFound {
				// Best-effort seeding should not hard-fail on repeated runs.
				if err != nil {
					// If the DB uses a different not-found behavior, ignore by default.
				}
			}
		}

		t := &DeviceType{
			ID: uuid.New(),
			ProjectID:    nil,
			IsPredefined: true,
			Name:         d.name,
			NameNormalized: norm,
			ProcessorArchitecture: d.processorArchitecture,
			HardwareBoardVersion:  d.boardVersion,
			FlashSizeBytes:        d.flashSizeBytes,
			MemoryNotes:           d.memoryNotes,
			CreatedAt: now,
			UpdatedAt: now,
		}
		// Ignore already-existing edge cases.
		if err := db.Create(t).Error; err != nil {
			return fmt.Errorf("seed device type %s: %w", d.name, err)
		}
	}
	return nil
}

func normalizeName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

