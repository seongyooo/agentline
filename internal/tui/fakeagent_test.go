package tui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/seonl/agentview/internal/agent/claude"
	"github.com/seonl/agentview/internal/project"
	"github.com/seonl/agentview/internal/state"
)

// The stand-in agent exists so the whole of AgentView can be exercised
// without paying for a session. That is only worth anything if it drives the
// same path a real one does, so this runs it through the real adapter and
// checks the interface fills in.
func TestStandInAgentDrivesTheWholeUI(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a helper binary")
	}
	bin := buildStandIn(t)

	root := t.TempDir()
	writeTree(t, root, "src/main.go", "README.md")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	session := claude.NewStream(root)
	session.Bin = bin
	stream, err := session.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}

	scanner := project.NewScanner(root)
	st := state.New(root)
	st.Project.Tree = scanner.NewTree()

	model, _ := New(st, scanner, stream).WithSender(session).Update(tea.WindowSizeMsg{Width: 100, Height: 32})
	m := model.(Model)

	if err := session.Send("write the readme"); err != nil {
		t.Fatal(err)
	}

	// Pump the loop the way Bubble Tea does, until the turn ends.
	deadline := time.After(60 * time.Second)
	for done := false; !done; {
		select {
		case e, ok := <-stream:
			if !ok {
				done = true
				break
			}
			next, _ := m.Update(eventMsg(e))
			m = next.(Model)

			// The turn's usage arrives alongside the status change that ends
			// it, so the queue is drained before anything is asserted.
			if m.st.Agent.Status == "waiting" && m.st.Agent.Reply != "" && len(stream) == 0 {
				done = true
			}
		case <-deadline:
			t.Fatalf("the stand-in never finished a turn; state was %+v", m.st.Agent)
		}
	}

	// How full the context is is asked for once the turn ends, so the reply
	// arrives after it. Draining lets it land before anything is asserted.
	if cmd := m.askForContextUsage(); cmd != nil {
		cmd()
	}
	deadline = time.After(10 * time.Second)
	for m.st.Agent.Session == nil || m.st.Agent.Session.ContextWindow == 0 {
		select {
		case e, ok := <-stream:
			if !ok {
				t.Fatal("stream closed before the context usage arrived")
			}
			next, _ := m.Update(eventMsg(e))
			m = next.(Model)
		case <-deadline:
			t.Fatal("the stand-in never reported how full the context is")
		}
	}

	out := ansi.Strip(m.View())
	for _, want := range []string{
		"write the readme", // the mission, from the prompt
		"fakeagent",        // the reply, saying plainly what produced it
		"Context:",         // how full the context is, which is what costs
		"5h:",              // the usage windows, both of them
		"7d:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("interface missing %q:\n%s", want, out)
		}
	}

	// It works on real files, so the tree shows what a real session would.
	if _, err := os.Stat(filepath.Join(root, "FAKE-NOTES.md")); err != nil {
		t.Errorf("the file it reported writing is not there: %v", err)
	}
	if len(m.st.Project.ActivityByPath) == 0 {
		t.Error("no file activity was recorded")
	}
	if !m.st.Agent.Progress.Known() {
		t.Error("no progress was reported")
	}
}

// Control requests have to reach it too, or the keys that use them cannot be
// tried without a real session.
func TestStandInAgentAnswersControlRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a helper binary")
	}
	bin := buildStandIn(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	session := claude.NewStream(t.TempDir())
	session.Bin = bin
	stream, err := session.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := session.SetPermissionMode("acceptEdits"); err != nil {
		t.Fatal(err)
	}

	st := state.New("/proj")
	deadline := time.After(30 * time.Second)
	for st.PermissionMode() != "acceptEdits" {
		select {
		case e, ok := <-stream:
			if !ok {
				t.Fatal("stream closed before the mode changed")
			}
			st.Apply(e)
		case <-deadline:
			t.Fatalf("mode never changed; it is %q", st.PermissionMode())
		}
	}

	// A model it does not know must be refused, so the error path can be seen.
	if err := session.SetModel("not-a-real-model"); err != nil {
		t.Fatal(err)
	}
	deadline = time.After(30 * time.Second)
	for {
		select {
		case e, ok := <-stream:
			if !ok {
				t.Fatal("stream closed before the refusal arrived")
			}
			st.Apply(e)
			if strings.Contains(st.Agent.Now.Target, "unknown model") {
				return
			}
		case <-deadline:
			t.Fatal("an unknown model was not refused")
		}
	}
}

// buildStandIn compiles the helper, so a change that breaks it is caught here
// rather than the next time someone tries to use it.
func buildStandIn(t *testing.T) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "fakeagent.exe")
	build := exec.Command("go", "build", "-o", bin, "github.com/seonl/agentview/cmd/fakeagent")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fakeagent: %v\n%s", err, out)
	}
	return bin
}
