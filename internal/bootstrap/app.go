package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"firmflow/internal/config"
	"firmflow/internal/database"
	authmodel "firmflow/internal/domain/auth/model"
	authrepo "firmflow/internal/domain/auth/repository"
	authsvc "firmflow/internal/domain/auth/service"
	devicemodel "firmflow/internal/domain/device/model"
	devicerepo "firmflow/internal/domain/device/repository"
	devicesvc "firmflow/internal/domain/device/service"
	firmwaremodel "firmflow/internal/domain/firmware/model"
	firmwarerepo "firmflow/internal/domain/firmware/repository"
	firmwaresvc "firmflow/internal/domain/firmware/service"
	projectmodel "firmflow/internal/domain/project/model"
	rbacmodel "firmflow/internal/domain/rbac/model"
	rbacrepo "firmflow/internal/domain/rbac/repository"
	rbacsvc "firmflow/internal/domain/rbac/service"
	"firmflow/internal/middleware"
	"firmflow/internal/platform/logger"
	"firmflow/internal/platform/mailer"
	"firmflow/internal/platform/storage"
	"firmflow/internal/transport/http/handlers"
	"firmflow/internal/transport/http/routes"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type App struct {
	Config     *config.Config
	Logger     *logrus.Logger
	DB         *gorm.DB
	Container  *Container
	HTTPServer *http.Server
}

type Container struct {
	HealthHandler   *handlers.HealthHandler
	AuthHandler     *handlers.AuthHandler
	AuthService     *authsvc.Service
	ProjectHandler  *handlers.ProjectHandler
	DeviceHandler   *handlers.DeviceHandler
	DeviceService   *devicesvc.Service
	FirmwareHandler *handlers.FirmwareHandler
	Authorizer      *rbacsvc.Authorizer
}

func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	log := logger.New(cfg.App.Env)
	db, err := database.NewPostgres(cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}

	engine := gin.New()
	engine.Use(middleware.RequestID())
	engine.Use(middleware.RequestLogger(log))
	engine.Use(middleware.Recovery(log))
	engine.Use(middleware.Logging(log))
	engine.Use(middleware.CORS(cfg.HTTP.CORSOrigins))
	engine.Use(middleware.ErrorHandler())

	healthHandler := handlers.NewHealthHandler(db)
	authRepository := authrepo.New(db)
	authService := authsvc.New(cfg.Auth, authRepository, mailer.NoopMailer{})
	authHandler := handlers.NewAuthHandler(authService)
	rbacRepository := rbacrepo.New(db)
	rbacAuthorizer := rbacsvc.NewAuthorizer(rbacRepository)
	projectService := rbacsvc.NewProjectService(rbacRepository, authRepository, rbacAuthorizer)
	projectHandler := handlers.NewProjectHandler(projectService)

	deviceRepository := devicerepo.New(db)
	deviceService := devicesvc.New(rbacRepository, authRepository, rbacAuthorizer, deviceRepository)
	deviceHandler := handlers.NewDeviceHandler(deviceService)

	objectStore, err := storage.NewLocalObjectStore(cfg.Storage.BasePath)
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}
	if p := strings.ToLower(strings.TrimSpace(cfg.Storage.Provider)); p != "" && p != "local" {
		return nil, fmt.Errorf("unsupported STORAGE_PROVIDER %q (only local is implemented)", cfg.Storage.Provider)
	}

	firmwareRepository := firmwarerepo.New(db)
	firmwareService := firmwaresvc.New(
		rbacRepository,
		authRepository,
		rbacAuthorizer,
		firmwareRepository,
		deviceRepository,
		objectStore,
		cfg.Storage.FirmwareMaxUploadBytes,
		cfg.Storage.Provider,
	)
	firmwareHandler := handlers.NewFirmwareHandler(firmwareService)

	container := &Container{
		HealthHandler:   healthHandler,
		AuthHandler:     authHandler,
		AuthService:     authService,
		ProjectHandler:  projectHandler,
		DeviceHandler:   deviceHandler,
		DeviceService:   deviceService,
		FirmwareHandler: firmwareHandler,
		Authorizer:      rbacAuthorizer,
	}
	routes.Register(engine, routes.Deps{
		Health:       container.HealthHandler,
		Auth:         container.AuthHandler,
		AuthMW:       middleware.RequireAuth(container.AuthService),
		Project:      container.ProjectHandler,
		Device:       container.DeviceHandler,
		Firmware:     container.FirmwareHandler,
		DeviceAuthMW: middleware.RequireDeviceAuth(deviceRepository),
		Authorizer:   container.Authorizer,
	})

	if cfg.DB.AutoMigrate {
		migrator := database.CompositeMigrator{
			Migrators: []database.Migrator{
				authmodel.Migrator{},
				rbacmodel.Migrator{},
				projectmodel.Migrator{},
				devicemodel.Migrator{},
				firmwaremodel.Migrator{},
			},
		}
		if err := database.RunMigrations(context.Background(), db, migrator, log); err != nil {
			return nil, fmt.Errorf("run migrations: %w", err)
		}
	}

	server := &http.Server{
		Addr:         ":" + cfg.HTTP.Port,
		Handler:      engine,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
	}

	return &App{
		Config:     cfg,
		Logger:     log,
		DB:         db,
		Container:  container,
		HTTPServer: server,
	}, nil
}

func (a *App) Close() error {
	sqlDB, err := a.DB.DB()
	if err != nil {
		return nil
	}
	return sqlDB.Close()
}
