package control

import (
	"strings"

	"reasonix/internal/event"
	"reasonix/internal/i18n"
)

func normalizeSubagentClearState(state string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "", "completed":
		return "completed", true
	case "failed":
		return "failed", true
	case "canceled":
		return "canceled", true
	case "all":
		return "all", true
	default:
		return "", false
	}
}

// SubagentStateText returns the localized text shown to users for a subagent state.
func SubagentStateText(state event.SubagentState) string {
	switch state {
	case event.SubagentCompleted:
		return i18n.M.SubagentStateCompleted
	case event.SubagentFailed:
		return i18n.M.SubagentStateFailed
	case event.SubagentCanceled:
		return i18n.M.SubagentStateCanceled
	case event.SubagentRunning:
		return i18n.M.SubagentStateRunning
	default:
		return i18n.M.SubagentStateUnknown
	}
}

func subagentClearStateText(state string) string {
	switch state {
	case "completed":
		return SubagentStateText(event.SubagentCompleted)
	case "failed":
		return SubagentStateText(event.SubagentFailed)
	case "canceled":
		return SubagentStateText(event.SubagentCanceled)
	case "all":
		return i18n.M.SubagentsClearAllLabel
	default:
		return state
	}
}

func subagentClearStateHint(state string) string {
	switch state {
	case "completed":
		return i18n.M.ArgSubagentsClearCompleted
	case "failed":
		return i18n.M.ArgSubagentsClearFailed
	case "canceled":
		return i18n.M.ArgSubagentsClearCanceled
	case "all":
		return i18n.M.ArgSubagentsClearAll
	default:
		return state
	}
}
