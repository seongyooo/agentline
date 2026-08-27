package state

import (
	"testing"
	"time"

	"github.com/seongyooo/agentline/internal/events"
)

func ask(id string) *events.Ask {
	return &events.Ask{ID: id, Tool: "Write", Title: "Write", Target: "Valve.cs"}
}

func TestAskBlocksTheAgent(t *testing.T) {
	s := New("/proj")
	s.Apply(events.Event{Type: events.PermissionAsk, Timestamp: time.Now(), Source: "claude", Ask: ask("a")})

	if s.Agent.Status != events.StatusNeedsInput {
		t.Errorf("status = %q, want needs_input", s.Agent.Status)
	}
	if s.Agent.Ask == nil || s.Agent.Ask.ID != "a" {
		t.Errorf("ask = %v, want the one that arrived", s.Agent.Ask)
	}
	// NOW has to say what is being waited on, not just that something is.
	if got := s.Agent.Now.Summary; got != "Write Valve.cs?" {
		t.Errorf("now = %q, want the question", got)
	}
}

func TestAnsweringUnblocks(t *testing.T) {
	s := New("/proj")
	now := time.Now()
	s.Apply(events.Event{Type: events.PermissionAsk, Timestamp: now, Source: "claude", Ask: ask("a")})
	s.Apply(events.Event{Type: events.PermissionAnswered, Timestamp: now, Source: "claude",
		Ask: &events.Ask{ID: "a", Allowed: true}})

	if s.Agent.Ask != nil {
		t.Errorf("ask survived its answer: %v", s.Agent.Ask)
	}
	if s.Agent.Status != events.StatusWorking {
		t.Errorf("status = %q, want working once it may carry on", s.Agent.Status)
	}
}

// A refusal is a decision, not a fault. Reporting it as an error would put the
// agent in a state the user themselves caused and cannot clear.
func TestRefusalIsNotAnError(t *testing.T) {
	s := New("/proj")
	now := time.Now()
	s.Apply(events.Event{Type: events.PermissionAsk, Timestamp: now, Source: "claude", Ask: ask("a")})
	s.Apply(events.Event{Type: events.PermissionAnswered, Timestamp: now, Source: "claude",
		Ask: &events.Ask{ID: "a", Allowed: false}})

	if s.Agent.Status == events.StatusError {
		t.Error("saying no put the session in an error state")
	}
	if s.Agent.Ask != nil {
		t.Error("ask survived a refusal")
	}
}

// AgentLine shutting down denies whatever is open. If that racing answer
// arrived after the next question, clearing it would drop a question nobody
// has answered and leave the agent blocked with a blank screen.
func TestALateAnswerCannotClearTheNextQuestion(t *testing.T) {
	s := New("/proj")
	now := time.Now()

	s.Apply(events.Event{Type: events.PermissionAsk, Timestamp: now, Source: "claude", Ask: ask("first")})
	s.Apply(events.Event{Type: events.PermissionAnswered, Timestamp: now, Source: "claude",
		Ask: &events.Ask{ID: "first", Allowed: true}})
	s.Apply(events.Event{Type: events.PermissionAsk, Timestamp: now, Source: "claude", Ask: ask("second")})

	s.Apply(events.Event{Type: events.PermissionAnswered, Timestamp: now, Source: "claude",
		Ask: &events.Ask{ID: "first", Allowed: false}})

	if s.Agent.Ask == nil || s.Agent.Ask.ID != "second" {
		t.Errorf("ask = %v; a stale answer cleared the live question", s.Agent.Ask)
	}
}
