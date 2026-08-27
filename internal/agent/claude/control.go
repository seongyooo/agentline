package claude

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Claude Code accepts control requests on the same stdin as prompts. They are
// how a session's settings are changed once it is running, which is otherwise
// only possible with a flag at launch.
//
// The wire format is not in the published SDK documentation; it was read out
// of the CLI itself, so it is pinned here rather than spread through the code
// and is covered by tests that would fail loudly if it drifted.
const (
	controlSetPermissionMode = "set_permission_mode"
	controlSetModel          = "set_model"
	controlInterrupt         = "interrupt"
	controlContextUsage      = "get_context_usage"
)

// PermissionModes are the modes a session can be switched between.
//
// bypassPermissions is deliberately absent: it turns off every check, and
// arriving there by pressing a key one time too many is not a mistake worth
// making possible. It remains available as a launch flag, where choosing it
// is deliberate.
var PermissionModes = []string{"default", "acceptEdits", "plan", "auto"}

// SetPermissionMode changes how the running session approves tool calls.
func (s *Stream) SetPermissionMode(mode string) error {
	return s.control(map[string]any{"subtype": controlSetPermissionMode, "mode": mode})
}

// SetModel changes the model the running session uses. An empty model returns
// it to the session default.
func (s *Stream) SetModel(model string) error {
	request := map[string]any{"subtype": controlSetModel}
	if model != "" {
		request["model"] = model
	}
	return s.control(request)
}

// Interrupt stops what the agent is currently doing.
func (s *Stream) Interrupt() error {
	return s.control(map[string]any{"subtype": controlInterrupt})
}

// RequestContextUsage asks how full the context window is.
//
// The share used is what says whether a session is getting expensive, and only
// the agent knows the window it is measuring against, so it is asked rather
// than worked out from a token count and a guess at the limit.
func (s *Stream) RequestContextUsage() error {
	return s.control(map[string]any{"subtype": controlContextUsage})
}

// control sends one control request.
//
// The reply arrives asynchronously on the output stream, where a failure
// becomes an error event. Waiting for it here would block the UI on a session
// that may be busy, and the outcome is visible either way.
func (s *Stream) control(request map[string]any) error {
	line, err := json.Marshal(map[string]any{
		"type":       "control_request",
		"request_id": requestID(),
		"request":    request,
	})
	if err != nil {
		return fmt.Errorf("encode control request: %w", err)
	}
	return s.write(line)
}

// requestID returns an identifier the agent echoes back on the response.
func requestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Only used to correlate a reply that is logged, not acted on.
		return "agentview"
	}
	return "agentview-" + hex.EncodeToString(b[:])
}
