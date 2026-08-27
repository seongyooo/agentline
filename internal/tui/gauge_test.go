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

// A bar is read as a proportion, so the two ends have to be honest: a window
// barely touched must not look untouched, and one not quite gone must not look
// gone. Rounding alone gets both wrong.
func TestGaugeEndsAreHonest(t *testing.T) {
	tests := []struct {
		name  string
		share float64
		full  bool
		empty bool
	}{
		{"barely used", 0.01, false, false},
		{"nearly gone", 0.99, false, false},
		{"untouched", 0, false, true},
		{"gone", 1, true, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bar := ansi.Strip(gaugeBar(tc.share, 20))

			if got := !strings.ContainsRune(bar, '░'); got != tc.full {
				t.Errorf("%q: reads as full = %v, want %v", bar, got, tc.full)
			}
			if got := !strings.ContainsRune(bar, '█'); got != tc.empty {
				t.Errorf("%q: reads as empty = %v, want %v", bar, got, tc.empty)
			}
		})
	}
}

// gaugeSession is a session with everything the bars can show.
func gaugeSession() *events.Session {
	return &events.Session{
		Model:         "claude-opus-5-20260514",
		ContextWindow: 200_000, ContextPercent: 0.31,
		Limits: map[string]events.Limit{
			"five_hour": {Used: 0.62, ResetsAt: time.Now().Add(2 * time.Hour)},
			"seven_day": {Used: 0.28, ResetsAt: time.Now().Add(90 * time.Hour)},
		},
	}
}

// The bars cost rows over the words they replace, and the words carry the
// same numbers. So the bars are allowed exactly when those extra rows are
// going spare — a rule worth pinning, because getting it backwards once meant
// the words always won and the bars never appeared at all.
func TestBarsAreSpentOnlyOnSpareRows(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	st.Apply(events.Event{Type: events.SessionInfo, Timestamp: time.Now(), Source: "claude-code", Session: gaugeSession()})

	model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := model.(Model)
	l := computeLayout(100, 30)

	tight := ansi.Strip(strings.Join(m.sessionLines(l, 0), "|"))
	if strings.ContainsRune(tight, 0x2588) {
		t.Errorf("bars drawn with no rows to spare: %q", tight)
	}
	if !strings.Contains(tight, "62%") {
		t.Errorf("the numbers went missing when the bars did: %q", tight)
	}

	roomy := ansi.Strip(strings.Join(m.sessionLines(l, 8), "|"))
	if !strings.ContainsRune(roomy, 0x2588) {
		t.Errorf("no bars drawn with rows going spare: %q", roomy)
	}
}

// End to end, on a terminal with room for both: the answer and the bars.
func TestATallTerminalShowsTheReplyAndTheBars(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()

	now := time.Now()
	st.Apply(events.Event{Type: events.AgentReply, Timestamp: now, Source: "claude-code",
		Message: strings.Repeat("the answer that has to survive being rendered. ", 8)})
	st.Apply(events.Event{Type: events.SessionInfo, Timestamp: now, Source: "claude-code", Session: gaugeSession()})

	model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: 100, Height: 44})
	out := ansi.Strip(model.(Model).View())

	for _, want := range []string{"the answer", "62%", string(rune(0x2588))} {
		if !strings.Contains(out, want) {
			t.Errorf("frame missing %q", want)
			t.Log(out)
		}
	}
}

// Nothing is drawn for a window the agent never mentioned. A bar at zero is a
// claim about a limit AgentLine has no knowledge of.
func TestNoBarForAnUnreportedWindow(t *testing.T) {
	s := &events.Session{Model: "claude-opus-5-20260514", ContextWindow: 200_000, ContextPercent: 0.4}

	rows := gaugeRows(s)
	if len(rows) != 1 || rows[0].label != "Context" {
		t.Errorf("rows = %+v, want only the context that was reported", rows)
	}
}

// Shortest window first: it is the one that runs out soonest, so it is the one
// read first.
func TestGaugesAreOrderedByHowSoonTheyRunOut(t *testing.T) {
	rows := gaugeRows(gaugeSession())

	var labels []string
	for _, r := range rows {
		labels = append(labels, r.label)
	}
	if want := []string{"Context", "5h", "7d"}; strings.Join(labels, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", labels, want)
	}
}

// The answer to a question must survive an ordinary terminal.
//
// It did not: ranked below MISSION, the reply was the second section dropped
// when the column was tight, so on anything shorter than about thirty rows a
// question got a restatement of itself and nothing else. Every size here was
// broken before, and the earlier version of this file dodged it by only ever
// checking a terminal tall enough to fit both.
func TestTheAnswerSurvivesAnOrdinaryTerminal(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()

	now := time.Now()
	st.Apply(events.Event{Type: events.UserPrompt, Message: "what does the valve do?", Timestamp: now, Source: "claude-code"})
	st.Apply(events.Event{Type: events.TaskProgress, Done: 4, Total: 4, Timestamp: now, Source: "claude-code"})
	st.Apply(events.Event{Type: events.AgentReply, Timestamp: now, Source: "claude-code",
		Message: "The valve controls the drainage flow between the rooms."})
	st.Apply(events.Event{Type: events.SessionInfo, Timestamp: now, Source: "claude-code", Session: gaugeSession()})
	st.Apply(events.Event{Type: events.AgentStatus, Status: events.StatusWaiting, Timestamp: now, Source: "claude-code"})

	for _, size := range []struct{ w, h int }{{80, 24}, {100, 24}, {100, 28}, {100, 30}, {120, 26}, {160, 40}} {
		model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: size.w, Height: size.h})
		out := ansi.Strip(model.(Model).View())

		if !strings.Contains(out, "drainage flow") {
			t.Errorf("%dx%d: the answer is not on screen", size.w, size.h)
			t.Log(out)
		}
	}
}

// The model names the panel when the panel has a name to put it in, and must
// not also be repeated in the status block underneath.
func TestTheModelIsNamedOnce(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	st.Apply(events.Event{Type: events.SessionInfo, Timestamp: time.Now(), Source: "claude-code", Session: gaugeSession()})

	for _, h := range []int{26, 28, 30, 40} {
		model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: 100, Height: h})
		out := ansi.Strip(model.(Model).View())

		if got := strings.Count(out, "Opus 5"); got != 1 {
			t.Errorf("100x%d: model named %d times, want once", h, got)
			t.Log(out)
		}
	}
}
