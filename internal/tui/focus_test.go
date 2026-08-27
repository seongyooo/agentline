package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/seongyooo/agentline/internal/events"
	"github.com/seongyooo/agentline/internal/project"
	"github.com/seongyooo/agentline/internal/state"
)

// answered returns a model whose agent has replied, at a given size.
//
// blocked and spinning put something above the reply in the column, which is
// what makes the reply get dropped while still existing — the case the focus
// rule has to agree with. Without one of them the column has room for
// everything and nothing diverges.
func answered(t *testing.T, w, h int, blocked, spinning bool) Model {
	t.Helper()

	st := state.New("/proj")
	st.Project.Tree = project.MockTree()

	now := time.Now()
	st.Apply(events.Event{Type: events.UserPrompt, Message: "fix the valve", Timestamp: now, Source: "claude-code"})
	st.Apply(events.Event{Type: events.AgentReply, Timestamp: now, Source: "claude-code",
		Message: strings.Repeat("the valve controls the drainage flow. ", 6)})

	if spinning {
		for i := 0; i < 6; i++ {
			st.Apply(events.Event{Type: events.FileEdit, Path: "Assets/Scripts/Puzzle/Valve.cs",
				Timestamp: now.Add(-time.Duration(6-i) * 20 * time.Second), Source: "claude-code"})
		}
		st.Apply(events.Event{Type: events.AgentStatus, Status: events.StatusWorking, Timestamp: now, Source: "claude-code"})
	}
	if blocked {
		st.Apply(events.Event{Type: events.PermissionAsk, Timestamp: now, Source: "claude-code",
			Ask: &events.Ask{ID: "a", Tool: "Write", Title: "Write", Target: "Valve.cs",
				Reason: "Claude requested permissions to edit Valve.cs which is a sensitive file.", Mode: "acceptEdits"}})
	}

	model, _ := New(st, nil, nil).WithSender(&approver{}).Update(tea.WindowSizeMsg{Width: w, Height: h})
	return model.(Model)
}

// Tab must never land somewhere there is nothing to see. Whether a panel can
// take focus was decided from whether its content existed, while whether it
// was drawn was decided by what survived the fit — so on a short terminal tab
// parked the arrow keys on a reply that had been dropped, and focus appeared
// to vanish.
func TestTabNeverLandsOnAPanelThatIsNotDrawn(t *testing.T) {
	cases := []struct {
		name              string
		blocked, spinning bool
	}{
		{"spinning", false, true},
		{"blocked", true, false},
		{"blocked and spinning", true, true},
		{"neither", false, false},
	}

	for _, c := range cases {
		for _, h := range []int{20, 22, 24, 26, 28, 30, 40} {
			m := answered(t, 100, h, c.blocked, c.spinning)

			for i := 0; i < 6; i++ {
				m, _ = key(m, tea.KeyTab)
				if m.focus != focusReply {
					continue
				}
				if !strings.Contains(ansi.Strip(m.View()), "REPLY") {
					t.Errorf("%s at height %d: focus is on a reply that is not on screen", c.name, h)
					break
				}
			}
		}
	}
}

// A panel can go out from under the focus with no key pressed at all.
func TestFocusLeavesAPanelThatDisappears(t *testing.T) {
	m := answered(t, 100, 40, false, true)

	// Reach the reply, which a tall terminal shows.
	for i := 0; i < 4 && m.focus != focusReply; i++ {
		m, _ = key(m, tea.KeyTab)
	}
	if m.focus != focusReply {
		t.Skip("this size does not offer the reply; nothing to check")
	}

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m = next.(Model)

	if m.focus == focusReply && !strings.Contains(ansi.Strip(m.View()), "REPLY") {
		t.Error("focus stayed on a panel the resize took away")
	}
}

// Leaving the prompt has to take the terminal cursor with it, or it is left
// blinking in a field that no longer has focus.
func TestLeavingThePromptHidesTheCursor(t *testing.T) {
	for _, k := range []tea.KeyType{tea.KeyEsc, tea.KeyTab} {
		m, _ := sendable(t)
		m = focusPromptKey(m)

		m, cmd := key(m, k)
		if m.inputFocused() {
			t.Fatalf("%v did not leave the prompt", k)
		}
		if cmd == nil {
			t.Errorf("%v left the prompt without hiding the cursor", k)
		}
	}
}
