package claude

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/seongyooo/agentline/internal/events"
)

// TestLivePermission drives a real Claude Code session through a permission
// prompt using AgentLine's own adapter. Set LIVE_PERMISSION=1.
func TestLivePermission(t *testing.T) {
	if os.Getenv("LIVE_PERMISSION") == "" {
		t.Skip("set LIVE_PERMISSION=1")
	}

	root := t.TempDir()
	s := NewStream(root)
	s.Bin = os.Getenv("CLAUDE_BIN")
	// The machine this runs on approves everything by default, so the mode is
	// forced back to one that actually asks.
	s.Args = []string{"--permission-mode", "default"}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	stream, err := s.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Send("Create a file named probe.txt in the current directory containing exactly the word hi. Do not explain anything."); err != nil {
		t.Fatal(err)
	}

	var asked *events.Ask
	for e := range stream {
		switch e.Type {
		case events.PermissionAsk:
			asked = e.Ask
			t.Logf("ASK   tool=%q target=%q mode=%q", asked.Tool, asked.Target, asked.Mode)
			t.Logf("      reason=%q", asked.Reason)
			if err := s.Answer(asked.ID, true, ""); err != nil {
				t.Fatal(err)
			}
		case events.PermissionAnswered:
			t.Logf("ANSWERED allowed=%v", e.Ask.Allowed)
		case events.AgentStatus:
			if e.Status == events.StatusWaiting && asked != nil {
				goto done
			}
		}
	}
done:
	if asked == nil {
		t.Fatal("the session never asked; the adapter is not receiving prompts")
	}
	if _, err := os.Stat(filepath.Join(root, "probe.txt")); err != nil {
		t.Fatalf("allowed the write but it did not happen: %v", err)
	}
	t.Log("round trip confirmed: asked, answered, written")
}
