package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/seongyooo/agentline/internal/events"
	"github.com/seongyooo/agentline/internal/git"
	"github.com/seongyooo/agentline/internal/state"
)

// The palette is small on purpose. Four state colours carry meaning — busy,
// good, warning, failure — and two greys carry none: one for chrome the eye
// should skip over, one for text that is real but secondary.
//
// Every colour is adaptive, so the greys stay grey on a light terminal instead
// of washing out, and none of them is load-bearing: each styled element is
// also distinguishable by symbol, position, or weight.
var (
	// Chrome: rules, dividers, the tree's box drawing. Deliberately the
	// quietest thing on screen.
	colorChrome = lipgloss.AdaptiveColor{Light: "252", Dark: "238"}
	// Secondary text: timestamps, paths, hints. Quiet but still readable.
	colorSubtle = lipgloss.AdaptiveColor{Light: "245", Dark: "245"}

	colorAccent = lipgloss.AdaptiveColor{Light: "26", Dark: "39"}   // busy, focus
	colorOK     = lipgloss.AdaptiveColor{Light: "28", Dark: "42"}   // finished, safe
	colorWarn   = lipgloss.AdaptiveColor{Light: "130", Dark: "179"} // changed, nearly full
	colorError  = lipgloss.AdaptiveColor{Light: "160", Dark: "203"} // failed, unrestricted
)

var (
	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	styleLabel    = lipgloss.NewStyle().Bold(true).Foreground(colorSubtle)
	styleValue    = lipgloss.NewStyle().Bold(true)
	styleDim      = lipgloss.NewStyle().Foreground(colorSubtle)
	styleRule     = lipgloss.NewStyle().Foreground(colorChrome)
	styleSelected = lipgloss.NewStyle().Reverse(true)
	// Files claimed but not yet written are dimmed, so they can be seen
	// arriving without reading as files that already exist.
	stylePending = lipgloss.NewStyle().Faint(true).Foreground(colorSubtle)
	styleFocus   = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	styleWorking = lipgloss.NewStyle().Foreground(colorAccent)
	styleOK      = lipgloss.NewStyle().Foreground(colorOK)
	styleWarn    = lipgloss.NewStyle().Foreground(colorWarn)
	styleError   = lipgloss.NewStyle().Foreground(colorError)

	// The tree's box drawing is structure, not content: it recedes so the
	// names sit in front of it.
	styleTree = lipgloss.NewStyle().Foreground(colorChrome)
	styleDir  = lipgloss.NewStyle().Bold(true)
	styleFile = lipgloss.NewStyle()

	// The filled part of a progress bar against the part still to do.
	styleBarFill = lipgloss.NewStyle().Foreground(colorAccent)
	styleBarDone = lipgloss.NewStyle().Foreground(colorOK)
	styleBarRest = lipgloss.NewStyle().Foreground(colorChrome)
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
		return "+", styleOK
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
	case state.ActionAsking:
		return "Asking"
	case state.ActionDone:
		return "Done"
	case state.ActionFailed:
		return "Failed"
	}
	return "Idle"
}

// verbStyle colours an action by what it does to the project, so a log of a
// dozen lines can be read by shape before it is read by word: writes stand out
// from reads, and a failure stands out from both.
func verbStyle(k state.ActionKind) lipgloss.Style {
	switch k {
	case state.ActionFailed:
		return styleError
	case state.ActionDeleting:
		return styleError
	case state.ActionDone:
		return styleOK
	case state.ActionWriting, state.ActionEditing, state.ActionCreating:
		return styleWarn
	case state.ActionRunning, state.ActionWorking:
		return styleWorking
	case state.ActionWaiting, state.ActionAsking:
		return styleWarn
	}
	return styleDim
}
