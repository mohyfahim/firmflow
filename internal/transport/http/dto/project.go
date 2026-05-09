package dto

type CreateProjectRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

type UpdateProjectRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

type InviteMemberRequest struct {
	Email  string `json:"email" binding:"required,email"`
	RoleID string `json:"role_id" binding:"required"`
}

type UpdateMemberRoleRequest struct {
	RoleID string `json:"role_id" binding:"required"`
}

type TransferOwnershipRequest struct {
	NewOwnerUserID string `json:"new_owner_user_id" binding:"required"`
}

type CreateCustomRoleRequest struct {
	Name           string   `json:"name" binding:"required"`
	Description    string   `json:"description"`
	PermissionKeys []string `json:"permission_keys" binding:"required,min=1,dive,required"`
}

type UpdateCustomRoleRequest struct {
	Name           *string  `json:"name"`
	Description    *string  `json:"description"`
	PermissionKeys []string `json:"permission_keys"`
}
