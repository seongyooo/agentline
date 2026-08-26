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

// A command is not a path. Abbreviating it the way a path is abbreviated
// corrupts it: `cmd //c "exit 4"` became `.../c "exit 4"`.
func TestCommandsAreNotAbbreviatedAsPaths(t *testing.T) {
	const command = `cmd //c "exit 4"; echo "exit code: $?"`

	m := sized(t, 120, 30)
	m, _ = send(m, events.Event{
		Type:      events.CommandStart,
		Command:   command,
		Timestamp: time.Now(),
		Source:    "claude-code",
	})

	out := ansi.Strip(m.View())
	if !strings.Contains(out, command) {
		t.Errorf("command was mangled in NOW:\n%s", out)
	}
	if strings.Contains(out, ".../c") {
		t.Error("command was abbreviated as if it were a path")
	}
}

// Error text is not a path either.
func TestErrorMessagesAreNotAbbreviatedAsPaths(t *testing.T) {
	const message = "cannot open src/main/resources/app.yaml"

	m := sized(t, 120, 30)
	m, _ = send(m, events.Event{
		Type:      events.AgentError,
		Message:   message,
		Timestamp: time.Now(),
		Source:    "claude-code",
	})

	if out := ansi.Strip(m.View()); !strings.Contains(out, message) {
		t.Errorf("error message was mangled:\n%s", out)
	}
}

// File paths are still abbreviated, since that is what the rule is for.
func TestFilePathsAreStillAbbreviated(t *testing.T) {
	m := sized(t, 120, 30)
	m, _ = send(m, edit("Assets/Scripts/Puzzle/DrainSystem.cs"))

	if out := ansi.Strip(m.View()); !strings.Contains(out, ".../Puzzle/DrainSystem.cs") {
		t.Errorf("long path was not abbreviated:\n%s", out)
	}
}

// With nothing observed, the UI must say so and point at where activity is
// expected from, so hooks that were never installed look different from an
// agent that is simply idle.
func TestUnwiredSetupIsDiagnosable(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()

	const hint = "Waiting for Claude Code hooks on 127.0.0.1:8787"
	model, _ := New(st, nil, nil).WithHint(hint).Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	out := ansi.Strip(model.(Model).View())
	if !strings.Contains(out, "No agent activity yet") {
		t.Errorf("idle UI does not say it has seen nothing:\n%s", out)
	}
	if !strings.Contains(out, hint) {
		t.Errorf("idle UI does not say where events are expected:\n%s", out)
	}
}

// Once activity arrives the hint has done its job and must get out of the way.
func TestHintDisappearsAfterFirstEvent(t *testing.T) {
	const hint = "Waiting for Claude Code hooks on 127.0.0.1:8787"

	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	model, _ := New(st, nil, nil).WithHint(hint).Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	m, _ := send(model.(Model), edit("Assets/Scripts/Puzzle/Valve.cs"))

	out := ansi.Strip(m.View())
	if strings.Contains(out, hint) {
		t.Error("hint is still shown after activity arrived")
	}
	if !strings.Contains(out, "Valve.cs") {
		t.Errorf("NOW did not switch to the observed activity:\n%s", out)
	}
}

// State handed to the UI already populated is real observed activity, so the
// idle message must not claim otherwise.
func TestPrepopulatedStateIsNotReportedAsIdle(t *testing.T) {
	if out := ansi.Strip(render(t, 100, 30)); strings.Contains(out, "No agent activity yet") {
		t.Errorf("state with activity was reported as idle:\n%s", out)
	}
}

// Revealing a file inserts rows above the selection. The cursor must follow
// the node the user picked, not the index it happened to sit at.
func TestSelectionSurvivesTreeReveal(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, "src/deep/nested/target.go", "zz-last.go")

	scanner := project.NewScanner(root)
	st := state.New(root)
	st.Project.Tree = scanner.NewTree()

	model, _ := New(st, scanner, nil).Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m := model.(Model)

	// Select the last row, which revealing a deep path will push down.
	for i := 0; i < len(m.rows())-1; i++ {
		m = press(m, tea.KeyDown)
	}
	chosen := m.selected()

	m, _ = send(m, edit("src/deep/nested/target.go"))

	if got := m.selected(); got != chosen {
		t.Errorf("selection moved from %q to %q after a reveal", chosen.Name, got.Name)
	}
}

// writeTree creates the given slash-separated files under root.
func writeTree(t *testing.T, root string, paths ...string) {
	t.Helper()

	for _, p := range paths {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
