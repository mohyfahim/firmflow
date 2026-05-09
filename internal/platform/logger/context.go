package logger

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

const contextLoggerKey = "request_logger"

func IntoContext(c *gin.Context, entry *logrus.Entry) {
	c.Set(contextLoggerKey, entry)
}

func FromContext(c *gin.Context, base *logrus.Logger) *logrus.Entry {
	if v, ok := c.Get(contextLoggerKey); ok {
		if entry, valid := v.(*logrus.Entry); valid {
			return entry
		}
	}
	return logrus.NewEntry(base)
}
