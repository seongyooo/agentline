package tui

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/seonl/agentview/internal/project"
)

// treeChrome is the rows the project panel spends on its label and spacer.
const treeChrome = 2

// header renders the product name and the agent's status on one row.
func (m Model) header(width int) string {
	symbol, text, style := statusLabel(m.st.Agent.Status)
	left := styleTitle.Render("AGENTVIEW")
	right := style.Render(fmt.Sprintf("%s %s  %s", symbol, strings.ToUpper(m.st.Agent.Agent), text))

	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return fitLine(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
}

// section is one labelled block of the mission panel, ranked by the priority
// order in IMPLEMENTATION.md §20. Lower rank survives longer.
type section struct {
	rank  int
	lines []string
}

// missionPanel renders MISSION, NOW, and optionally NEXT, dropping whole
// sections by rank rather than letting a tight panel clip NOW's target.
func (m Model) missionPanel(l Layout) []string {
	action := m.st.Agent.Now
	now := []string{styleLabel.Render("NOW"), actionVerb(action.Kind)}
	if action.Target != "" {
		now = append(now, styleValue.Render(shorten(action.Target)))
	}

	sections := []section{
		{rank: 3, lines: []string{styleLabel.Render("MISSION"), valueOrDash(m.st.Agent.Mission)}},
		{rank: 2, lines: now},
	}
	if l.ShowNext {
		sections = append(sections, section{rank: 7, lines: []string{styleLabel.Render("NEXT"), valueOrDash(m.st.Agent.Next)}})
	}
	return fitSections(sections, l.BodyHeight)
}

// fitSections drops the lowest-ranked sections until the rest fit, then joins
// the survivors with blank separators, preserving their original order.
func fitSections(sections []section, height int) []string {
	keep := make([]bool, len(sections))
	for i := range keep {
		keep[i] = true
	}

	for used := sectionsHeight(sections, keep); used > height; used = sectionsHeight(sections, keep) {
		worst := -1
		for i, s := range sections {
			if keep[i] && (worst == -1 || s.rank > sections[worst].rank) {
				worst = i
			}
		}
		if worst == -1 {
			break // nothing left to drop; fit() will clip the remainder
		}
		keep[worst] = false
	}

	var lines []string
	for i, s := range sections {
		if !keep[i] {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, s.lines...)
	}
	return lines
}

// sectionsHeight is the rows the kept sections need, blank separators included.
func sectionsHeight(sections []section, keep []bool) int {
	total, shown := 0, 0
	for i, s := range sections {
		if !keep[i] {
			continue
		}
		if shown > 0 {
			total++ // separator
		}
		total += len(s.lines)
		shown++
	}
	return total
}

// treePanel renders the project structure with activity markers.
func (m Model) treePanel(l Layout, now time.Time) []string {
	if m.st.Project.Tree == nil {
		return []string{styleLabel.Render("PROJECT"), "", styleDim.Render("No project loaded")}
	}

	rows := m.rows()
	visible := l.BodyHeight - treeChrome
	start, end, _ := m.tree.window(len(rows), visible)

	lines := []string{m.treeHeading(l.TreeWidth, len(rows), visible), ""}
	for i := start; i < end; i++ {
		lines = append(lines, m.treeRow(rows[i], l.TreeWidth, now, i == m.tree.cursor))
	}
	return lines
}

// treeHeading labels the panel and, when the tree scrolls, shows the cursor's
// position so hidden rows are never a surprise.
func (m Model) treeHeading(width, total, visible int) string {
	label := styleLabel.Render("PROJECT")
	if total <= visible || total == 0 {
		return label
	}

	position := styleDim.Render(fmt.Sprintf("%d/%d", m.tree.cursor+1, total))
	gap := width - ansi.StringWidth(label) - ansi.StringWidth(position)
	if gap < 1 {
		return label
	}
	return label + strings.Repeat(" ", gap) + position
}

// treeRow renders one tree entry with its activity marker at the right edge.
func (m Model) treeRow(row project.Row, width int, now time.Time, selected bool) string {
	name := row.Node.Name
	if row.Node.Dir {
		name += "/"
	}
	label := row.Prefix + name
	if row.Node.Err != nil {
		label += " (unreadable)"
	}

	marker, markerStyle := activityMarker(m.st.NodeLevel(row.Node, now))

	// The selected row is drawn in reverse video across the full column, so
	// it reads as selected without relying on color.
	if selected {
		line := label
		if marker != "" {
			line = fitLine(label, width-2) + " " + marker
		}
		return styleSelected.Render(fitLine(line, width))
	}

	switch {
	case row.Node.Placeholder:
		return styleDim.Render(fitLine(label, width))
	case marker == "":
		return fitLine(label, width)
	default:
		return fitLine(label, width-2) + " " + markerStyle.Render(marker)
	}
}

// activityPanel renders the recent activity log, oldest first.
func (m Model) activityPanel(l Layout) []string {
	lines := []string{styleLabel.Render("ACTIVITY")}

	recent := m.st.Recent(l.ActivityRows)
	if len(recent) == 0 {
		return append(lines, styleDim.Render("No activity yet"))
	}
	for _, a := range recent {
		entry := fmt.Sprintf("%s  %-9s %s", a.At.Format("15:04"), actionVerb(a.Kind), shorten(a.Target))
		lines = append(lines, styleDim.Render(strings.TrimRight(entry, " ")))
	}
	return lines
}

// inputBar renders the prompt affordance. Not yet wired to an agent; it is
// shown dimmed so it does not read as an active input.
func (m Model) inputBar(width int) string {
	prompt := styleDim.Render("> Ask Claude Code...")
	hint := styleDim.Render("q quit")

	gap := width - ansi.StringWidth(prompt) - ansi.StringWidth(hint)
	if gap < 1 {
		return fitLine(prompt, width)
	}
	return prompt + strings.Repeat(" ", gap) + hint
}

// rule renders a full-width horizontal separator.
func rule(width int) string {
	return styleRule.Render(strings.Repeat("─", max(width, 0)))
}

// valueOrDash renders a value, or an em dash when it is unknown. AgentView
// never invents a value it has not observed.
func valueOrDash(v string) string {
	if v == "" {
		return styleDim.Render("—")
	}
	return v
}

// shorten trims a path to its last two segments so long paths stay readable.
func shorten(p string) string {
	if p == "" {
		return ""
	}
	p = path.Clean(strings.ReplaceAll(p, "\\", "/"))
	parts := strings.Split(p, "/")
	if len(parts) <= 2 {
		return p
	}
	return ".../" + strings.Join(parts[len(parts)-2:], "/")
}
