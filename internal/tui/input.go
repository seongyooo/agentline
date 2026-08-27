package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/seongyooo/agentline/internal/agent/claude"
)

// Sender submits a prompt to the agent. It is satisfied by a source that owns
// the session; when AgentLine is only observing one, there is nothing to send
// to and the input box stays inert.
type Sender interface {
	Send(prompt string) error
}

// Controller changes a running session's settings.
//
// These are things a flag can only decide at launch, so a session AgentLine
// owns can offer them and one it is merely watching cannot.
type Controller interface {
	SetPermissionMode(mode string) error
	SetModel(model string) error
	RequestContextUsage() error
}

// askForContextUsage asks how full the context is, which only the agent can
// answer: it knows the window it is measuring against.
func (m Model) askForContextUsage() tea.Cmd {
	controller, ok := m.sender.(Controller)
	if !ok {
		return nil
	}
	return func() tea.Msg {
		// A failure here means one figure is missing from a status line, so
		// it is not worth interrupting anything over.
		_ = controller.RequestContextUsage()
		return nil
	}
}

// modelChangedMsg reports the outcome of a model change.
type modelChangedMsg struct {
	model string
	err   error
}

// setModel switches the running session's model.
//
// "default" is the agent's own word for returning to the session default, and
// is sent as an empty model rather than as a model of that name.
func (m *Model) setModel(model string) tea.Cmd {
	controller, ok := m.sender.(Controller)
	if !ok {
		return nil
	}

	requested := model
	if model == "default" {
		requested = ""
	}
	return func() tea.Msg {
		return modelChangedMsg{model: model, err: controller.SetModel(requested)}
	}
}

// modeChangedMsg reports the outcome of a permission mode change.
type modeChangedMsg struct {
	mode string
	err  error
}

// cyclePermissionMode asks the session for the next mode in the cycle.
//
// The change is only recorded once the session accepts it, so the header
// never shows a mode the agent is not actually in.
func (m *Model) cyclePermissionMode() tea.Cmd {
	controller, ok := m.sender.(Controller)
	if !ok {
		return nil
	}
	next := nextPermissionMode(m.st.PermissionMode())

	return func() tea.Msg {
		return modeChangedMsg{mode: next, err: controller.SetPermissionMode(next)}
	}
}

// nextPermissionMode is the mode after the current one, wrapping around. An
// unfamiliar mode starts the cycle from the beginning rather than being
// treated as a position in it.
func nextPermissionMode(current string) string {
	for i, mode := range claude.PermissionModes {
		if mode == current {
			return claude.PermissionModes[(i+1)%len(claude.PermissionModes)]
		}
	}
	return claude.PermissionModes[0]
}

// Restarter starts a fresh session, discarding accumulated context.
//
// A session AgentLine owns is never compacted, so its context only grows and
// every further turn re-sends all of it. Starting over is the only way to get
// that cost back down.
type Restarter interface {
	Restart() error
}

// restartedMsg reports the outcome of starting a fresh session.
type restartedMsg struct{ err error }

// restartSession starts a new session if the source supports it.
func (m *Model) restartSession() tea.Cmd {
	restarter, ok := m.sender.(Restarter)
	if !ok {
		return nil
	}
	return func() tea.Msg {
		return restartedMsg{err: restarter.Restart()}
	}
}

// promptSentMsg reports the outcome of a submitted prompt.
type promptSentMsg struct{ err error }

// newInput builds the prompt field.
func newInput() textinput.Model {
	in := textinput.New()
	in.Prompt = "> "
	in.Placeholder = "Ask Claude Code..."
	in.CharLimit = 2000
	return in
}

// canSend reports whether there is a session to submit prompts to.
func (m Model) canSend() bool { return m.sender != nil }

// submitPrompt sends what has been typed and clears the field.
//
// The send runs off the UI goroutine: writing to the agent's stdin can block
// if it is busy, and the interface must stay responsive while it does.
func (m *Model) submitPrompt() tea.Cmd {
	prompt := strings.TrimSpace(m.input.Value())
	if prompt == "" || !m.canSend() {
		return nil
	}

	// Commands AgentLine answers itself are only intercepted when it can
	// actually carry them out. Otherwise the text goes to the agent as
	// typed, rather than being swallowed by a feature that is not there.
	if _, ok := m.sender.(Controller); ok {
		if m.openPickerFor(prompt) {
			return nil
		}
		if model, ok := strings.CutPrefix(prompt, "/model "); ok {
			m.input.Reset()
			return m.setModel(strings.TrimSpace(model))
		}
	}
	m.input.Reset()

	sender := m.sender
	return func() tea.Msg {
		return promptSentMsg{err: sender.Send(prompt)}
	}
}

// updateInput forwards a key to the prompt field's editor.
func (m *Model) updateInput(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return cmd
}
