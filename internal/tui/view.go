package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// View composes the panels into exactly Width x Height cells so the layout
// never scrolls or wraps unexpectedly.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "" // no size reported yet
	}
	if m.width < minWidth || m.height < minHeight {
		return fmt.Sprintf("Terminal too small (need %d×%d)\n", minWidth, minHeight)
	}

	l := computeLayout(m.width, m.height)
	now := time.Now()

	lines := []string{m.header(m.width)}
	// Framed panels draw their own top edge, so the rule that separated the
	// header from the body becomes a second line beside it. A blank row does
	// the same job and lets the frames breathe.
	if l.Boxed {
		lines = append(lines, strings.Repeat(" ", m.width))
	} else {
		lines = append(lines, m.rule(l, '┬'))
	}

	lines = append(lines, m.body(l, now)...)

	if l.ActivityRows > 0 {
		if l.Boxed {
			lines = append(lines, m.activityBox(l)...)
		} else {
			lines = append(lines, m.rule(l, '┴'))
			lines = append(lines, fit(m.activityPanel(l), m.width, l.ActivityHeight()-1)...)
		}
	}
	// The session's shape sits under the log it is drawn from, above the
	// input bar, where it reads as a summary of everything above it.
	if l.ShowPulse {
		lines = append(lines, fitLine(m.pulseLine(m.width), m.width))
	}

	// An open choice and the commands still matching both take the rule's
	// row rather than one of their own, so neither costs the layout anything.
	switch {
	case m.picker.open:
		lines = append(lines, fitLine(m.pickerLine(m.width), m.width))
	case m.slashHint(m.width) != "":
		lines = append(lines, fitLine(styleDim.Render(m.slashHint(m.width)), m.width))
	default:
		if l.Boxed {
			lines = append(lines, strings.Repeat(" ", m.width))
			break
		}
		// With no activity log the body runs to the bottom, so this is the
		// rule that closes the tree's divider.
		if l.ActivityRows == 0 {
			lines = append(lines, m.rule(l, '┴'))
			break
		}
		lines = append(lines, rule(m.width))
	}
	lines = append(lines, m.inputBar(m.width))

	return strings.Join(lines, "\n")
}

// rule renders a separator across the frame, joined to the tree's divider
// where it crosses it. The junction costs nothing and is the difference
// between a drawn frame and three unrelated lines lying across the screen.
func (m Model) rule(l Layout, junction rune) string {
	at := l.TreeWidth + 1
	if !l.ShowTree || at >= m.width {
		return rule(m.width)
	}

	line := []rune(strings.Repeat("─", m.width))
	line[at] = junction
	return styleRule.Render(string(line))
}

// body renders the mission panel, beside the project tree when there is room.
func (m Model) body(l Layout, now time.Time) []string {
	// Looking at a tree entry replaces the mission column: the question being
	// asked is about that entry, not about the agent's goal.
	panel := m.missionPanel(l)
	if m.inspecting != nil {
		panel = m.inspectPanel(l, m.inspecting, now)
	}
	if l.Boxed {
		return m.boxedBody(l, panel, now)
	}

	mission := fit(panel, l.MissionWidth(), l.BodyHeight)
	if !l.ShowTree {
		return mission
	}

	tree := fit(m.treePanel(l, now), l.TreeWidth, l.BodyHeight)
	divider := styleRule.Render("│")

	joined := make([]string, l.BodyHeight)
	for i := range joined {
		joined[i] = tree[i] + " " + divider + " " + mission[i]
	}
	return joined
}

// boxedBody puts the two columns in frames side by side.
//
// The frame carries each panel's name, which is why it costs no rows: the two
// edges replace the heading and the blank line that used to sit under it.
func (m Model) boxedBody(l Layout, panel []string, now time.Time) []string {
	mission := box(m.missionTitle(), panel, l.MissionWidth(), l.BodyHeight, false)
	if !l.ShowTree {
		return mission
	}

	tree := box("PROJECT", m.treePanel(l, now), l.TreeWidth, l.BodyHeight, m.focus == focusTree)
	tree[len(tree)-1] = boxNote(tree[len(tree)-1], m.treeNote(l), m.focus == focusTree)

	joined := make([]string, l.BodyHeight)
	for i := range joined {
		joined[i] = tree[i] + strings.Repeat(" ", boxedGap) + mission[i]
	}
	return joined
}

// missionTitle names the column by what is in it, which changes: looking at a
// tree entry replaces the agent's goal with a description of that entry.
func (m Model) missionTitle() string {
	if m.inspecting == nil {
		return "AGENT"
	}
	if m.inspecting.Dir {
		return "DIRECTORY"
	}
	return "FILE"
}

// treeNote is the cursor's place in the whole tree, shown on the frame's
// bottom edge when there is more of it than fits.
func (m Model) treeNote(l Layout) string {
	total := len(m.rows())
	if total <= l.BodyHeight-l.PanelChrome() {
		return ""
	}
	return fmt.Sprintf("%d/%d", m.tree.cursor+1, total)
}

// activityBox frames the log, with its position on the bottom edge.
func (m Model) activityBox(l Layout) []string {
	lines := box("ACTIVITY", m.activityPanel(l), m.width, l.ActivityHeight(), m.focus == focusActivity)

	total, rows := len(m.st.Agent.Activity), l.ActivityRows
	if total > rows {
		start := clamp(total-rows-m.activityScroll, 0, max(total-rows, 0))
		note := fmt.Sprintf("%d-%d/%d", start+1, min(start+rows, total), total)
		lines[len(lines)-1] = boxNote(lines[len(lines)-1], note, m.focus == focusActivity)
	}
	return lines
}

// fit forces lines to exactly width columns and height rows, padding with
// blanks and truncating overflow.
func fit(lines []string, width, height int) []string {
	out := make([]string, height)
	for i := range out {
		if i < len(lines) {
			out[i] = fitLine(lines[i], width)
			continue
		}
		out[i] = strings.Repeat(" ", max(width, 0))
	}
	return out
}

// fitLine clips or pads a single line to exactly width visible columns,
// measuring around ANSI escape sequences.
func fitLine(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = ansi.Truncate(s, width, "…")
	if gap := width - ansi.StringWidth(s); gap > 0 {
		s += strings.Repeat(" ", gap)
	}
	return s
}
