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

	"github.com/seongyooo/agentline/internal/events"
	"github.com/seongyooo/agentline/internal/project"
	"github.com/seongyooo/agentline/internal/state"
)

// treeChrome is the rows the project panel spends on its label and spacer.
const treeChrome = 2

// heavyContext is where a session's accumulated context is worth pointing out.
// A session AgentLine owns is never compacted, so past this the same history
// is re-sent on every turn and the cost per turn stops falling.
const heavyContext = 120_000

// header renders the product name and the agent's status on one row.
func (m Model) header(width int) string {
	symbol, text, style := statusLabel(m.st.Agent.Status)
	// Derived, not reported: no adapter can see repetition, so this is not a
	// status an adapter is allowed to send. It sits on top of WORKING, which
	// is what the agent genuinely is.
	if m.st.Agent.Spin != nil {
		symbol, text, style = "◆", "SPINNING", styleWarn
	}

	left := styleTitle.Render("AGENTLINE")
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
		{rank: 4, lines: m.missionLines(l.MissionInner())},
	}
	// While a question stands, NEEDS YOU is what the agent is doing. Showing
	// NOW as well printed the same sentence twice, one line apart.
	if m.st.Agent.Ask == nil {
		sections = append(sections, section{rank: 2, lines: now})
	}
	// A blocked agent goes above everything and is never dropped to make
	// room: a panel that hid the question would leave the session stopped
	// with nothing on screen saying why.
	if lines := m.askLines(l.MissionInner()); lines != nil {
		sections = append([]section{{rank: 0, lines: lines}}, sections...)
	}
	// Below a question the agent is blocked on, above everything else. It is
	// not urgent the way a question is — nothing is waiting on it — but it is
	// the only thing on screen that says the work is going nowhere.
	if lines := m.spinLines(l.MissionInner()); lines != nil {
		sections = append([]section{{rank: 1, lines: lines}}, sections...)
	}
	// The answer, in a box that scrolls. It outranks MISSION deliberately.
	// MISSION restates a prompt the user typed themselves; the reply is the
	// thing they are waiting for, and on any terminal shorter than about
	// thirty rows there is only room for one of them. Ranked below MISSION,
	// as it was, the answer to a question simply never appeared.
	if lines := m.replyLines(); lines != nil {
		sections = append(sections, section{rank: 3, lines: m.replyPanel(l, lines)})
	}
	if l.ShowNext {
		sections = append(sections, section{rank: 7, lines: []string{styleLabel.Render("NEXT"), valueOrDash(m.st.Agent.Next)}})
	}

	// Session facts are pinned to the bottom of the column rather than
	// flowing with the content above them. They are status, like the branch
	// on the bottom bar, and should not compete with what the agent is doing
	// for a place in the panel.
	// What the status block may spend is whatever the panels above it do not
	// want. Deciding the other way round — pick a status rendering, then fit
	// the content into what is left — is how a taller status block silently
	// ate the reply.
	body := l.BodyHeight - l.FrameRows()
	status := m.sessionLines(l, body-wantedHeight(sections))
	budget := max(body-len(status), 0)

	// Clamped, not just padded. When the content cannot be cut down to the
	// budget by dropping sections, it overruns it, and appending the status
	// to an overrun leaves it stranded mid-panel — or trimmed off the bottom
	// entirely by the fit that follows.
	lines := fitSections(sections, budget)
	if len(lines) > budget {
		lines = lines[:budget]
	}
	for len(lines) < budget {
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
	return max(computeLayout(m.width, m.height).MissionInner(), 20)
}

// replyPanel renders the visible window of the reply, with its position when
// there is more than fits.
func (m Model) replyPanel(l Layout, lines []string) []string {
	rows := m.replyRows(l)
	start := clamp(m.replyScroll, 0, max(len(lines)-rows, 0))
	end := min(start+rows, len(lines))

	out := []string{m.panelHeading("REPLY", focusReply, len(lines), rows, start, l.MissionInner())}
	return append(out, lines[start:end]...)
}

