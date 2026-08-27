package claude

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/seongyooo/agentline/internal/events"
)

// realAsk is a request captured from a live Claude Code session, unedited.
//
// The permission protocol is not documented; it was read out of the CLI and
// then confirmed by running one. Pinning the real payload here is what will
// say so if the shape ever moves, instead of AgentLine quietly never asking
// again.
const realAsk = `{"type":"control_request","request_id":"2bd4fd6d-7468-41f0-9de9-bd34469500a8","request":{` +
	`"subtype":"can_use_tool","tool_name":"Write","display_name":"Write",` +
	`"input":{"file_path":"C:\\proj\\probe.txt","content":"hi"},"description":"probe.txt",` +
	`"permission_suggestions":[{"type":"setMode","mode":"acceptEdits","destination":"session"}],` +
	`"decision_reason":"Claude requested permissions to edit C:\\proj\\probe.txt which is a sensitive file.",` +
	`"decision_reason_type":"safetyCheck","classifier_approvable":true,"tool_use_id":"toolu_018J9ECKii9QgvVzeRgaXc1w"}}`

func TestPermissionRequestBecomesAnAsk(t *testing.T) {
	tr := newStreamTranslator("/proj")

	got := tr.translateLine([]byte(realAsk))
	if len(got) != 1 || got[0].Type != events.PermissionAsk {
		t.Fatalf("translated to %v, want one permission ask", got)
	}

	ask := got[0].Ask
	if ask == nil {
		t.Fatal("ask event carries no ask")
	}
	if ask.ID != "2bd4fd6d-7468-41f0-9de9-bd34469500a8" {
		t.Errorf("ID = %q; without the right one the answer goes nowhere", ask.ID)
	}
	if ask.Tool != "Write" || ask.Title != "Write" {
		t.Errorf("tool = %q/%q, want Write", ask.Tool, ask.Title)
	}
	if ask.Target != "probe.txt" {
		t.Errorf("target = %q, want probe.txt", ask.Target)
	}
	if !strings.Contains(ask.Reason, "sensitive file") {
		t.Errorf("reason = %q, want the agent's own explanation", ask.Reason)
	}
	// The agent offered a mode that would stop it asking about this kind of
	// thing again, and that offer is the third answer the UI can give.
	if ask.Mode != "acceptEdits" {
		t.Errorf("suggested mode = %q, want acceptEdits", ask.Mode)
	}
}

// A control request AgentLine does not understand must be left alone rather
// than answered with a guess.
func TestUnknownControlRequestIsIgnored(t *testing.T) {
	tr := newStreamTranslator("/proj")

	line := `{"type":"control_request","request_id":"x","request":{"subtype":"something_new"}}`
	if got := tr.translateLine([]byte(line)); got != nil {
		t.Errorf("translated an unknown control request to %v", got)
	}
}

// answerOf runs one answer against a stream and returns what went out.
func answerOf(t *testing.T, allow bool, message string) map[string]any {
	t.Helper()

	var out strings.Builder
	s := NewStream("/proj")
	s.stdin = nopCloser{&out}
	s.out = make(chan events.Event, 4)

	if got := s.translator.translateLine([]byte(realAsk)); len(got) != 1 {
		t.Fatalf("setup: ask did not translate: %v", got)
	}
	if err := s.Answer("2bd4fd6d-7468-41f0-9de9-bd34469500a8", allow, message); err != nil {
		t.Fatal(err)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &decoded); err != nil {
		t.Fatalf("answer is not JSON: %v\n%s", err, out.String())
	}
	return decoded
}

func TestAllowEchoesTheInputBack(t *testing.T) {
	got := answerOf(t, true, "")

	if got["type"] != "control_response" {
		t.Errorf("type = %v, want control_response", got["type"])
	}
	response, _ := got["response"].(map[string]any)
	if response["request_id"] != "2bd4fd6d-7468-41f0-9de9-bd34469500a8" {
		t.Errorf("request_id = %v; the agent matches the answer on it", response["request_id"])
	}

	decision, _ := response["response"].(map[string]any)
	if decision["behavior"] != behaviorAllow {
		t.Errorf("behavior = %v, want allow", decision["behavior"])
	}
	// Approving a different call than the one shown on screen would be a lie
	// about what the user agreed to.
	input, _ := decision["updatedInput"].(map[string]any)
	if input["file_path"] != `C:\proj\probe.txt` || input["content"] != "hi" {
		t.Errorf("updatedInput = %v, want the input that was asked about", input)
	}
}

func TestDenyCarriesTheReason(t *testing.T) {
	got := answerOf(t, false, "The user declined this.")

	response, _ := got["response"].(map[string]any)
	decision, _ := response["response"].(map[string]any)

	if decision["behavior"] != behaviorDeny {
		t.Errorf("behavior = %v, want deny", decision["behavior"])
	}
	if decision["message"] != "The user declined this." {
		t.Errorf("message = %v; the agent is not told why", decision["message"])
	}
	if _, ok := decision["updatedInput"]; ok {
		t.Error("a denial carried an input, which would read as approval")
	}
}

// Two answers for one request would be a protocol error. The second is dropped.
func TestAnsweringTwiceWritesOnce(t *testing.T) {
	var out strings.Builder
	s := NewStream("/proj")
	s.stdin = nopCloser{&out}
	s.out = make(chan events.Event, 4)

	s.translator.translateLine([]byte(realAsk))
	const id = "2bd4fd6d-7468-41f0-9de9-bd34469500a8"

	for i := 0; i < 3; i++ {
		if err := s.Answer(id, true, ""); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Count(strings.TrimSpace(out.String()), "\n") + 1; got != 1 {
		t.Errorf("wrote %d answers for one request", got)
	}
}

// Before AgentLine took the prompts, an unanswerable call was refused by the
// agent instantly. Now it waits, so going away without answering has to become
// an ordinary refusal rather than a session hung on a question.
func TestShutdownRefusesWhatIsStillOpen(t *testing.T) {
	var out strings.Builder
	s := NewStream("/proj")
	s.stdin = nopCloser{&out}
	s.out = make(chan events.Event, 4)

	s.translator.translateLine([]byte(realAsk))

	closed := s.denyOutstanding("AgentLine closed")
	if len(closed) != 1 || closed[0].Type != events.PermissionAnswered || closed[0].Ask.Allowed {
		t.Fatalf("shutdown produced %v, want one refusal", closed)
	}
	if !strings.Contains(out.String(), behaviorDeny) {
		t.Errorf("nothing was written to release the agent:\n%s", out.String())
	}
}

// nopCloser makes a buffer usable where the session's stdin goes.
type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }
