package routes

import (
	rbacperm "firmflow/internal/domain/rbac/permission"
	rbacsvc "firmflow/internal/domain/rbac/service"
	"firmflow/internal/middleware"
	"firmflow/internal/transport/http/handlers"

	"github.com/gin-gonic/gin"
)

type Deps struct {
	Health     *handlers.HealthHandler
	Auth       *handlers.AuthHandler
	AuthMW     gin.HandlerFunc
	Project    *handlers.ProjectHandler
	Authorizer *rbacsvc.Authorizer
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
	}
}
