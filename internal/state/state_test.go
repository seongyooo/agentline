package state

import (
	"fmt"
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

// A command must read the same way while it runs and when it finishes, or the
// activity log describes the same command twice in two different vocabularies.
func TestCommandKeepsItsDescriptionWhenItEnds(t *testing.T) {
	now := time.Now()
	s := New("/proj")

	start := ev(events.CommandStart, now)
	start.Command = "go test ./internal/git"
	start.Message = "Run the git package tests"
	s.Apply(start)

	end := ev(events.CommandEnd, now)
	end.Command, end.Message = start.Command, start.Message
	s.Apply(end)

	if got := s.Agent.Now.Summary; got != start.Message {
		t.Errorf("Summary = %q, want %q", got, start.Message)
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

// Several hooks can report the same state in a row; the log should show what
// changed, not repeat an entry already on screen.
func TestRepeatedActionIsNotLoggedTwice(t *testing.T) {
	now := time.Now()
	s := New("/proj")

	waiting := ev(events.AgentStatus, now)
	waiting.Status = events.StatusWaiting
	s.Apply(waiting)
	s.Apply(waiting)

	if got := len(s.Agent.Activity); got != 1 {
		t.Errorf("Activity = %d entries, want 1", got)
	}
}

// A genuine return to an earlier action is still new activity.
func TestReturningToAnActionIsLogged(t *testing.T) {
	now := time.Now()
	s := New("/proj")

	read := ev(events.FileRead, now)
	read.Path = "a.go"
	s.Apply(read)

	other := ev(events.FileRead, now)
	other.Path = "b.go"
	s.Apply(other)

	s.Apply(read)

	if got := len(s.Agent.Activity); got != 3 {
		t.Errorf("Activity = %d entries, want 3", got)
	}
}

func TestActivityIsBounded(t *testing.T) {
	now := time.Now()
	s := New("/proj")

	// Distinct paths, so every event is genuinely new activity rather than a
	// repeat the log would collapse.
	for i := 0; i < maxActivity+20; i++ {
		e := ev(events.FileRead, now)
		e.Path = fmt.Sprintf("file%d.cs", i)
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
