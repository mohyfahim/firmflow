package model

import (
	"context"

	"gorm.io/gorm"
)

type Migrator struct{}

func (Migrator) Migrate(_ context.Context, db *gorm.DB) error {
	if err := db.AutoMigrate(
		&Firmware{},
		&FirmwareDeviceType{},
	); err != nil {
		return err
	}
	// Partial unique indexes: allow same version string after soft-delete.
	_ = db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS ux_firmware_project_version_active
			ON firmwares (project_id, version_normalized)
			WHERE deleted_at IS NULL
	`).Error
	_ = db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS ux_firmware_storage_key_active
			ON firmwares (storage_key)
			WHERE deleted_at IS NULL
	`).Error
	return nil
}
