package database

import (
	"context"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Migrator interface {
	Migrate(ctx context.Context, db *gorm.DB) error
}

type NoopMigrator struct{}

func (NoopMigrator) Migrate(_ context.Context, _ *gorm.DB) error {
	return nil
}

func RunMigrations(ctx context.Context, db *gorm.DB, migrator Migrator, log *logrus.Logger) error {
	log.Info("running migrations")
	if err := migrator.Migrate(ctx, db); err != nil {
		return err
	}
	log.Info("migrations completed")
	return nil
}

type CompositeMigrator struct {
	Migrators []Migrator
}

func (c CompositeMigrator) Migrate(ctx context.Context, db *gorm.DB) error {
	for _, m := range c.Migrators {
		if err := m.Migrate(ctx, db); err != nil {
			return err
		}
	}
	return nil
}
