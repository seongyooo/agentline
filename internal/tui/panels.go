package tui

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/seonl/agentview/internal/events"
	"github.com/seonl/agentview/internal/project"
	"github.com/seonl/agentview/internal/state"
)

// treeChrome is the rows the project panel spends on its label and spacer.
const treeChrome = 2

// heavyContext is where a session's accumulated context is worth pointing out.
// A session AgentView owns is never compacted, so past this the same history
// is re-sent on every turn and the cost per turn stops falling.
const heavyContext = 120_000

// header renders the product name and the agent's status on one row.
func (m Model) header(width int) string {
	symbol, text, style := statusLabel(m.st.Agent.Status)

	left := styleTitle.Render("AGENTVIEW")
	// How tool calls are being approved changes what the agent will do
	// without asking, so it belongs beside the status rather than buried.
	if mode := m.st.PermissionMode(); mode != "" {
		label, style := modeLabel(mode)
		left += "   " + style.Render(label)
	}
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
	parts = append(parts, limitParts(s)...)

	// The context this session carries. A turn re-sends everything before it,
	// so this number climbing is what makes a long session expensive, and it
	// is the one thing a usage readout here is actually for.
	var detail []string
	if s.InputTokens > 0 {
		context := fmt.Sprintf("ctx %s", compactCount(s.InputTokens))
		if s.InputTokens >= heavyContext {
			// A session AgentView owns is never compacted, so a context this
			// large will keep costing this much on every further turn.
			context += " — restart with ctrl+n to clear"
		}
		detail = append(detail, context)
	}
	if s.Turns > 0 {
		detail = append(detail, fmt.Sprintf("%d turns", s.Turns))
	}
	if s.CostUSD > 0 {
		detail = append(detail, fmt.Sprintf("$%.2f", s.CostUSD))
	}

	if len(parts) == 0 && len(detail) == 0 {
		return nil
	}

	style := styleDim
	if _, peak, ok := s.Peak(); ok && peak.Used >= 0.9 {
		// The window closest to running out is the one worth noticing.
		style = styleWarn
	}

	lines := []string{styleLabel.Render("SESSION")}
	if len(parts) > 0 {
		lines = append(lines, style.Render(strings.Join(parts, "   ")))
	}
	if len(detail) > 0 {
		lines = append(lines, styleDim.Render(strings.Join(detail, "   ")))
	}
	return lines
}

// compactCount renders a token count in the units people read them in.
func compactCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.0fk", float64(n)/1_000)
	}
	return fmt.Sprint(n)
}

// shortModel trims a model id to the part a person recognizes.
func shortModel(model string) string {
	model = strings.TrimPrefix(model, "claude-")
	if i := strings.Index(model, "-2"); i > 0 {
		model = model[:i] // drop the date suffix
	}
	return model
}

// limitOrder is the order usage windows are shown in, shortest first, so the
// one that runs out soonest is read first.
var limitOrder = []string{"five_hour", "seven_day", "weekly", "monthly"}

// limitParts renders each usage window the agent reported.
func limitParts(s *events.Session) []string {
	seen := map[string]bool{}
	names := make([]string, 0, len(s.Limits))

	for _, name := range limitOrder {
		if _, ok := s.Limits[name]; ok {
			names = append(names, name)
			seen[name] = true
		}
	}
	// Anything reported that this list does not know about is still shown,
	// rather than dropped for not being recognised.
	for name := range s.Limits {
		if !seen[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names[len(seen):])

	parts := make([]string, 0, len(names))
	for _, name := range names {
		limit := s.Limits[name]

		text := fmt.Sprintf("%s %d%%", limitLabel(name), int(limit.Used*100+0.5))
		if limit.Overage {
			text += " over"
		}
		if !limit.ResetsAt.IsZero() {
			text += " ↻" + resetLabel(limit.ResetsAt)
		}
		parts = append(parts, text)
	}
	return parts
}

// limitLabel names a usage window in words.
func limitLabel(limit string) string {
	switch limit {
	case "five_hour":
		return "5h"
	case "seven_day", "weekly":
		return "week"
	case "monthly":
		return "month"
	}
	return strings.ReplaceAll(limit, "_", " ")
}

// resetLabel says when a window rolls over: a clock time if that is today,
// and a date once it is far enough out that a clock time would mislead.
func resetLabel(at time.Time) string {
	if until := time.Until(at); until < 24*time.Hour {
		return at.Format("15:04")
	}
	return at.Format("Jan 2")
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

	// The field is rendered here rather than by the widget, which pads to a
	// character count. Korean and other CJK text takes two cells per
	// character, so a widget-padded field runs two cells past its box for
	// every Hangul syllable and the line wraps around the screen.
	field := m.input.Prompt + m.promptText(width)
	m.placeCaret(field)

	return fitLine(field, width)
}

// placeCaret puts the terminal cursor after the text being typed.
//
// An input method draws the syllable it is composing at the terminal's cursor,
// so this is what keeps Korean appearing where the user is typing instead of
// at the corner of the screen.
func (m Model) placeCaret(field string) {
	if m.caret == nil {
		return
	}
	if !m.inputFocused() {
		m.caret.Hide()
		return
	}
	// The bar is the last row, and columns are one-based.
	m.caret.Set(ansi.StringWidth(field)+1, m.height)
}

// promptText is the value with the caret end kept in view.
//
// Everything is measured in terminal cells, so a line of Korean occupies what
// it actually occupies. When the text is wider than the box the front is
// dropped, since the end is where the caret is while composing.
func (m Model) promptText(width int) string {
	value := m.input.Value()
	if value == "" && !m.inputFocused() {
		return styleDim.Render(m.input.Placeholder)
	}

	visible := max(width-ansi.StringWidth(m.input.Prompt)-1, 4)
	if ansi.StringWidth(value) <= visible {
		return value
	}

	// Drop leading characters until the tail fits, leaving room for the mark
	// that says text was dropped.
	for ansi.StringWidth(value) > visible-1 && value != "" {
		_, size := utf8.DecodeRuneInString(value)
		value = value[size:]
	}
	return "…" + value
}

// modeLabel names a permission mode and colours it by how much the agent can
// do without asking.
//
// The colour is the point: the mode decides what happens to the user's files
// without a prompt, so how far it has been opened up should be readable at a
// glance rather than needing the words to be parsed.
func modeLabel(mode string) (string, lipgloss.Style) {
	switch mode {
	case "plan":
		// Nothing is changed at all, which is the most restricted of them.
		return "planning", styleWorking
	case "default", "manual":
		return "asks first", styleOK
	case "acceptEdits":
		// Files change without asking, but commands still stop for approval.
		return "edits allowed", styleWarn
	case "auto", "dontAsk":
		return "auto", styleError
	case "bypassPermissions":
		return "no checks", styleError
	}
	return mode, styleDim
}

// inputHint says what the keys do, which changes with focus.
func (m Model) inputHint() string {
	switch {
	case m.picker.open:
		return "← → choose   enter apply   esc cancel"
	case m.inputFocused() && len(m.slashMatches()) > 0:
		return "tab complete   enter send"
	case m.inputFocused():
		return "enter send   shift+tab mode   esc cancel"
	case m.inspecting != nil:
		return "esc back   q quit"
	case m.focus == focusTree:
		if m.canSend() {
			return "enter inspect   i prompt   q quit"
		}
		return "enter inspect   tab focus   q quit"
	case !m.canSend():
		return "tab focus   q quit"
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
