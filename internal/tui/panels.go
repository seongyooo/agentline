package tui

import (
	"fmt"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/seonl/agentview/internal/events"
	"github.com/seonl/agentview/internal/project"
	"github.com/seonl/agentview/internal/state"
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
	now := m.nowLines()

	sections := []section{
		{rank: 3, lines: m.missionLines(l.MissionWidth())},
		{rank: 2, lines: now},
	}
	// The answer, in a box that scrolls. It sits beside MISSION rather than
	// replacing it: progress is about the goal, the reply is about the turn.
	if lines := m.replyLines(); lines != nil {
		sections = append(sections, section{rank: 6, lines: m.replyPanel(l, lines)})
	}
	if l.ShowNext {
		sections = append(sections, section{rank: 7, lines: []string{styleLabel.Render("NEXT"), valueOrDash(m.st.Agent.Next)}})
	}

	// Session facts are pinned to the bottom of the column rather than
	// flowing with the content above them. They are status, like the branch
	// on the bottom bar, and should not compete with what the agent is doing
	// for a place in the panel.
	status := m.sessionLines()
	lines := fitSections(sections, l.BodyHeight-len(status))

	for len(lines) < l.BodyHeight-len(status) {
		lines = append(lines, "")
	}
	return append(lines, status...)
}

// missionLines renders the goal with its progress bar beside it, since the
// count measures progress towards that goal and nothing else.
func (m Model) missionLines(width int) []string {
	lines := []string{styleLabel.Render("MISSION"), valueOrDash(m.st.Agent.Mission)}

	if p := m.st.Agent.Progress; p.Known() {
		lines = append(lines, progressBar(p, width))
	}
	return lines
}

// replyLines is the agent's answer, wrapped to the panel. It returns nil when
// there is nothing to show, which also keeps the panel out of the tab order.
func (m Model) replyLines() []string {
	reply := m.st.Agent.Reply
	if reply == "" {
		return nil
	}
	return wrap(reply, m.replyWidth())
}

// replyWidth is the column the reply is wrapped to.
func (m Model) replyWidth() int {
	if m.width < minWidth || m.height < minHeight {
		return 40
	}
	return max(computeLayout(m.width, m.height).MissionWidth(), 20)
}

// replyPanel renders the visible window of the reply, with its position when
// there is more than fits.
func (m Model) replyPanel(l Layout, lines []string) []string {
	rows := m.replyRows(l)
	start := clamp(m.replyScroll, 0, max(len(lines)-rows, 0))
	end := min(start+rows, len(lines))

	out := []string{m.panelHeading("REPLY", focusReply, len(lines), rows, start, l.MissionWidth())}
	return append(out, lines[start:end]...)
}

// replyRows is how many lines of the reply are shown at once. It is a small
// window on purpose: AgentView reports that a turn landed and what it said,
// and a transcript is what the plan says it must not become (§18).
func (m Model) replyRows(l Layout) int {
	return clamp(l.BodyHeight/3, 2, 6)
}

func (m Model) replyMaxScroll(l Layout) int {
	return max(len(m.replyLines())-m.replyRows(l), 0)
}

// replyRegion is the body rows the reply occupies, for hit testing.
func (m Model) replyRegion(l Layout) region {
	lines := m.missionLines(l.MissionWidth())
	nowLines := m.nowLines()

	start := len(lines) + 1 + len(nowLines) + 1
	return region{start: start, end: start + m.replyRows(l) + 1}
}

// panelHeading labels a scrollable panel, marking focus and showing the
// position when there is more than fits.
func (m Model) panelHeading(label string, area focusArea, total, rows, start, width int) string {
	style := styleLabel
	if m.focus == area {
		style = styleFocus
		label += " ◂"
	}
	head := style.Render(label)

	if total <= rows {
		return head
	}
	position := styleDim.Render(fmt.Sprintf("%d-%d/%d", start+1, min(start+rows, total), total))

	gap := width - ansi.StringWidth(head) - ansi.StringWidth(position)
	if gap < 1 {
		return head
	}
	return head + strings.Repeat(" ", gap) + position
}

