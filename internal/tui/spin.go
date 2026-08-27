package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/seongyooo/agentline/internal/state"
)

// The screen states what was counted and stops there.
//
// A refactor and a loop look identical from the outside — the same file
// rewritten six times is either one — so AgentLine says "six times" and lets
// the person reading it decide. Calling it stuck would be a judgement it has
// no way to make, and being wrong about it once is enough to make the whole
// panel ignorable.

// spinLines renders the evidence, or nil when there is none.
func (m Model) spinLines(width int) []string {
	spin := m.st.Agent.Spin
	if spin == nil {
		return nil
	}

	head := styleWarn.Bold(true).Render("SPINNING")
	if since := spinElapsed(spin, time.Now()); since != "" {
		gap := width - ansi.StringWidth(ansi.Strip(head)) - len(since)
		if gap > 0 {
			head += strings.Repeat(" ", gap) + styleDim.Render(since)
		}
	}

	lines := []string{head}
	for i, f := range spin.Findings {
		if i == spinFindingRows {
			break
		}
		lines = append(lines, fitLine(findingLine(f), width))
	}
	return append(lines, styleWarn.Render("x interrupt   esc dismiss   i redirect"))
}

// spinFindingRows bounds the evidence shown. Three counts make the case; a
// fourth is the same case again, and the panel has other things to say.
const spinFindingRows = 3

// findingLine states one count in words.
func findingLine(f state.Finding) string {
	switch f.Kind {
	case state.RepeatedEdit:
		return fmt.Sprintf("%s written %d times", shorten(f.Target), f.Count)
	case state.RepeatedCommand:
		return fmt.Sprintf("%s run %d times", f.Target, f.Count)
	case state.RepeatedFailure:
		return fmt.Sprintf("%d failures in the last few minutes", f.Count)
	case state.NoNewGround:
		return fmt.Sprintf("%d actions, nothing new touched", f.Count)
	}
	return ""
}

// spinElapsed is how long the repetition has been going, which is what turns a
// count into a rate: six edits in twenty seconds is work, six in six minutes
// is something else.
func spinElapsed(spin *state.Spin, now time.Time) string {
	if spin.Since.IsZero() {
		return ""
	}
	since := now.Sub(spin.Since)
	if since < time.Minute {
		return fmt.Sprintf("%ds", int(since.Seconds()))
	}
	return fmt.Sprintf("%dm %02ds", int(since.Minutes()), int(since.Seconds())%60)
}

// spinAlert is the line that has to work with no screen: seen on a desktop
// notification while looking at something else.
func spinAlert(spin *state.Spin) string {
	if len(spin.Findings) == 0 {
		return "AgentLine: the agent is repeating itself"
	}
	return "AgentLine: " + findingLine(spin.Findings[0])
}

// spinning routes a key to the evidence on screen, and reports whether it took
// it. Only two keys are claimed, and neither of them is one that already meant
// something else.
func (m Model) spinning(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.st.Agent.Spin == nil {
		return nil, false
	}

	switch msg.String() {
	case "x":
		// Stopping is not deciding anything: the agent reports what it was
		// doing and waits, which is the point — a person can then say what
		// to do instead.
		return m.interrupt(), true
	case "esc":
		m.st.DismissSpin()
		return nil, true
	}
	return nil, false
}

// Stopper stops what a session is currently doing without ending it.
type Stopper interface {
	Interrupt() error
}

// interrupt stops the agent off the UI goroutine.
func (m Model) interrupt() tea.Cmd {
	stopper, ok := m.sender.(Stopper)
	if !ok {
		return nil
	}
	return func() tea.Msg {
		return promptSentMsg{err: stopper.Interrupt()}
	}
}
