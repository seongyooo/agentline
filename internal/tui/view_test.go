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

func seeded() *state.State {
	st := state.New("/proj")
	st.Agent.Mission = "Water Room Puzzle"
	st.Project.Tree = project.MockTree()
	st.Apply(events.Event{
		Type:      events.FileEdit,
		Path:      "Assets/Scripts/Puzzle/DrainSystem.cs",
		Timestamp: time.Now(),
		Source:    "claude-code",
	})
	return st
}

func render(t *testing.T, w, h int) string {
	t.Helper()
	return sized(t, w, h).View()
}

// sized returns a model that has been told its terminal size.
func sized(t *testing.T, w, h int) Model {
	t.Helper()
	m, _ := New(seeded(), nil, nil).Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m.(Model)
}

// press sends a key and returns the updated model.
func press(m Model, key tea.KeyType) Model {
	next, _ := m.Update(tea.KeyMsg{Type: key})
	return next.(Model)
}

func TestViewRendersCoreSections(t *testing.T) {
	out := render(t, 100, 30)
	for _, want := range []string{
		"AGENTLINE", "CLAUDE-CODE", "WORKING",
		"PROJECT", "Assets/", "DrainSystem.cs",
		"MISSION", "Water Room Puzzle",
		"NOW", "Editing", "NEXT", "ACTIVITY",
		"Ask Claude Code",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q", want)
		}
	}
}

// The view must occupy exactly the terminal it was given, or Bubble Tea will
// scroll the alt screen and the layout will tear on resize.
func TestViewOccupiesExactTerminal(t *testing.T) {
	for _, h := range []int{12, 16, 20, 24, 30, 50} {
		for _, w := range []int{40, 55, 72, 80, 120, 200} {
			lines := strings.Split(render(t, w, h), "\n")
			if len(lines) != h {
				t.Errorf("%dx%d: %d lines, want %d", w, h, len(lines), h)
				continue
			}
			for i, line := range lines {
				if got := ansi.StringWidth(line); got != w {
					t.Errorf("%dx%d line %d: width %d, want %d: %q", w, h, i, got, w, ansi.Strip(line))
				}
			}
		}
	}
}

func TestViewRejectsTinyTerminal(t *testing.T) {
	if out := render(t, 20, 5); !strings.Contains(out, "too small") {
		t.Errorf("view = %q, want a too-small notice", out)
	}
}

func TestNarrowTerminalHidesTree(t *testing.T) {
	if out := render(t, 50, 30); strings.Contains(out, "PROJECT") {
		t.Error("narrow terminal should not render the project panel")
	}
}

func TestUnknownNextRendersDash(t *testing.T) {
	if out := render(t, 100, 30); !strings.Contains(out, "—") {
		t.Error("unknown NEXT should render an em dash")
	}
}

// The edited file and every ancestor directory must carry a marker, so the
// tree answers "where is the agent working" at a glance.
func TestTreeMarksActivePathAndAncestors(t *testing.T) {
	m, _ := New(seeded(), nil, nil).Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	tree := strings.Join(m.(Model).treePanel(computeLayout(100, 40), time.Now()), "\n")

	for _, want := range []string{"Puzzle/", "DrainSystem.cs", "Scripts/", "Assets/"} {
		line := lineContaining(tree, want)
		if line == "" {
			t.Errorf("tree missing %q", want)
			continue
		}
		if !strings.Contains(line, "●") {
			t.Errorf("%q not marked active: %q", want, ansi.Strip(line))
		}
	}
	// An untouched sibling must stay unmarked.
	if line := lineContaining(tree, "Prefabs/"); strings.Contains(line, "●") {
		t.Errorf("inactive directory marked active: %q", ansi.Strip(line))
	}
}

// NOW outranks MISSION, so a cramped panel must keep the file being edited
// even when it has to drop the mission entirely.
func TestNowSurvivesCrampedPanel(t *testing.T) {
	for _, h := range []int{12, 14, 16, 18, 20, 24, 30} {
		out := render(t, 50, h)
		if !strings.Contains(out, "NOW") {
			t.Errorf("height %d: NOW panel dropped", h)
		}
		if !strings.Contains(out, "DrainSystem.cs") {
			t.Errorf("height %d: NOW lost its target file:\n%s", h, ansi.Strip(out))
		}
	}
}

// MISSION outranks the activity log in §20, so the log must never claim rows
// that push the mission out of the body.
func TestMissionOutranksActivityLog(t *testing.T) {
	for _, h := range []int{12, 14, 16, 18, 20, 24, 30} {
		for _, w := range []int{50, 80, 120} {
			out := render(t, w, h)
			if strings.Contains(out, "MISSION") {
				continue
			}
			if strings.Contains(out, "ACTIVITY") {
				t.Errorf("%dx%d: activity log kept while MISSION was dropped:\n%s", w, h, ansi.Strip(out))
			}
		}
	}
}

