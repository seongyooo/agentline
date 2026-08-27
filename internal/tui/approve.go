package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/seongyooo/agentline/internal/events"
)

// answering routes a key to the question the agent is blocked on, and reports
// whether it took it.
//
// Only the answer keys are claimed. Quitting still works, because being unable
// to leave until a question is answered would be a worse trap than the one
// this feature exists to solve.
func (m Model) answering(msg tea.KeyMsg) (tea.Cmd, bool) {
	ask := m.st.Agent.Ask
	if ask == nil {
		return nil, false
	}

	switch msg.String() {
	case "y":
		return m.answer(ask, true, ""), true
	case "n":
		// The agent is told a person said no, not that something failed. It
		// decides what to do next, and it should know which it was.
		return m.answer(ask, false, "The user declined this."), true
	case "a":
		if ask.Mode == "" {
			return nil, false
		}
		// Allow this one and stop being asked about its kind. The order
		// matters: the mode change has to be in flight before the answer, or
		// the agent can ask the next question under the old mode.
		return tea.Batch(m.applyPermissionMode(ask.Mode), m.answer(ask, true, "")), true
	}
	return nil, false
}

// answer replies to the agent off the UI goroutine.
func (m Model) answer(ask *events.Ask, allow bool, message string) tea.Cmd {
	approver, ok := m.sender.(Approver)
	if !ok {
		return nil
	}

	id := ask.ID
	return func() tea.Msg {
		// Reported the same way a failed prompt is: the bar is where a
		// session that will not take instructions says so.
		return promptSentMsg{err: approver.Answer(id, allow, message)}
	}
}

// askLines renders the question in place of nothing else moving.
//
// None of the wording is AgentLine's. The tool, what it would act on and why
// it is being asked at all are the agent's own words, shown as given: this is
// a safety judgement AgentLine did not make and must not paraphrase.
func (m Model) askLines(width int) []string {
	ask := m.st.Agent.Ask
	if ask == nil {
		return nil
	}

	lines := []string{styleWarn.Bold(true).Render("NEEDS YOU")}
	lines = append(lines, styleValue.Render(fitLine(askTitle(ask), width)))

	// Two lines of reason. It is context for a decision, not the decision,
	// and a paragraph here would push the keys off a short panel.
	if ask.Reason != "" {
		reason := wrap(ask.Reason, width)
		for i, line := range reason[:min(len(reason), askReasonRows)] {
			// A sentence cut off mid-word has to look cut off, or it reads
			// as the agent's whole explanation when it is half of one.
			if i == askReasonRows-1 && len(reason) > askReasonRows {
				// Truncated with no mark of its own, so the one added here is
				// the only one: fitLine appends its own and two in a row read
				// as a rendering fault rather than as a cut sentence.
				line = fitLine(ansi.Truncate(line, width-1, "")+"…", width)
			}
			lines = append(lines, styleDim.Render(line))
		}
	}
	return append(lines, styleWarn.Render(askKeys(ask)))
}

// askReasonRows bounds the explanation shown with a question.
const askReasonRows = 2

// askTitle names what is being asked for, in the agent's own terms.
func askTitle(a *events.Ask) string {
	name := a.Title
	if name == "" {
		name = a.Tool
	}
	switch {
	case name == "" && a.Target == "":
		return "Permission needed"
	case a.Target == "":
		return name
	case name == "":
		return a.Target
	}
	return name + "   " + a.Target
}

// askKeys spells out the answers, including the standing one the agent itself
// offered.
func askKeys(a *events.Ask) string {
	keys := "y allow   n deny"
	if a.Mode != "" {
		label, _ := modeLabel(a.Mode)
		keys += "   a allow + " + label
	}
	return keys
}

// askAlert is the one line that has to work with no screen at all: a desktop
// notification, seen while looking at something else entirely.
func askAlert(a *events.Ask) string {
	text := "AgentLine needs you — " + askTitle(a)
	if a.Reason != "" {
		text += ": " + a.Reason
	}
	return text
}
