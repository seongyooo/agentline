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
	"github.com/seonl/agentview/internal/project"
	"github.com/seonl/agentview/internal/state"
)

// scanned returns a model over a real directory, so the tree behaves the way
// it does in the application rather than as a fixed fixture.
func scanned(t *testing.T, files ...string) (Model, string, *project.Scanner) {
	t.Helper()

	root := t.TempDir()
	writeTree(t, root, files...)

	scanner := project.NewScanner(root)
	st := state.New(root)
	st.Project.Tree = scanner.NewTree()

	m, _ := New(st, scanner, nil).Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	return m.(Model), root, scanner
}

func tree(m Model) string {
	return ansi.Strip(strings.Join(m.treePanel(computeLayout(100, 40), time.Now()), "\n"))
}

// The tree is a snapshot of one scan, so a file created after startup would
// never appear without a refresh. This is what made a written README invisible.
func TestNewFileAppearsInTheTree(t *testing.T) {
	m, root, _ := scanned(t, "existing.md")

	if strings.Contains(tree(m), "README.md") {
		t.Fatal("README.md is somehow already there")
	}

	// The agent writes it, then reports the write.
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ = send(m, events.Event{
		Type: events.FileCreate, Path: "README.md",
		Timestamp: time.Now(), Source: "claude-code",
	})

	if !strings.Contains(tree(m), "README.md") {
		t.Errorf("new file did not appear:\n%s", tree(m))
	}
}

// A file inside a directory created at the same time must appear too.
func TestNewFileInNewDirectoryAppears(t *testing.T) {
	m, root, _ := scanned(t, "existing.md")

	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "guide.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ = send(m, events.Event{
		Type: events.FileCreate, Path: "docs/guide.md",
		Timestamp: time.Now(), Source: "claude-code",
	})

	if out := tree(m); !strings.Contains(out, "guide.md") {
		t.Errorf("file in a new directory did not appear:\n%s", out)
	}
}

// A write the agent has announced shows up before the file exists, dimmed, so
// it can be seen arriving.
func TestAnnouncedWriteAppearsAsPending(t *testing.T) {
	m, _, _ := scanned(t, "existing.md")

	m, _ = send(m, events.Event{
		Type: events.FilePending, Path: "README.md",
		Timestamp: time.Now(), Source: "claude-code",
	})

	if out := tree(m); !strings.Contains(out, "README.md") {
		t.Fatalf("announced file did not appear:\n%s", out)
	}
	if !m.st.Pending("README.md") {
		t.Error("file is not marked pending")
	}
}

// Once the write lands the file stops being pending, which is what turns it
// from dim to ordinary.
func TestPendingClearsWhenTheWriteLands(t *testing.T) {
	m, root, _ := scanned(t, "existing.md")
	now := time.Now()

	m, _ = send(m, events.Event{Type: events.FilePending, Path: "README.md", Timestamp: now, Source: "claude-code"})
	if !m.st.Pending("README.md") {
		t.Fatal("not pending after being announced")
	}

	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ = send(m, events.Event{Type: events.FileCreate, Path: "README.md", Timestamp: now, Source: "claude-code"})

	if m.st.Pending("README.md") {
		t.Error("still pending after the write landed")
	}
	if out := tree(m); !strings.Contains(out, "README.md") {
		t.Errorf("file vanished after the write:\n%s", out)
	}
}

// A refused write reports no failure of its own, so the claim has to be
// dropped when the turn ends or it would linger forever.
func TestPendingClearsWhenTheTurnEnds(t *testing.T) {
	m, _, _ := scanned(t, "existing.md")
	now := time.Now()

	m, _ = send(m, events.Event{Type: events.FilePending, Path: "denied.md", Timestamp: now, Source: "claude-code"})
	m, _ = send(m, events.Event{
		Type: events.AgentStatus, Status: events.StatusWaiting,
		Timestamp: now, Source: "claude-code",
	})

	if m.st.Pending("denied.md") {
		t.Error("a write that never happened is still shown as pending")
	}
}

// Refreshing a directory must not collapse what the user had opened.
func TestRefreshKeepsExpandedDirectories(t *testing.T) {
	m, root, scanner := scanned(t, "src/deep/file.go", "other.md")

	scanner.Reveal(m.st.Project.Tree, "src/deep/file.go")
	before := len(m.rows())

	if err := os.WriteFile(filepath.Join(root, "new.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ = send(m, events.Event{
		Type: events.FileCreate, Path: "new.md",
		Timestamp: time.Now(), Source: "claude-code",
	})

	if got := len(m.rows()); got < before {
		t.Errorf("tree collapsed on refresh: %d rows, was %d", got, before)
	}
	if out := tree(m); !strings.Contains(out, "file.go") {
		t.Errorf("expanded directory was closed by the refresh:\n%s", out)
	}
}
