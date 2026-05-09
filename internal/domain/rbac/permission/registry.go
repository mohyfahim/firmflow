package permission

// Central permission registry keys for project-scoped RBAC.
const (
	ProjectRead       = "project.read"
	ProjectUpdate     = "project.update"
	ProjectDelete     = "project.delete"
	MemberRead        = "member.read"
	MemberInvite      = "member.invite"
	MemberUpdateRole  = "member.update_role"
	MemberRemove      = "member.remove"
	RoleRead          = "role.read"
	RoleCreate        = "role.create"
	RoleUpdate        = "role.update"
	RoleDelete        = "role.delete"
	DeviceRead        = "device.read"
	DeviceCreate      = "device.create"
	DeviceUpdate      = "device.update"
	DeviceBlock       = "device.block"
	DeviceTokenRotate = "device.token.rotate"
	DeviceAssignGroup = "device.assign_group"
	FirmwareRead      = "firmware.read"
	FirmwareUpload    = "firmware.upload"
	CampaignRead      = "campaign.read"
	CampaignCreate    = "campaign.create"
	CampaignUpdate    = "campaign.update"
	CampaignPause     = "campaign.pause"
	CampaignCancel    = "campaign.cancel"
	AuditRead         = "audit.read"
	DashboardRead     = "dashboard.read"
)

// All returns every registered permission key (for seeding and validation).
func All() []string {
	return []string{
		ProjectRead, ProjectUpdate, ProjectDelete,
		MemberRead, MemberInvite, MemberUpdateRole, MemberRemove,
		RoleRead, RoleCreate, RoleUpdate, RoleDelete,
		DeviceRead, DeviceCreate, DeviceUpdate, DeviceBlock, DeviceTokenRotate, DeviceAssignGroup,
		FirmwareRead, FirmwareUpload,
		CampaignRead, CampaignCreate, CampaignUpdate, CampaignPause, CampaignCancel,
		AuditRead, DashboardRead,
	}
}
