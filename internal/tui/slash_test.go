package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/seonl/agentview/internal/agent/claude"
	"github.com/seonl/agentview/internal/events"
	"github.com/seonl/agentview/internal/project"
	"github.com/seonl/agentview/internal/state"
)

// controllingSender records prompts and permission mode changes.
type controllingSender struct {
	fakeSender
	modes   []string
	modeErr error
}

func (c *controllingSender) SetPermissionMode(mode string) error {
	if c.modeErr != nil {
		return c.modeErr
	}
	c.modes = append(c.modes, mode)
	return nil
}

// announced returns a model whose session has told AgentView what it accepts.
func announced(t *testing.T, commands ...string) (Model, *controllingSender) {
	t.Helper()

	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	st.Apply(events.Event{
		Type: events.SessionInfo, Timestamp: time.Now(), Source: "claude-code",
		Session: &events.Session{
			Model: "claude-opus-5-20260514",
			Capabilities: events.Capabilities{
				PermissionMode: "default",
				SlashCommands:  commands,
			},
		},
	})

	sender := &controllingSender{}
	model, _ := New(st, nil, nil).WithSender(sender).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return focusPromptKey(model.(Model)), sender
}

// Only commands the session announced are offered, so nothing is presented
// that the agent would not accept.
func TestSlashCompletionOffersAnnouncedCommands(t *testing.T) {
	m, _ := announced(t, "model", "compact", "clear")
	m = typeText(m, "/mo")

	matches := m.slashMatches()
	if len(matches) != 1 || matches[0] != "model" {
		t.Errorf("matches = %v, want [model]", matches)
	}
	if out := ansi.Strip(m.View()); !strings.Contains(out, "/model") {
		t.Errorf("the match is not shown:\n%s", out)
	}
}

func TestSlashCompletionFillsInTheCommand(t *testing.T) {
	m, _ := announced(t, "model", "compact")
	m = typeText(m, "/mo")

	m, _ = key(m, tea.KeyTab)
	if got := m.input.Value(); got != "/model " {
		t.Errorf("value = %q, want %q", got, "/model ")
	}
}

// Completion stops where the choice begins, rather than picking a candidate
// on the user's behalf.
func TestAmbiguousCompletionStopsAtTheSharedPart(t *testing.T) {
	m, _ := announced(t, "compact", "config", "clear")

	// "c" narrows to three commands that agree on nothing further, so
	// completion must not pick one of them.
	m = typeText(m, "/c")
	m, _ = key(m, tea.KeyTab)
	if got := m.input.Value(); got != "/c" {
		t.Errorf("value = %q, want it unchanged while three commands match", got)
	}
	if m.inputFocused() != true {
		t.Error("tab left the prompt while commands were still being chosen")
	}

	// "clear" drops out here, and the two that remain agree on one more
	// letter, which is as far as completion may go.
	m.input.SetValue("/co")
	m.completeSlash()
	if got := m.input.Value(); got != "/co" {
		t.Errorf("value = %q, want %q", got, "/co")
	}

	// One match left: the whole command, and a space to start the argument.
	m.input.SetValue("/com")
	m.completeSlash()
	if got := m.input.Value(); got != "/compact " {
		t.Errorf("value = %q, want %q", got, "/compact ")
	}
}

// With nothing to complete, tab does what it always did.
func TestTabLeavesTheFieldWhenThereIsNothingToComplete(t *testing.T) {
	m, _ := announced(t, "model")
	m = typeText(m, "fix the valve")

	m, _ = key(m, tea.KeyTab)
	if m.inputFocused() {
		t.Error("tab did not leave the prompt")
	}
}

// A slash inside a sentence is a path or a URL, not a command.
func TestSlashInsideTextIsNotACommand(t *testing.T) {
	m, _ := announced(t, "model")
	m = typeText(m, "read src/main.go")

	if matches := m.slashMatches(); matches != nil {
		t.Errorf("matches = %v, want none for a path", matches)
	}
}

// Once the command is typed the rest is its argument, and completing it would
// overwrite what the user is writing.
func TestArgumentIsNotCompleted(t *testing.T) {
	m, _ := announced(t, "model")
	m = typeText(m, "/model opu")

	if matches := m.slashMatches(); matches != nil {
		t.Errorf("matches = %v, want none while typing an argument", matches)
	}
}

