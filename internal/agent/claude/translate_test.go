package claude

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/seongyooo/agentline/internal/events"
)

const root = `C:\proj`

func tr() *translator { return newTranslator(root) }

// decode parses a payload exactly as it arrives over the wire, so the tests
// exercise the JSON tags rather than hand-built structs.
func decode(t *testing.T, raw string) payload {
	t.Helper()
	var p payload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

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

// This payload is a real PostToolUse capture, trimmed to the fields the
// adapter reads. See docs/hook-spike.md.
const readPayload = `{
  "hook_event_name": "PostToolUse",
  "tool_name": "Read",
  "tool_use_id": "toolu_01NNFgav41Kn3PztJDhS5rmK",
  "cwd": "C:\\proj",
  "tool_input": { "file_path": "C:\\proj\\notes.md" }
}`

func TestReadBecomesFileRead(t *testing.T) {
	got := only(t, tr().translate(decode(t, readPayload)))

	if got.Type != events.FileRead {
		t.Errorf("Type = %q, want %q", got.Type, events.FileRead)
	}
	if got.Path != "notes.md" {
		t.Errorf("Path = %q, want %q", got.Path, "notes.md")
	}
	if got.Source != SourceName {
		t.Errorf("Source = %q, want %q", got.Source, SourceName)
	}
}

func TestFileToolsMapToFileEvents(t *testing.T) {
	tests := []struct {
		tool string
		want events.Type
	}{
		{"Read", events.FileRead},
		{"Edit", events.FileEdit},
		{"NotebookEdit", events.FileEdit},
		{"Write", events.FileCreate},
	}

	for _, tc := range tests {
		t.Run(tc.tool, func(t *testing.T) {
			p := payload{
				HookEventName: hookPostToolUse,
				ToolName:      tc.tool,
				ToolInput:     toolInput{FilePath: filepath.Join(root, "src", "main.go")},
			}
			if got := only(t, tr().translate(p)); got.Type != tc.want {
				t.Errorf("Type = %q, want %q", got.Type, tc.want)
			}
		})
	}
}

// The shell tool is called PowerShell on Windows. Matching only "Bash" would
// silently drop every command event there.
func TestBothShellToolNamesProduceCommandEvents(t *testing.T) {
	for _, tool := range []string{"Bash", "PowerShell"} {
		t.Run(tool, func(t *testing.T) {
			start := only(t, tr().translate(payload{
				HookEventName: hookPreToolUse,
				ToolName:      tool,
				ToolInput:     toolInput{Command: "go test ./..."},
			}))
			if start.Type != events.CommandStart {
				t.Errorf("Type = %q, want %q", start.Type, events.CommandStart)
			}
			if start.Command != "go test ./..." {
				t.Errorf("Command = %q", start.Command)
			}

			end := only(t, tr().translate(payload{
				HookEventName: hookPostToolUse,
				ToolName:      tool,
				ToolInput:     toolInput{Command: "go test ./..."},
			}))
			if end.Type != events.CommandEnd {
				t.Errorf("Type = %q, want %q", end.Type, events.CommandEnd)
			}
			if end.Failed {
				t.Error("a successful command was marked failed")
			}
		})
	}
}

// A failing command produces PostToolUseFailure, never PostToolUse, and the
// exit code is only in prose — so failure is recorded without inventing a code.
func TestFailedCommandIsMarkedFailedWithoutAnExitCode(t *testing.T) {
	const raw = `{
	  "hook_event_name": "PostToolUseFailure",
	  "tool_name": "PowerShell",
	  "error": "Exit code 3\n3",
	  "is_interrupt": false,
	  "tool_input": { "command": "cmd /c exit 3" }
	}`

	got := only(t, tr().translate(decode(t, raw)))

	if got.Type != events.CommandEnd {
		t.Errorf("Type = %q, want %q", got.Type, events.CommandEnd)
	}
	if !got.Failed {
		t.Error("Failed = false, want true")
	}
	if got.ExitCode != nil {
		t.Errorf("ExitCode = %v, want nil: the code was never reported as a field", *got.ExitCode)
	}
	if got.Message != "Exit code 3" {
		t.Errorf("Message = %q, want %q", got.Message, "Exit code 3")
	}
}

func TestFailedFileToolBecomesAgentError(t *testing.T) {
	got := only(t, tr().translate(payload{
		HookEventName: hookPostToolUseFailure,
		ToolName:      "Edit",
		Error:         "File does not exist",
		ToolInput:     toolInput{FilePath: filepath.Join(root, "gone.go")},
	}))

	if got.Type != events.AgentError {
		t.Errorf("Type = %q, want %q", got.Type, events.AgentError)
	}
	if got.Message != "File does not exist" {
		t.Errorf("Message = %q", got.Message)
	}
}

// Stop ends the turn, so the agent is waiting. It is not reported as DONE:
// whether the mission is complete is not observable from a hook.
func TestStopBecomesWaitingNotDone(t *testing.T) {
	for _, hook := range []string{hookStop, hookSessionEnd} {
		t.Run(hook, func(t *testing.T) {
			got := only(t, tr().translate(payload{HookEventName: hook}))

			if got.Type != events.AgentStatus {
				t.Fatalf("Type = %q, want %q", got.Type, events.AgentStatus)
			}
			if got.Status != events.StatusWaiting {
				t.Errorf("Status = %q, want %q", got.Status, events.StatusWaiting)
			}
		})
	}
}

// The prompt is what MISSION is derived from, so it must be carried through
// verbatim rather than summarized in the adapter.
func TestUserPromptIsCarriedThrough(t *testing.T) {
	const raw = `{
	  "hook_event_name": "UserPromptSubmit",
	  "prompt": "Read notes.md, append a line, then run: echo done",
	  "session_id": "fb4d627a"
	}`

	got := only(t, tr().translate(decode(t, raw)))

	if got.Type != events.UserPrompt {
		t.Errorf("Type = %q, want %q", got.Type, events.UserPrompt)
	}
	if want := "Read notes.md, append a line, then run: echo done"; got.Message != want {
		t.Errorf("Message = %q, want %q", got.Message, want)
	}
}

// A prompt hook with no text still says the agent has been given work.
func TestUserPromptSubmitWithoutTextMarksWorking(t *testing.T) {
	got := only(t, tr().translate(payload{HookEventName: hookUserPromptSubmit}))

	if got.Status != events.StatusWorking {
		t.Errorf("Status = %q, want %q", got.Status, events.StatusWorking)
	}
}

// PreToolUse only means "about to", and a write can still be denied. So a
// write is announced as claimed rather than done, which is what lets the file
// appear in the tree while it is being written without ever claiming a change
// that did not happen.
func TestWritesAreAnnouncedAsPendingNotDone(t *testing.T) {
	for _, tool := range []string{"Edit", "Write", "NotebookEdit"} {
		t.Run(tool, func(t *testing.T) {
			got := only(t, tr().translate(payload{
				HookEventName: hookPreToolUse,
				ToolName:      tool,
				ToolInput:     toolInput{FilePath: filepath.Join(root, "x.go")},
			}))

			if got.Type != events.FilePending {
				t.Errorf("Type = %q, want %q", got.Type, events.FilePending)
			}
			if got.Path != "x.go" {
				t.Errorf("Path = %q, want x.go", got.Path)
			}
		})
	}
}

// A read changes nothing, so there is nothing to show before it happens.
func TestReadsAreNotAnnouncedBeforeTheyRun(t *testing.T) {
	got := tr().translate(payload{
		HookEventName: hookPreToolUse,
		ToolName:      "Read",
		ToolInput:     toolInput{FilePath: filepath.Join(root, "x.go")},
	})
	if got != nil {
		t.Errorf("got %+v, want nothing before a read has run", got)
	}
}

// A write outside the project is dropped rather than claimed under a path it
// does not have.
func TestPendingWriteOutsideProjectIsDropped(t *testing.T) {
	got := tr().translate(payload{
		HookEventName: hookPreToolUse,
		ToolName:      "Write",
		ToolInput:     toolInput{FilePath: `C:\elsewhere\secret.txt`},
	})
	if got != nil {
		t.Errorf("got %+v, want nothing for a write outside the project", got)
	}
}

func TestUnknownHooksProduceNothing(t *testing.T) {
	for _, hook := range []string{"SessionStart", "PreCompact", "MessageDisplay", ""} {
		if got := tr().translate(payload{HookEventName: hook}); got != nil {
			t.Errorf("%q produced %+v, want nothing", hook, got)
		}
	}
}

func TestUnknownToolProducesNoFileEvent(t *testing.T) {
	got := tr().translate(payload{
		HookEventName: hookPostToolUse,
		ToolName:      "WebFetch",
		ToolInput:     toolInput{FilePath: filepath.Join(root, "x.go")},
	})
	if got != nil {
		t.Errorf("got %+v, want nothing for a tool with no file meaning", got)
	}
}

// Hook paths are absolute; the event model and tree are root-relative.
func TestPathRelativization(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"nested file", filepath.Join(root, "internal", "tui", "view.go"), "internal/tui/view.go"},
		{"root file", filepath.Join(root, "go.mod"), "go.mod"},
		{"outside the project", `C:\elsewhere\secret.txt`, ""},
		{"parent of the project", `C:\`, ""},
		{"empty", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tr().relative(tc.in); got != tc.want {
				t.Errorf("relative(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A file touched outside the project must be dropped, not shown under a path
// it does not have.
func TestFileOutsideProjectIsDropped(t *testing.T) {
	got := tr().translate(payload{
		HookEventName: hookPostToolUse,
		ToolName:      "Read",
		ToolInput:     toolInput{FilePath: `C:\elsewhere\secret.txt`},
	})
	if got != nil {
		t.Errorf("got %+v, want nothing for a file outside the project", got)
	}
}
