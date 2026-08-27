package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/seongyooo/agentline/internal/events"
)

// The agent's own plan, in its own words.
//
// AgentLine does not break work into steps. That is a judgement about the
// work, and §24 rules out judgements of that kind. But an agent that keeps a
// list has already made them and is sending them, item by item, on the same
// stream everything else arrives on — so this is reading, not inferring. Until
// now the text was decoded away and only the count survived, which left a
// progress bar that could not say what it was counting.

// planRows bounds the list. A plan longer than this is scrolled past rather
// than read, and the panel has other things to say.
const planRows = 5

// planLines renders the list, or nil when the agent keeps none.
func (m Model) planLines(width int) []string {
	tasks := m.st.Agent.Tasks
	if len(tasks) == 0 {
		return nil
	}

	heading := styleLabel.Render("PLAN")
	if p := m.st.Agent.Progress; p.Known() {
		count := fmt.Sprintf("%d/%d", p.Done, p.Total)
		if gap := width - ansi.StringWidth(ansi.Strip(heading)) - len(count); gap > 0 {
			heading += strings.Repeat(" ", gap) + styleDim.Render(count)
		}
	}

	lines := []string{heading}
	for _, task := range planWindow(tasks, planRows) {
		mark, style := taskMark(task)
		lines = append(lines, fitLine(style.Render(mark+" "+taskText(task)), width))
	}
	return lines
}

// planWindow picks which items to show when the list is longer than the room
// for it, keeping the one being worked on in view.
//
// A plan is read to find out where in it the agent is. Showing the first five
// of nine would answer that only until the agent reached the sixth.
func planWindow(tasks []events.Task, rows int) []events.Task {
	if len(tasks) <= rows {
		return tasks
	}

	at := 0
	for i, task := range tasks {
		if task.Now {
			at = i
			break
		}
	}

	start := clamp(at-rows/2, 0, len(tasks)-rows)
	return tasks[start : start+rows]
}

// taskMark is the symbol and colour for a task's state.
//
// The symbol carries it, so a plan is readable with no colour at all — the
// three states have to be told apart, and that is exactly what §21 says colour
// may not be the only thing doing.
func taskMark(task events.Task) (string, lipgloss.Style) {
	switch {
	case task.Done:
		return "✓", styleOK
	case task.Now:
		return "●", styleWorking
	}
	return "○", styleDim
}

// taskText is the imperative wording, which is what a list of things to do is
// made of. The present-tense form belongs in NOW, where something is actually
// happening — the agent supplies both so each can go where it reads, and using
// the same one twice printed the same sentence two lines apart.
func taskText(task events.Task) string {
	if task.Text != "" {
		return task.Text
	}
	return task.Doing
}

// activeTask is the task the agent says it is on, if it named one.
func (m Model) activeTask() (events.Task, bool) {
	for _, task := range m.st.Agent.Tasks {
		if task.Now {
			return task, true
		}
	}
	return events.Task{}, false
}
