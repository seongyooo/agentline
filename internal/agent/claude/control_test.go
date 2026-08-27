package claude

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/seonl/agentview/internal/events"
)

// The control wire format is not in the published documentation — it was read
// out of the CLI itself — so these tests pin the exact shape. If the agent
// ever changes it, they are what says so, rather than a request that is
// silently ignored.

// encodeControl builds the request a control call sends.
func encodeControl(t *testing.T, request map[string]any) map[string]any {
	t.Helper()

	line, err := json.Marshal(map[string]any{
		"type":       "control_request",
		"request_id": requestID(),
		"request":    request,
	})
	if err != nil {
		t.Fatal(err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(line, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestPermissionModeRequestShape(t *testing.T) {
	got := encodeControl(t, map[string]any{"subtype": controlSetPermissionMode, "mode": "acceptEdits"})

	if got["type"] != "control_request" {
		t.Errorf("type = %v, want control_request", got["type"])
	}
	if id, _ := got["request_id"].(string); id == "" {
		t.Error("no request_id, which the agent echoes on its reply")
	}

	request, _ := got["request"].(map[string]any)
	if request["subtype"] != "set_permission_mode" {
		t.Errorf("subtype = %v, want set_permission_mode", request["subtype"])
	}
	if request["mode"] != "acceptEdits" {
		t.Errorf("mode = %v, want acceptEdits", request["mode"])
	}
}

func TestModelRequestShape(t *testing.T) {
	request, _ := encodeControl(t, map[string]any{"subtype": controlSetModel, "model": "opus"})["request"].(map[string]any)

	if request["subtype"] != "set_model" {
		t.Errorf("subtype = %v, want set_model", request["subtype"])
	}
	if request["model"] != "opus" {
		t.Errorf("model = %v, want opus", request["model"])
	}
}

// Every request carries its own id, or replies could not be told apart.
func TestRequestIDsAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := requestID()
		if seen[id] {
			t.Fatalf("duplicate request id %q", id)
		}
		seen[id] = true
	}
}

// A session that has shut down cannot be controlled, and says so.
func TestControlWithoutASessionFails(t *testing.T) {
	s := NewStream(t.TempDir())

	for name, call := range map[string]func() error{
		"permission mode": func() error { return s.SetPermissionMode("plan") },
		"model":           func() error { return s.SetModel("opus") },
		"interrupt":       s.Interrupt,
	} {
		if err := call(); err != ErrNotRunning {
			t.Errorf("%s: err = %v, want %v", name, err, ErrNotRunning)
		}
	}
}

// The session announces what it is and what it accepts; the UI offers that
// rather than a list of its own.
func TestInitAnnouncementIsRead(t *testing.T) {
	tr := newStreamTranslator("/proj")

	line := `{"type":"system","subtype":"init","session_id":"abc",
	  "model":"claude-opus-5-20260514","permissionMode":"acceptEdits",
	  "slash_commands":["model","compact","clear"]}`

	got := tr.translateLine([]byte(line))
	if len(got) != 2 {
		t.Fatalf("got %d events, want a status and a session report: %+v", len(got), got)
	}

	session := got[1].Session
	if session == nil {
		t.Fatal("no session report")
	}
	if session.Model != "claude-opus-5-20260514" {
		t.Errorf("Model = %q", session.Model)
	}
	if session.Capabilities.PermissionMode != "acceptEdits" {
		t.Errorf("PermissionMode = %q, want acceptEdits", session.Capabilities.PermissionMode)
	}
	if got, want := strings.Join(session.Capabilities.SlashCommands, ","), "model,compact,clear"; got != want {
		t.Errorf("SlashCommands = %q, want %q", got, want)
	}
}

// A refused control request must surface rather than vanish.
func TestControlFailureBecomesAnError(t *testing.T) {
	tr := newStreamTranslator("/proj")

	line := `{"type":"control_response","response":{"subtype":"error",
	  "request_id":"agentview-1","error":"set_model: model must be a string"}}`

	got := tr.translateLine([]byte(line))
	if len(got) != 1 || got[0].Type != events.AgentError {
		t.Fatalf("got %+v, want one error event", got)
	}
	if got[0].Message != "set_model: model must be a string" {
		t.Errorf("Message = %q", got[0].Message)
	}
}

// A request that worked shows up as the change it made, not as a message.
func TestSuccessfulControlResponseIsQuiet(t *testing.T) {
	tr := newStreamTranslator("/proj")

	line := `{"type":"control_response","response":{"subtype":"success","request_id":"agentview-1"}}`
	if got := tr.translateLine([]byte(line)); got != nil {
		t.Errorf("got %+v, want nothing for a request that succeeded", got)
	}
}

// The cycle must not include the mode that turns every check off.
func TestPermissionModesExcludeBypass(t *testing.T) {
	for _, mode := range PermissionModes {
		if mode == "bypassPermissions" {
			t.Error("bypassPermissions is in the cycle")
		}
	}
	if len(PermissionModes) < 2 {
		t.Error("a cycle needs somewhere to go")
	}
}