// replyRows is how many lines of the reply are shown at once. It is a small
// window on purpose: AgentLine reports that a turn landed and what it said,
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
	position := ""
	if total > rows {
		position = fmt.Sprintf("%d-%d/%d", start+1, min(start+rows, total), total)
	}
	return m.heading(label, area, position, width)
}

// heading renders a panel's label with its focus marker and, when there is
// one, a position on the right.
func (m Model) heading(label string, area focusArea, position string, width int) string {
	style := styleLabel
	if m.focus == area {
		// Marked as well as styled, so which panel the keys reach is still
		// visible in a terminal that renders no colour.
		style = styleFocus
		label += " ◂"
	}
	head := style.Render(label)

	if position == "" {
		return head
	}
	gap := width - ansi.StringWidth(head) - len(position)
	if gap < 1 {
		return head
	}
	return head + strings.Repeat(" ", gap) + styleDim.Render(position)
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

// wantedHeight is the rows every section would take if none were dropped.
func wantedHeight(sections []section) int {
	keep := make([]bool, len(sections))
	for i := range keep {
		keep[i] = true
	}
	return sectionsHeight(sections, keep)
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
	visible := l.BodyHeight - l.PanelChrome()
	start, end, _ := m.tree.window(len(rows), visible)

	var lines []string
	if !l.Boxed {
		lines = []string{m.treeHeading(l.TreeWidth, len(rows), visible), ""}
	}
	for i := start; i < end; i++ {
		lines = append(lines, m.treeRow(rows[i], l.TreeInner(), now, i == m.tree.cursor))
	}
	return lines
}

// treeHeading labels the panel and, when the tree scrolls, shows the cursor's
// position so hidden rows are never a surprise.
// treeHeading labels the panel, marking focus the same way the other
// scrollable panels do and showing the cursor's position when the tree is
// longer than the window.
func (m Model) treeHeading(width, total, visible int) string {
	// The cursor's place in the whole tree, not the window's: the tree is
	// navigated, so where the selection sits is the useful number.
	position := ""
	if total > visible && total > 0 {
		position = fmt.Sprintf("%d/%d", m.tree.cursor+1, total)
	}
	return m.heading("PROJECT", focusTree, position, width)
}

// treeRow renders one tree entry with its activity marker at the right edge.
func (m Model) treeRow(row project.Row, width int, now time.Time, selected bool) string {
	label := treeLabel(row)
	marker, markerStyle := m.rowMarker(row.Node, now)

	// The selected row is drawn in reverse video across the full column, so
	// it reads as selected without relying on color. Only while the tree has
	// focus, though: a highlight that looks the same whether or not the
	// arrow keys reach the tree says nothing about where they will go.
	if selected {
		line := label
		if marker != "" {
			line = fitLine(label, width-2) + " " + marker
		}
		if m.focus != focusTree {
			return styleDim.Render(fitLine(line, width))
		}
		return styleSelected.Render(fitLine(line, width))
	}

	// A file the agent has claimed but not yet written is dimmed, so it can be
	// seen arriving without being mistaken for one that exists.
	body := treeBody(row)
	switch {
	case m.st.Pending(row.Node.Path):
		body = stylePending.Render(label)
	case row.Node.Placeholder:
		body = styleDim.Render(label)
	}

	if marker == "" {
		return fitLine(body, width)
	}
	return fitLine(body, width-2) + " " + markerStyle.Render(marker)
}

// treeLabel is a row as plain text: its box-drawing prefix, its disclosure
// mark, and the name.
func treeLabel(row project.Row) string {
	return row.Prefix + treeMark(row.Node) + treeName(row.Node)
}

// treeBody is the same text with the structure pushed behind the content: the
// box drawing recedes to chrome and the directory names carry the weight, so
// the shape of the tree is read without the lines competing with the names.
func treeBody(row project.Row) string {
	style := styleFile
	if row.Node.Dir {
		style = styleDir
	}
	return styleTree.Render(row.Prefix+treeMark(row.Node)) + style.Render(treeName(row.Node))
}

// treeName is what the entry is called, with the slash that says it is a
// directory and the note that says it could not be read.
func treeName(n *project.Node) string {
	name := n.Name
	if n.Dir {
		name += "/"
	}
	if n.Err != nil {
		name += " (unreadable)"
	}
	return name
}

// treeMark says whether a directory is open or closed, which the trailing
// slash does not. Files spend the same two columns on nothing, so the names
// stay aligned down the panel instead of stepping in and out.
func treeMark(n *project.Node) string {
	if !n.Dir || n.Placeholder {
		return "  "
	}
	if n.Expanded {
		return "▾ "
	}
	return "▸ "
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
		if l.Boxed {
			return []string{styleDim.Render("No activity yet")}
		}
		return []string{styleLabel.Render("ACTIVITY"), styleDim.Render("No activity yet")}
	}

	// The log follows the newest entry unless the user has scrolled back, so
	// live activity stays visible without fighting them for the viewport.
	rows := l.ActivityRows
	start := clamp(len(all)-rows-m.activityScroll, 0, max(len(all)-rows, 0))
	end := min(start+rows, len(all))

	var lines []string
	if !l.Boxed {
		lines = append(lines, m.panelHeading("ACTIVITY", focusActivity, len(all), rows, start, m.width))
	}
	for _, a := range all[start:end] {
		lines = append(lines, activityLine(a))
	}
	return lines
}

