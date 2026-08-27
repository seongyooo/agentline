package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/seonl/agentview/internal/events"
	"github.com/seonl/agentview/internal/project"
	"github.com/seonl/agentview/internal/state"
)

// The layout measures with x/ansi and the screen renderer measures with
// go-runewidth. If they disagree about any character the UI draws, the
// renderer's cursor bookkeeping drifts and the output corrupts — which is
// what made Hangul appear to the left of the prompt marker.
//
// This is the regression guard: the two tables must agree, character by
// character, over everything AgentView puts on screen.
func TestWidthTablesAgreeOnEveryGlyphDrawn(t *testing.T) {
	for _, r := range glyphsDrawn(t) {
		s := string(r)

		byLayout := ansi.StringWidth(s)
		byRenderer := runewidth.RuneWidth(r)
		if byLayout != byRenderer {
			t.Errorf("%q U+%04X: layout says %d cells, renderer says %d",
				s, r, byLayout, byRenderer)
		}
	}
}

// The ambiguous characters this UI is built from are the ones that broke, so
// they are named explicitly rather than left to whatever a render happens to
// contain.
func TestAmbiguousCharactersMeasureTheSameBothWays(t *testing.T) {
	for _, r := range []rune{'─', '│', '├', '└', '●', '◐', '█', '░', '…', '·', '↻', '◂', '✓', '✗', '○', '⚠'} {
		if byLayout, byRenderer := ansi.StringWidth(string(r)), runewidth.RuneWidth(r); byLayout != byRenderer {
			t.Errorf("%q U+%04X: layout says %d, renderer says %d", string(r), r, byLayout, byRenderer)
		}
	}
}

// Korean must measure the same both ways too, since it is what the user types.
func TestHangulMeasuresTheSameBothWays(t *testing.T) {
	for _, s := range []string{"안", "녕", "하", "세", "요", "가", "한글"} {
		if byLayout, byRenderer := ansi.StringWidth(s), runewidth.StringWidth(s); byLayout != byRenderer {
			t.Errorf("%q: layout says %d, renderer says %d", s, byLayout, byRenderer)
		}
	}
}

// A rendered line must be the same width by both measures, or the renderer
// will place the next one in the wrong column.
func TestRenderedLinesMeasureTheSameBothWays(t *testing.T) {
	m := populated(t, 100, 30)

	for i, line := range strings.Split(m.View(), "\n") {
		byLayout := ansi.StringWidth(line)
		byRenderer := runewidth.StringWidth(ansi.Strip(line))
		if byLayout != byRenderer {
			t.Errorf("line %d: layout says %d cells, renderer says %d: %q",
				i, byLayout, byRenderer, ansi.Strip(line))
		}
	}
}

// glyphsDrawn collects every distinct character a populated render produces.
func glyphsDrawn(t *testing.T) []rune {
	t.Helper()

	seen := map[rune]bool{}
	var out []rune
	for _, r := range ansi.Strip(populated(t, 100, 30).View()) {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out
}

// populated renders a model carrying every kind of content the UI shows, so
// the checks above see the real glyph set rather than an empty frame.
func populated(t *testing.T, w, h int) Model {
	t.Helper()

	st := state.New("/proj")
	st.Project.Tree = project.MockTree()

	now := time.Now()
	for _, e := range []events.Event{
		{Type: events.UserPrompt, Message: "리드미를 작성해줘", Timestamp: now, Source: "claude-code"},
		{Type: events.TaskProgress, Done: 2, Total: 5, Timestamp: now, Source: "claude-code"},
		{Type: events.FileEdit, Path: "Assets/Scripts/Puzzle/Valve.cs", Timestamp: now, Source: "claude-code"},
		{Type: events.CommandStart, Command: "go test ./...", Message: "테스트 실행", Timestamp: now, Source: "claude-code"},
		{Type: events.AgentReply, Message: "완료했습니다. 자세한 내용은 커밋을 확인해주세요.", Timestamp: now, Source: "claude-code"},
		{Type: events.SessionInfo, Timestamp: now, Source: "claude-code", Session: &events.Session{
			Model: "claude-opus-5-20260514", Turns: 3, InputTokens: 140_000, CostUSD: 0.5,
			Limits: map[string]events.Limit{
				"five_hour": {Used: 0.62, ResetsAt: now.Add(2 * time.Hour)},
				"seven_day": {Used: 0.28, ResetsAt: now.Add(90 * time.Hour)},
			},
		}},
	} {
		st.Apply(e)
	}

	model, _ := New(st, nil, nil).WithSender(&fakeSender{}).Update(tea.WindowSizeMsg{Width: w, Height: h})
	return model.(Model)
}