// wrap breaks text to a column without splitting words where it can be helped.
func wrap(text string, width int) []string {
	if width < 4 {
		width = 4
	}

	var out []string
	for _, paragraph := range strings.Split(text, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}

		line := ""
		for _, word := range words {
			switch {
			case line == "":
				line = word
			case ansi.StringWidth(line)+1+ansi.StringWidth(word) <= width:
				line += " " + word
			default:
				out = append(out, line)
				line = word
			}
		}
		out = append(out, line)
	}
	return out
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

	marker, markerStyle := m.rowMarker(row.Node, now)

	// The selected row is drawn in reverse video across the full column, so
	// it reads as selected without relying on color.
	if selected {
		line := label
		if marker != "" {
			line = fitLine(label, width-2) + " " + marker
		}
		return styleSelected.Render(fitLine(line, width))
	}

	// A file the agent has claimed but not yet written is dimmed, so it can be
	// seen arriving without being mistaken for one that exists.
	if m.st.Pending(row.Node.Path) {
		if marker == "" {
			return stylePending.Render(fitLine(label, width))
		}
		return stylePending.Render(fitLine(label, width-2)) + " " + markerStyle.Render(marker)
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

// rowMarker picks the single symbol shown at the right of a tree row.
//
// Agent activity outranks Git status, and shares one gutter with it rather
// than adding a second column. A file the agent just touched is almost
// certainly modified too, so showing both would spend width on a symbol that
// says nothing new — and what the agent is doing right now is the more
// urgent fact. Git fills the gutter only once activity has decayed away.
func (m Model) rowMarker(n *project.Node, now time.Time) (string, lipgloss.Style) {
	if marker, style := activityMarker(m.st.NodeLevel(n, now)); marker != "" {
		return marker, style
	}
	if n.Dir {
		return "", styleDim // directories are not tracked by Git
	}
	return gitMarker(m.st.Project.Git.Of(n.Path))
}

// activityPanel renders the recent activity log, oldest first.
func (m Model) activityPanel(l Layout) []string {
	all := m.st.Agent.Activity
	if len(all) == 0 {
		return []string{styleLabel.Render("ACTIVITY"), styleDim.Render("No activity yet")}
	}

	// The log follows the newest entry unless the user has scrolled back, so
	// live activity stays visible without fighting them for the viewport.
	rows := l.ActivityRows
	start := clamp(len(all)-rows-m.activityScroll, 0, max(len(all)-rows, 0))
	end := min(start+rows, len(all))

	lines := []string{m.panelHeading("ACTIVITY", focusActivity, len(all), rows, start, m.width)}
	for _, a := range all[start:end] {
		entry := fmt.Sprintf("%s  %-9s %s", a.At.Format("15:04"), actionVerb(a.Kind), activityDetail(a))
		lines = append(lines, styleDim.Render(strings.TrimRight(entry, " ")))
	}
	return lines
}

// activityMaxScroll is how far back the log can be scrolled.
func (m Model) activityMaxScroll(l Layout) int {
	return max(len(m.st.Agent.Activity)-l.ActivityRows, 0)
}

// activityDetail prefers the agent's own description of an entry, falling back
// to the target when it did not give one.
func activityDetail(a state.Action) string {
	if a.Summary != "" {
		return a.Summary
	}
	return displayTarget(a)
}

// sessionLines reports what the agent said about its own run: the model, and
// how much of a usage window is gone.
//
// It is a status line, not a dashboard. There are no token counts or costs
// here, and everything shown was reported by the agent rather than measured
// or estimated by AgentView.
func (m Model) sessionLines() []string {
	s := m.st.Agent.Session
	if s == nil {
		return nil
	}

	var parts []string
	if s.Model != "" {
		parts = append(parts, shortModel(s.Model))
	}
	if s.Limit != "" {
		usage := fmt.Sprintf("%s %d%%", limitLabel(s.Limit), int(s.Used*100+0.5))
		if s.Overage {
			usage += " over"
		}
		if !s.ResetsAt.IsZero() {
			usage += " · resets " + s.ResetsAt.Format("15:04")
		}
		parts = append(parts, usage)
	}
	if len(parts) == 0 {
		return nil
	}

	style := styleDim
	if s.Used >= 0.9 {
		style = styleWarn // close enough to the limit to be worth noticing
	}
	return []string{styleLabel.Render("SESSION"), style.Render(strings.Join(parts, "   "))}
}

// shortModel trims a model id to the part a person recognizes.
func shortModel(model string) string {
	model = strings.TrimPrefix(model, "claude-")
	if i := strings.Index(model, "-2"); i > 0 {
		model = model[:i] // drop the date suffix
	}
	return model
}

// limitLabel names a usage window in words.
func limitLabel(limit string) string {
	switch limit {
	case "five_hour":
		return "5h"
	case "seven_day", "weekly":
		return "week"
	}
	return strings.ReplaceAll(limit, "_", " ")
}

// nowLines renders the NOW panel's body.
//
// Before any event has arrived there is nothing observed to report, so it says
// so and names where events are expected from. That turns an unwired setup —
// hooks not installed, wrong address — into something the developer can see
// and fix, instead of a UI that looks like an idle agent.
func (m Model) nowLines() []string {
	if !m.observedActivity() {
		lines := []string{styleLabel.Render("NOW"), styleDim.Render("No agent activity yet")}
		if m.hint != "" {
			lines = append(lines, styleDim.Render(m.hint))
		}
		return lines
	}

	action := m.st.Agent.Now
	verb := actionVerb(action.Kind)
	// How long the current action has been running is the one progress signal
	// that is always observable, and it is what separates slow from stuck.
	if elapsed := m.elapsed(action); elapsed != "" {
		verb += styleDim.Render("   " + elapsed)
	}

	lines := []string{styleLabel.Render("NOW"), verb}

	// What the agent said it is doing, when it said anything. A raw command
	// shows what was typed but not what it is for, so the description leads
	// and the command becomes the detail underneath it.
	if summary := action.Summary; summary != "" {
		lines = append(lines, styleValue.Render(summary))
		if detail := displayTarget(action); detail != "" && detail != summary {
			lines = append(lines, styleDim.Render(detail))
		}
		return lines
	}

	if action.Target != "" {
		lines = append(lines, styleValue.Render(displayTarget(action)))
	}
	return lines
}

// elapsed renders how long the current action has been running, once that is
// long enough to be worth saying.
func (m Model) elapsed(action state.Action) string {
	if action.At.IsZero() || m.st.Agent.Status != events.StatusWorking {
		return ""
	}
	since := time.Since(action.At)
	if since < 3*time.Second {
		return ""
	}

	if since < time.Minute {
		return fmt.Sprintf("%ds", int(since.Seconds()))
	}
	return fmt.Sprintf("%dm %02ds", int(since.Minutes()), int(since.Seconds())%60)
}

// progressBar renders the agent's task count.
//
// The tally is spelled out beside the bar, so the panel is readable when the
// blocks do not render and so the number is never left to be eyeballed off a
// bar. It counts tasks, not effort: three of seven tasks done says nothing
// about how much work is left, and the label stays a plain count for that
// reason.
func progressBar(p state.Progress, width int) string {
	count := fmt.Sprintf("%d/%d", p.Done, p.Total)

	bars := clamp(width-len(count)-1, 4, 24)
	filled := int(p.Fraction() * float64(bars))

	bar := strings.Repeat("█", filled) + strings.Repeat("░", bars-filled)
	return styleWorking.Render(bar) + " " + count
}

// displayTarget renders an action's target, abbreviating it only when it is
// actually a path.
func displayTarget(a state.Action) string {
	if targetsAPath(a.Kind) {
		return shorten(a.Target)
	}
	return a.Target
}

// inputBar renders the prompt affordance. Not yet wired to an agent; it is
// shown dimmed so it does not read as an active input.
func (m Model) inputBar(width int) string {
	hint := styleDim.Render(m.inputHint())
	if branch := m.branchLabel(); branch != "" {
		hint = styleDim.Render(branch+"   ") + hint
	}

	prompt := m.promptField(width - ansi.StringWidth(hint) - 2)

	gap := width - ansi.StringWidth(prompt) - ansi.StringWidth(hint)
	if gap < 1 {
		return fitLine(prompt, width)
	}
	return prompt + strings.Repeat(" ", gap) + hint
}

// promptField renders the prompt itself: editable when AgentView owns the
// session, dimmed and inert when it is only watching one.
func (m Model) promptField(width int) string {
	if !m.canSend() {
		return fitLine(styleDim.Render("> Ask Claude Code..."), width)
	}
	if m.sendErr != nil {
		return fitLine(styleError.Render("> "+firstLine(m.sendErr.Error())), width)
	}

	// The field is sized in characters, but Korean and other CJK text takes
	// two cells per character, so a field that fits by count can still
	// overflow its box. It is given a conservative character budget and then
	// clamped by measured width, which holds whatever the text contains.
	field := m.input
	field.Width = max(width/2, 8)

	return fitLine(m.scrolledPrompt(field, width), width)
}

// scrolledPrompt keeps the caret in view when the text is wider than the box.
//
// Without this a long line would be cut at the box's edge and the user would
// be typing somewhere they cannot see.
func (m Model) scrolledPrompt(field textinput.Model, width int) string {
	rendered := field.View()
	if ansi.StringWidth(rendered) <= width {
		return rendered
	}

	// Show the tail, which is where the caret is while composing.
	value := field.Value()
	visible := max(width-4, 4)
	for ansi.StringWidth(value) > visible && len(value) > 0 {
		_, size := utf8.DecodeRuneInString(value)
		value = value[size:]
	}
	return field.Prompt + "…" + value
}

// inputHint says what the keys do, which changes with focus.
func (m Model) inputHint() string {
	switch {
	case !m.canSend():
		return "q quit"
	case m.inputFocused():
		return "enter send   esc cancel"
	default:
		return "tab focus   i prompt   q quit"
	}
}

// firstLine trims a message to its first line for a one-line bar.
func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// branchLabel names the current branch, marked with an asterisk when the
// working tree has changes. It is context for the rest of the screen, not a
// Git panel: no counts, no ahead/behind, nothing to act on.
func (m Model) branchLabel() string {
	status := m.st.Project.Git

	branch := status.Branch
	if status.Detached {
		branch = "detached"
	}
	if branch == "" {
		return ""
	}
	if status.Dirty() {
		branch += "*"
	}
	return branch
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
