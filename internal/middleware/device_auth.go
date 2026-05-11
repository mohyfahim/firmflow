package middleware

import (
	"strings"

	apperrors "firmflow/internal/common/errors"
	"firmflow/internal/common/response"
	security "firmflow/internal/domain/auth/security"
	devicerepo "firmflow/internal/domain/device/repository"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequireDeviceAuth authenticates a device token for device-facing endpoints.
// Header format: Authorization: Device <raw_token>
func RequireDeviceAuth(deviceRepo *devicerepo.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		authz := c.GetHeader("Authorization")
		if !strings.HasPrefix(authz, "Device ") {
			response.Fail(c, apperrors.Unauthorized())
			c.Abort()
			return
		}
		raw := strings.TrimSpace(strings.TrimPrefix(authz, "Device "))
		if raw == "" {
			response.Fail(c, apperrors.Unauthorized())
			c.Abort()
			return
		}

		tokenHash := security.HashToken(raw)
		device, _, err := deviceRepo.GetDeviceByActiveTokenHash(c.Request.Context(), tokenHash)
		if err != nil {
			// Repository returns errors with stable messages; map them to readable API errors.
			switch err.Error() {
			case "device_blocked":
				response.Fail(c, apperrors.New("device_blocked", "device is blocked", 403, nil))
			case "device_polling_disabled":
				response.Fail(c, apperrors.New("device_polling_disabled", "device polling is disabled", 403, nil))
			case "device_token_revoked":
				response.Fail(c, apperrors.New("device_token_revoked", "device token has been revoked", 401, nil))
			default:
				response.Fail(c, apperrors.Unauthorized())
			}
			c.Abort()
			return
		}

		c.Set("device_id", device.ID.String())
		c.Set("device_project_id", device.ProjectID.String())
		c.Set("device_token_revoked_at", device.TokenRevokedAt) // useful for debugging

		// Defensive: ensure IDs parse (prevents downstream panics).
		_, _ = uuid.Parse(device.ID.String())
		c.Next()
	}
}

