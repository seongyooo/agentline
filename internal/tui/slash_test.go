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
	models  []string
	modeErr error
}

func (c *controllingSender) SetPermissionMode(mode string) error {
	if c.modeErr != nil {
		return c.modeErr
	}
	c.modes = append(c.modes, mode)
	return nil
}

func (c *controllingSender) SetModel(model string) error {
	c.models = append(c.models, model)
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

// Commands the agent owns are sent as typed; interpreting them is its job.
func TestSlashCommandIsSentAsTypedText(t *testing.T) {
	m, sender := announced(t, "compact")
	m = typeText(m, "/compact now")

	_, cmd := key(m, tea.KeyEnter)
	cmd()

	if len(sender.sent) != 1 || sender.sent[0] != "/compact now" {
		t.Errorf("sent = %v, want [/compact now]", sender.sent)
	}
}

// A session AgentView owns has no picker of its own, so /model with a name is
// carried out over the control protocol rather than sent as text that would
// do nothing.
func TestModelCommandSwitchesTheModel(t *testing.T) {
	m, sender := announced(t, "model")
	m = typeText(m, "/model sonnet")

	_, cmd := key(m, tea.KeyEnter)
	updated, _ := m.Update(cmd())
	m = updated.(Model)

	if len(sender.models) != 1 || sender.models[0] != "sonnet" {
		t.Errorf("asked for %v, want [sonnet]", sender.models)
	}
	if len(sender.sent) != 0 {
		t.Errorf("also sent %v as text", sender.sent)
	}
}

// "default" is the agent's word for returning to the session default, and is
// sent as an empty model rather than as a model of that name.
func TestDefaultModelResetsRatherThanNamingAModel(t *testing.T) {
	m, sender := announced(t, "model")
	m = typeText(m, "/model default")

	_, cmd := key(m, tea.KeyEnter)
	cmd()

	if len(sender.models) != 1 || sender.models[0] != "" {
		t.Errorf("asked for %v, want [\"\"]", sender.models)
	}
}

// Only observing a session means AgentView cannot switch models, so the text
// goes to the agent rather than being swallowed by a feature that is absent.
func TestModelCommandFallsBackToTextWithoutAController(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	sender := &fakeSender{}

	model, _ := New(st, nil, nil).WithSender(sender).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := typeText(focusPromptKey(model.(Model)), "/model opus")

	_, cmd := key(m, tea.KeyEnter)
	cmd()

	if len(sender.sent) != 1 || sender.sent[0] != "/model opus" {
		t.Errorf("sent = %v, want the text passed through", sender.sent)
	}
}

// A bare /model opens the picker, which is what the session cannot offer.
func TestBareModelCommandOpensThePicker(t *testing.T) {
	m, sender := announced(t, "model")
	m = typeText(m, "/model")

	m, _ = key(m, tea.KeyEnter)
	if !m.picker.open {
		t.Fatal("picker did not open")
	}
	if len(sender.sent) != 0 {
		t.Errorf("also sent %v as text", sender.sent)
	}
	if out := ansi.Strip(m.View()); !strings.Contains(out, "MODEL") {
		t.Errorf("picker not shown:\n%s", out)
	}
}

func TestPickerAppliesTheSelection(t *testing.T) {
	m, sender := announced(t, "model")
	m = typeText(m, "/model")
	m, _ = key(m, tea.KeyEnter)

	before := m.picker.options[m.picker.cursor]
	m, _ = key(m, tea.KeyRight)
	chosen := m.picker.options[m.picker.cursor]
	if chosen == before {
		t.Fatal("the selection did not move")
	}

	m, cmd := key(m, tea.KeyEnter)
	if m.picker.open {
		t.Error("picker stayed open after choosing")
	}
	cmd()

	if len(sender.models) != 1 || sender.models[0] != chosen {
		t.Errorf("asked for %v, want [%s]", sender.models, chosen)
	}
}

// While the picker is open the arrows move the selection, not the caret.
func TestPickerTakesTheArrowKeys(t *testing.T) {
	m, _ := announced(t, "model")
	m = typeText(m, "/model")
	m, _ = key(m, tea.KeyEnter)

	m, _ = key(m, tea.KeyDown)
	if m.picker.cursor != 1 {
		t.Errorf("cursor = %d, want 1", m.picker.cursor)
	}

	// The ends hold still rather than wrapping.
	for i := 0; i < 20; i++ {
		m, _ = key(m, tea.KeyUp)
	}
	if m.picker.cursor != 0 {
		t.Errorf("cursor = %d, want it to stop at the start", m.picker.cursor)
	}
}

func TestEscapeClosesThePickerWithoutChoosing(t *testing.T) {
	m, sender := announced(t, "model")
	m = typeText(m, "/model")
	m, _ = key(m, tea.KeyEnter)

	m, _ = key(m, tea.KeyEsc)
	if m.picker.open {
		t.Error("picker stayed open")
	}
	if len(sender.models) != 0 {
		t.Errorf("a model was set anyway: %v", sender.models)
	}
}

// The model the session is on comes first, so switching back is one key away.
func TestPickerStartsFromTheCurrentModel(t *testing.T) {
	m, _ := announced(t, "model")
	m.st.SetModel("claude-sonnet-5-20260514")

	m = typeText(m, "/model")
	m, _ = key(m, tea.KeyEnter)

	if got := m.picker.options[0]; got != "sonnet" {
		t.Errorf("first option = %q, want the current model", got)
	}
	// And it is listed once, not twice.
	var count int
	for _, option := range m.picker.options {
		if option == "sonnet" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("sonnet appears %d times", count)
	}
}

// The selection has to be visible, or the picker is a list with no way to
// tell what pressing enter would choose.
func TestPickerMarksTheSelection(t *testing.T) {
	m, _ := announced(t, "model")
	m = typeText(m, "/model")
	m, _ = key(m, tea.KeyEnter)

	first := m.pickerLine(100)
	m, _ = key(m, tea.KeyRight)
	second := m.pickerLine(100)

	// Stripped of styling, as a monochrome terminal would show it: the
	// marking must still say which option enter would choose.
	if ansi.Strip(first) == ansi.Strip(second) {
		t.Error("the selection is invisible without colour")
	}
	if ansi.StringWidth(first) != ansi.StringWidth(second) {
		t.Error("moving the selection shifted the list")
	}

	options := strings.Fields(ansi.Strip(first))
	if !strings.Contains(ansi.Strip(first), "["+m.picker.options[0]+"]") {
		t.Errorf("the first option is not marked: %v", options)
	}
}

// The picker must not break the layout at any width.
func TestPickerKeepsTheLayout(t *testing.T) {
	for _, w := range []int{40, 72, 100, 200} {
		st := state.New("/proj")
		st.Project.Tree = project.MockTree()

		model, _ := New(st, nil, nil).WithSender(&controllingSender{}).Update(tea.WindowSizeMsg{Width: w, Height: 30})
		m := typeText(focusPromptKey(model.(Model)), "/model")
		m, _ = key(m, tea.KeyEnter)

		for i, line := range strings.Split(m.View(), "\n") {
			if got := ansi.StringWidth(line); got != w {
				t.Errorf("width %d line %d: got %d", w, i, got)
			}
		}
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
