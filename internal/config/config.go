package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App       AppConfig
	HTTP      HTTPConfig
	DB        DBConfig
	Auth      AuthConfig
	Mail      MailConfig
	Storage   StorageConfig
	RateLimit RateLimitConfig
}

type AppConfig struct {
	Name string
	Env  string
}

type HTTPConfig struct {
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
	CORSOrigins     []string
}

type DBConfig struct {
	Host            string
	Port            int
	User            string
	Password        string
	Name            string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	AutoMigrate     bool
	TablePrefix     string
}

type AuthConfig struct {
	JWTIssuer          string
	JWTSecret          string
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	BcryptCost         int
	EnableTOTP         bool
	TOTPEncryptionKey  string
	MaxLoginAttempts   int
	LoginAttemptWindow time.Duration
}

type MailConfig struct {
	Provider    string
	FromAddress string
}

type StorageConfig struct {
	Provider               string
	BasePath               string
	Bucket                 string
	FirmwareMaxUploadBytes int64
}

type RateLimitConfig struct {
	Enabled           bool
	RequestsPerMinute int
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		App: AppConfig{
			Name: getEnv("APP_NAME", "firmflow-api"),
			Env:  getEnv("APP_ENV", "development"),
		},
		HTTP: HTTPConfig{
			Port:            getEnv("HTTP_PORT", "8080"),
			ReadTimeout:     getEnvDuration("HTTP_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:    getEnvDuration("HTTP_WRITE_TIMEOUT", 15*time.Second),
			ShutdownTimeout: getEnvDuration("HTTP_SHUTDOWN_TIMEOUT", 15*time.Second),
			CORSOrigins:     getEnvCSV("CORS_ALLOWED_ORIGINS", []string{"*"}),
		},
		DB: DBConfig{
			Host:            getEnv("DB_HOST", "localhost"),
			Port:            getEnvInt("DB_PORT", 5432),
			User:            getEnv("DB_USER", "postgres"),
			Password:        getEnv("DB_PASSWORD", "postgres"),
			Name:            getEnv("DB_NAME", "firmflow"),
			SSLMode:         getEnv("DB_SSL_MODE", "disable"),
			MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 15*time.Minute),
			ConnMaxIdleTime: getEnvDuration("DB_CONN_MAX_IDLE_TIME", 5*time.Minute),
			AutoMigrate:     getEnvBool("DB_AUTO_MIGRATE", false),
			TablePrefix:     getEnv("DB_TABLE_PREFIX", ""),
		},
		Auth: AuthConfig{
			JWTIssuer:          getEnv("AUTH_JWT_ISSUER", "firmflow"),
			JWTSecret:          getEnv("AUTH_JWT_SECRET", "change-me-in-production"),
			AccessTokenTTL:     getEnvDuration("AUTH_ACCESS_TOKEN_TTL", 15*time.Minute),
			RefreshTokenTTL:    getEnvDuration("AUTH_REFRESH_TOKEN_TTL", 720*time.Hour),
			BcryptCost:         getEnvInt("AUTH_BCRYPT_COST", 12),
			EnableTOTP:         getEnvBool("AUTH_ENABLE_TOTP", true),
			TOTPEncryptionKey:  getEnv("AUTH_TOTP_ENCRYPTION_KEY", "01234567890123456789012345678901"),
			MaxLoginAttempts:   getEnvInt("AUTH_MAX_LOGIN_ATTEMPTS", 5),
			LoginAttemptWindow: getEnvDuration("AUTH_LOGIN_ATTEMPT_WINDOW", 15*time.Minute),
		},
		Mail: MailConfig{
			Provider:    getEnv("MAIL_PROVIDER", "noop"),
			FromAddress: getEnv("MAIL_FROM_ADDRESS", "noreply@firmflow.local"),
		},
		Storage: StorageConfig{
			Provider:               getEnv("STORAGE_PROVIDER", "local"),
			BasePath:               getEnv("STORAGE_BASE_PATH", "./tmp/storage"),
			Bucket:                 getEnv("STORAGE_BUCKET", ""),
			FirmwareMaxUploadBytes: getEnvInt64("FIRMWARE_MAX_UPLOAD_BYTES", 64<<20),
		},
		RateLimit: RateLimitConfig{
			Enabled:           getEnvBool("RATE_LIMIT_ENABLED", true),
			RequestsPerMinute: getEnvInt("RATE_LIMIT_REQUESTS_PER_MINUTE", 120),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Validate() error {
	if c.HTTP.Port == "" {
		return fmt.Errorf("HTTP_PORT must not be empty")
	}
	if c.DB.Host == "" || c.DB.User == "" || c.DB.Name == "" {
		return fmt.Errorf("database host, user, and name are required")
	}
	if c.Storage.FirmwareMaxUploadBytes < 0 {
		return fmt.Errorf("FIRMWARE_MAX_UPLOAD_BYTES must not be negative")
	}
	return nil
}

func getEnv(key, fallback string) string {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	return val
}

func getEnvInt64(key string, fallback int64) int64 {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	val, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return val
}

func getEnvInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return val
}

func getEnvBool(key string, fallback bool) bool {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	val, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return val
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	val, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return val
}

func getEnvCSV(key string, fallback []string) []string {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	return splitAndTrim(raw)
}
