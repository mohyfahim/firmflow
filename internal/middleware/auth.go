package middleware

import (
	"strings"

	apperrors "firmflow/internal/common/errors"
	"firmflow/internal/common/response"
	"firmflow/internal/domain/auth/service"

	"github.com/gin-gonic/gin"
)

func RequireAuth(authService *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		authz := c.GetHeader("Authorization")
		if !strings.HasPrefix(authz, "Bearer ") {
			response.Fail(c, apperrors.Unauthorized())
			c.Abort()
			return
		}
		token := strings.TrimPrefix(authz, "Bearer ")
		claims, err := authService.ParseAccessToken(token)
		if err != nil {
			response.Fail(c, apperrors.Unauthorized())
			c.Abort()
			return
		}
		c.Set("auth_user_id", claims.UserID)
		c.Set("auth_session_id", claims.SessionID)
		c.Next()
	}
}
