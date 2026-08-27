package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/seonl/agentview/internal/events"
	"github.com/seonl/agentview/internal/git"
)

// enter opens the entry under the cursor.
func enter(m Model) Model {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return next.(Model)
}

func esc(m Model) Model {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	return next.(Model)
}

// selectNamed moves the cursor to the first row with the given name.
func selectNamed(t *testing.T, m Model, name string) Model {
	t.Helper()

	for i, row := range m.rows() {
		if row.Node.Name == name {
			m.tree.cursor = i
			return m
		}
	}
	t.Fatalf("%q not in the tree", name)
	return m
}

func TestEnterInspectsTheSelectedFile(t *testing.T) {
	m, _, _ := scanned(t, "src/main.go")
	m = selectNamed(t, m, "src")
	m = enter(m)

	out := ansi.Strip(m.View())
	if !strings.Contains(out, "DIRECTORY") {
		t.Errorf("directory not described:\n%s", out)
	}
	// The mission column is given over to the entry.
	if strings.Contains(out, "MISSION") || strings.Contains(out, "NEXT") {
		t.Errorf("mission panel is still shown alongside the entry:\n%s", out)
	}
}

func TestInspectShowsObservedFacts(t *testing.T) {
	m, root, _ := scanned(t, "notes.md")
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte(strings.Repeat("x", 2048)), 0o644); err != nil {
		t.Fatal(err)
	}

	m = enter(selectNamed(t, m, "notes.md"))
	out := ansi.Strip(m.View())

	for _, want := range []string{"FILE", "notes.md", "KB", "changed"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}

// What the agent did with a file is the part AgentView actually knows and the
// reason to look at an entry here rather than in an editor.
func TestInspectShowsAgentActivity(t *testing.T) {
	m, _, _ := scanned(t, "notes.md")

	m, _ = send(m, events.Event{
		Type: events.FileEdit, Path: "notes.md",
		Timestamp: time.Now(), Source: "claude-code",
	})
	m = enter(selectNamed(t, m, "notes.md"))

	out := ansi.Strip(m.View())
	if !strings.Contains(out, "AGENT ACTIVITY") {
		t.Fatalf("no activity section:\n%s", out)
	}
	if !strings.Contains(out, "Editing") {
		t.Errorf("the edit is not listed:\n%s", out)
	}
}

// A directory collects what happened to the files inside it.
func TestInspectDirectoryIncludesItsFiles(t *testing.T) {
	m, _, _ := scanned(t, "src/main.go")

	m, _ = send(m, events.Event{
		Type: events.FileEdit, Path: "src/main.go",
		Timestamp: time.Now(), Source: "claude-code",
	})
	m = enter(selectNamed(t, m, "src"))

	if out := ansi.Strip(m.View()); !strings.Contains(out, "Editing") {
		t.Errorf("work inside the directory is not counted:\n%s", out)
	}
}

func TestInspectSaysWhenNothingHappened(t *testing.T) {
	m, _, _ := scanned(t, "untouched.md")
	m = enter(selectNamed(t, m, "untouched.md"))

	if out := ansi.Strip(m.View()); !strings.Contains(out, "none this session") {
		t.Errorf("an untouched file should say so:\n%s", out)
	}
}

func TestInspectShowsGitStatus(t *testing.T) {
	m, _, _ := scanned(t, "changed.md")
	m.st.Project.Git = git.Status{
		Branch: "main",
		Files:  map[string]git.FileStatus{"changed.md": git.Modified},
	}

	m = enter(selectNamed(t, m, "changed.md"))
	if out := ansi.Strip(m.View()); !strings.Contains(out, "modified since the last commit") {
		t.Errorf("git status not reported:\n%s", out)
	}
}

// A file the agent has announced does not exist yet, and saying it is missing
// would be wrong.
func TestInspectDistinguishesAnnouncedFromMissing(t *testing.T) {
	m, _, _ := scanned(t, "existing.md")

	m, _ = send(m, events.Event{
		Type: events.FilePending, Path: "planned.md",
		Timestamp: time.Now(), Source: "claude-code",
	})
	m = enter(selectNamed(t, m, "planned.md"))

	if out := ansi.Strip(m.View()); !strings.Contains(out, "not written yet") {
		t.Errorf("an announced file should say it is on its way:\n%s", out)
	}
}

func TestEscapeReturnsToTheMission(t *testing.T) {
	m, _, _ := scanned(t, "notes.md")
	m = enter(selectNamed(t, m, "notes.md"))

	if out := ansi.Strip(m.View()); strings.Contains(out, "MISSION") {
		t.Fatalf("mission shown while inspecting:\n%s", out)
	}

	m = esc(m)
	if out := ansi.Strip(m.View()); !strings.Contains(out, "MISSION") {
		t.Errorf("mission did not come back:\n%s", out)
	}
}

// The panel must not break the layout, whatever it is describing.
func TestInspectKeepsTheLayout(t *testing.T) {
	for _, w := range []int{72, 100, 160} {
		m, _, _ := scanned(t, "a/very/deeply/nested/file-with-a-long-name.go")
		model, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		m = model.(Model)

		for i := range m.rows() {
			m.tree.cursor = i
			view := enter(m).View()

			for row, line := range strings.Split(view, "\n") {
				if got := ansi.StringWidth(line); got != w {
					t.Fatalf("width %d, row %d of entry %d: got %d", w, row, i, got)
				}
			}
		}
	}
}
