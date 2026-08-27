package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/seongyooo/agentline/internal/project"
	"github.com/seongyooo/agentline/internal/state"
)

// fakeSender records what the UI submitted.
type fakeSender struct {
	sent []string
	err  error
}

func (f *fakeSender) Send(prompt string) error {
	f.sent = append(f.sent, prompt)
	return f.err
}

// sendable returns a model that owns a session, with its prompt field live.
func sendable(t *testing.T) (Model, *fakeSender) {
	t.Helper()

	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	sender := &fakeSender{}

	m, _ := New(st, nil, nil).WithSender(sender).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m.(Model), sender
}

// typeText feeds each character through the message loop, as a user would.
func typeText(m Model, text string) Model {
	for _, r := range text {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	return m
}

func key(m Model, k tea.KeyType) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyMsg{Type: k})
	return next.(Model), cmd
}

// focusPromptKey jumps straight to the prompt. Tab cycles through every
// scrollable panel, so it is not a direct route to the prompt any more.
func focusPromptKey(m Model) Model {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	return next.(Model)
}

func TestPromptIsSubmittedOnEnter(t *testing.T) {
	m, sender := sendable(t)

	m = focusPromptKey(m)
	m = typeText(m, "fix the valve")
	m, cmd := key(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("enter produced no command")
	}
	cmd() // the send runs off the UI goroutine

	if len(sender.sent) != 1 || sender.sent[0] != "fix the valve" {
		t.Errorf("sent = %v, want [fix the valve]", sender.sent)
	}
	if got := m.input.Value(); got != "" {
		t.Errorf("field still holds %q after sending", got)
	}
}

// While typing, keys are text. "q" must not quit and the arrows must not
// drive the tree.
func TestTypingDoesNotTriggerNavigation(t *testing.T) {
	m, _ := sendable(t)
	before := m.tree.cursor

	m = focusPromptKey(m)
	m = typeText(m, "quit down")
	m, cmd := key(m, tea.KeyDown)

	if cmd != nil {
		t.Error("an arrow key while typing produced a command")
	}
	if m.tree.cursor != before {
		t.Errorf("cursor moved to %d while typing", m.tree.cursor)
	}
	if !strings.Contains(m.input.Value(), "quit down") {
		t.Errorf("field holds %q; the text was swallowed", m.input.Value())
	}
}

func TestQuitStillWorksWhileNavigating(t *testing.T) {
	m, _ := sendable(t)

	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}); cmd == nil {
		t.Error("q did not quit while navigating")
	}
}

// Ctrl+C must always work, even mid-sentence.
func TestCtrlCQuitsWhileTyping(t *testing.T) {
	m, _ := sendable(t)
	m = focusPromptKey(m)
	m = typeText(m, "half a thought")

	if _, cmd := key(m, tea.KeyCtrlC); cmd == nil {
		t.Error("ctrl+c did not quit while typing")
	}
}

func TestEscReturnsFocusToTheTree(t *testing.T) {
	m, _ := sendable(t)

	m = focusPromptKey(m)
	if !m.inputFocused() {
		t.Fatal("i did not focus the prompt")
	}

	m, _ = key(m, tea.KeyEsc)
	if m.inputFocused() {
		t.Error("esc did not return focus to the tree")
	}

	m, _ = key(m, tea.KeyDown)
	if m.tree.cursor == 0 {
		t.Error("tree did not respond after focus returned")
	}
}

func TestEmptyPromptIsNotSent(t *testing.T) {
	m, sender := sendable(t)

	m = focusPromptKey(m)
	m = typeText(m, "   ")
	if _, cmd := key(m, tea.KeyEnter); cmd != nil {
		cmd()
	}

	if len(sender.sent) != 0 {
		t.Errorf("sent %v, want nothing", sender.sent)
	}
}

// A send that fails must say so rather than vanish.
func TestFailedSendIsShown(t *testing.T) {
	m, sender := sendable(t)
	sender.err = errors.New("session is not running")

	m = focusPromptKey(m)
	m = typeText(m, "hello")
	_, cmd := key(m, tea.KeyEnter)

	next, _ := m.Update(cmd())
	m = next.(Model)

	if out := ansi.Strip(m.View()); !strings.Contains(out, "session is not running") {
		t.Errorf("failure not shown:\n%s", out)
	}
}

// Only observing a session means there is nothing to submit to, and the box
// must not pretend otherwise.
func TestObserverModeHasNoLivePrompt(t *testing.T) {
	m := sized(t, 100, 30)

	if m = focusPromptKey(m); m.inputFocused() {
		t.Error("prompt took focus with no session to send to")
	}
	// Cycling must skip it too, rather than parking focus somewhere inert.
	for i := 0; i < 4; i++ {
		m, _ = key(m, tea.KeyTab)
		if m.inputFocused() {
			t.Fatal("tab cycled onto a prompt that cannot send")
		}
	}
	if out := ansi.Strip(m.View()); !strings.Contains(out, "Ask ") {
		t.Errorf("placeholder missing:\n%s", out)
	}
}

// The prompt names the agent that is actually running. Telling someone to ask
// Claude Code while Codex is doing the work is a small lie with no reason.
func TestPlaceholderNamesTheRunningAgent(t *testing.T) {
	tests := []struct {
		agent string
		want  string
	}{
		{"", "Ask the agent..."},
		{"claude-code", "Ask Claude Code..."},
		{"codex", "Ask codex..."},
	}

	for _, tc := range tests {
		t.Run(tc.agent, func(t *testing.T) {
			m, _ := sendable(t)
			if tc.agent != "" {
				m.st.Agent.Agent = tc.agent
			} else {
				m.st.Agent.Agent = "—"
			}
			if got := m.placeholder(); got != tc.want {
				t.Errorf("placeholder = %q, want %q", got, tc.want)
			}
		})
	}
}

// The hint has to say which keys apply, since they change with focus.
func TestHintFollowsFocus(t *testing.T) {
	m, _ := sendable(t)

	if got := m.inputHint(); !strings.Contains(got, "prompt") {
		t.Errorf("hint = %q, want it to offer the prompt", got)
	}

	m = focusPromptKey(m)
	if got := m.inputHint(); !strings.Contains(got, "enter") {
		t.Errorf("hint = %q, want it to explain sending", got)
	}
}

// Typing must not break the layout at any width.
func TestPromptKeepsTheLayoutIntact(t *testing.T) {
	for _, w := range []int{40, 60, 80, 120, 200} {
		st := state.New("/proj")
		st.Project.Tree = project.MockTree()

		model, _ := New(st, nil, nil).WithSender(&fakeSender{}).Update(tea.WindowSizeMsg{Width: w, Height: 30})
		m := model.(Model)

		m = focusPromptKey(m)
		m = typeText(m, strings.Repeat("a very long instruction ", 10))

		for i, line := range strings.Split(m.View(), "\n") {
			if got := ansi.StringWidth(line); got != w {
				t.Errorf("width %d line %d: got %d", w, i, got)
			}
		}
	}
}
