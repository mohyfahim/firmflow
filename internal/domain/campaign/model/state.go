package model

// Campaign lifecycle statuses.
const (
	StatusDraft     = "draft"
	StatusScheduled = "scheduled"
	StatusActive    = "active"
	StatusPaused    = "paused"
	StatusCancelled = "cancelled"
	StatusCompleted = "completed"
)

// RolloutKind controls how/when a campaign becomes active and how targets are selected.
const (
	RolloutKindImmediate     = "immediate"      // activate as soon as created (respecting percentage slice)
	RolloutKindTimeScheduled = "time_scheduled" // activate when scheduled_start_at <= now (UTC)
	RolloutKindPercentage    = "percentage"     // percentage slice; schedule optional like immediate
)

// AssignmentStatus tracks per-device progress within a campaign.
const (
	AssignmentPending    = "pending"
	AssignmentOffered    = "offered"
	AssignmentDownloaded = "downloaded"
	AssignmentInstalled  = "installed"
	AssignmentFailed     = "failed"
)

// TerminalAssignment returns true when no further OTA progress is expected.
func TerminalAssignment(status string) bool {
	switch status {
	case AssignmentInstalled, AssignmentFailed:
		return true
	default:
		return false
	}
}

// CanTransition enforces the campaign state machine.
func CanTransition(from, to string) bool {
	if from == to {
		return true
	}
	switch from {
	case StatusDraft:
		return to == StatusScheduled || to == StatusActive || to == StatusCancelled
	case StatusScheduled:
		return to == StatusActive || to == StatusCancelled
	case StatusActive:
		return to == StatusPaused || to == StatusCancelled || to == StatusCompleted
	case StatusPaused:
		return to == StatusActive || to == StatusCancelled
	case StatusCompleted, StatusCancelled:
		return false
	default:
		return false
	}
}

// IsBlockingProjectDelete reports whether this status should block project soft-delete.
func IsBlockingProjectDelete(status string) bool {
	return status != StatusCompleted && status != StatusCancelled
}
