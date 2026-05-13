package routes

import (
	rbacperm "firmflow/internal/domain/rbac/permission"
	rbacsvc "firmflow/internal/domain/rbac/service"
	"firmflow/internal/middleware"
	"firmflow/internal/transport/http/handlers"

	"github.com/gin-gonic/gin"
)

type Deps struct {
	Health       *handlers.HealthHandler
	Auth         *handlers.AuthHandler
	AuthMW       gin.HandlerFunc
	Project      *handlers.ProjectHandler
	Device       *handlers.DeviceHandler
	Firmware     *handlers.FirmwareHandler
	DeviceAuthMW gin.HandlerFunc
	Authorizer   *rbacsvc.Authorizer
}

func Register(router *gin.Engine, deps Deps) {
	health := router.Group("/health")
	{
		health.GET("/live", deps.Health.Liveness)
		health.GET("/ready", deps.Health.Readiness)
	}

	api := router.Group("/api/v1")
	auth := api.Group("/auth")
	{
		auth.POST("/register", deps.Auth.Register)
		auth.POST("/verify-email", deps.Auth.VerifyEmail)
		auth.POST("/login", deps.Auth.Login)
		auth.POST("/refresh", deps.Auth.Refresh)
		auth.POST("/logout", deps.Auth.Logout)
		auth.POST("/forgot-password", deps.Auth.ForgotPassword)
		auth.POST("/reset-password", deps.Auth.ResetPassword)
	}

	me := api.Group("/me")
	me.Use(deps.AuthMW)
	{
		me.GET("/profile", deps.Auth.GetProfile)
		me.PATCH("/profile", deps.Auth.UpdateProfile)
		me.POST("/change-password", deps.Auth.ChangePassword)
		me.GET("/sessions", deps.Auth.ListSessions)
		me.DELETE("/sessions/others", deps.Auth.RevokeOtherSessions)
		me.DELETE("/sessions/:sessionID", deps.Auth.RevokeSession)
		me.POST("/2fa/enable", deps.Auth.BeginEnable2FA)
		me.POST("/2fa/confirm", deps.Auth.ConfirmEnable2FA)
		me.POST("/2fa/disable", deps.Auth.Disable2FA)
		me.DELETE("", deps.Auth.DeleteAccount)
	}

	protected := api.Group("")
	protected.Use(deps.AuthMW)
	{
		protected.POST("/projects", deps.Project.CreateProject)
		protected.GET("/projects", deps.Project.ListProjects)

		pg := protected.Group("/projects/:projectID")
		pg.GET("", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.ProjectRead), deps.Project.GetProject)
		pg.GET("/summary", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.DashboardRead), deps.Project.GetProjectSummary)
		pg.PATCH("", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.ProjectUpdate), deps.Project.UpdateProject)
		pg.DELETE("", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.ProjectDelete), deps.Project.DeleteProject)
		pg.POST("/archive", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.ProjectUpdate), deps.Project.ArchiveProject)
		pg.POST("/unarchive", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.ProjectUpdate), deps.Project.UnarchiveProject)
		pg.GET("/audit-logs", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.AuditRead), deps.Project.ListProjectAuditLogs)
		pg.GET("/members", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.MemberRead), deps.Project.ListMembers)
		pg.POST("/members", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.MemberInvite), deps.Project.InviteMember)
		pg.PATCH("/members/:userID", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.MemberUpdateRole), deps.Project.UpdateMemberRole)
		pg.DELETE("/members/:userID", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.MemberRemove), deps.Project.RemoveMember)
		pg.POST("/ownership/transfer", deps.Project.TransferOwnership)

		pg.GET("/roles", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.RoleRead), deps.Project.ListProjectRoles)
		pg.GET("/roles/assignable", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.MemberRead), deps.Project.ListAssignableRoles)
		pg.POST("/roles", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.RoleCreate), deps.Project.CreateCustomRole)
		pg.PATCH("/roles/:roleID", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.RoleUpdate), deps.Project.UpdateCustomRole)
		pg.DELETE("/roles/:roleID", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.RoleDelete), deps.Project.DeleteCustomRole)

		// Device types and devices (device.* permissions)
		pg.GET("/device-types", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.DeviceRead), deps.Device.ListDeviceTypes)
		pg.POST("/device-types", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.DeviceCreate), deps.Device.CreateCustomDeviceType)
		pg.PATCH("/device-types/:deviceTypeID", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.DeviceUpdate), deps.Device.UpdateCustomDeviceType)
		pg.DELETE("/device-types/:deviceTypeID", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.DeviceUpdate), deps.Device.DeleteCustomDeviceType)

		pg.GET("/device-groups", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.DeviceRead), deps.Device.ListDeviceGroups)
		pg.POST("/device-groups", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.DeviceUpdate), deps.Device.CreateDeviceGroup)
		pg.PATCH("/device-groups/:groupID", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.DeviceUpdate), deps.Device.UpdateDeviceGroup)
		pg.DELETE("/device-groups/:groupID", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.DeviceUpdate), deps.Device.DeleteDeviceGroup)
		pg.POST("/device-groups/:groupID/members", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.DeviceAssignGroup), deps.Device.AddDevicesToGroup)
		pg.POST("/device-groups/:groupID/members/remove", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.DeviceAssignGroup), deps.Device.RemoveDevicesFromGroup)

		pg.GET("/devices", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.DeviceRead), deps.Device.ListDevices)
		// Bulk: service enforces action-specific permissions (assign_group / block / token_rotate).
		pg.POST("/devices/bulk", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.DeviceRead), deps.Device.BulkDevices)
		pg.POST("/devices", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.DeviceCreate), deps.Device.RegisterDevice)
		pg.GET("/devices/:deviceID", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.DeviceRead), deps.Device.GetDeviceTwin)
		pg.POST("/devices/:deviceID/block", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.DeviceBlock), deps.Device.BlockDevice)
		pg.POST("/devices/:deviceID/unblock", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.DeviceBlock), deps.Device.UnblockDevice)
		pg.POST("/devices/:deviceID/rotate-token", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.DeviceTokenRotate), deps.Device.RotateDeviceToken)

		pg.GET("/firmwares", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.FirmwareRead), deps.Firmware.ListFirmware)
		pg.POST("/firmwares", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.FirmwareUpload), deps.Firmware.UploadFirmware)
		pg.GET("/firmwares/:firmwareID", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.FirmwareRead), deps.Firmware.GetFirmware)
		pg.GET("/firmwares/:firmwareID/download", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.FirmwareRead), deps.Firmware.DownloadFirmware)
		pg.DELETE("/firmwares/:firmwareID", middleware.RequireProjectPermission(deps.Authorizer, rbacperm.FirmwareUpload), deps.Firmware.DeleteFirmware)
	}

	// Device-facing endpoints (device auth only).
	deviceAPI := api.Group("/device")
	deviceAPI.Use(deps.DeviceAuthMW)
	{
		deviceAPI.POST("/poll", deps.Device.DevicePoll)
		deviceAPI.POST("/report", deps.Device.DeviceReport)
	}
}
