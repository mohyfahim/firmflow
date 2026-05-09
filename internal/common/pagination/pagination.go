package pagination

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

type Params struct {
	Page     int
	PageSize int
	Sort     string
}

type Meta struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalItems int64 `json:"total_items"`
	TotalPages int64 `json:"total_pages"`
}

func FromQuery(c *gin.Context) Params {
	page := parseIntOrDefault(c.Query("page"), 1)
	if page < 1 {
		page = 1
	}
	pageSize := parseIntOrDefault(c.Query("page_size"), 20)
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	return Params{
		Page:     page,
		PageSize: pageSize,
		Sort:     c.DefaultQuery("sort", "-created_at"),
	}
}

func BuildMeta(page, pageSize int, total int64) Meta {
	totalPages := total / int64(pageSize)
	if total%int64(pageSize) != 0 {
		totalPages++
	}
	return Meta{
		Page:       page,
		PageSize:   pageSize,
		TotalItems: total,
		TotalPages: totalPages,
	}
}

func parseIntOrDefault(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}
