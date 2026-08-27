package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/seonl/agentview/internal/events"
	"github.com/seonl/agentview/internal/git"
	"github.com/seonl/agentview/internal/state"
)

// Color carries state, never decoration, and never meaning on its own: every
// styled element is also distinguishable by symbol or position.
var (
	styleTitle    = lipgloss.NewStyle().Bold(true)
	styleLabel    = lipgloss.NewStyle().Faint(true)
	styleValue    = lipgloss.NewStyle().Bold(true)
	styleDim      = lipgloss.NewStyle().Faint(true)
	styleRule     = lipgloss.NewStyle().Faint(true)
	styleSelected = lipgloss.NewStyle().Reverse(true)
	// Files claimed but not yet written are dimmed, so they can be seen
	// arriving without reading as files that already exist.
	stylePending = lipgloss.NewStyle().Faint(true)
	styleFocus   = lipgloss.NewStyle().Bold(true)
	styleWorking = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	styleOK      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

// statusLabel maps a status to a symbol, text, and style. The symbol carries
// the meaning so the UI stays readable in a monochrome terminal.
func statusLabel(s events.Status) (symbol, text string, style lipgloss.Style) {
	switch s {
	case events.StatusWorking:
		return "●", "WORKING", styleWorking
	case events.StatusWaiting:
		return "○", "WAITING", styleDim
	case events.StatusNeedsInput:
		return "⚠", "NEEDS INPUT", styleWarn
	case events.StatusDone:
		return "✓", "DONE", styleOK
	case events.StatusError:
		return "✗", "ERROR", styleError
	}
	return "·", "UNKNOWN", styleDim
}

// activityMarker maps an activity level to a symbol that decays visibly.
func activityMarker(l state.ActivityLevel) (string, lipgloss.Style) {
	switch l {
	case state.Current:
		return "●", styleWorking
	case state.Recent:
		return "◐", styleWorking
	case state.Modified:
		return "·", styleDim
	}
	return "", styleDim
}

// gitMarker maps a file's Git status to a symbol.
func gitMarker(s git.FileStatus) (string, lipgloss.Style) {
	switch s {
	case git.Modified:
		return "M", styleWarn
	case git.Untracked:
		return "+", styleDim
	}
	return "", styleDim
}

// targetsAPath reports whether an action's target is a file path. Commands and
// error messages are not paths, and abbreviating them the way a path is
// abbreviated would corrupt them: `cmd //c "exit 4"` would come out as
// `.../c "exit 4"`.
func targetsAPath(k state.ActionKind) bool {
	switch k {
	case state.ActionReading, state.ActionEditing, state.ActionCreating, state.ActionDeleting:
		return true
	}
	return false
}

// actionVerb is the human-readable verb for an action kind.
func actionVerb(k state.ActionKind) string {
	switch k {
	case state.ActionWorking:
		return "Working"
	case state.ActionWriting:
		return "Writing"
	case state.ActionReading:
		return "Reading"
	case state.ActionEditing:
		return "Editing"
	case state.ActionCreating:
		return "Creating"
	case state.ActionDeleting:
		return "Deleting"
	case state.ActionRunning:
		return "Running"
	case state.ActionWaiting:
		return "Waiting for input"
	case state.ActionDone:
		return "Done"
	case state.ActionFailed:
		return "Failed"
	}
	return "Idle"
}
