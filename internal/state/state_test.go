package state

import (
	"testing"
	"time"

	"github.com/seonl/agentview/internal/events"
)

func ev(t events.Type, ts time.Time) events.Event {
	return events.Event{Type: t, Timestamp: ts, Source: "claude-code"}
}

func TestApplyFileEditSetsNowAndActivity(t *testing.T) {
	now := time.Now()
	s := New("/proj")

	e := ev(events.FileEdit, now)
	e.Path = "Assets/Scripts/Drain.cs"
	s.Apply(e)

	if s.Agent.Now.Kind != ActionEditing {
		t.Errorf("Now.Kind = %q, want %q", s.Agent.Now.Kind, ActionEditing)
	}
	if s.Agent.Now.Target != e.Path {
		t.Errorf("Now.Target = %q, want %q", s.Agent.Now.Target, e.Path)
	}
	if s.Agent.Status != events.StatusWorking {
		t.Errorf("Status = %q, want %q", s.Agent.Status, events.StatusWorking)
	}
	if _, ok := s.Project.ActivityByPath[e.Path]; !ok {
		t.Error("path not recorded in ActivityByPath")
	}
}

func TestApplyIgnoresInvalidEvent(t *testing.T) {
	s := New("/proj")
	s.Apply(ev(events.FileEdit, time.Now())) // missing Path

	if len(s.Agent.Activity) != 0 {
		t.Errorf("Activity = %d entries, want 0", len(s.Agent.Activity))
	}
}

func TestStatusTransitions(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		to   events.Status
		want ActionKind
	}{
		{"working to waiting", events.StatusWaiting, ActionWaiting},
		{"working to done", events.StatusDone, ActionDone},
		{"working to needs input", events.StatusNeedsInput, ActionWaiting},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := New("/proj")
			s.Agent.Status = events.StatusWorking

			e := ev(events.AgentStatus, now)
			e.Status = tc.to
			s.Apply(e)

			if s.Agent.Status != tc.to {
				t.Errorf("Status = %q, want %q", s.Agent.Status, tc.to)
			}
			if s.Agent.Now.Kind != tc.want {
				t.Errorf("Now.Kind = %q, want %q", s.Agent.Now.Kind, tc.want)
			}
		})
	}
}

// NOW must never contradict the header: a "working" status with nothing more
// specific observed yet reads as Working, not Idle.
func TestWorkingStatusReplacesIdle(t *testing.T) {
	now := time.Now()
	s := New("/proj")

	e := ev(events.AgentStatus, now)
	e.Status = events.StatusWorking
	s.Apply(e)

	if s.Agent.Now.Kind != ActionWorking {
		t.Errorf("Now.Kind = %q, want %q", s.Agent.Now.Kind, ActionWorking)
	}
}

// A specific action is more informative than the status summary, so a
// "working" report must not overwrite it.
func TestWorkingStatusKeepsSpecificAction(t *testing.T) {
	now := time.Now()
	s := New("/proj")

	edit := ev(events.FileEdit, now)
	edit.Path = "Drain.cs"
	s.Apply(edit)

	working := ev(events.AgentStatus, now)
	working.Status = events.StatusWorking
	s.Apply(working)

	if s.Agent.Now.Kind != ActionEditing {
		t.Errorf("Now.Kind = %q, want %q", s.Agent.Now.Kind, ActionEditing)
	}
	if s.Agent.Now.Target != "Drain.cs" {
		t.Errorf("Now.Target = %q, want Drain.cs", s.Agent.Now.Target)
	}
}

// A finished command must not keep reading as running.
func TestCommandEndMovesNowOffRunning(t *testing.T) {
	now := time.Now()
	s := New("/proj")

	start := ev(events.CommandStart, now)
	start.Command = "go test ./..."
	s.Apply(start)

	exitOK := 0
	end := ev(events.CommandEnd, now)
	end.Command = "go test ./..."
	end.ExitCode = &exitOK
	s.Apply(end)

	if s.Agent.Now.Kind != ActionDone {
		t.Errorf("Now.Kind = %q, want %q", s.Agent.Now.Kind, ActionDone)
	}
}

func TestCommandEndNonZeroExitIsError(t *testing.T) {
	now := time.Now()
	s := New("/proj")

	code := 1
	e := ev(events.CommandEnd, now)
	e.Command = "dotnet test"
	e.ExitCode = &code
	s.Apply(e)

	if s.Agent.Status != events.StatusError {
		t.Errorf("Status = %q, want %q", s.Agent.Status, events.StatusError)
	}
}

func TestActivityIsBounded(t *testing.T) {
	now := time.Now()
	s := New("/proj")

	for i := 0; i < maxActivity+20; i++ {
		e := ev(events.FileRead, now)
		e.Path = "a.cs"
		s.Apply(e)
	}

	if len(s.Agent.Activity) != maxActivity {
		t.Errorf("Activity = %d entries, want %d", len(s.Agent.Activity), maxActivity)
	}
}

func TestActivityDecay(t *testing.T) {
	now := time.Now()
	s := New("/proj")

	e := ev(events.FileEdit, now)
	e.Path = "Drain.cs"
	s.Apply(e)

	tests := []struct {
		name string
		at   time.Time
		want ActivityLevel
	}{
		{"just now", now, Current},
		{"one minute later", now.Add(time.Minute), Recent},
		{"ten minutes later", now.Add(10 * time.Minute), Modified},
		{"one hour later", now.Add(time.Hour), Inactive},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.LevelAt("Drain.cs", tc.at); got != tc.want {
				t.Errorf("LevelAt = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLevelAtUnknownPathIsInactive(t *testing.T) {
	s := New("/proj")
	if got := s.LevelAt("nope.cs", time.Now()); got != Inactive {
		t.Errorf("LevelAt = %v, want %v", got, Inactive)
	}
}
