package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// A session AgentLine owns has no interactive picker of its own, so /model
// typed into the prompt does nothing on its own. AgentLine answers it here
// instead, applying the choice over the control protocol.

// modelAliases are the names the CLI documents for --model.
//
// They are suggestions, not a claim about what exists: any name can be typed
// after /model and is sent as given, and the agent is what decides whether it
// is real. A family added later can be used the day it ships; only this list
// lags, and it is never presented as complete.
var modelAliases = []string{"opus", "sonnet", "haiku", "fable", "default"}

// picker is an open choice waiting for the user.
type picker struct {
	open    bool
	options []string
	cursor  int
}

// modelPicker returns the picker for choosing a model.
func (m Model) modelPicker() picker {
	options := make([]string, 0, len(modelAliases)+1)

	// What the session is on now goes first, so switching back is one key
	// away and the current choice is visible in the same list.
	if current := m.currentModelAlias(); current != "" {
		options = append(options, current)
	}
	for _, alias := range modelAliases {
		if alias != options[0] {
			options = append(options, alias)
		}
	}
	return picker{open: true, options: options}
}

// currentModelAlias is the family the session is running, when it said.
func (m Model) currentModelAlias() string {
	session := m.st.Agent.Session
	if session == nil || session.Model == "" {
		return modelAliases[0]
	}

	name := shortModel(session.Model)
	for _, alias := range modelAliases {
		if strings.HasPrefix(name, alias) {
			return alias
		}
	}
	return name
}

// pickerCommand is the command a prompt opens a picker for, if any.
//
// Only a bare command opens one: with an argument the user has already said
// what they want, and interrupting that with a list would be in the way.
func pickerCommand(value string) (string, bool) {
	command := strings.TrimSpace(value)
	if !strings.HasPrefix(command, "/") {
		return "", false
	}
	return strings.TrimPrefix(command, "/"), true
}

// openPickerFor opens the picker a submitted command calls for, and reports
// whether it took the submission.
func (m *Model) openPickerFor(value string) bool {
	command, ok := pickerCommand(value)
	if !ok || command != "model" {
		return false
	}

	m.picker = m.modelPicker()
	m.input.Reset()
	return true
}

// movePicker moves the selection, stopping at the ends rather than wrapping:
// a list this short is easier to read when the edges hold still.
func (m *Model) movePicker(delta int) {
	m.picker.cursor = clamp(m.picker.cursor+delta, 0, len(m.picker.options)-1)
}

// choosePicker applies the selection and closes the picker.
func (m *Model) choosePicker() tea.Cmd {
	if !m.picker.open || len(m.picker.options) == 0 {
		return nil
	}
	choice := m.picker.options[m.picker.cursor]
	m.picker = picker{}

	return m.setModel(choice)
}

// closePicker abandons the choice.
func (m *Model) closePicker() { m.picker = picker{} }

// pickerLine renders the open choice, marking the selection.
func (m Model) pickerLine(width int) string {
	if !m.picker.open {
		return ""
	}

	parts := make([]string, 0, len(m.picker.options)+1)
	parts = append(parts, styleLabel.Render("MODEL"))

	// Brackets mark the selection as well as the highlight does, so the
	// picker still says what enter would choose in a terminal that renders
	// no styling at all. Both forms are the same width, so moving the
	// selection does not shift the list around.
	for i, option := range m.picker.options {
		if i == m.picker.cursor {
			parts = append(parts, styleSelected.Render("["+option+"]"))
			continue
		}
		parts = append(parts, styleDim.Render(" "+option+" "))
	}

	line := strings.Join(parts, " ")
	if ansi.StringWidth(line) > width {
		return fitLine(line, width)
	}
	return line
}
