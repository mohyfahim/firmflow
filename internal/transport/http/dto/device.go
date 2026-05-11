package dto

// Custom device type management.
type CreateCustomDeviceTypeRequest struct {
	Name                  string  `json:"name" binding:"required"`
	ProcessorArchitecture string  `json:"processor_architecture" binding:"required"`
	HardwareBoardVersion  string  `json:"hardware_board_version" binding:"required"`
	FlashSizeBytes        int64   `json:"flash_size_bytes" binding:"required,gt=0"`
	MemoryNotes           *string `json:"memory_notes,omitempty"`
}

type UpdateCustomDeviceTypeRequest struct {
	Name                  *string `json:"name"`
	ProcessorArchitecture *string `json:"processor_architecture"`
	HardwareBoardVersion *string `json:"hardware_board_version"`
	FlashSizeBytes        *int64  `json:"flash_size_bytes"`
	MemoryNotes           *string `json:"memory_notes,omitempty"`
}

// Register device under a project.
type RegisterDeviceRequest struct {
	Name              string `json:"name" binding:"required"`
	DeviceTypeID     string `json:"device_type_id" binding:"required"`
	HardwareIdentifier string `json:"hardware_identifier" binding:"required"`
}

type ReportDeviceRequest struct {
	CurrentFirmwareVersion string `json:"current_firmware_version" binding:"required"`
}

// Device groups
type CreateDeviceGroupRequest struct {
	Name        string  `json:"name" binding:"required"`
	Description *string `json:"description,omitempty"`
}

type UpdateDeviceGroupRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

type DeviceGroupMembersRequest struct {
	DeviceIDs []string `json:"device_ids" binding:"required,min=1,dive,required"`
}

// DeviceFilterBody is reusable for bulk "apply_to_filter" and mirrors list query semantics.
type DeviceFilterBody struct {
	Online            *bool   `json:"online"`
	Blocked           *bool   `json:"blocked"`
	DeviceTypeID      *string `json:"device_type_id"`
	GroupID           *string `json:"group_id"`
	FirmwareVersion   *string `json:"firmware_version"`
	LastSeenFrom      *string `json:"last_seen_from"` // RFC3339
	LastSeenTo        *string `json:"last_seen_to"`
	Search            *string `json:"q"`
}

type BulkDevicesRequest struct {
	Action          string           `json:"action" binding:"required,oneof=add_to_group remove_from_group block unblock rotate_tokens"`
	ApplyToFilter   bool             `json:"apply_to_filter"`
	Filter          DeviceFilterBody `json:"filter"`
	DeviceIDs       []string         `json:"device_ids"`
	GroupID         *string          `json:"group_id"`
}

