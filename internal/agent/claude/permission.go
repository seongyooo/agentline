package claude

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/seongyooo/agentline/internal/events"
)

// Claude Code asks for permission over the same stream it reports on, but only
// when it is told to. Without the flag below it auto-denies anything the
// current mode does not already allow, which is what a spike against the real
// binary observed and what docs/hook-spike.md recorded as a dead end.
//
// The wire format is not documented. It was read out of the CLI and then
// confirmed by running one: the request arrives, an answer is written back,
// and the tool call proceeds. What that observation produced is pinned here.
//
//	CLI  → AgentLine   {"type":"control_request","request_id":"…",
//	                    "request":{"subtype":"can_use_tool","tool_name":"Write",
//	                               "display_name":"Write","input":{…},
//	                               "description":"probe.txt",
//	                               "decision_reason":"…","tool_use_id":"…",
//	                               "permission_suggestions":[{"type":"setMode",…}]}}
//
//	AgentLine → CLI    {"type":"control_response","response":{
//	                      "subtype":"success","request_id":"…",
//	                      "response":{"behavior":"allow","updatedInput":{…}}}}
//
// A denial carries {"behavior":"deny","message":"…"} instead.
const (
	// permissionPromptTool routes permission prompts to us over the stream.
	permissionPromptTool = "stdio"

	controlCanUseTool = "can_use_tool"

	behaviorAllow = "allow"
	behaviorDeny  = "deny"
)

// askStore holds what an outstanding ask needs in order to be answered.
//
// The neutral event carries only what a person reads. Answering means echoing
// the provider's own tool input back to it unchanged, so that stays here, in
// the adapter that received it, rather than travelling through the event model
// where nothing else would know what to do with it.
type askStore struct {
	mu   sync.Mutex
	open map[string]json.RawMessage
}

func newAskStore() *askStore {
	return &askStore{open: map[string]json.RawMessage{}}
}

func (a *askStore) put(id string, input json.RawMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.open[id] = input
}

// take removes an ask and reports whether it was still open, so answering the
// same ask twice cannot write two responses for one request.
func (a *askStore) take(id string) (json.RawMessage, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	input, ok := a.open[id]
	delete(a.open, id)
	return input, ok
}

// drain empties the store and returns what was in it.
func (a *askStore) drain() map[string]json.RawMessage {
	a.mu.Lock()
	defer a.mu.Unlock()

	open := a.open
	a.open = map[string]json.RawMessage{}
	return open
}

// Answer allows or denies an outstanding permission request.
//
// The agent is blocked until this is written, so a session AgentLine owns must
// answer everything it is asked — see denyOutstanding for the case where the
// user never does.
func (s *Stream) Answer(id string, allow bool, message string) error {
	input, ok := s.translator.asks.take(id)
	if !ok {
		// Already answered, or never ours. Writing a second response for one
		// request is worse than doing nothing.
		return nil
	}
	if err := s.writeAnswer(id, input, allow, message); err != nil {
		return err
	}
	// The agent does not acknowledge an answer, it just carries on, so the
	// event that closes the ask is emitted here rather than waited for.
	s.send(s.translator.answered(id, allow))
	return nil
}

func (s *Stream) writeAnswer(id string, input json.RawMessage, allow bool, message string) error {
	decision := map[string]any{"behavior": behaviorDeny, "message": message}
	if allow {
		// The input is echoed back rather than reconstructed: it is the
		// contract's way of letting a client edit a call before approving it,
		// and passing back anything but what was asked about would approve a
		// different call than the one shown on screen.
		decision = map[string]any{"behavior": behaviorAllow, "updatedInput": input}
	}

	line, err := json.Marshal(map[string]any{
		"type": "control_response",
		"response": map[string]any{
			"subtype":    "success",
			"request_id": id,
			"response":   decision,
		},
	})
	if err != nil {
		return fmt.Errorf("encode permission answer: %w", err)
	}
	return s.write(line)
}

// denyOutstanding answers everything still open, so a session is never left
// blocked on a question nobody is there to answer.
//
// This is the price of asking at all: before AgentLine took the prompts, an
// unanswerable call was refused instantly by the agent itself. Now the agent
// waits, and if AgentLine goes away while it waits, the session hangs until
// its stream is closed under it. Denying explicitly turns that into an ordinary
// refusal the agent can report and move on from.
func (s *Stream) denyOutstanding(message string) []events.Event {
	open := s.translator.asks.drain()
	if len(open) == 0 {
		return nil
	}

	out := make([]events.Event, 0, len(open))
	for id, input := range open {
		if err := s.writeAnswer(id, input, false, message); err != nil {
			// The stream is already going away; there is nothing left to try.
			break
		}
		out = append(out, s.translator.answered(id, false))
	}
	return out
}

// controlRequest turns a permission prompt into an ask the UI can put in front
// of someone, and keeps the raw tool input back for the answer.
func (t *streamTranslator) controlRequest(e streamEvent) []events.Event {
	// This is the only control request the agent sends. Anything else is a
	// protocol AgentLine does not know, and guessing at it would mean
	// answering a question it did not understand.
	if e.Request == nil || e.Request.Subtype != controlCanUseTool || e.RequestID == "" {
		return nil
	}
	t.asks.put(e.RequestID, e.Request.Input)

	ask := &events.Ask{
		ID:     e.RequestID,
		Tool:   e.Request.ToolName,
		Title:  e.Request.DisplayName,
		Target: e.Request.Description,
		Reason: firstLine(e.Request.Reason),
	}
	// A suggestion is the agent offering a mode that would let this and
	// everything like it through. Only the first is taken: the UI has room
	// for one key, and a list of ways to loosen permissions is not something
	// to page through while an agent waits.
	for _, suggestion := range e.Request.Suggestions {
		if suggestion.Type == "setMode" && suggestion.Mode != "" {
			ask.Mode = suggestion.Mode
			break
		}
	}

	return []events.Event{t.event(events.PermissionAsk, func(ev *events.Event) { ev.Ask = ask })}
}

// answered reports that an ask is closed, so the UI stops waiting on it.
func (t *translator) answered(id string, allowed bool) events.Event {
	return t.event(events.PermissionAnswered, func(ev *events.Event) {
		ev.Ask = &events.Ask{ID: id, Allowed: allowed}
	})
}
