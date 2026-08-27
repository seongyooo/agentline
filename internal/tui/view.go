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

	lines := []string{m.header(m.width), rule(m.width)}
	lines = append(lines, m.body(l, now)...)
	if l.ActivityRows > 0 {
		lines = append(lines, rule(m.width))
		lines = append(lines, fit(m.activityPanel(l), m.width, l.ActivityHeight()-1)...)
	}
	lines = append(lines, rule(m.width), m.inputBar(m.width))

	return strings.Join(lines, "\n")
}

// body renders the mission panel, beside the project tree when there is room.
func (m Model) body(l Layout, now time.Time) []string {
	// Looking at a tree entry replaces the mission column: the question being
	// asked is about that entry, not about the agent's goal.
	panel := m.missionPanel(l)
	if m.inspecting != nil {
		panel = m.inspectPanel(l, m.inspecting, now)
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
