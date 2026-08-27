package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/seonl/agentview/internal/events"
	"github.com/seonl/agentview/internal/project"
	"github.com/seonl/agentview/internal/state"
)

// Korean, Japanese and Chinese characters occupy two terminal cells each, so
// a field sized by counting characters overflows its box. Typing Korean was
// pushing the prompt out past the edge of the screen.
func TestKoreanPromptStaysInsideTheBox(t *testing.T) {
	for _, w := range []int{40, 60, 80, 100, 160} {
		st := state.New("/proj")
		st.Project.Tree = project.MockTree()

		model, _ := New(st, nil, nil).WithSender(&fakeSender{}).Update(tea.WindowSizeMsg{Width: w, Height: 30})
		m := focusPromptKey(model.(Model))
		m = typeText(m, "리드미 파일을 만들어서 프로젝트 설명을 자세히 적어줘")

		for i, line := range strings.Split(m.View(), "\n") {
			if got := ansi.StringWidth(line); got != w {
				t.Errorf("width %d line %d: got %d cells: %q", w, i, got, ansi.Strip(line))
			}
		}
	}
}

// The widget pads its field to a character count, which runs two cells past
// the box for every Hangul syllable. The bar must be measured in cells, not
// characters, however much CJK text it holds.
func TestPromptBarIsMeasuredInCells(t *testing.T) {
	m, _ := sendable(t)
	m = focusPromptKey(m)

	for _, text := range []string{"", "hello", "안녕하세요", "안녕하세요 hello 반갑습니다"} {
		m.input.SetValue(text)
		if got := ansi.StringWidth(m.inputBar(100)); got != 100 {
			t.Errorf("bar with %q is %d cells, want 100", text, got)
		}
	}
}

// Mixed-width text is the common case in practice.
func TestMixedWidthPromptStaysInsideTheBox(t *testing.T) {
	m, _ := sendable(t)
	m = focusPromptKey(m)
	m = typeText(m, "README.md 파일에 AgentView 설명을 추가하고 테스트도 같이 작성해줘")

	for i, line := range strings.Split(m.View(), "\n") {
		if got := ansi.StringWidth(line); got != 100 {
			t.Errorf("line %d: got %d cells: %q", i, got, ansi.Strip(line))
		}
	}
}

// Typing past the width must keep the end of the line in view, or the user
// would be typing somewhere they cannot see.
func TestLongPromptKeepsTheEndVisible(t *testing.T) {
	m, _ := sendable(t)
	m = focusPromptKey(m)
	m = typeText(m, strings.Repeat("가나다라마바사", 20)+"끝")

	if out := ansi.Strip(m.View()); !strings.Contains(out, "끝") {
		t.Errorf("the caret end of the line is not visible:\n%s", out)
	}
}

// Wide characters must not break the panels either.
func TestKoreanTextInPanelsKeepsTheLayout(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()

	now := time.Now()
	st.Apply(events.Event{Type: events.UserPrompt, Message: "프로젝트 문서를 정리해줘", Timestamp: now, Source: "claude-code"})
	st.Apply(events.Event{
		Type: events.AgentReply, Timestamp: now, Source: "claude-code",
		Message: "리드미를 새로 썼습니다. 설치 방법은 빌드 절차가 확정되지 않아 넣지 않았고, " +
			"금방 낡을 내용을 넣는 것보다는 비워 두는 편이 낫다고 판단했습니다.",
	})

	for _, w := range []int{40, 72, 100, 160} {
		model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: w, Height: 30})
		for i, line := range strings.Split(model.(Model).View(), "\n") {
			if got := ansi.StringWidth(line); got != w {
				t.Errorf("width %d line %d: got %d cells: %q", w, i, got, ansi.Strip(line))
			}
		}
	}
}

// The session line reports what the agent said about its own run, and nothing
// AgentView measured or guessed.
func TestSessionLineShowsReportedFacts(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	st.Apply(events.Event{
		Type: events.SessionInfo, Timestamp: time.Now(), Source: "claude-code",
		Session: &events.Session{
			Model: "claude-opus-5-20260514",
			Limits: map[string]events.Limit{
				"five_hour": {Used: 0.62, ResetsAt: time.Now().Add(2 * time.Hour)},
			},
		},
	})

	model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	out := ansi.Strip(model.(Model).View())

	for _, want := range []string{"SESSION", "opus-5", "5h 62%"} {
		if !strings.Contains(out, want) {
			t.Errorf("session line missing %q:\n%s", want, out)
		}
	}
}

// Both usage windows are reported separately, so a weekly report must not
// erase the five-hour one.
func TestBothUsageWindowsAreShown(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	st.Apply(events.Event{
		Type: events.SessionInfo, Timestamp: time.Now(), Source: "claude-code",
		Session: &events.Session{
			Model: "claude-opus-5-20260514",
			Limits: map[string]events.Limit{
				"five_hour": {Used: 0.62, ResetsAt: time.Now().Add(2 * time.Hour)},
				"seven_day": {Used: 0.31, ResetsAt: time.Now().Add(80 * time.Hour)},
			},
		},
	})

	model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	out := ansi.Strip(model.(Model).View())

	for _, want := range []string{"5h 62%", "week 31%"} {
		if !strings.Contains(out, want) {
			t.Errorf("session line missing %q:\n%s", want, out)
		}
	}
	// Shortest window first: it is the one that runs out soonest.
	if strings.Index(out, "5h 62%") > strings.Index(out, "week 31%") {
		t.Errorf("weekly window is shown before the five-hour one:\n%s", out)
	}
}

// The context a session carries is what makes a long conversation expensive,
// so it has to be visible rather than inferred from a slowly draining quota.
func TestSessionLineShowsContextSize(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	st.Apply(events.Event{
		Type: events.SessionInfo, Timestamp: time.Now(), Source: "claude-code",
		Session: &events.Session{
			Model: "claude-opus-5-20260514",
			Turns: 7, InputTokens: 143_000, OutputTokens: 2_100, CostUSD: 1.37,
		},
	})

	model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	out := ansi.Strip(model.(Model).View())

	for _, want := range []string{"ctx 143k", "7 turns", "$1.37"} {
		if !strings.Contains(out, want) {
			t.Errorf("session line missing %q:\n%s", want, out)
		}
	}
}

// Nothing reported means nothing shown: AgentView does not estimate usage.
func TestNoSessionLineWithoutAReport(t *testing.T) {
	if out := ansi.Strip(render(t, 100, 30)); strings.Contains(out, "SESSION") {
		t.Errorf("session line shown with nothing reported:\n%s", out)
	}
}

func TestSessionLineKeepsTheLayout(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	st.Apply(events.Event{
		Type: events.SessionInfo, Timestamp: time.Now(), Source: "claude-code",
		Session: &events.Session{
			Model: "claude-some-very-long-model-name-20260514",
			Limits: map[string]events.Limit{
				"five_hour": {Used: 0.97, Overage: true, ResetsAt: time.Now().Add(time.Hour)},
			},
		},
	})

	for _, w := range []int{40, 72, 100, 200} {
		model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: w, Height: 30})
		for i, line := range strings.Split(model.(Model).View(), "\n") {
			if got := ansi.StringWidth(line); got != w {
				t.Errorf("width %d line %d: got %d", w, i, got)
			}
		}
	}
}
