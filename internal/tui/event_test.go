package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/seonl/agentview/internal/events"
	"github.com/seonl/agentview/internal/state"
)

// send feeds one event through the message loop and returns the updated model
// plus the command it issued.
func send(m Model, e events.Event) (Model, tea.Cmd) {
	next, cmd := m.Update(eventMsg(e))
	return next.(Model), cmd
}

func edit(path string) events.Event {
	return events.Event{Type: events.FileEdit, Path: path, Timestamp: time.Now(), Source: "mock"}
}

// The pipeline must keep pumping: every event handled has to re-arm the wait,
// or the UI would freeze after the first one.
func TestEventKeepsThePipelinePumping(t *testing.T) {
	stream := make(chan events.Event, 1)
	m, _ := New(seeded(), nil, stream).Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	_, cmd := send(m.(Model), edit("Assets/Scripts/Puzzle/Valve.cs"))
	if cmd == nil {
		t.Fatal("handling an event did not re-arm the event wait")
	}

	stream <- edit("Assets/Scripts/Rooms/WaterRoom.cs")
	if _, ok := cmd().(eventMsg); !ok {
		t.Error("re-armed command did not deliver the next event")
	}
}

func TestEventUpdatesNowAndActivity(t *testing.T) {
	m := sized(t, 100, 30)
	m, _ = send(m, edit("Assets/Scripts/Puzzle/Valve.cs"))

	out := m.View()
	for _, want := range []string{"Editing", "Valve.cs", "WORKING"} {
		if !strings.Contains(ansi.Strip(out), want) {
			t.Errorf("view missing %q after a file_edit:\n%s", want, ansi.Strip(out))
		}
	}
}

// §14: the tree must react to activity, not just the NOW panel.
func TestEventMarksTheTree(t *testing.T) {
	m := sized(t, 100, 40)
	m, _ = send(m, edit("Assets/Scripts/Rooms/WaterRoom.cs"))

	tree := ansi.Strip(strings.Join(m.treePanel(computeLayout(100, 40), time.Now()), "\n"))
	line := lineContaining(tree, "WaterRoom.cs")
	if line == "" {
		t.Fatalf("WaterRoom.cs not in tree:\n%s", tree)
	}
	if !strings.Contains(line, "●") {
		t.Errorf("edited file not marked active: %q", line)
	}
}

// Status transitions must reach the header, which is the first thing a
// developer looks at.
func TestStatusTransitionsReachTheHeader(t *testing.T) {
	tests := []struct {
		status events.Status
		want   string
	}{
		{events.StatusWorking, "WORKING"},
		{events.StatusWaiting, "WAITING"},
		{events.StatusNeedsInput, "NEEDS INPUT"},
		{events.StatusDone, "DONE"},
		{events.StatusError, "ERROR"},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			m := sized(t, 100, 30)
			m, _ = send(m, events.Event{
				Type:      events.AgentStatus,
				Status:    tc.status,
				Timestamp: time.Now(),
				Source:    "mock",
			})

			if header := ansi.Strip(m.header(100)); !strings.Contains(header, tc.want) {
				t.Errorf("header = %q, want it to contain %q", header, tc.want)
			}
		})
	}
}

// A malformed event must be ignored, not crash the UI or corrupt state (§28).
func TestMalformedEventIsIgnored(t *testing.T) {
	live, _ := New(seeded(), nil, make(chan events.Event)).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := live.(Model)
	before := m.View()

	m, cmd := send(m, events.Event{Type: events.FileEdit, Source: "mock"}) // no path, no timestamp
	if cmd == nil {
		t.Error("a bad event must not stop the pipeline")
	}
	if m.View() != before {
		t.Error("malformed event changed the rendered state")
	}
}

// When the source ends, the UI keeps the last observed state rather than
// blanking or inventing one.
func TestSourceClosedKeepsLastState(t *testing.T) {
	m := sized(t, 100, 30)
	m, _ = send(m, edit("Assets/Scripts/Puzzle/Valve.cs"))
	before := m.View()

	next, cmd := m.Update(sourceClosedMsg{})
	m = next.(Model)

	if cmd != nil {
		t.Error("a closed source should not re-arm the wait")
	}
	if m.View() != before {
		t.Error("view changed after the source closed")
	}
}

func TestNilStreamProducesNoWait(t *testing.T) {
	if cmd := waitForEvent(nil); cmd != nil {
		t.Error("a model with no event source should not wait for events")
	}
}

// The idle redraw must keep rescheduling itself, or activity markers would
// freeze at whatever age they had when the agent went quiet.
func TestDecayTickReschedulesItself(t *testing.T) {
	m := sized(t, 100, 30)

	next, cmd := m.Update(decayMsg(time.Now()))
	if cmd == nil {
		t.Fatal("decay tick did not reschedule")
	}
	if next.(Model).View() != m.View() {
		t.Error("a decay tick changed the state; it should only redraw")
	}
}

func TestInitStartsBothTheStreamAndTheDecayTick(t *testing.T) {
	if cmd := New(state.New("/proj"), nil, nil).Init(); cmd == nil {
		t.Error("Init returned nothing; the decay tick must run even with no source")
	}
}
