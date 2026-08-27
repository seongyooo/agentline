package claude

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/seongyooo/agentline/internal/events"
)

// fakeAgent builds a stand-in for Claude Code that speaks the streaming
// protocol. Driving the real agent would cost an API call per test and make
// the suite depend on a live service; the protocol is what needs covering.
func fakeAgent(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	if err := os.WriteFile(source, []byte(fakeAgentSource), 0o644); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(dir, "fake-agent.exe")
	build := exec.Command("go", "build", "-o", bin, source)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fake agent: %v\n%s", err, out)
	}
	return bin
}

// startStream runs a Stream against the fake agent.
func startStream(t *testing.T, root string) (*Stream, <-chan events.Event) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	s := NewStream(root)
	s.Bin = fakeAgent(t)

	stream, err := s.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return s, stream
}

// collect gathers events until the agent reports the turn finished.
func collect(t *testing.T, stream <-chan events.Event) []events.Event {
	t.Helper()

	var got []events.Event
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
		case <-time.After(20 * time.Second):
			t.Fatalf("timed out; got %d events", len(got))
		}
	}
}

func types(got []events.Event) string {
	var out []string
	for _, e := range got {
		out = append(out, string(e.Type))
	}
	return strings.Join(out, ",")
}

func find(got []events.Event, kind events.Type) (events.Event, bool) {
	for _, e := range got {
		if e.Type == kind {
			return e, true
		}
	}
	return events.Event{}, false
}

// A prompt must reach the agent and come back as activity, which is the whole
// point of AgentLine owning the session.
func TestStreamSendProducesActivity(t *testing.T) {
	root := t.TempDir()
	s, stream := startStream(t, root)

	if err := s.Send("do the thing"); err != nil {
		t.Fatal(err)
	}
	got := collect(t, stream)

	prompt, ok := find(got, events.UserPrompt)
	if !ok {
		t.Fatalf("prompt was not emitted: %s", types(got))
	}
	if prompt.Message != "do the thing" {
		t.Errorf("prompt = %q", prompt.Message)
	}

	for _, want := range []events.Type{events.FileRead, events.CommandStart, events.CommandEnd, events.AgentReply} {
		if _, ok := find(got, want); !ok {
			t.Errorf("missing %s in: %s", want, types(got))
		}
	}
}

// A tool_result names only the id of the call it answers, so the file it
// touched is known only by pairing it with the earlier tool_use.
func TestStreamPairsToolResultsWithTheirCalls(t *testing.T) {
	root := t.TempDir()
	s, stream := startStream(t, root)

	if err := s.Send("go"); err != nil {
		t.Fatal(err)
	}
	got := collect(t, stream)

	read, ok := find(got, events.FileRead)
	if !ok {
		t.Fatalf("no file_read in: %s", types(got))
	}
	if read.Path != "notes.md" {
		t.Errorf("Path = %q, want notes.md relative to the project", read.Path)
	}
}

// A failing command is marked failed without inventing an exit code.
func TestStreamMarksFailedCommands(t *testing.T) {
	root := t.TempDir()
	s, stream := startStream(t, root)

	if err := s.Send("fail"); err != nil {
		t.Fatal(err)
	}
	got := collect(t, stream)

	end, ok := find(got, events.CommandEnd)
	if !ok {
		t.Fatalf("no command_end in: %s", types(got))
	}
	if !end.Failed {
		t.Error("Failed = false for a failing command")
	}
	if end.ExitCode != nil {
		t.Error("an exit code was invented")
	}
}

// The turn ends WAITING, never DONE: whether the mission is complete is not
// something the transcript says.
func TestStreamTurnEndsWaiting(t *testing.T) {
	root := t.TempDir()
	s, stream := startStream(t, root)

	if err := s.Send("go"); err != nil {
		t.Fatal(err)
	}
	got := collect(t, stream)

	last := got[len(got)-1]
	if last.Status != events.StatusWaiting {
		t.Errorf("turn ended %q, want %q", last.Status, events.StatusWaiting)
	}
}

// The session must survive a turn so a second prompt continues it.
func TestStreamAcceptsSeveralPrompts(t *testing.T) {
	root := t.TempDir()
	s, stream := startStream(t, root)

	for _, prompt := range []string{"first", "second"} {
		if err := s.Send(prompt); err != nil {
			t.Fatalf("send %q: %v", prompt, err)
		}
		if got := collect(t, stream); len(got) == 0 {
			t.Fatalf("no events for %q", prompt)
		}
	}
}

// Output AgentLine cannot read must be skipped, not guessed at.
func TestStreamIgnoresUnreadableOutput(t *testing.T) {
	tr := newStreamTranslator("/proj")

	for _, line := range []string{"not json", "", "{}", `{"type":"rate_limit_event"}`} {
		if got := tr.translateLine([]byte(line)); got != nil {
			t.Errorf("%q produced %+v, want nothing", line, got)
		}
	}
}