// The command text is the agent's to interpret; AgentView sends it unchanged.
func TestSlashCommandIsSentAsTypedText(t *testing.T) {
	m, sender := announced(t, "model")
	m = typeText(m, "/model opus")

	_, cmd := key(m, tea.KeyEnter)
	cmd()

	if len(sender.sent) != 1 || sender.sent[0] != "/model opus" {
		t.Errorf("sent = %v, want [/model opus]", sender.sent)
	}
}

// A session that announced no commands offers none.
func TestNoCommandsOfferedWithoutAnAnnouncement(t *testing.T) {
	m, _ := sendable(t)
	m = typeText(focusPromptKey(m), "/mo")

	if matches := m.slashMatches(); matches != nil {
		t.Errorf("matches = %v, want none", matches)
	}
}

// Shift+Tab moves to the next mode and says so once the session accepts it.
func TestShiftTabCyclesPermissionMode(t *testing.T) {
	m, sender := announced(t, "model")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if cmd == nil {
		t.Fatal("shift+tab produced no command")
	}
	m = next.(Model)

	updated, _ := m.Update(cmd())
	m = updated.(Model)

	want := nextPermissionMode("default")
	if len(sender.modes) != 1 || sender.modes[0] != want {
		t.Errorf("asked for %v, want [%s]", sender.modes, want)
	}
	if got := m.st.PermissionMode(); got != want {
		t.Errorf("recorded %q, want %q", got, want)
	}
}

// The header must not claim a mode the session refused.
func TestRefusedModeChangeIsNotRecorded(t *testing.T) {
	m, sender := announced(t, "model")
	sender.modeErr = errModeRefused

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	updated, _ := m.Update(cmd())
	m = updated.(Model)

	if got := m.st.PermissionMode(); got != "default" {
		t.Errorf("mode = %q, want it unchanged after a refusal", got)
	}
	if out := ansi.Strip(m.View()); !strings.Contains(out, "refused") {
		t.Errorf("the refusal is not shown:\n%s", out)
	}
}

// The mode is shown, since it decides what the agent will do without asking.
func TestPermissionModeIsShown(t *testing.T) {
	m, _ := announced(t, "model")

	if out := ansi.Strip(m.View()); !strings.Contains(out, "asks first") {
		t.Errorf("mode not shown:\n%s", out)
	}
}

// Cycling never lands on the mode that turns every check off.
func TestCycleSkipsBypassPermissions(t *testing.T) {
	mode := "default"
	for i := 0; i < 3*len(claude.PermissionModes); i++ {
		mode = nextPermissionMode(mode)
		if mode == "bypassPermissions" {
			t.Fatal("cycling reached bypassPermissions")
		}
	}
}

// An unfamiliar mode starts the cycle rather than being treated as a position
// in it.
func TestUnknownModeStartsTheCycle(t *testing.T) {
	if got := nextPermissionMode("bypassPermissions"); got != claude.PermissionModes[0] {
		t.Errorf("next = %q, want %q", got, claude.PermissionModes[0])
	}
}

// Only observing a session means there is nothing to change.
func TestObserverCannotChangeMode(t *testing.T) {
	m := sized(t, 100, 30)

	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab}); cmd != nil {
		t.Error("mode change offered with no session to change")
	}
}

// The command list must not break the layout at any width.
func TestSlashHintKeepsTheLayout(t *testing.T) {
	for _, w := range []int{40, 72, 100, 200} {
		st := state.New("/proj")
		st.Project.Tree = project.MockTree()
		st.Apply(events.Event{
			Type: events.SessionInfo, Timestamp: time.Now(), Source: "claude-code",
			Session: &events.Session{Capabilities: events.Capabilities{
				SlashCommands: []string{"compact", "config", "clear", "context", "cost", "commit"},
			}},
		})

		model, _ := New(st, nil, nil).WithSender(&controllingSender{}).Update(tea.WindowSizeMsg{Width: w, Height: 30})
		m := typeText(focusPromptKey(model.(Model)), "/c")

		for i, line := range strings.Split(m.View(), "\n") {
			if got := ansi.StringWidth(line); got != w {
				t.Errorf("width %d line %d: got %d", w, i, got)
			}
		}
	}
}

var errModeRefused = refusedError{}

type refusedError struct{}

func (refusedError) Error() string { return "mode change refused" }
