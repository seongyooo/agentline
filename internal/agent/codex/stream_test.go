package codex

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/seongyooo/agentline/internal/events"
)

// startStandIn runs the stand-in Codex against root and guarantees it is gone
// before the test returns — the session's working directory is the temporary
// one the test then removes, and Windows will not delete a directory a live
// process is sitting in.
func startStandIn(t *testing.T, root string) (*Stream, <-chan events.Event) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds a helper binary")
	}

	bin := filepath.Join(t.TempDir(), "fakeagent.exe")
	build := exec.Command("go", "build", "-o", bin, "github.com/seongyooo/agentline/cmd/fakeagent")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fakeagent: %v\n%s", err, out)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)

	s := NewStream(root)
	s.Bin = bin
	stream, err := s.Events(ctx)
	if err != nil {
		cancel()
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cancel()
		done := time.After(30 * time.Second)
		for {
			select {
			case _, ok := <-stream:
				if !ok {
					return
				}
			case <-done:
				t.Error("the session did not shut down")
				return
			}
		}
	})
	return s, stream
}

// turn reads until the turn ends, collecting what arrived.
func turn(t *testing.T, stream <-chan events.Event) []events.Event {
	t.Helper()

	var got []events.Event
	deadline := time.After(60 * time.Second)
	for {
		select {
		case e, ok := <-stream:
			if !ok {
				return got
			}
			if !e.Valid() {
				t.Errorf("invalid event reached the stream: %+v", e)
			}
			got = append(got, e)
			if e.Type == events.AgentStatus && e.Status == events.StatusWaiting {
				return got
			}
		case <-deadline:
			t.Fatalf("the turn never ended; got %d events", len(got))
		}
	}
}

func find(got []events.Event, kind events.Type) (events.Event, bool) {
	for _, e := range got {
		if e.Type == kind {
			return e, true
		}
	}
	return events.Event{}, false
}

func types(got []events.Event) string {
	var out []string
	for _, e := range got {
		out = append(out, string(e.Type))
	}
	return strings.Join(out, ",")
}

// A prompt has to reach the agent and come back as activity, which is the
// whole point of AgentLine owning the session.
func TestSendProducesActivity(t *testing.T) {
	root := t.TempDir()
	s, stream := startStandIn(t, root)

	if err := s.Send("write the readme"); err != nil {
		t.Fatal(err)
	}
	got := turn(t, stream)

	prompt, ok := find(got, events.UserPrompt)
	if !ok {
		t.Fatalf("the prompt was not emitted: %s", types(got))
	}
	if prompt.Message != "write the readme" {
		t.Errorf("prompt = %q", prompt.Message)
	}

	created, ok := find(got, events.FileCreate)
	if !ok {
		t.Fatalf("no file was reported: %s", types(got))
	}
	// Codex reports absolute paths; the tree needs them relative.
	if created.Path != "FAKE-NOTES.md" {
		t.Errorf("Path = %q, want it relative to the project", created.Path)
	}
	if _, err := os.Stat(filepath.Join(root, "FAKE-NOTES.md")); err != nil {
		t.Errorf("the file it reported writing is not there: %v", err)
	}

	if _, ok := find(got, events.AgentReply); !ok {
		t.Errorf("no reply in: %s", types(got))
	}
}

// The turn's cost has to reach the UI, since a conversation that grows is
// what makes a session expensive.
func TestTurnReportsUsage(t *testing.T) {
	s, stream := startStandIn(t, t.TempDir())

	if err := s.Send("run the tests"); err != nil {
		t.Fatal(err)
	}
	got := turn(t, stream)

	// Usage arrives with the status change that ends the turn.
	deadline := time.After(5 * time.Second)
	for {
		if _, ok := find(got, events.SessionInfo); ok {
			break
		}
		select {
		case e := <-stream:
			got = append(got, e)
		case <-deadline:
			t.Fatalf("no usage reported: %s", types(got))
		}
	}
}

// A failing command is marked failed and carries the code Codex reported.
func TestFailingCommandIsReported(t *testing.T) {
	s, stream := startStandIn(t, t.TempDir())

	if err := s.Send("make it fail"); err != nil {
		t.Fatal(err)
	}
	got := turn(t, stream)

	end, ok := find(got, events.CommandEnd)
	if !ok {
		t.Fatalf("no command_end in: %s", types(got))
	}
	if !end.Failed {
		t.Error("Failed = false for a command that failed")
	}
	if end.ExitCode == nil {
		t.Error("Codex reports an exit code; it was not carried through")
	}
}

// `codex exec` exits after each turn, so continuity depends on resuming the
// thread it reported. Without that every prompt would start over.
func TestSecondPromptResumesTheThread(t *testing.T) {
	s, stream := startStandIn(t, t.TempDir())

	if err := s.Send("first"); err != nil {
		t.Fatal(err)
	}
	turn(t, stream)

	s.mu.Lock()
	thread := s.threadID
	s.mu.Unlock()
	if thread == "" {
		t.Fatal("the thread id was not remembered")
	}

	if err := s.Send("second"); err != nil {
		t.Fatalf("second prompt: %v", err)
	}
	if got := turn(t, stream); len(got) == 0 {
		t.Fatal("no events for the second prompt")
	}
}

// Restarting forgets the thread, which is what clears the context a long
// conversation has accumulated.
func TestRestartForgetsTheThread(t *testing.T) {
	s, stream := startStandIn(t, t.TempDir())

	if err := s.Send("first"); err != nil {
		t.Fatal(err)
	}
	turn(t, stream)

	if err := s.Restart(); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	thread := s.threadID
	s.mu.Unlock()

	if thread != "" {
		t.Errorf("thread = %q, want it forgotten", thread)
	}
}

// Two turns at once would race over the same files, so a second prompt while
// one is running is refused rather than run.
func TestOverlappingTurnsAreRefused(t *testing.T) {
	s, stream := startStandIn(t, t.TempDir())

	if err := s.Send("run the tests"); err != nil {
		t.Fatal(err)
	}
	if err := s.Send("and again"); err == nil {
		t.Error("a second turn was started while one was running")
	}
	turn(t, stream)
}

func TestSendWithoutASessionFails(t *testing.T) {
	if err := NewStream(t.TempDir()).Send("hello"); err != ErrNotRunning {
		t.Errorf("err = %v, want %v", err, ErrNotRunning)
	}
}

// A missing executable must be reported, not leave a UI that looks live but
// never updates.
func TestMissingExecutableIsReported(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewStream(t.TempDir())
	s.Bin = filepath.Join(t.TempDir(), "definitely-not-here")
	if _, err := s.Events(ctx); err != nil {
		t.Fatal(err)
	}

	if err := s.Send("hello"); err == nil {
		t.Error("sending to a missing executable succeeded")
	}
}
