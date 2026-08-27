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
	m = typeText(m, "README.md 파일에 AgentLine 설명을 추가하고 테스트도 같이 작성해줘")

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
// AgentLine measured or guessed.
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
	// The bar says how full at a glance and the number says how full exactly.
	// Both are checked: a terminal that draws no blocks still has the whole
	// fact, and a reader who wants the shape need not read the number.
	for _, want := range []string{"Opus 5", "5h", "62%", "resets ", "█", "░"} {
		if !strings.Contains(out, want) {
			t.Errorf("session gauges missing %q", want)
			t.Log(out)
		}
	}
}

// How full the context is comes from the agent, which is the only thing that
// knows the window it is measuring against.
func TestContextShareIsShown(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	st.Apply(events.Event{
		Type: events.SessionInfo, Timestamp: time.Now(), Source: "claude-code",
		Session: &events.Session{
			Model:         "claude-opus-5-20260514",
			ContextWindow: 200_000, ContextUsed: 146_000, ContextPercent: 0.73,
		},
	})

	model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	out := ansi.Strip(model.(Model).View())

	for _, want := range []string{"Context", "73%"} {
		if !strings.Contains(out, want) {
			t.Errorf("context share missing %q", want)
			t.Log(out)
		}
	}
}

// Nothing is shown until the agent has said, rather than a share worked out
// from a token count and a guess at the limit.
func TestNoContextShareUntilReported(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	st.Apply(events.Event{
		Type: events.SessionInfo, Timestamp: time.Now(), Source: "claude-code",
		Session: &events.Session{Model: "claude-opus-5-20260514", InputTokens: 143_000},
	})

	model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if out := ansi.Strip(model.(Model).View()); strings.Contains(out, "Context:") {
		t.Errorf("a context share was shown without one being reported:\n%s", out)
	}
}

// The session line belongs at the bottom of its column, whatever is above it.
func TestSessionLineSitsAtTheBottom(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()

	now := time.Now()
	st.Apply(events.Event{Type: events.UserPrompt, Message: "do the thing", Timestamp: now, Source: "claude-code"})
	st.Apply(events.Event{
		Type: events.AgentReply, Timestamp: now, Source: "claude-code",
		Message: strings.Repeat("a long answer that fills the panel and then some more. ", 12),
	})
	st.Apply(events.Event{
		Type: events.SessionInfo, Timestamp: now, Source: "claude-code",
		Session: &events.Session{
			Model:         "claude-opus-5-20260514",
			ContextWindow: 200_000, ContextPercent: 0.4,
		},
	})

	for _, h := range []int{20, 24, 30, 40} {
		model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: 100, Height: h})
		lines := strings.Split(ansi.Strip(model.(Model).View()), "\n")

		// The status block is pinned to the foot of the column rather than
		// flowing with the content above it, so the last row of the column
		// that holds anything must belong to it.
		l := computeLayout(100, h)
		body := lines[headerRows : headerRows+l.BodyHeight]

		found := false
		for i := len(body) - 1; i >= 0 && !found; i-- {
			text := strings.TrimSpace(strings.Trim(body[i], "│╭╮╰╯─ "))
			if text == "" {
				continue
			}
			found = strings.Contains(text, "%") || strings.Contains(text, "Opus 5")
			if !found {
				t.Errorf("height %d: the column ends on %q, not on the session status", h, text)
			}
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

	for _, want := range []string{"5h", "62%", "7d", "31%"} {
		if !strings.Contains(out, want) {
			t.Errorf("session line missing %q:\n%s", want, out)
		}
	}
	// Shortest window first: it is the one that runs out soonest.
	if strings.Index(out, "5h") > strings.Index(out, "7d") {
		t.Errorf("weekly window is shown before the five-hour one:\n%s", out)
	}
}

// A session AgentLine owns is never compacted, so a context this full keeps
// costing what it costs on every further turn, and the way out is offered.
func TestFullContextOffersARestart(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	st.Apply(events.Event{
		Type: events.SessionInfo, Timestamp: time.Now(), Source: "claude-code",
		Session: &events.Session{
			Model:         "claude-opus-5-20260514",
			ContextWindow: 200_000, ContextPercent: 0.86,
		},
	})

	model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	out := ansi.Strip(model.(Model).View())

	if !strings.Contains(out, "86%") {
		t.Errorf("context share not shown:\n%s", out)
	}
	if !strings.Contains(out, "ctrl+n") {
		t.Errorf("no way out offered for a context this full:\n%s", out)
	}
}

// A context with room left says nothing about restarting.
func TestRoomyContextSaysNothingAboutRestarting(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	st.Apply(events.Event{
		Type: events.SessionInfo, Timestamp: time.Now(), Source: "claude-code",
		Session: &events.Session{ContextWindow: 200_000, ContextPercent: 0.2},
	})

	model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if out := ansi.Strip(model.(Model).View()); strings.Contains(out, "ctrl+n") {
		t.Errorf("restart offered for a context with room left:\n%s", out)
	}
}

// The check is against what the line actually says. It used to look for the
// word SESSION, which the session line has never contained, so it passed on
// every frame, including ones showing a line that was never reported.
func TestNoSessionLineWithoutAReport(t *testing.T) {
	out := ansi.Strip(render(t, 100, 30))
	for _, absent := range []string{"Context:", "5h:", "7d:"} {
		if strings.Contains(out, absent) {
			t.Errorf("session line shows %q with nothing reported", absent)
			t.Log(out)
		}
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
