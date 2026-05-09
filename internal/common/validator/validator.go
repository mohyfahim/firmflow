package validator

import (
	apperrors "firmflow/internal/common/errors"

	"github.com/gin-gonic/gin"
)

func BindJSON(c *gin.Context, dst interface{}) error {
	if err := c.ShouldBindJSON(dst); err != nil {
		return apperrors.BadRequest("invalid request body", err.Error())
	}
	return nil
}
