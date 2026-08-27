package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/seonl/agentview/internal/events"
	"github.com/seonl/agentview/internal/project"
	"github.com/seonl/agentview/internal/state"
)

func progress(done, total int) events.Event {
	return events.Event{
		Type:      events.TaskProgress,
		Done:      done,
		Total:     total,
		Timestamp: time.Now(),
		Source:    "claude-code",
	}
}

func TestProgressShowsTheAgentsTaskCount(t *testing.T) {
	m := sized(t, 100, 40)
	m, _ = send(m, progress(3, 7))

	out := ansi.Strip(m.View())
	if !strings.Contains(out, "PROGRESS") {
		t.Errorf("no progress panel:\n%s", out)
	}
	if !strings.Contains(out, "3/7") {
		t.Errorf("count not spelled out:\n%s", out)
	}
	if !strings.Contains(out, "█") {
		t.Errorf("no bar drawn:\n%s", out)
	}
}

// With no task list there is nothing to count, and a number must not be
// invented for work whose real extent cannot be seen.
func TestNoProgressShownWithoutATaskList(t *testing.T) {
	m := sized(t, 100, 40)

	if out := ansi.Strip(m.View()); strings.Contains(out, "PROGRESS") {
		t.Errorf("progress shown with no task list:\n%s", out)
	}
}

// A count of nothing says nothing, and a nonsensical one must be rejected
// rather than rendered.
func TestImpossibleCountsAreRejected(t *testing.T) {
	tests := []struct {
		name        string
		done, total int
	}{
		{"no tasks", 0, 0},
		{"more done than exist", 5, 3},
		{"negative", -1, 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := sized(t, 100, 40)
			m, _ = send(m, progress(tc.done, tc.total))

			if out := ansi.Strip(m.View()); strings.Contains(out, "PROGRESS") {
				t.Errorf("rendered an impossible count:\n%s", out)
			}
		})
	}
}

func TestProgressBarFillsWithCompletion(t *testing.T) {
	empty := ansi.Strip(progressBar(state.Progress{Done: 0, Total: 4}, 40))
	half := ansi.Strip(progressBar(state.Progress{Done: 2, Total: 4}, 40))
	full := ansi.Strip(progressBar(state.Progress{Done: 4, Total: 4}, 40))

	if strings.Contains(empty, "█") {
		t.Errorf("empty bar has filled blocks: %q", empty)
	}
	if strings.Count(half, "█") == 0 || strings.Count(half, "░") == 0 {
		t.Errorf("half bar is not half: %q", half)
	}
	if strings.Contains(full, "░") {
		t.Errorf("full bar still has empty blocks: %q", full)
	}
}

// The bar must not push the layout out of shape at any width.
func TestProgressKeepsTheLayoutIntact(t *testing.T) {
	for _, w := range []int{40, 60, 80, 120, 200} {
		st := state.New("/proj")
		st.Project.Tree = project.MockTree()
		st.Apply(progress(2, 9))

		model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: w, Height: 30})
		for i, line := range strings.Split(model.(Model).View(), "\n") {
			if got := ansi.StringWidth(line); got != w {
				t.Errorf("width %d line %d: got %d", w, i, got)
			}
		}
	}
}

// How long the current action has run is the one progress signal that is
// always observable, and it is what separates slow from stuck.
func TestElapsedTimeAppearsWhileWorking(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	st.Apply(events.Event{
		Type:      events.CommandStart,
		Command:   "go test ./...",
		Timestamp: time.Now().Add(-90 * time.Second),
		Source:    "claude-code",
	})

	model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	out := ansi.Strip(model.(Model).View())

	if !strings.Contains(out, "1m 30s") {
		t.Errorf("elapsed time not shown:\n%s", out)
	}
}

// A just-started action does not need a timer yet; showing one would only add
// noise to every quick step.
func TestElapsedTimeHiddenWhenFresh(t *testing.T) {
	m := sized(t, 100, 30)
	m, _ = send(m, events.Event{
		Type:      events.CommandStart,
		Command:   "go build",
		Timestamp: time.Now(),
		Source:    "claude-code",
	})

	if got := m.elapsed(m.st.Agent.Now); got != "" {
		t.Errorf("elapsed = %q, want nothing for a fresh action", got)
	}
}

// An idle agent is not spending time on anything, so no timer runs.
func TestElapsedTimeHiddenWhenNotWorking(t *testing.T) {
	m := sized(t, 100, 30)
	m, _ = send(m, events.Event{
		Type:      events.AgentStatus,
		Status:    events.StatusWaiting,
		Timestamp: time.Now().Add(-5 * time.Minute),
		Source:    "claude-code",
	})

	if got := m.elapsed(m.st.Agent.Now); got != "" {
		t.Errorf("elapsed = %q, want nothing while waiting", got)
	}
}
