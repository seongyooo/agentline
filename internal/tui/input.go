package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Sender submits a prompt to the agent. It is satisfied by a source that owns
// the session; when AgentView is only observing one, there is nothing to send
// to and the input box stays inert.
type Sender interface {
	Send(prompt string) error
}

// Restarter starts a fresh session, discarding accumulated context.
//
// A session AgentView owns is never compacted, so its context only grows and
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
