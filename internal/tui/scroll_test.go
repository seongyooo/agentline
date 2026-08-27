package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/seongyooo/agentline/internal/events"
	"github.com/seongyooo/agentline/internal/project"
	"github.com/seongyooo/agentline/internal/state"
)

// withReply returns a model showing a long agent answer.
func withReply(t *testing.T, reply string) Model {
	t.Helper()

	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	st.Apply(events.Event{
		Type: events.AgentReply, Message: reply,
		Timestamp: time.Now(), Source: "claude-code",
	})

	m, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m.(Model)
}

const longReply = "Added a README covering what AgentLine is, how to run it in either mode, " +
	"and the keys it responds to. I left installation out because the build steps are not " +
	"settled yet, and a section that goes stale immediately is worse than no section at all. " +
	"The architecture overview points at the plan rather than repeating it, so the two cannot " +
	"drift apart. Screenshots are missing because the UI is still changing shape."

// A long answer must be reachable rather than cut off at the panel's edge.
func TestLongReplyCanBeScrolled(t *testing.T) {
	m := withReply(t, longReply)
	l := computeLayout(100, 30)

	lines := m.replyLines()
	if len(lines) <= m.replyRows(l) {
		t.Fatalf("reply fits in %d rows; nothing to scroll", m.replyRows(l))
	}

	first := ansi.Strip(m.View())
	if !strings.Contains(first, "Added a README") {
		t.Errorf("reply does not start at the top:\n%s", first)
	}

	m = focusReplyPanel(t, m)
	for i := 0; i < 20; i++ {
		m, _ = key(m, tea.KeyDown)
	}

	scrolled := ansi.Strip(m.View())
	if scrolled == first {
		t.Error("scrolling the reply changed nothing")
	}
	if !strings.Contains(scrolled, "Screenshots") {
		t.Errorf("the end of the reply is unreachable:\n%s", scrolled)
	}
}

// The panel says where in the answer the reader is, so the rest is not hidden
// without a trace.
func TestScrolledReplyShowsItsPosition(t *testing.T) {
	m := withReply(t, longReply)
	l := computeLayout(100, 30)

	want := fmt.Sprintf("/%d", len(m.replyLines()))
	if out := ansi.Strip(m.View()); !strings.Contains(out, want) {
		t.Errorf("no position for a reply of %d lines in %d rows:\n%s", len(m.replyLines()), m.replyRows(l), out)
	}
}

// Scrolling must stop at the ends rather than run off into blank space.
func TestReplyScrollIsBounded(t *testing.T) {
	m := focusReplyPanel(t, withReply(t, longReply))
	l := computeLayout(100, 30)

	for i := 0; i < 50; i++ {
		m, _ = key(m, tea.KeyDown)
	}
	if m.replyScroll > m.replyMaxScroll(l) {
		t.Errorf("scrolled past the end: %d > %d", m.replyScroll, m.replyMaxScroll(l))
	}

	for i := 0; i < 50; i++ {
		m, _ = key(m, tea.KeyUp)
	}
	if m.replyScroll != 0 {
		t.Errorf("scrolled above the start: %d", m.replyScroll)
	}
}

// The activity log follows the newest entry until the user scrolls back.
func TestActivityLogCanBeScrolled(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	for i := 0; i < 30; i++ {
		st.Apply(events.Event{
			Type: events.FileRead, Path: fmt.Sprintf("file%02d.cs", i),
			Timestamp: time.Now(), Source: "claude-code",
		})
	}

	model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := model.(Model)

	l := computeLayout(100, 30)
	log := func(m Model) string {
		return ansi.Strip(strings.Join(m.activityPanel(l), "\n"))
	}

	if !strings.Contains(log(m), "file29.cs") {
		t.Errorf("log is not following the newest entry:\n%s", log(m))
	}

	m = focusArea_(t, m, focusActivity)
	for i := 0; i < 10; i++ {
		m, _ = key(m, tea.KeyUp)
	}

	scrolled := log(m)
	if strings.Contains(scrolled, "file29.cs") {
		t.Errorf("scrolling back did not move the window:\n%s", scrolled)
	}
	if !strings.Contains(scrolled, "file19.cs") {
		t.Errorf("earlier entries are not reachable:\n%s", scrolled)
	}

	// Scrolling forward again must return to following the newest entry.
	for i := 0; i < 20; i++ {
		m, _ = key(m, tea.KeyDown)
	}
	if !strings.Contains(log(m), "file29.cs") {
		t.Errorf("could not get back to the newest entry:\n%s", log(m))
	}
}

