package handlers

import (
	apperrors "firmflow/internal/common/errors"
	"firmflow/internal/common/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type HealthHandler struct {
	db *gorm.DB
}

func NewHealthHandler(db *gorm.DB) *HealthHandler {
	return &HealthHandler{db: db}
}

func (h *HealthHandler) Liveness(c *gin.Context) {
	response.OK(c, gin.H{
		"status": "ok",
	})
}

func (h *HealthHandler) Readiness(c *gin.Context) {
	sqlDB, err := h.db.DB()
	if err != nil {
		response.Fail(c, apperrors.New("db_unavailable", "database is not ready", 503, nil))
		return
	}
	if err := sqlDB.PingContext(c.Request.Context()); err != nil {
		response.Fail(c, apperrors.New("db_unavailable", "database is not ready", 503, nil))
		return
	}

	response.OK(c, gin.H{
		"status": "ready",
	})
}
