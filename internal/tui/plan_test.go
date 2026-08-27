package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/seongyooo/agentline/internal/events"
	"github.com/seongyooo/agentline/internal/project"
	"github.com/seongyooo/agentline/internal/state"
)

func planned(t *testing.T, w, h int, tasks []events.Task, done int) Model {
	t.Helper()

	st := state.New("/proj")
	st.Project.Tree = project.MockTree()

	now := time.Now()
	st.Apply(events.Event{Type: events.UserPrompt, Message: "wire the valve", Timestamp: now, Source: "claude-code"})
	st.Apply(events.Event{Type: events.TaskProgress, Done: done, Total: len(tasks),
		Timestamp: now, Source: "claude-code", Tasks: tasks})
	st.Apply(events.Event{Type: events.FileEdit, Path: "Assets/Scripts/Puzzle/Valve.cs", Timestamp: now, Source: "claude-code"})

	model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: w, Height: h})
	return model.(Model)
}

func sampleTasks() []events.Task {
	return []events.Task{
		{Text: "Read the drainage system", Done: true},
		{Text: "Add the valve interface", Done: true},
		{Text: "Wire the valve to the pump", Doing: "Wiring the valve to the pump", Now: true},
		{Text: "Update the room script"},
		{Text: "Run the tests"},
	}
}

// The plan is the agent's own, and every state has to be tellable apart with
// no colour: three symbols, not three shades.
func TestThePlanShowsEachStep(t *testing.T) {
	out := ansi.Strip(planned(t, 100, 34, sampleTasks(), 2).View())

	for _, want := range []string{
		"PLAN",
		"✓ Read the drainage system",
		"✓ Add the valve interface",
		"● Wire the valve to the pump",
		"○ Update the room script",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("frame missing %q", want)
			t.Log(out)
		}
	}
}

// The two wordings exist so each can go where it reads: the present tense in
// NOW, where something is happening, and the imperative in the list. Using one
// in both places printed the same sentence twice, two lines apart.
func TestTheTwoWordingsGoToDifferentPlaces(t *testing.T) {
	out := ansi.Strip(planned(t, 100, 34, sampleTasks(), 2).View())

	if !strings.Contains(out, "Wiring the valve to the pump") {
		t.Error("NOW does not say what the agent says it is doing")
	}
	if !strings.Contains(out, "Wire the valve to the pump") {
		t.Error("the plan does not list the step in its own wording")
	}
	if strings.Count(out, "Wiring the valve to the pump") != 1 {
		t.Errorf("the present-tense wording appears more than once")
		t.Log(out)
	}
}

// A plan longer than the room for it has to keep the step in hand in view: it
// is read to find out where the agent is, and the first five of nine answer
// that only until the agent reaches the sixth.
func TestALongPlanKeepsTheCurrentStepInView(t *testing.T) {
	var tasks []events.Task
	for i := 0; i < 12; i++ {
		tasks = append(tasks, events.Task{Text: "step " + string(rune('a'+i)), Done: i < 8})
	}
	tasks[8].Now = true

	shown := planWindow(tasks, planRows)
	if len(shown) != planRows {
		t.Fatalf("window of %d, want %d", len(shown), planRows)
	}
	for _, task := range shown {
		if task.Now {
			return
		}
	}
	t.Errorf("the step in hand fell outside the window: %+v", shown)
}

// An agent that keeps no list gets no panel, rather than an empty one.
func TestNoPlanPanelWithoutAList(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: 100, Height: 34})

	if out := ansi.Strip(model.(Model).View()); strings.Contains(out, "PLAN") {
		t.Errorf("a plan panel appeared with no plan:\n%s", out)
	}
}
