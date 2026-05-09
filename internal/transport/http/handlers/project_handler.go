package handlers

import (
	"time"

	apperrors "firmflow/internal/common/errors"
	"firmflow/internal/common/pagination"
	"firmflow/internal/common/response"
	"firmflow/internal/common/validator"
	rbacsvc "firmflow/internal/domain/rbac/service"
	"firmflow/internal/transport/http/dto"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ProjectHandler struct {
	svc *rbacsvc.ProjectService
}

func NewProjectHandler(svc *rbacsvc.ProjectService) *ProjectHandler {
	return &ProjectHandler{svc: svc}
}

func parseUserID(c *gin.Context) (uuid.UUID, error) {
	return uuid.Parse(c.GetString("auth_user_id"))
}

func (h *ProjectHandler) CreateProject(c *gin.Context) {
	var req dto.CreateProjectRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	p, err := h.svc.CreateProject(c.Request.Context(), uid, req.Name, req.Description)
	if err != nil {
		c.Error(err)
		return
	}
	response.Created(c, p)
}

func (h *ProjectHandler) ListProjects(c *gin.Context) {
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	params := pagination.FromQuery(c)
	items, total, err := h.svc.ListMyProjects(c.Request.Context(), uid, c.Query("q"), c.Query("sort"), params.Page, params.PageSize)
	if err != nil {
		c.Error(err)
		return
	}
	meta := pagination.BuildMeta(params.Page, params.PageSize, total)
	response.WithMeta(c, 200, items, meta)
}

func (h *ProjectHandler) GetProject(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	detail, err := h.svc.GetProjectDetail(c.Request.Context(), uid, projectID)
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, detail)
}

func (h *ProjectHandler) GetProjectSummary(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	sum, err := h.svc.GetProjectSummary(c.Request.Context(), uid, projectID)
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, sum)
}

func (h *ProjectHandler) UpdateProject(c *gin.Context) {
	var req dto.UpdateProjectRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	if err := h.svc.UpdateProject(c.Request.Context(), uid, projectID, req.Name, req.Description); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "project updated"})
}

func (h *ProjectHandler) DeleteProject(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	if err := h.svc.DeleteProject(c.Request.Context(), uid, projectID); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "project deleted"})
}

func (h *ProjectHandler) ArchiveProject(c *gin.Context) {
	h.setArchived(c, true)
}

func (h *ProjectHandler) UnarchiveProject(c *gin.Context) {
	h.setArchived(c, false)
}

func (h *ProjectHandler) setArchived(c *gin.Context, archive bool) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	if err := h.svc.ArchiveProject(c.Request.Context(), uid, projectID, archive); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"archived": archive})
}

func (h *ProjectHandler) ListProjectAuditLogs(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	params := pagination.FromQuery(c)
	var actorFilter *uuid.UUID
	if raw := c.Query("actor_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			c.Error(apperrors.BadRequest("invalid actor_id", nil))
			return
		}
		actorFilter = &id
	}
	eventPrefix := c.Query("event")
	var from, to *time.Time
	if raw := c.Query("from"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			c.Error(apperrors.BadRequest("invalid from (RFC3339)", nil))
			return
		}
		from = &t
	}
	if raw := c.Query("to"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			c.Error(apperrors.BadRequest("invalid to (RFC3339)", nil))
			return
		}
		to = &t
	}
	logs, total, err := h.svc.ListProjectAuditLogs(c.Request.Context(), uid, projectID, actorFilter, eventPrefix, from, to, params.Page, params.PageSize)
	if err != nil {
		c.Error(err)
		return
	}
	meta := pagination.BuildMeta(params.Page, params.PageSize, total)
	response.WithMeta(c, 200, logs, meta)
}

func (h *ProjectHandler) ListMembers(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	list, err := h.svc.ListMembers(c.Request.Context(), uid, projectID)
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, list)
}

func (h *ProjectHandler) InviteMember(c *gin.Context) {
	var req dto.InviteMemberRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	roleID, err := uuid.Parse(req.RoleID)
	if err != nil {
		c.Error(apperrors.BadRequest("invalid role id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	if err := h.svc.InviteMember(c.Request.Context(), uid, projectID, req.Email, roleID); err != nil {
		c.Error(err)
		return
	}
	response.Created(c, gin.H{"message": "member invited"})
}

func (h *ProjectHandler) UpdateMemberRole(c *gin.Context) {
	var req dto.UpdateMemberRoleRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	targetUserID, err := uuid.Parse(c.Param("userID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid user id", nil))
		return
	}
	roleID, err := uuid.Parse(req.RoleID)
	if err != nil {
		c.Error(apperrors.BadRequest("invalid role id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	if err := h.svc.UpdateMemberRole(c.Request.Context(), uid, projectID, targetUserID, roleID); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "member updated"})
}

func (h *ProjectHandler) RemoveMember(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	targetUserID, err := uuid.Parse(c.Param("userID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid user id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	if err := h.svc.RemoveMember(c.Request.Context(), uid, projectID, targetUserID); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "member removed"})
}

func (h *ProjectHandler) TransferOwnership(c *gin.Context) {
	var req dto.TransferOwnershipRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	newOwner, err := uuid.Parse(req.NewOwnerUserID)
	if err != nil {
		c.Error(apperrors.BadRequest("invalid new owner id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	if err := h.svc.TransferOwnership(c.Request.Context(), uid, projectID, newOwner); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "ownership transferred"})
}

func (h *ProjectHandler) ListAssignableRoles(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	roles, err := h.svc.ListAssignableRoles(c.Request.Context(), uid, projectID)
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, roles)
}

func (h *ProjectHandler) ListProjectRoles(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	payload, err := h.svc.ListProjectRoles(c.Request.Context(), uid, projectID)
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, payload)
}

func (h *ProjectHandler) CreateCustomRole(c *gin.Context) {
	var req dto.CreateCustomRoleRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	role, err := h.svc.CreateCustomRole(c.Request.Context(), uid, projectID, req.Name, req.Description, req.PermissionKeys)
	if err != nil {
		c.Error(err)
		return
	}
	response.Created(c, role)
}

func (h *ProjectHandler) UpdateCustomRole(c *gin.Context) {
	var req dto.UpdateCustomRoleRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	roleID, err := uuid.Parse(c.Param("roleID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid role id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	role, err := h.svc.UpdateCustomRole(c.Request.Context(), uid, projectID, roleID, req.Name, req.Description, req.PermissionKeys)
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, role)
}

func (h *ProjectHandler) DeleteCustomRole(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid project id", nil))
		return
	}
	roleID, err := uuid.Parse(c.Param("roleID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid role id", nil))
		return
	}
	uid, err := parseUserID(c)
	if err != nil {
		c.Error(apperrors.Unauthorized())
		return
	}
	if err := h.svc.DeleteCustomRole(c.Request.Context(), uid, projectID, roleID); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "role deleted"})
}