// activityLine renders one log entry in three weights: the time is the least
// of it, the verb is coloured by what it did, and the target is left plain
// because it is the part being scanned for.
func activityLine(a state.Action) string {
	line := styleDim.Render(a.At.Format("15:04")) + "  " +
		verbStyle(a.Kind).Render(fmt.Sprintf("%-9s", actionVerb(a.Kind)))

	if detail := activityDetail(a); detail != "" {
		return line + " " + detail
	}
	return strings.TrimRight(line, " ")
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
// or estimated by AgentLine.
// It reads the way Claude Code's own status line does, so the two say the
// same things in the same shape and neither has to be translated into the
// other while looking between them:
//
//	Opus 5 | Context: 73% used
//	5h: 85% (reset 8/27 19:40) | 7d: 39% (reset 8/31 13:00)
func (m Model) sessionLines(l Layout, room int) []string {
	width := l.MissionInner()
	s := m.st.Agent.Session
	if s == nil {
		return nil
	}

	var head []string
	// A framed panel already carries the model in its title. Repeating it a
	// few rows below was the bars-only rule applied in one place and not the
	// other.
	if s.Model != "" && !l.Boxed {
		head = append(head, modelName(s.Model))
	}
	if context := contextLabel(s); context != "" {
		head = append(head, context)
	}
	limits := limitParts(s)

	if len(head) == 0 && len(limits) == 0 {
		return nil
	}

	// The words are the fallback, and what the bars are measured against: the
	// status block is shown either way, so the only question is whether the
	// rows the bars cost *on top of* the words are rows the panel can spare.
	// Comparing their whole height instead made the words always win.
	var words []string
	if len(head) > 0 {
		words = append(words, contextStyle(s).Render(fitLine(strings.Join(head, " | "), width)))
	}
	if len(limits) > 0 {
		words = append(words, limitStyle(s).Render(fitLine(strings.Join(limits, " | "), width)))
	}

	lines := words
	if bars := m.gaugeLines(s, l); bars != nil && len(bars)-len(words) <= room {
		lines = bars
	}

	// A session AgentLine owns is never compacted, so a context this full
	// keeps costing what it costs on every further turn.
	if s.ContextPercent >= heavyContextShare {
		lines = append(lines, styleWarn.Render(fitLine("ctrl+n starts a fresh session", width)))
	}
	return lines
}

// heavyContextShare is where a context is full enough to be worth acting on.
const heavyContextShare = 0.8

// contextLabel says how full the context is, and nothing at all until the
// agent has said — the window it measures against is not AgentLine's to guess.
func contextLabel(s *events.Session) string {
	if s.ContextWindow <= 0 {
		return ""
	}
	return fmt.Sprintf("Context: %d%% used", int(s.ContextPercent*100+0.5))
}

func contextStyle(s *events.Session) lipgloss.Style {
	if s.ContextPercent >= heavyContextShare {
		return styleWarn
	}
	return styleDim
}

func limitStyle(s *events.Session) lipgloss.Style {
	if _, peak, ok := s.Peak(); ok && peak.Used >= 0.9 {
		// The window closest to running out is the one worth noticing.
		return styleWarn
	}
	return styleDim
}

// modelName is the model written the way it is spoken: "Opus 5", not the id.
func modelName(model string) string {
	short := shortModel(model)

	words := strings.Split(short, "-")
	for i, word := range words {
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
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

		text := fmt.Sprintf("%s: %d%%", limitLabel(name), int(limit.Used*100+0.5))
		if limit.Overage {
			text += " over"
		}
		if !limit.ResetsAt.IsZero() {
			text += " (reset " + limit.ResetsAt.Format("1/2 15:04") + ")"
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
		return "7d"
	case "monthly":
		return "30d"
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
	verb := verbStyle(action.Kind).Bold(true).Render(actionVerb(action.Kind))
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

	// The two halves are styled apart, so the bar reads as a proportion at a
	// glance rather than as one coloured block, and it turns green once the
	// last task is done.
	fill := styleBarFill
	if p.Done >= p.Total {
		fill = styleBarDone
	}
	bar := fill.Render(strings.Repeat("█", filled)) + styleBarRest.Render(strings.Repeat("░", bars-filled))
	return bar + " " + styleValue.Render(count)
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
		hint = branchStyled(branch) + styleDim.Render("   ") + hint
	}

	prompt := m.promptField(width - ansi.StringWidth(hint) - 2)

	gap := width - ansi.StringWidth(prompt) - ansi.StringWidth(hint)
	if gap < 1 {
		return fitLine(prompt, width)
	}
	return prompt + strings.Repeat(" ", gap) + hint
}

// promptField renders the prompt itself: editable when AgentLine owns the
// session, dimmed and inert when it is only watching one.
func (m Model) promptField(width int) string {
	if !m.canSend() {
		return fitLine(styleDim.Render("> "+m.placeholder()), width)
	}
	if m.sendErr != nil {
		return fitLine(styleError.Render("> "+firstLine(m.sendErr.Error())), width)
	}

	// The field is rendered here rather than by the widget, which pads to a
	// character count. Korean and other CJK text takes two cells per
	// character, so a widget-padded field runs two cells past its box for
	// every Hangul syllable and the line wraps around the screen.
	field := promptMark(m.input.Prompt) + m.promptText(width)
	m.placeCaret(field)

	return fitLine(field, width)
}

// promptMark colours the caret marker, so the one live input on the screen is
// findable without reading the bar.
func promptMark(mark string) string {
	return styleFocus.Render(mark)
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
		return styleDim.Render(m.placeholder())
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
	// Nothing else can be done that matters as much as unblocking the agent,
	// so the bar says so even though the panel says it too.
	case m.st.Agent.Ask != nil:
		return askKeys(m.st.Agent.Ask) + "   q quit"
	case m.st.Agent.Spin != nil && !m.inputFocused():
		return "x interrupt   esc dismiss   i redirect   q quit"
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

// branchStyled colours the dirty marker rather than the branch name, so
// uncommitted work registers at the edge of the eye without the name having to
// be read at all.
func branchStyled(branch string) string {
	if name, ok := strings.CutSuffix(branch, "*"); ok {
		return styleDim.Render(name) + styleWarn.Render("*")
	}
	return styleDim.Render(branch)
}

// rule renders a full-width horizontal separator.
func rule(width int) string {
	return styleRule.Render(strings.Repeat("─", max(width, 0)))
}

// valueOrDash renders a value, or an em dash when it is unknown. AgentLine
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
