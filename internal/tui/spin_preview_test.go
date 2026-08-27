package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/seongyooo/agentline/internal/events"
	"github.com/seongyooo/agentline/internal/project"
	"github.com/seongyooo/agentline/internal/state"
)

// spinningSession seeds a session that has been going round in circles: the
// same file rewritten, the same command failing, nothing new reached.
func spinningSession(t *testing.T, w, h int) Model {
	t.Helper()

	st := state.New("/proj")
	st.Project.Tree = project.MockTree()

	now := time.Now()
	st.Apply(events.Event{Type: events.UserPrompt, Message: "Make the valve tests pass",
		Timestamp: now.Add(-12 * time.Minute), Source: "claude-code"})

	// Ground already covered, so nothing in the loop below is new.
	for _, p := range []string{"Assets/Scripts/Puzzle/Valve.cs", "Assets/Scripts/Rooms/WaterRoom.cs"} {
		st.Apply(events.Event{Type: events.FileRead, Path: p, Timestamp: now.Add(-11 * time.Minute), Source: "claude-code"})
	}

	for i := 0; i < 6; i++ {
		at := now.Add(-4*time.Minute + time.Duration(i)*35*time.Second)
		st.Apply(events.Event{Type: events.FileEdit, Path: "Assets/Scripts/Puzzle/Valve.cs",
			Timestamp: at, Source: "claude-code"})
		st.Apply(events.Event{Type: events.CommandStart, Command: "dotnet test --filter Valve",
			Timestamp: at.Add(5 * time.Second), Source: "claude-code"})
		if i < 4 {
			st.Apply(events.Event{Type: events.CommandEnd, Command: "dotnet test --filter Valve",
				Failed: true, Timestamp: at.Add(20 * time.Second), Source: "claude-code"})
		}
	}
	st.Apply(events.Event{Type: events.AgentStatus, Status: events.StatusWorking,
		Timestamp: now, Source: "claude-code"})
	st.Apply(events.Event{Type: events.SessionInfo, Timestamp: now, Source: "claude-code",
		Session: &events.Session{Model: "claude-opus-5-20260514", ContextWindow: 200_000, ContextPercent: 0.44}})

	model, _ := New(st, nil, nil).WithSender(&approver{}).Update(tea.WindowSizeMsg{Width: w, Height: h})
	return model.(Model)
}

// TestLookSpinning shows what a session going nowhere looks like. Set LOOK=1.
func TestLookSpinning(t *testing.T) {
	if os.Getenv("LOOK") == "" {
		t.Skip("set LOOK=1")
	}

	m := spinningSession(t, 100, 30)
	fmt.Println(strings.Repeat("=", 100))
	fmt.Println(m.View())
	fmt.Println(strings.Repeat("=", 100))
}

// The counts have to be on screen without colour: the panel says what was
// counted, and the header says it too, since the panel can be scrolled past.
func TestSpinningIsVisibleWithoutColour(t *testing.T) {
	out := ansi.Strip(spinningSession(t, 100, 30).View())

	for _, want := range []string{
		"SPINNING",        // the header, and the panel
		"Valve.cs",        // what is being rewritten
		"written 6 times", // how many times, which is the whole point
		"run 6 times",     // and the command that keeps being run
		"x interrupt",     // what can be done about it
	} {
		if !strings.Contains(out, want) {
			t.Errorf("frame missing %q", want)
			t.Log(out)
		}
	}
}

// Stopping is the point of noticing. It must reach the session.
func TestXInterruptsASpinningAgent(t *testing.T) {
	m := spinningSession(t, 100, 30)
	session := m.sender.(*approver)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if cmd == nil {
		t.Fatal("x produced no command")
	}
	runCmd(cmd)

	if session.interrupts != 1 {
		t.Errorf("interrupts = %d, want 1", session.interrupts)
	}
}

// Waving it away has to work, or the panel becomes something to work around.
func TestEscDismissesTheEvidence(t *testing.T) {
	m := spinningSession(t, 100, 30)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m = next.(Model); m.st.Agent.Spin != nil {
		t.Error("esc did not dismiss the evidence")
	}
	if strings.Contains(ansi.Strip(m.View()), "SPINNING") {
		t.Error("the frame still reports spinning after it was dismissed")
	}
}

// Nothing about noticing may take the keys that already meant something.
func TestSpinningLeavesTheOrdinaryKeysAlone(t *testing.T) {
	m := spinningSession(t, 100, 30)

	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}); cmd == nil {
		t.Error("q did not quit while spinning")
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if !next.(Model).inputFocused() {
		t.Error("i did not reach the prompt while spinning")
	}
}
