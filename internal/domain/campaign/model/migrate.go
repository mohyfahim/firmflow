package model

import (
	"context"

	"gorm.io/gorm"
)

type Migrator struct{}

func (Migrator) Migrate(_ context.Context, db *gorm.DB) error {
	if err := db.AutoMigrate(
		&Campaign{},
		&CampaignTargetGroup{},
		&CampaignTargetDevice{},
		&CampaignDeviceAssignment{},
	); err != nil {
		return err
	}
	_ = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_campaigns_scheduler
			ON campaigns (status, scheduled_start_at)
			WHERE deleted_at IS NULL
	`).Error
	return nil
}
