package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/seongyooo/agentline/internal/events"
)

// A share of something finite is a length, and reading it as a number means
// converting it back into one. These are the only bars on the screen besides
// the mission's task count, and like it they keep their number: a bar says how
// full at a glance, the number says how full exactly, and a terminal that
// draws no blocks still has the whole fact.
//
// Everything drawn here was reported by the agent. AgentLine measures no usage
// and estimates none, so a window the agent never mentioned has no row.
const (
	// gaugeLabel is the column the names line up in: Context, 5h, 7d, 30d.
	gaugeLabel = 8

	// gaugeMin is the shortest bar worth drawing. Below it the shape says
	// less than the number does and the text form is used instead.
	gaugeMin = 10

	// gaugeMax keeps a bar from stretching across a wide column, where length
	// stops reading as a proportion and starts reading as a wall.
	gaugeMax = 24
)

// gaugeLines renders the session's model and its usage windows as bars,
// or nil when the column is too narrow for them to be worth the rows.
func (m Model) gaugeLines(s *events.Session, l Layout) []string {
	width := l.MissionInner()
	rows := gaugeRows(s)
	if len(rows) == 0 {
		return nil
	}

	// The widest note decides the bar, so every row lines up whatever its own
	// reset time happens to be.
	note := 0
	for _, r := range rows {
		note = max(note, len(r.note))
	}

	bar := width - gaugeLabel - gaugeShareWidth - note - 2
	if bar < gaugeMin {
		return nil
	}
	bar = min(bar, gaugeMax)

	var lines []string
	// A framed panel carries the model in its title, where it costs nothing.
	// Unframed there is nowhere to put it but a row of its own.
	if s.Model != "" && !l.Boxed {
		lines = append(lines, styleDim.Render(fitLine(modelName(s.Model), width)))
	}
	for _, r := range rows {
		lines = append(lines, fitLine(gaugeRow(r, bar, note), width))
	}

	// A session AgentLine owns is never compacted, so a context this full
	// keeps costing what it costs on every further turn.
	if s.ContextPercent >= heavyContextShare {
		lines = append(lines, styleWarn.Render(fitLine("ctrl+n starts a fresh session", width)))
	}
	return lines
}

// gaugeShareWidth is the room the percentage takes, right-aligned: " 100%".
const gaugeShareWidth = 5

// gauge is one labelled proportion.
type gauge struct {
	label string
	share float64
	note  string
}

// gaugeRows is what the agent reported, in the order it is worth reading:
// how full the conversation is, then the window that runs out soonest.
func gaugeRows(s *events.Session) []gauge {
	var rows []gauge
	if s.ContextWindow > 0 {
		rows = append(rows, gauge{label: "Context", share: s.ContextPercent})
	}

	for _, name := range limitNames(s) {
		limit := s.Limits[name]

		note := ""
		if !limit.ResetsAt.IsZero() {
			note = "resets " + limit.ResetsAt.Format("1/2 15:04")
		}
		if limit.Overage {
			note = "over  " + note
		}
		rows = append(rows, gauge{label: limitLabel(name), share: limit.Used, note: note})
	}
	return rows
}

// limitNames orders the reported windows, shortest first, keeping anything
// unrecognised rather than dropping it for not being on the list.
func limitNames(s *events.Session) []string {
	seen := map[string]bool{}
	names := make([]string, 0, len(s.Limits))

	for _, name := range limitOrder {
		if _, ok := s.Limits[name]; ok {
			names = append(names, name)
			seen[name] = true
		}
	}
	for name := range s.Limits {
		if !seen[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names[len(seen):])
	return names
}

// gaugeRow draws one row: name, bar, share, and when it resets.
func gaugeRow(g gauge, bar, note int) string {
	line := styleDim.Render(fmt.Sprintf("%-*s", gaugeLabel, g.label)) +
		gaugeBar(g.share, bar) +
		shareStyle(g.share).Render(fmt.Sprintf("%*d%%", gaugeShareWidth-1, int(g.share*100+0.5)))

	if note == 0 {
		return line
	}
	return line + "  " + styleDim.Render(g.note)
}

// gaugeBar draws the proportion itself.
//
// The filled part rounds down and is never allowed to disappear while there is
// anything at all: a window one percent used should not look untouched, and a
// window not quite full should not look finished.
func gaugeBar(share float64, width int) string {
	filled := int(share * float64(width))
	if share > 0 && filled == 0 {
		filled = 1
	}
	if share < 1 && filled == width {
		filled = width - 1
	}
	filled = clamp(filled, 0, width)

	return shareStyle(share).Render(strings.Repeat("█", filled)) +
		styleBarRest.Render(strings.Repeat("░", width-filled))
}

// shareStyle colours a proportion by how close it is to running out. Below the
// threshold it says nothing, which is the point: a bar that is always coloured
// has no colour left for the moment it matters.
func shareStyle(share float64) lipgloss.Style {
	switch {
	case share >= gaugeCritical:
		return styleError
	case share >= heavyContextShare:
		return styleWarn
	}
	return styleWorking
}

// gaugeCritical is where a window is close enough to gone to be alarming.
const gaugeCritical = 0.95
