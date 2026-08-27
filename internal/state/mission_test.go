package state

import (
	"strings"
	"testing"
	"time"

	"github.com/seongyooo/agentline/internal/events"
)

func prompt(text string) events.Event {
	return events.Event{
		Type:      events.UserPrompt,
		Message:   text,
		Timestamp: time.Now(),
		Source:    "claude-code",
	}
}

func TestMissionComesFromThePrompt(t *testing.T) {
	s := New("/proj")
	s.Apply(prompt("Connect the drainage system to the valve"))

	if want := "Connect the drainage system to the valve"; s.Agent.Mission != want {
		t.Errorf("Mission = %q, want %q", s.Agent.Mission, want)
	}
}

// A prompt means the user just handed over work. NOW must agree with the
// header rather than still reading Idle underneath a WORKING status.
func TestPromptMarksTheAgentWorking(t *testing.T) {
	s := New("/proj")
	s.Apply(prompt("do the thing"))

	if s.Agent.Status != events.StatusWorking {
		t.Errorf("Status = %q, want %q", s.Agent.Status, events.StatusWorking)
	}
	if s.Agent.Now.Kind != ActionWorking {
		t.Errorf("Now.Kind = %q, want %q", s.Agent.Now.Kind, ActionWorking)
	}
}

// A prompt arriving mid-task must not wipe out the specific action on screen.
func TestPromptKeepsASpecificAction(t *testing.T) {
	s := New("/proj")

	edit := ev(events.FileEdit, time.Now())
	edit.Path = "Drain.cs"
	s.Apply(edit)
	s.Apply(prompt("also check the valve"))

	if s.Agent.Now.Kind != ActionEditing {
		t.Errorf("Now.Kind = %q, want %q", s.Agent.Now.Kind, ActionEditing)
	}
}

// Only the first line is used, so a long prompt still reads as a goal.
func TestMissionUsesTheFirstLine(t *testing.T) {
	s := New("/proj")
	s.Apply(prompt("Fix the water room puzzle\n\nDetails:\n- valve sticks\n- drain never empties"))

	if want := "Fix the water room puzzle"; s.Agent.Mission != want {
		t.Errorf("Mission = %q, want %q", s.Agent.Mission, want)
	}
}

func TestMissionCollapsesWhitespace(t *testing.T) {
	s := New("/proj")
	s.Apply(prompt("   Fix   the\tvalve   "))

	if want := "Fix the valve"; s.Agent.Mission != want {
		t.Errorf("Mission = %q, want %q", s.Agent.Mission, want)
	}
}

// The mission must be the user's words, not a paraphrase: nothing is inferred.
func TestMissionIsNotRewritten(t *testing.T) {
	const text = "Refactor DrainSystem.cs so the valve closes on overflow"

	s := New("/proj")
	s.Apply(prompt(text))

	if s.Agent.Mission != text {
		t.Errorf("Mission = %q, want the prompt verbatim %q", s.Agent.Mission, text)
	}
}

// A newer instruction describes what is happening now; an older one would
// describe work the user has already moved on from.
func TestLatestPromptWins(t *testing.T) {
	s := New("/proj")
	s.Apply(prompt("First task"))
	s.Apply(prompt("Actually, do the second task instead"))

	if want := "Actually, do the second task instead"; s.Agent.Mission != want {
		t.Errorf("Mission = %q, want %q", s.Agent.Mission, want)
	}
}

// An explicit mission outranks anything derived from prompts.
func TestPinnedMissionSurvivesPrompts(t *testing.T) {
	s := New("/proj")
	s.PinMission("Ship the release")
	s.Apply(prompt("check the logs real quick"))

	if want := "Ship the release"; s.Agent.Mission != want {
		t.Errorf("Mission = %q, want the pinned %q", s.Agent.Mission, want)
	}
}

// Pinning nothing must not lock the mission to an empty string.
func TestPinningEmptyLeavesMissionDerivable(t *testing.T) {
	s := New("/proj")
	s.PinMission("")
	s.Apply(prompt("Do the work"))

	if want := "Do the work"; s.Agent.Mission != want {
		t.Errorf("Mission = %q, want %q", s.Agent.Mission, want)
	}
}

// A prompt that is only whitespace says nothing; the previous mission stands.
func TestBlankPromptDoesNotClearTheMission(t *testing.T) {
	s := New("/proj")
	s.Apply(prompt("Real mission"))
	s.Apply(prompt("   \n  "))

	if want := "Real mission"; s.Agent.Mission != want {
		t.Errorf("Mission = %q, want %q", s.Agent.Mission, want)
	}
}

func TestEmptyPromptIsInvalid(t *testing.T) {
	if prompt("").Valid() {
		t.Error("a prompt event with no text should be rejected")
	}
}

// A long prompt is kept whole in state; shortening it is the renderer's job.
func TestLongPromptIsNotTruncatedInState(t *testing.T) {
	long := strings.Repeat("make it better ", 40)

	s := New("/proj")
	s.Apply(prompt(long))

	if got := len(s.Agent.Mission); got < 500 {
		t.Errorf("Mission is %d chars; state should keep the prompt whole", got)
	}
}
