package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"firmflow/internal/config"
	"firmflow/internal/database"
	authmodel "firmflow/internal/domain/auth/model"
	authrepo "firmflow/internal/domain/auth/repository"
	authsvc "firmflow/internal/domain/auth/service"
	campaignmodel "firmflow/internal/domain/campaign/model"
	campaignrepo "firmflow/internal/domain/campaign/repository"
	campaignsvc "firmflow/internal/domain/campaign/service"
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
	"firmflow/internal/transport/devotcp"
	"firmflow/internal/transport/http/handlers"
	"firmflow/internal/transport/http/routes"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type App struct {
	Config              *config.Config
	Logger              *logrus.Logger
	DB                  *gorm.DB
	Container           *Container
	HTTPServer          *http.Server
	cancelCampaignSched context.CancelFunc
	cancelDeviceOTA     context.CancelFunc
	otaWG               *sync.WaitGroup
}

type Container struct {
	HealthHandler   *handlers.HealthHandler
	AuthHandler     *handlers.AuthHandler
	AuthService     *authsvc.Service
	ProjectHandler  *handlers.ProjectHandler
	DeviceHandler   *handlers.DeviceHandler
	DeviceService   *devicesvc.Service
	FirmwareHandler *handlers.FirmwareHandler
	CampaignHandler *handlers.CampaignHandler
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

	campaignRepository := campaignrepo.New(db)
	campaignService := campaignsvc.New(
		rbacRepository,
		authRepository,
		rbacAuthorizer,
		campaignRepository,
		firmwareRepository,
		deviceRepository,
	)
	campaignHandler := handlers.NewCampaignHandler(campaignService)
	deviceHandler := handlers.NewDeviceHandler(deviceService, campaignService)

	container := &Container{
		HealthHandler:   healthHandler,
		AuthHandler:     authHandler,
		AuthService:     authService,
		ProjectHandler:  projectHandler,
		DeviceHandler:   deviceHandler,
		DeviceService:   deviceService,
		FirmwareHandler: firmwareHandler,
		CampaignHandler: campaignHandler,
		Authorizer:      rbacAuthorizer,
	}
	routes.Register(engine, routes.Deps{
		Health:       container.HealthHandler,
		Auth:         container.AuthHandler,
		AuthMW:       middleware.RequireAuth(container.AuthService),
		Project:      container.ProjectHandler,
		Device:       container.DeviceHandler,
		Firmware:     container.FirmwareHandler,
		Campaign:     container.CampaignHandler,
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
				campaignmodel.Migrator{},
			},
		}
		if err := database.RunMigrations(context.Background(), db, migrator, log); err != nil {
			return nil, fmt.Errorf("run migrations: %w", err)
		}
	}

	schedCtx, cancelSched := context.WithCancel(context.Background())
	go campaignsvc.RunScheduler(schedCtx, log, campaignService, 30*time.Second)

	var cancelDeviceOTA context.CancelFunc
	var otaWG *sync.WaitGroup

	server := &http.Server{
		Addr:         ":" + cfg.HTTP.Port,
		Handler:      engine,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
	}

	if addr := strings.TrimSpace(cfg.DeviceOTA.TCPListenAddr); addr != "" {
		wg := &sync.WaitGroup{}
		otaWG = wg
		otaHandler := &devotcp.Handler{
			Log:                   log,
			DeviceRepo:            deviceRepository,
			DeviceSvc:             deviceService,
			CampaignSvc:           campaignService,
			FirmwareSvc:           firmwareService,
			PublicDownloadBaseURL: cfg.DeviceOTA.PublicDownloadBaseURL,
			TokenTTL:              cfg.DeviceOTA.DownloadTokenTTL,
		}
		otaCtx, cancelOTA := context.WithCancel(context.Background())
		cancelDeviceOTA = cancelOTA
		otaWG.Add(1)
		go func() {
			defer otaWG.Done()
			if err := devotcp.Serve(otaCtx, log, addr, otaHandler); err != nil {
				log.WithError(err).Error("device OTA tcp server exited")
			}
		}()
	}

	return &App{
		Config:              cfg,
		Logger:              log,
		DB:                  db,
		Container:           container,
		HTTPServer:          server,
		cancelCampaignSched: cancelSched,
		cancelDeviceOTA:     cancelDeviceOTA,
		otaWG:               otaWG,
	}, nil
}

func (a *App) StopSchedulers() {
	if a.cancelCampaignSched != nil {
		a.cancelCampaignSched()
	}
}

func (a *App) StopOTA() {
	if a.cancelDeviceOTA != nil {
		a.cancelDeviceOTA()
	}
	if a.otaWG != nil {
		a.otaWG.Wait()
	}
}

func (a *App) Close() error {
	sqlDB, err := a.DB.DB()
	if err != nil {
		return nil
	}
	return sqlDB.Close()
}