// Tabbing round must come back to the tree, and say so when it does. A
// highlight that looks the same whether or not the keys reach the tree is
// indistinguishable from tab having done nothing.
func TestTabReturnsToTheTreeAndSaysSo(t *testing.T) {
	m, _ := sendable(t)

	if m.focus != focusTree {
		t.Fatalf("focus starts at %v, want the tree", m.focus)
	}
	if !strings.Contains(ansi.Strip(m.View()), "PROJECT ◂") {
		t.Errorf("the tree does not show it has focus:\n%s", ansi.Strip(m.View()))
	}

	// Round the cycle and back.
	var reached bool
	for i := 0; i < 6; i++ {
		m, _ = key(m, tea.KeyTab)
		if m.focus == focusTree {
			reached = true
			break
		}
	}
	if !reached {
		t.Fatal("tabbing never came back to the tree")
	}
	if !strings.Contains(ansi.Strip(m.View()), "PROJECT ◂") {
		t.Errorf("the tree does not show it has focus after tabbing back:\n%s", ansi.Strip(m.View()))
	}

	// And the keys really do reach it.
	before := m.tree.cursor
	m, _ = key(m, tea.KeyDown)
	if m.tree.cursor == before {
		t.Error("the arrow keys did not reach the tree")
	}
}

// With focus elsewhere the tree must not look like the keys still reach it.
func TestTreeStopsLookingSelectedWhenFocusLeaves(t *testing.T) {
	// Checked on the whole frame, not the panel alone: the focus marker lives
	// on the panel's frame when there is one, and colour is not available to
	// carry it — a terminal that renders none must still show where the keys go.
	m, _ := sendable(t)
	focused := ansi.Strip(m.View())

	m = focusPromptKey(m)
	blurred := ansi.Strip(m.View())

	if focused == blurred {
		t.Error("the tree looks identical whether or not it has focus")
	}
	if !strings.Contains(focused, "PROJECT ◂") {
		t.Error("the tree does not show that the keys reach it")
	}
	if strings.Contains(blurred, "PROJECT ◂") {
		t.Error("the tree still claims focus after it moved away")
	}
}

// A click moves focus, so the panels are reachable without tabbing to them.
func TestClickMovesFocus(t *testing.T) {
	m, _ := sendable(t)

	// The prompt bar is the last row.
	next, _ := m.Update(tea.MouseMsg{
		X: 5, Y: m.height - 1,
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
	m = next.(Model)

	if !m.inputFocused() {
		t.Error("clicking the prompt bar did not focus it")
	}
}

func TestClickOnTreeFocusesTree(t *testing.T) {
	m, _ := sendable(t)
	m = focusPromptKey(m)

	next, _ := m.Update(tea.MouseMsg{
		X: 3, Y: headerRows + 2,
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
	m = next.(Model)

	if m.focus != focusTree {
		t.Errorf("focus = %v, want the tree", m.focus)
	}
}

// The wheel scrolls what is under the pointer without stealing focus, which
// is what a mouse user expects.
func TestWheelScrollsWithoutMovingFocus(t *testing.T) {
	m, _ := sendable(t)
	before := m.focus

	next, _ := m.Update(tea.MouseMsg{
		X: 3, Y: headerRows + 2,
		Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown,
	})
	m = next.(Model)

	if m.focus != before {
		t.Errorf("wheel moved focus to %v", m.focus)
	}
	if m.tree.cursor == 0 {
		t.Error("wheel did not scroll the tree under the pointer")
	}
}

// focusReplyPanel puts focus on the reply, whatever the tab order is.
func focusReplyPanel(t *testing.T, m Model) Model {
	t.Helper()
	return focusArea_(t, m, focusReply)
}

func focusArea_(t *testing.T, m Model, want focusArea) Model {
	t.Helper()

	for i := 0; i < 5; i++ {
		if m.focus == want {
			return m
		}
		m, _ = key(m, tea.KeyTab)
	}
	t.Fatalf("could not reach focus %v", want)
	return m
}
