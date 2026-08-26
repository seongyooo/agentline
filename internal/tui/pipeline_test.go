package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/seonl/agentview/internal/agent"
	"github.com/seonl/agentview/internal/project"
	"github.com/seonl/agentview/internal/state"
)

// TestPipelineEndToEnd drives a real Mock source through a real Model using
// Bubble Tea's own command mechanism, with no events injected by hand. This is
// the Phase 4 checkpoint: source → normalized event → reducer → state → view,
// with no agent running.
func TestPipelineEndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const target = "Assets/Scripts/Puzzle/Valve.cs"
	src := &agent.Mock{Paths: []string{target}}
	stream, err := src.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}

	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	model, _ := New(st, nil, stream).Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m := model.(Model)

	// Pump the loop exactly as Bubble Tea does: run the command, feed its
	// message back, repeat until the source closes.
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init did not start waiting for events")
	}

	var seen int
	for cmd != nil && seen < 50 {
		msg := cmd()
		if _, done := msg.(sourceClosedMsg); done {
			break
		}
		seen++

		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(Model)
	}

	if seen == 0 {
		t.Fatal("no events reached the model")
	}

	// The script ends waiting, having edited the target along the way.
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "WAITING") {
		t.Errorf("header did not settle on WAITING:\n%s", out)
	}
	if !strings.Contains(out, "Valve.cs") {
		t.Errorf("activity log never showed the edited file:\n%s", out)
	}
	if got := m.st.Agent.Agent; got != agent.MockName {
		t.Errorf("agent = %q, want %q", got, agent.MockName)
	}
}

// A source that fails to start must leave a usable, static UI (§28).
func TestPipelineSurvivesSourceThatCannotStart(t *testing.T) {
	stream, err := (&agent.Mock{}).Events(context.Background()) // no paths
	if err == nil {
		t.Fatal("expected the source to refuse to start")
	}

	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	model, _ := New(st, nil, stream).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := model.(Model)

	if cmd := m.Init(); cmd != nil {
		t.Error("model waited on a source that never started")
	}
	if out := m.View(); !strings.Contains(out, "AGENTVIEW") {
		t.Error("UI is not usable without an event source")
	}
}
