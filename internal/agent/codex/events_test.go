package codex

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/seongyooo/agentline/internal/events"
)

// The adapter works in whatever paths the agent reports, which differ by
// platform, so the tests are built for the one running them.
func nativePath(segments ...string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(append([]string{`C:\`}, segments...)...)
	}
	return filepath.Join(append([]string{"/"}, segments...)...)
}

var root = nativePath("proj")

func tr() *translator { return newTranslator(root) }

func only(t *testing.T, got []events.Event) events.Event {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(got), got)
	}
	if !got[0].Valid() {
		t.Fatalf("translated event is invalid: %+v", got[0])
	}
	return got[0]
}

// A command is announced when it starts, since it is slow enough that NOW
// should say it is running.
func TestCommandStartIsAnnounced(t *testing.T) {
	line := `{"type":"item.started","item":{"id":"i1","type":"command_execution",
	  "command":"go test ./...","status":"in_progress"}}`

	got := only(t, tr().translateLine([]byte(line)))
	if got.Type != events.CommandStart {
		t.Errorf("Type = %q, want %q", got.Type, events.CommandStart)
	}
	if got.Command != "go test ./..." {
		t.Errorf("Command = %q", got.Command)
	}
}

// Codex reports a real exit code, so unlike the Claude adapter there is a
// number to carry rather than only the fact of failure.
func TestCommandEndCarriesTheExitCode(t *testing.T) {
	line := `{"type":"item.completed","item":{"id":"i1","type":"command_execution",
	  "command":"go test ./...","exit_code":0,"status":"completed"}}`

	got := only(t, tr().translateLine([]byte(line)))
	if got.Type != events.CommandEnd {
		t.Fatalf("Type = %q, want %q", got.Type, events.CommandEnd)
	}
	if got.Failed {
		t.Error("a command that exited 0 was marked failed")
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Errorf("ExitCode = %v, want 0", got.ExitCode)
	}
}

func TestFailedCommandIsMarkedFailed(t *testing.T) {
	line := `{"type":"item.completed","item":{"id":"i1","type":"command_execution",
	  "command":"go build ./...","exit_code":2,"status":"failed"}}`

	got := only(t, tr().translateLine([]byte(line)))
	if !got.Failed {
		t.Error("Failed = false for a command that failed")
	}
	if got.ExitCode == nil || *got.ExitCode != 2 {
		t.Errorf("ExitCode = %v, want 2", got.ExitCode)
	}
}

// A non-zero exit is a failure even where the status does not say so.
func TestNonZeroExitIsAFailure(t *testing.T) {
	line := `{"type":"item.completed","item":{"id":"i1","type":"command_execution",
	  "command":"false","exit_code":1,"status":"completed"}}`

	if got := only(t, tr().translateLine([]byte(line))); !got.Failed {
		t.Error("a non-zero exit was not treated as a failure")
	}
}

// A patch says what it did to each file, so each becomes its own event.
func TestFileChangesBecomeOneEventPerFile(t *testing.T) {
	line := `{"type":"item.completed","item":{"id":"i1","type":"file_change",
	  "status":"completed","changes":[
	    {"path":"src/added.go","kind":"add"},
	    {"path":"src/edited.go","kind":"update"},
	    {"path":"src/gone.go","kind":"delete"}
	  ]}}`

	got := tr().translateLine([]byte(line))
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3: %+v", len(got), got)
	}

	want := []struct {
		path string
		kind events.Type
	}{
		{"src/added.go", events.FileCreate},
		{"src/edited.go", events.FileEdit},
		{"src/gone.go", events.FileDelete},
	}
	for i, w := range want {
		if got[i].Type != w.kind {
			t.Errorf("event %d: Type = %q, want %q", i, got[i].Type, w.kind)
		}
		if got[i].Path != w.path {
			t.Errorf("event %d: Path = %q, want %q", i, got[i].Path, w.path)
		}
	}
}

// A patch that failed changed nothing, so no file event is emitted for it.
func TestFailedPatchReportsNoFileChanges(t *testing.T) {
	line := `{"type":"item.completed","item":{"id":"i1","type":"file_change",
	  "status":"failed","changes":[{"path":"src/main.go","kind":"update"}]}}`

	got := only(t, tr().translateLine([]byte(line)))
	if got.Type != events.AgentError {
		t.Errorf("Type = %q, want %q", got.Type, events.AgentError)
	}
}

// A file outside the project is dropped rather than shown under a path it
// does not have.
func TestFileOutsideTheProjectIsDropped(t *testing.T) {
	outside := nativePath("elsewhere", "secret.txt")
	line := `{"type":"item.completed","item":{"id":"i1","type":"file_change",
	  "status":"completed","changes":[{"path":` + quote(outside) + `,"kind":"update"}]}}`

	if got := tr().translateLine([]byte(line)); got != nil {
		t.Errorf("got %+v, want nothing for a file outside the project", got)
	}
}

// The turn ending means the agent is waiting, not that the work is done.
func TestTurnCompletedMeansWaiting(t *testing.T) {
	line := `{"type":"turn.completed","usage":{"input_tokens":1200,
	  "cached_input_tokens":40000,"cache_write_input_tokens":2000,"output_tokens":800}}`

	got := tr().translateLine([]byte(line))
	if len(got) != 2 {
		t.Fatalf("got %d events, want a status and a usage report: %+v", len(got), got)
	}
	if got[0].Status != events.StatusWaiting {
		t.Errorf("Status = %q, want %q", got[0].Status, events.StatusWaiting)
	}

	session := got[1].Session
	if session == nil {
		t.Fatal("no session report")
	}
	// Cached input still counts towards the context the turn carried.
	if want := 1200 + 40000 + 2000; session.InputTokens != want {
		t.Errorf("InputTokens = %d, want %d", session.InputTokens, want)
	}
}

func TestAgentMessageBecomesAReply(t *testing.T) {
	line := `{"type":"item.completed","item":{"id":"i1","type":"agent_message",
	  "text":"Added the section and ran the tests."}}`

	got := only(t, tr().translateLine([]byte(line)))
	if got.Type != events.AgentReply {
		t.Errorf("Type = %q, want %q", got.Type, events.AgentReply)
	}
	if got.Message != "Added the section and ran the tests." {
		t.Errorf("Message = %q", got.Message)
	}
}

func TestTurnFailureBecomesAnError(t *testing.T) {
	line := `{"type":"turn.failed","error":{"message":"model overloaded\nretry later"}}`

	got := only(t, tr().translateLine([]byte(line)))
	if got.Type != events.AgentError {
		t.Fatalf("Type = %q, want %q", got.Type, events.AgentError)
	}
	if got.Message != "model overloaded" {
		t.Errorf("Message = %q, want the first line only", got.Message)
	}
}

// Output AgentLine cannot read is skipped, not guessed at.
func TestUnreadableLinesProduceNothing(t *testing.T) {
	for _, line := range []string{"not json", "", "{}", `{"type":"item.updated"}`,
		`{"type":"item.completed","item":{"type":"reasoning","text":"thinking"}}`} {
		if got := tr().translateLine([]byte(line)); got != nil {
			t.Errorf("%q produced %+v, want nothing", line, got)
		}
	}
}

// Reasoning text is not shown: AgentLine reports observable activity, not the
// agent's internal monologue.
func TestReasoningIsNotReported(t *testing.T) {
	line := `{"type":"item.completed","item":{"id":"i1","type":"reasoning",
	  "text":"I should check the tests first"}}`

	if got := tr().translateLine([]byte(line)); got != nil {
		t.Errorf("got %+v, want nothing for reasoning", got)
	}
}

func quote(s string) string {
	out := make([]rune, 0, len(s)+2)
	out = append(out, '"')
	for _, r := range s {
		if r == '\\' || r == '"' {
			out = append(out, '\\')
		}
		out = append(out, r)
	}
	return string(append(out, '"'))
}
