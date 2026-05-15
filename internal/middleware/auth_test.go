package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"firmflow/internal/config"
	authrepo "firmflow/internal/domain/auth/repository"
	authsvc "firmflow/internal/domain/auth/service"
	"firmflow/internal/platform/mailer"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRequireAuthUnauthorizedWithoutToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _ := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	svc := authsvc.New(config.AuthConfig{
		JWTIssuer:         "test",
		JWTSecret:         "test-secret",
		AccessTokenTTL:    time.Minute,
		RefreshTokenTTL:   time.Hour,
		BcryptCost:        4,
		TOTPEncryptionKey: "01234567890123456789012345678901",
	}, authrepo.New(db), mailer.NoopMailer{}, logrus.New())

	r := gin.New()
	r.Use(RequireAuth(svc))
	r.GET("/private", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
