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

	"github.com/seongyooo/agentline/internal/agent/claude"
	"github.com/seongyooo/agentline/internal/events"
	"github.com/seongyooo/agentline/internal/project"
	"github.com/seongyooo/agentline/internal/state"
)

// The stand-in agent exists so the whole of AgentLine can be exercised
// without paying for a session. That is only worth anything if it drives the
// same path a real one does, so this runs it through the real adapter and
// checks the interface fills in.
func TestStandInAgentDrivesTheWholeUI(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a helper binary")
	}
	root := t.TempDir()
	writeTree(t, root, "src/main.go", "README.md")

	session, stream := startStandIn(t, root)

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

// A thing you run to try out an interface must not put a repository at risk.
// It reads real files, which is what makes the tree behave, but the only file
// it ever writes is its own.
func TestStandInAgentWritesOnlyItsOwnFile(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a helper binary")
	}
	root := t.TempDir()
	writeTree(t, root, "src/main.go", "README.md", "go.mod")

	before := snapshot(t, root)
	session, stream := startStandIn(t, root)

	// The editing branch runs first, while the only files present are the
	// project's own. Creating the scratch file first would put it at the head
	// of the list and hide a stand-in that edits whatever it finds there.
	for _, prompt := range []string{"have a look around", "write the readme", "look again"} {
		if err := session.Send(prompt); err != nil {
			t.Fatal(err)
		}
		drainTurn(t, stream)
	}

	for path, was := range before {
		now, err := describe(filepath.Join(root, path))
		if err != nil {
			t.Errorf("%s is gone: %v", path, err)
			continue
		}
		if now != was {
			t.Errorf("%s was written to", path)
		}
	}
}

// fileState is enough to tell whether a file was written to at all.
//
// The content alone is not: the flaw this guards against rewrote files with
// their own contents, which reads as unchanged while still being a write that
// a crash could truncate. The modification time is what catches it.
type fileState struct {
	content  string
	modified time.Time
}

func describe(path string) (fileState, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return fileState{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fileState{}, err
	}
	return fileState{content: string(content), modified: info.ModTime()}, nil
}

// snapshot records every file under root as it stands.
func snapshot(t *testing.T, root string) map[string]fileState {
	t.Helper()

	files := map[string]fileState{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		state, err := describe(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[rel] = state
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// drainTurn reads until the turn ends.
func drainTurn(t *testing.T, stream <-chan events.Event) {
	t.Helper()

	deadline := time.After(60 * time.Second)
	for {
		select {
		case e, ok := <-stream:
			if !ok {
				return
			}
			if e.Type == events.AgentStatus && e.Status == events.StatusWaiting {
				return
			}
		case <-deadline:
			t.Fatal("the turn never ended")
		}
	}
}

// Control requests have to reach it too, or the keys that use them cannot be
// tried without a real session.
func TestStandInAgentAnswersControlRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a helper binary")
	}
	session, stream := startStandIn(t, t.TempDir())

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
	build := exec.Command("go", "build", "-o", bin, "github.com/seongyooo/agentline/cmd/fakeagent")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fakeagent: %v\n%s", err, out)
	}
	return bin
}

// startStandIn runs the helper against root and guarantees it is gone before
// the test returns.
//
// The session's working directory is the temporary directory the test is
// about to have removed, and Windows will not delete a directory a live
// process is sitting in. Waiting for the stream to close is what makes that
// removal deterministic rather than a race against a process shutting down.
func startStandIn(t *testing.T, root string) (*claude.Stream, <-chan events.Event) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)

	session := claude.NewStream(root)
	session.Bin = buildStandIn(t)

	stream, err := session.Events(ctx)
	if err != nil {
		cancel()
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cancel()

		// The stream closes once the process has exited, so draining it is
		// how the test knows there is nothing left holding the directory.
		done := time.After(30 * time.Second)
		for {
			select {
			case _, ok := <-stream:
				if !ok {
					return
				}
			case <-done:
				t.Error("the session did not shut down")
				return
			}
		}
	})
	return session, stream
}
