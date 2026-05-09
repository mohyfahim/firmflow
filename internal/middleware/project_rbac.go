package middleware

import (
	apperrors "firmflow/internal/common/errors"
	"firmflow/internal/common/response"
	rbacsvc "firmflow/internal/domain/rbac/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequireProjectPermission ensures the authenticated user is a project member with the given permission key.
func RequireProjectPermission(authorizer *rbacsvc.Authorizer, permissionKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		pid, err := uuid.Parse(c.Param("projectID"))
		if err != nil {
			response.Fail(c, apperrors.BadRequest("invalid project id", nil))
			c.Abort()
			return
		}
		uid, err := uuid.Parse(c.GetString("auth_user_id"))
		if err != nil {
			response.Fail(c, apperrors.Unauthorized())
			c.Abort()
			return
		}
		m, err := authorizer.AuthorizeProject(c.Request.Context(), pid, uid, permissionKey)
		if err != nil {
			response.Fail(c, err)
			c.Abort()
			return
		}
		c.Set("project_membership", m)
		c.Next()
	}
}