// A scrolling tree must say where the cursor is, so hidden rows are never a
// surprise.
func TestScrollingTreeReportsPosition(t *testing.T) {
	const w, h = 80, 24
	l := computeLayout(w, h)
	visible := l.BodyHeight - treeChrome

	m := sized(t, w, h)
	if len(m.rows()) <= visible {
		t.Skip("tree fits at this size; nothing to scroll")
	}

	panel := ansi.Strip(strings.Join(m.treePanel(l, time.Now()), "\n"))
	if want := fmt.Sprintf("1/%d", len(m.rows())); !strings.Contains(panel, want) {
		t.Errorf("heading missing position %q:\n%s", want, panel)
	}
}

// The cursor must stay on screen as it moves past the bottom of the viewport.
func TestTreeScrollsToFollowCursor(t *testing.T) {
	const w, h = 80, 24
	l := computeLayout(w, h)
	visible := l.BodyHeight - treeChrome

	m := sized(t, w, h)
	total := len(m.rows())
	if total <= visible {
		t.Skip("tree fits at this size; nothing to scroll")
	}

	for i := 0; i < total+5; i++ {
		m = press(m, tea.KeyDown)
		start, end, _ := m.tree.window(len(m.rows()), visible)
		if m.tree.cursor < start || m.tree.cursor >= end {
			t.Fatalf("step %d: cursor %d outside window [%d,%d)", i, m.tree.cursor, start, end)
		}
	}

	if m.tree.cursor != total-1 {
		t.Errorf("cursor = %d, want %d (clamped to last row)", m.tree.cursor, total-1)
	}
}

func TestCursorStopsAtTop(t *testing.T) {
	m := sized(t, 80, 24)
	for i := 0; i < 5; i++ {
		m = press(m, tea.KeyUp)
	}
	if m.tree.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.tree.cursor)
	}
}

func TestCollapseAndExpandChangeVisibleRows(t *testing.T) {
	m := sized(t, 100, 40)
	before := len(m.rows())

	m = press(m, tea.KeyLeft) // collapse the root
	collapsed := len(m.rows())
	if collapsed != 1 {
		t.Errorf("collapsed root shows %d rows, want 1", collapsed)
	}

	m = press(m, tea.KeyRight) // expand it again
	if got := len(m.rows()); got != before {
		t.Errorf("re-expanded tree shows %d rows, want %d", got, before)
	}
}

// Left collapses an open directory; pressing it again on the now-closed
// directory walks up to the parent.
func TestCollapseThenLeftSelectsParent(t *testing.T) {
	m := sized(t, 100, 40)
	m = press(m, tea.KeyDown) // "Scripts/", an expanded child of the root

	m = press(m, tea.KeyLeft)
	if m.selected().Expanded {
		t.Fatal("first Left should collapse the directory")
	}
	if m.tree.cursor != 1 {
		t.Fatalf("collapsing moved the cursor to %d, want it to stay at 1", m.tree.cursor)
	}

	m = press(m, tea.KeyLeft)
	if m.tree.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (the parent)", m.tree.cursor)
	}
}

// Left on a file walks straight up to its directory.
func TestLeftOnFileSelectsParent(t *testing.T) {
	m := sized(t, 100, 40)
	for m.selected().Dir {
		m = press(m, tea.KeyDown)
	}
	file := m.selected()

	m = press(m, tea.KeyLeft)
	parent := m.selected()
	if !parent.Dir {
		t.Fatalf("selected %q, want a directory", parent.Name)
	}
	if !strings.HasPrefix(file.Path, parent.Path+"/") {
		t.Errorf("selected %q, which is not the parent of %q", parent.Path, file.Path)
	}
}

func TestQuitKeys(t *testing.T) {
	quit := map[string]tea.KeyMsg{
		"q":      {Type: tea.KeyRunes, Runes: []rune("q")},
		"ctrl+c": {Type: tea.KeyCtrlC},
	}
	for name, msg := range quit {
		t.Run(name, func(t *testing.T) {
			if _, cmd := New(seeded(), nil, nil).Update(msg); cmd == nil {
				t.Errorf("key %q did not produce a quit command", name)
			}
		})
	}
}

// Esc is reserved for cancel/close (§22) and must not exit the application.
func TestEscDoesNotQuit(t *testing.T) {
	if _, cmd := New(seeded(), nil, nil).Update(tea.KeyMsg{Type: tea.KeyEsc}); cmd != nil {
		t.Error("Esc produced a command; it should be inert for now")
	}
}

func lineContaining(out, want string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(ansi.Strip(line), want) {
			return line
		}
	}
	return ""
}
