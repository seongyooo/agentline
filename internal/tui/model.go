// Package tui renders application state. It holds no event-processing,
// filesystem, or agent-specific logic.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/seonl/agentview/internal/project"
	"github.com/seonl/agentview/internal/state"
)

// minWidth/minHeight are the smallest usable terminal; below this the UI
// refuses to render rather than showing an unreadable layout.
const (
	minWidth  = 40
	minHeight = 12
)

// Model is the Bubble Tea model. It renders a *state.State it does not mutate,
// apart from the tree's expand/collapse flags driven by the user.
type Model struct {
	st      *state.State
	scanner *project.Scanner
	tree    treeView
	width   int
	height  int
}

// New returns a Model rendering the given state. The scanner may be nil, in
// which case directories cannot be expanded beyond what is already loaded.
func New(st *state.State, scanner *project.Scanner) Model {
	return Model{st: st, scanner: scanner}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

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
