package repository

import (
	"context"
	"time"

	"firmflow/internal/domain/auth/model"

	"github.com/google/uuid"
)

// AuditLogFilters filters project-scoped audit queries.
type AuditLogFilters struct {
	TargetType string
	TargetID   string
	ActorID    *uuid.UUID
	EventPrefix string
	From       *time.Time
	To         *time.Time
	Offset     int
	Limit      int
}

func (r *Repository) ListAuditLogs(ctx context.Context, f AuditLogFilters) ([]model.AuditLog, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.AuditLog{})
	if f.TargetType != "" {
		q = q.Where("target_type = ?", f.TargetType)
	}
	if f.TargetID != "" {
		q = q.Where("target_id = ?", f.TargetID)
	}
	if f.ActorID != nil {
		q = q.Where("actor_user_id = ?", *f.ActorID)
	}
	if f.EventPrefix != "" {
		q = q.Where("event LIKE ?", f.EventPrefix+"%")
	}
	if f.From != nil {
		q = q.Where("created_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("created_at <= ?", *f.To)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if f.Limit <= 0 {
		f.Limit = 20
	}
	if f.Limit > 100 {
		f.Limit = 100
	}

	var rows []model.AuditLog
	err := q.Order("created_at DESC").Offset(f.Offset).Limit(f.Limit).Find(&rows).Error
	return rows, total, err
}