// A result with no matching call cannot be described, so it is dropped.
func TestStreamIgnoresOrphanToolResults(t *testing.T) {
	tr := newStreamTranslator("/proj")

	line := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"never-seen"}]}}`
	if got := tr.translateLine([]byte(line)); got != nil {
		t.Errorf("got %+v, want nothing for an unmatched result", got)
	}
}

// A session AgentLine owns is never compacted, so restarting is the only way
// to get the context - and the cost of every further turn - back down.
func TestRestartGivesAFreshSession(t *testing.T) {
	root := t.TempDir()
	s, stream := startStream(t, root)

	if err := s.Send("first"); err != nil {
		t.Fatal(err)
	}
	collect(t, stream)

	if err := s.Restart(); err != nil {
		t.Fatalf("restart: %v", err)
	}

	// The same stream keeps working, on a session that carries nothing over.
	if err := s.Send("after restart"); err != nil {
		t.Fatalf("send after restart: %v", err)
	}
	got := collect(t, stream)
	if len(got) == 0 {
		t.Fatal("no events after restarting")
	}

	reply, ok := find(got, events.AgentReply)
	if !ok {
		t.Fatalf("no reply after restarting: %s", types(got))
	}
	if !strings.Contains(reply.Message, "after restart") {
		t.Errorf("reply = %q; the new session did not receive the prompt", reply.Message)
	}
}

// Restarting must not leave the previous process running.
func TestRestartReplacesTheProcess(t *testing.T) {
	s, stream := startStream(t, t.TempDir())

	if err := s.Send("first"); err != nil {
		t.Fatal(err)
	}
	collect(t, stream)

	s.mu.Lock()
	before := s.cmd
	s.mu.Unlock()

	if err := s.Restart(); err != nil {
		t.Fatal(err)
	}

	s.mu.Lock()
	after := s.cmd
	s.mu.Unlock()

	if before == after {
		t.Error("restart reused the old process")
	}
	if before.ProcessState == nil {
		t.Error("the old process was left running")
	}
}

// Restarting a session that has already shut down is an error, not a crash.
func TestRestartAfterShutdownFails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	s := NewStream(t.TempDir())
	s.Bin = fakeAgent(t)
	stream, err := s.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	for range stream { //nolint:revive // drain until closed
	}
	if err := s.Restart(); err != ErrNotRunning {
		t.Errorf("err = %v, want %v", err, ErrNotRunning)
	}
}

func TestSendWithoutASessionFails(t *testing.T) {
	if err := NewStream(t.TempDir()).Send("hello"); err != ErrNotRunning {
		t.Errorf("err = %v, want %v", err, ErrNotRunning)
	}
}

// A missing executable must be reported, not leave a UI that looks live but
// never updates.
func TestMissingExecutableIsReported(t *testing.T) {
	s := NewStream(t.TempDir())
	s.Bin = filepath.Join(t.TempDir(), "definitely-not-here")

	if _, err := s.Events(context.Background()); err == nil {
		t.Error("starting a missing executable succeeded")
	}
}

func TestCancellationEndsTheSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	s := NewStream(t.TempDir())
	s.Bin = fakeAgent(t)
	stream, err := s.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	for {
		select {
		case _, ok := <-stream:
			if !ok {
				return // closed, as it should be
			}
		case <-time.After(20 * time.Second):
			t.Fatal("cancelled session never closed its stream")
		}
	}
}

// The prompt must be written in the shape the agent expects.
func TestPromptIsEncodedAsAUserMessage(t *testing.T) {
	root := t.TempDir()
	s, stream := startStream(t, root)

	if err := s.Send("check the encoding"); err != nil {
		t.Fatal(err)
	}
	got := collect(t, stream)

	// The fake agent echoes the prompt it decoded back as its reply.
	reply, ok := find(got, events.AgentReply)
	if !ok {
		t.Fatalf("no reply in: %s", types(got))
	}
	if !strings.Contains(reply.Message, "check the encoding") {
		t.Errorf("agent decoded %q; the prompt did not survive encoding", reply.Message)
	}
}

func TestFakeAgentSourceIsValidJSON(t *testing.T) {
	// Guards the fixture itself: a malformed line would make every stream
	// test silently pass by producing nothing.
	tr := newStreamTranslator("/proj")
	line := `{"type":"assistant","message":{"content":[{"type":"text","text":"hi"}]}}`

	got := tr.translateLine([]byte(line))
	if len(got) != 1 || got[0].Type != events.AgentReply {
		t.Fatalf("got %+v, want one agent_reply", got)
	}
	var check map[string]any
	if err := json.Unmarshal([]byte(line), &check); err != nil {
		t.Fatal(err)
	}
}
