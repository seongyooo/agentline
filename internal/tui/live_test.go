package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/seonl/agentview/internal/agent"
	"github.com/seonl/agentview/internal/project"
	"github.com/seonl/agentview/internal/state"
)

// TestLivePreview prints the NOW/status line after each event so the pipeline
// can be watched frame by frame. Set LIVE_PREVIEW=1 to run it.
func TestLivePreview(t *testing.T) {
	if os.Getenv("LIVE_PREVIEW") == "" {
		t.Skip("set LIVE_PREVIEW=1")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	root := project.FindRoot(".")
	scanner := project.NewScanner(root)
	st := state.New(root)
	st.Agent.Mission = "Phase 4: event infrastructure"
	st.Project.Tree = scanner.NewTree()

	src := &agent.Mock{Paths: []string{"internal/tui/model.go", "internal/agent/mock.go"}}
	stream, err := src.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}

	model, _ := New(st, scanner, stream).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := model.(Model)

	frame := 0
	for cmd := m.Init(); cmd != nil; {
		msg := cmd()
		if _, done := msg.(sourceClosedMsg); done {
			break
		}

		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(Model)

		frame++
		fmt.Printf("frame %d  %s\n", frame, summarize(m))
	}

	fmt.Printf("\n--- final view ---\n%s\n", m.View())
}

// summarize renders the header status and the NOW line on one row.
func summarize(m Model) string {
	_, status, _ := statusLabel(m.st.Agent.Status)
	now := actionVerb(m.st.Agent.Now.Kind)
	if target := m.st.Agent.Now.Target; target != "" {
		now += " " + shorten(target)
	}
	return fmt.Sprintf("%-12s %s", status, strings.TrimSpace(ansi.Strip(now)))
}
