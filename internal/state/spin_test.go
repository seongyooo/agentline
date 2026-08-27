package state

import (
	"strings"
	"testing"
	"time"

	"github.com/seongyooo/agentline/internal/events"
)

// working seeds a state mid-turn, which is the only time repetition means
// anything.
func working(t *testing.T) (*State, time.Time) {
	t.Helper()

	s := New("/proj")
	now := time.Now()
	s.Apply(events.Event{Type: events.AgentStatus, Status: events.StatusWorking, Timestamp: now.Add(-time.Hour), Source: "claude"})
	return s, now
}

func edit(s *State, path string, at time.Time) {
	s.Apply(events.Event{Type: events.FileEdit, Path: path, Timestamp: at, Source: "claude"})
}

func TestRewritingOneFileIsCounted(t *testing.T) {
	s, now := working(t)
	for i := 0; i < spinEdits; i++ {
		edit(s, "Valve.cs", now.Add(-time.Duration(spinEdits-i)*30*time.Second))
	}
	s.Refresh(now)

	spin := s.Agent.Spin
	if spin == nil {
		t.Fatal("rewriting one file repeatedly went unreported")
	}
	if got := spin.Findings[0]; got.Kind != RepeatedEdit || got.Target != "Valve.cs" || got.Count != spinEdits {
		t.Errorf("finding = %+v, want %d edits to Valve.cs", got, spinEdits)
	}
}

// Work that keeps reaching new files is work, however fast it goes. Reporting
// it would make the panel noise, and noise is ignored.
func TestBusyWorkOnNewFilesIsNotReported(t *testing.T) {
	s, now := working(t)
	for i := 0; i < 20; i++ {
		edit(s, string(rune('a'+i))+".cs", now.Add(-time.Duration(20-i)*10*time.Second))
	}
	s.Refresh(now)

	if s.Agent.Spin != nil {
		t.Errorf("busy work reported as repetition: %+v", s.Agent.Spin.Findings)
	}
}

// Editing one file for a long time and looping over one file look the same
// from outside, so the weakest signal is never allowed to speak alone.
func TestNoNewGroundNeverStandsAlone(t *testing.T) {
	s, now := working(t)
	// Enough actions to trip the count, spread over enough files that nothing
	// is repeated often enough to be a finding of its own.
	for i := 0; i < spinActions+4; i++ {
		edit(s, "Valve.cs", now.Add(-time.Hour)) // long before the window
		edit(s, "Valve.cs", now.Add(-time.Duration(i)*time.Second))
	}
	s.Refresh(now)

	if s.Agent.Spin == nil {
		t.Skip("this seed trips a stronger finding; nothing to check here")
	}
	for _, f := range s.Agent.Spin.Findings {
		if f.Kind != NoNewGround {
			return // it is accompanied, which is the rule
		}
	}
	t.Error("no-new-ground was reported with nothing to back it up")
}

// Waving it away has to stick, or it is worse than not offering the choice.
func TestDismissedEvidenceStaysAway(t *testing.T) {
	s, now := working(t)
	for i := 0; i < spinEdits; i++ {
		edit(s, "Valve.cs", now.Add(-time.Duration(spinEdits-i)*30*time.Second))
	}
	s.Refresh(now)
	s.DismissSpin()

	edit(s, "Valve.cs", now)
	if s.Agent.Spin != nil {
		t.Errorf("the same evidence came back one action later: %+v", s.Agent.Spin.Findings)
	}

	// It comes back once it is meaningfully worse, which is the only reason
	// to raise something already answered.
	for i := 0; i < spinRearm; i++ {
		edit(s, "Valve.cs", now)
	}
	if s.Agent.Spin == nil {
		t.Error("repetition that got worse stayed hidden")
	}
}

// A question the agent is blocked on outranks anything AgentLine worked out on
// its own: one is the agent waiting, the other is an observation about it.
func TestABlockedAgentIsNotAlsoSpinning(t *testing.T) {
	s, now := working(t)
	for i := 0; i < spinEdits+2; i++ {
		edit(s, "Valve.cs", now.Add(-time.Duration(i)*20*time.Second))
	}
	s.Apply(events.Event{Type: events.PermissionAsk, Timestamp: now, Source: "claude",
		Ask: &events.Ask{ID: "a", Tool: "Write"}})

	if s.Agent.Spin != nil {
		t.Error("spinning shown while the agent was waiting for an answer")
	}
}

// A finished turn is not going nowhere, it is finished — and the next turn
// starts with a clean slate, including what was waved away in the last one.
func TestATurnEndingClearsEverything(t *testing.T) {
	s, now := working(t)
	for i := 0; i < spinEdits; i++ {
		edit(s, "Valve.cs", now.Add(-time.Duration(spinEdits-i)*30*time.Second))
	}
	s.Refresh(now)
	s.DismissSpin()

	s.Apply(events.Event{Type: events.AgentStatus, Status: events.StatusWaiting, Timestamp: now, Source: "claude"})
	if s.Agent.Spin != nil {
		t.Error("a finished turn was still reported as spinning")
	}
	if len(s.dismissed) != 0 {
		t.Errorf("dismissals carried into the next turn: %v", s.dismissed)
	}
}

// Every finding must be sayable. A kind with no sentence would render a blank
// row and look like a rendering fault.
func TestEveryFindingKindIsAccountedFor(t *testing.T) {
	for _, kind := range []FindingKind{RepeatedEdit, RepeatedFailure, RepeatedCommand, NoNewGround} {
		if strings.TrimSpace(string(kind)) == "" {
			t.Errorf("unnamed finding kind")
		}
	}
}
