package middleware

import (
	"time"

	platformlogger "firmflow/internal/platform/logger"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func RequestLogger(log *logrus.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		entry := log.WithFields(logrus.Fields{
			"request_id": c.GetString("request_id"),
			"method":     c.Request.Method,
			"path":       c.Request.URL.Path,
		})
		platformlogger.IntoContext(c, entry)
		c.Next()
	}
}

func Logging(log *logrus.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		entry := platformlogger.FromContext(c, log).WithFields(logrus.Fields{
			"status":     c.Writer.Status(),
			"latency_ms": time.Since(start).Milliseconds(),
			"client_ip":  c.ClientIP(),
		})

		if len(c.Errors) > 0 {
			entry.WithField("error", c.Errors.String()).Error("request completed with errors")
			return
		}
		entry.Info("request completed")
	}
}
