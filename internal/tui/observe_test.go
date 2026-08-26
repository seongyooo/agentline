package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/seonl/agentview/internal/agent/claude"
	"github.com/seonl/agentview/internal/project"
	"github.com/seonl/agentview/internal/state"
)

// TestObserveLiveClaude renders the full UI against a real Claude Code
// session, printing a frame per event. It is the Phase 6 acceptance check —
// what a developer actually sees while the agent works — and it needs a real
// session, so it only runs on request:
//
//	OBSERVE_ROOT=<project-dir> go test ./internal/tui -run TestObserveLive -v
//
// The project must have AgentView's hooks installed, pointing at OBSERVE_ADDR.
func TestObserveLiveClaude(t *testing.T) {
	root := os.Getenv("OBSERVE_ROOT")
	if root == "" {
		t.Skip("set OBSERVE_ROOT=<project-dir>")
	}
	addr := os.Getenv("OBSERVE_ADDR")
	if addr == "" {
		addr = claude.DefaultAddr
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	adapter := claude.New(root, addr)
	stream, err := adapter.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}

	scanner := project.NewScanner(root)
	st := state.New(root)
	st.Agent.Mission = os.Getenv("OBSERVE_MISSION")
	st.Project.Tree = scanner.NewTree()

	hint := fmt.Sprintf("Waiting for Claude Code hooks on %s", adapter.Addr)
	model, _ := New(st, scanner, stream).WithHint(hint).Update(tea.WindowSizeMsg{Width: 100, Height: 32})
	m := model.(Model)

	fmt.Printf("=== before any activity ===\n%s\n\n", ansi.Strip(m.View()))

	idle := time.NewTimer(90 * time.Second)
	defer idle.Stop()

	var frames int
	for done := false; !done; {
		select {
		case e, ok := <-stream:
			if !ok {
				done = true
				break
			}
			frames++

			next, _ := m.Update(eventMsg(e))
			m = next.(Model)
			fmt.Printf("=== frame %d: %s ===\n%s\n\n", frames, e.Type, ansi.Strip(m.View()))

			idle.Reset(25 * time.Second)

		case <-idle.C:
			done = true
		case <-ctx.Done():
			done = true
		}
	}

	if frames == 0 {
		t.Fatal("no events arrived: are the hooks installed and pointing here?")
	}

	// Let activity age, then confirm the markers decayed without new events.
	fmt.Printf("=== %d frames, %d dropped ===\n", frames, adapter.Dropped())
	fmt.Printf("status: %s\nnow:    %s %s\ntouched: %d files\n",
		m.st.Agent.Status, m.st.Agent.Now.Kind, m.st.Agent.Now.Target, len(m.st.Project.ActivityByPath))

	if strings.Contains(ansi.Strip(m.View()), "No agent activity yet") {
		t.Error("UI still claims it has seen nothing after real activity")
	}
	if adapter.Dropped() > 0 {
		t.Errorf("%d events were dropped", adapter.Dropped())
	}
}
