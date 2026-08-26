package claude

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/seonl/agentview/internal/events"
	"github.com/seonl/agentview/internal/state"
)

// TestLiveClaudeSession runs the adapter against a real Claude Code session
// and prints the normalized events it produces. It is the Phase 5 acceptance
// check, and it costs a real API call, so it only runs on request:
//
//	LIVE_CLAUDE=<project-dir> go test ./internal/agent/claude -run TestLive -v
//
// The project directory must already have AgentView's hooks installed
// (agentview -print-hooks) pointing at LIVE_ADDR.
func TestLiveClaudeSession(t *testing.T) {
	root := os.Getenv("LIVE_CLAUDE")
	if root == "" {
		t.Skip("set LIVE_CLAUDE=<project-dir>")
	}
	addr := os.Getenv("LIVE_ADDR")
	if addr == "" {
		addr = DefaultAddr
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	a := New(root, addr)
	stream, err := a.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("adapter listening on %s, root %s\n\n", a.Addr, root)

	st := state.New(root)
	idle := time.NewTimer(90 * time.Second)
	defer idle.Stop()

	var count int
	for {
		select {
		case e, ok := <-stream:
			if !ok {
				report(t, st, count, a.Dropped())
				return
			}
			count++
			st.Apply(e)
			fmt.Printf("%2d  %-14s %-12s %s\n", count, e.Type, e.Status, describe(e))

			idle.Reset(30 * time.Second)

		case <-idle.C:
			report(t, st, count, a.Dropped())
			return

		case <-ctx.Done():
			report(t, st, count, a.Dropped())
			return
		}
	}
}

func describe(e events.Event) string {
	switch {
	case e.Path != "":
		return e.Path
	case e.Command != "":
		if e.Failed {
			return e.Command + "  [failed: " + e.Message + "]"
		}
		return e.Command
	}
	return e.Message
}

func report(t *testing.T, st *state.State, count int, dropped int64) {
	t.Helper()

	fmt.Printf("\n%d events, %d dropped\n", count, dropped)
	fmt.Printf("status: %s\nnow:    %s %s\n", st.Agent.Status, st.Agent.Now.Kind, st.Agent.Now.Target)
	fmt.Printf("files touched: %d\n", len(st.Project.ActivityByPath))

	if count == 0 {
		t.Error("no events arrived: are the hooks installed and pointing at this address?")
	}
	if dropped > 0 {
		t.Errorf("%d events were dropped", dropped)
	}
}
