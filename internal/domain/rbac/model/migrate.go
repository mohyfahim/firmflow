package model

import (
	"context"

	"gorm.io/gorm"
)

type Migrator struct{}

func (Migrator) Migrate(_ context.Context, db *gorm.DB) error {
	if err := db.AutoMigrate(
		&Permission{},
		&Role{},
		&Project{},
		&ProjectMembership{},
	); err != nil {
		return err
	}
	if err := ensurePartialIndexes(db); err != nil {
		return err
	}
	return SeedRegistry(db)
}

func ensurePartialIndexes(db *gorm.DB) error {
	// PostgreSQL partial unique indexes (safe no-op attempt on SQLite where supported).
	_ = db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_roles_predefined_slug
		ON roles (slug)
		WHERE project_id IS NULL AND is_predefined = true AND deleted_at IS NULL
	`).Error
	_ = db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_roles_custom_name_per_project
		ON roles (project_id, name_normalized)
		WHERE is_predefined = false AND deleted_at IS NULL
	`).Error
	return nil
}
