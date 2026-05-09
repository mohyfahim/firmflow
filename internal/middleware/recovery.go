package middleware

import (
	"fmt"

	"firmflow/internal/common/response"
	platformlogger "firmflow/internal/platform/logger"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func Recovery(log *logrus.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		platformlogger.FromContext(c, log).WithFields(logrus.Fields{
			"request_id": c.GetString("request_id"),
			"path":       c.Request.URL.Path,
			"method":     c.Request.Method,
			"panic":      fmt.Sprintf("%v", recovered),
		}).Error("panic recovered")

		c.Abort()
		response.Fail(c, fmt.Errorf("panic: %v", recovered))
	})
}
