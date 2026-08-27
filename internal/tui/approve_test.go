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

// approver records how a session was answered.
type approver struct {
	fakeSender
	answers []answer
	modes   []string
}

type answer struct {
	id      string
	allow   bool
	message string
}

func (a *approver) Answer(id string, allow bool, message string) error {
	a.answers = append(a.answers, answer{id, allow, message})
	return nil
}

func (a *approver) SetPermissionMode(mode string) error {
	a.modes = append(a.modes, mode)
	return nil
}
func (a *approver) SetModel(string) error      { return nil }
func (a *approver) RequestContextUsage() error { return nil }

// runCmd executes a command the way the runtime would, following a batch into
// the commands it holds instead of stopping at the message that carries them.
func runCmd(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			runCmd(c)
		}
	}
}

// blocked returns a model whose agent is waiting on a permission answer.
func blocked(t *testing.T, ask *events.Ask) (Model, *approver) {
	t.Helper()

	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	session := &approver{}

	model, _ := New(st, nil, nil).WithSender(session).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := model.(Model)
	m.applyEvent(events.Event{
		Type: events.PermissionAsk, Timestamp: time.Now(), Source: "claude-code", Ask: ask,
	})
	return m, session
}

func sampleAsk() *events.Ask {
	return &events.Ask{
		ID:     "req-1",
		Tool:   "Write",
		Title:  "Write",
		Target: "DrainSystem.cs",
		Reason: "Claude requested permissions to edit DrainSystem.cs which is a sensitive file.",
		Mode:   "acceptEdits",
	}
}

// Being blocked has to be visible without reading anything: the status word
// changes, and the question is on screen with the agent's own reason for it.
func TestBlockedAgentShowsTheQuestion(t *testing.T) {
	m, _ := blocked(t, sampleAsk())

	if got := m.st.Agent.Status; got != events.StatusNeedsInput {
		t.Errorf("status = %q, want %q", got, events.StatusNeedsInput)
	}

	out := ansi.Strip(m.View())
	for _, want := range []string{"NEEDS INPUT", "NEEDS YOU", "Write", "DrainSystem.cs", "sensitive file", "y allow", "n deny"} {
		if !strings.Contains(out, want) {
			t.Errorf("frame is missing %q:\n%s", want, out)
		}
	}
}

func TestYAllowsAndNDenies(t *testing.T) {
	tests := []struct {
		key   string
		allow bool
	}{{"y", true}, {"n", false}}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			m, session := blocked(t, sampleAsk())

			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tc.key)})
			if cmd == nil {
				t.Fatalf("%q produced no answer", tc.key)
			}
			runCmd(cmd) // the reply goes out off the UI goroutine

			if len(session.answers) != 1 {
				t.Fatalf("answers = %v, want one", session.answers)
			}
			got := session.answers[0]
			if got.id != "req-1" || got.allow != tc.allow {
				t.Errorf("answered %+v, want id req-1 allow=%v", got, tc.allow)
			}
			// A refusal must reach the agent as a decision, not as silence,
			// or it cannot tell being told no from something breaking.
			if !tc.allow && got.message == "" {
				t.Error("denied with no message; the agent is not told why")
			}
			_ = next
		})
	}
}

// The agent offers a mode that would stop it having to ask about this kind of
// thing again. Taking that offer must both answer and switch.
func TestASuggestedModeIsApplied(t *testing.T) {
	m, session := blocked(t, sampleAsk())

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if cmd == nil {
		t.Fatal("a produced nothing")
	}
	runCmd(cmd)

	if len(session.answers) != 1 || !session.answers[0].allow {
		t.Errorf("answers = %v, want one allow", session.answers)
	}
	if len(session.modes) != 1 || session.modes[0] != "acceptEdits" {
		t.Errorf("modes = %v, want [acceptEdits]", session.modes)
	}
}

// With no suggestion there is no standing answer to give, and "a" must not
// silently mean something else.
func TestAWithoutASuggestionDoesNothing(t *testing.T) {
	ask := sampleAsk()
	ask.Mode = ""
	m, session := blocked(t, ask)

	runCmdOf(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}))
	if len(session.answers) != 0 {
		t.Errorf("answered %v with nothing suggested", session.answers)
	}
}

// Being stuck behind an unanswerable question would be worse than the problem
// this feature solves.
func TestQuitStillWorksWhileBlocked(t *testing.T) {
	m, _ := blocked(t, sampleAsk())

	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}); cmd == nil {
		t.Error("q did not quit while blocked")
	}
}

// The alert is what makes looking away safe, and what makes it worthless is
// firing on every redraw until the question is answered.
func TestAlertRingsOnceForOneQuestion(t *testing.T) {
	alert := &Alert{}

	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	model, _ := New(st, nil, nil).WithSender(&approver{}).WithAlert(alert).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := model.(Model)

	m.applyEvent(events.Event{Type: events.PermissionAsk, Timestamp: time.Now(), Source: "claude-code", Ask: sampleAsk()})
	if text, armed := alert.take(); !armed || !strings.Contains(text, "DrainSystem.cs") {
		t.Fatalf("first ask did not ring: armed=%v text=%q", armed, text)
	}

	// Anything else arriving while the same question stands must stay quiet.
	m.applyEvent(events.Event{Type: events.FileRead, Path: "Assets/Scripts/Puzzle/Valve.cs", Timestamp: time.Now(), Source: "claude-code"})
	if _, armed := alert.take(); armed {
		t.Error("rang again while the same question was still unanswered")
	}
}

// The alert text is built from the agent's own words, which AgentLine puts
// inside a terminal escape sequence. Anything that could close that sequence
// early has to be gone before it gets there.
func TestAlertTextCannotSmuggleEscapes(t *testing.T) {
	got := alertText("edit \x1b]9;evil\a file\nnow")

	for _, forbidden := range []string{"\x1b", "\a", "\n"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("alertText kept %q: %q", forbidden, got)
		}
	}
	if !strings.Contains(got, "file") {
		t.Errorf("alertText dropped the message: %q", got)
	}
}

// runCmdOf runs whatever an Update returned, ignoring the model.
func runCmdOf(_ tea.Model, cmd tea.Cmd) { runCmd(cmd) }
