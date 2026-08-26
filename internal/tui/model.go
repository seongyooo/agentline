// Package tui renders application state. It holds no event-processing,
// filesystem, or agent-specific logic; it delegates to the reducer and the
// project scanner and renders whatever they produce.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/seonl/agentview/internal/events"
	"github.com/seonl/agentview/internal/project"
	"github.com/seonl/agentview/internal/state"
)

// minWidth/minHeight are the smallest usable terminal; below this the UI
// refuses to render rather than showing an unreadable layout.
const (
	minWidth  = 40
	minHeight = 12
)

// eventMsg carries one normalized agent event into the message loop.
type eventMsg events.Event

// sourceClosedMsg reports that the event stream ended. The UI keeps showing
// the last observed state rather than blanking or inventing a new one.
type sourceClosedMsg struct{}

// Model is the Bubble Tea model. It renders a *state.State it mutates only
// through the reducer, plus the tree's expand/collapse flags.
type Model struct {
	st      *state.State
	scanner *project.Scanner
	stream  <-chan events.Event

	tree   treeView
	width  int
	height int
}

// New returns a Model rendering the given state.
//
// scanner may be nil, in which case directories cannot be expanded beyond what
// is already loaded. stream may be nil, in which case the UI is static.
func New(st *state.State, scanner *project.Scanner, stream <-chan events.Event) Model {
	return Model{st: st, scanner: scanner, stream: stream}
}

func (m Model) Init() tea.Cmd {
	return waitForEvent(m.stream)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case eventMsg:
		m.applyEvent(events.Event(msg))
		return m, waitForEvent(m.stream)

	case sourceClosedMsg:
		m.stream = nil
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		// Esc is reserved for cancelling and closing popups (§22), so it is
		// deliberately not a quit key.
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up":
			m.moveCursor(-1)
		case "down":
			m.moveCursor(1)
		case "right":
			m.expand()
		case "left":
			m.collapse()
		case "enter":
			m.toggle()
		default:
			return m, nil
		}
	}

	m.syncScroll()
	return m, nil
}

// applyEvent folds one event into the application state. The reducer owns
// every state change; the tree is opened so the touched file is visible,
// which is the whole point of the project panel.
func (m *Model) applyEvent(e events.Event) {
	m.st.Apply(e)

	if e.Path != "" && m.scanner != nil {
		m.scanner.Reveal(m.st.Project.Tree, e.Path)
	}
	m.syncScroll()
}

// waitForEvent blocks on the next event from the source. Bubble Tea runs it
// off the UI goroutine, and Update re-issues it to keep the stream pumping.
func waitForEvent(stream <-chan events.Event) tea.Cmd {
	if stream == nil {
		return nil
	}
	return func() tea.Msg {
		e, ok := <-stream
		if !ok {
			return sourceClosedMsg{}
		}
		return eventMsg(e)
	}
}
