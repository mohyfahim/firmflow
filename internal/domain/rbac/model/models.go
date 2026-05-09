package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Permission is a registry row (global, not project-scoped).
type Permission struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	Key         string    `gorm:"size:128;uniqueIndex;not null"`
	Description string    `gorm:"size:512"`
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`
}

// Role is either a global predefined template (ProjectID nil, IsPredefined true, Slug set)
// or a custom project role (ProjectID set, IsPredefined false, unique name per project).
type Role struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey"`
	ProjectID        *uuid.UUID     `gorm:"type:uuid;index"`
	Slug             string         `gorm:"size:32;index"` // predefined: owner|admin|developer|viewer; empty for custom
	Name             string         `gorm:"size:128;not null"`
	NameNormalized   string         `gorm:"size:128;not null;index"`
	IsPredefined     bool           `gorm:"not null;default:false;index"`
	Description      string         `gorm:"size:512"`
	Permissions      []Permission   `gorm:"many2many:role_permissions;"`
	CreatedAt        time.Time      `gorm:"not null"`
	UpdatedAt        time.Time      `gorm:"not null"`
	DeletedAt        gorm.DeletedAt `gorm:"index"`
}

// ProjectMembership binds one user to one project with exactly one effective role.
type ProjectMembership struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	ProjectID uuid.UUID `gorm:"type:uuid;uniqueIndex:ux_project_member;not null"`
	UserID    uuid.UUID `gorm:"type:uuid;uniqueIndex:ux_project_member;not null;index"`
	RoleID    uuid.UUID `gorm:"type:uuid;index;not null"`
	Project   Project   `gorm:"foreignKey:ProjectID"`
	Role      Role      `gorm:"foreignKey:RoleID"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (p *Permission) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

func (r *Role) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

func (m *ProjectMembership) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}
