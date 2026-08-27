package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/seongyooo/agentline/internal/state"
)

// pulseLevels are the eighths a column can be drawn at, tallest last.
var pulseLevels = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// pulseLine draws the whole session as one row.
//
// This is the one thing on screen that is about the session rather than about
// this moment, and the one thing the agent's own terminal cannot show: it is a
// scrollback, and a scrollback has no shape. Twenty minutes away and a glance
// here says where the work happened, where it went quiet, and where it broke.
//
// Height is a count of observed actions and nothing else — no weighting, no
// guess at effort. Colour names the most notable kind in the column, so a
// failure is findable, but the heights carry the whole story on their own for
// a terminal that renders no colour.
func (m Model) pulseLine(width int) string {
	label := styleLabel.Render("PULSE")
	span := m.pulseSpan()

	// The bar takes what the label and the times leave. Below a certain width
	// there is no picture worth drawing, only noise.
	bars := width - ansi.StringWidth(label) - ansi.StringWidth(ansi.Strip(span)) - 4
	if bars < pulseMinWidth {
		return ""
	}

	columns := m.st.Agent.Pulse.Columns(bars)
	if len(columns) == 0 {
		return ""
	}

	line := label + "  " + pulseBars(columns, bars)
	gap := width - ansi.StringWidth(line) - ansi.StringWidth(ansi.Strip(span))
	if gap < 1 {
		return fitLine(line, width)
	}
	return line + strings.Repeat(" ", gap) + span
}

// pulseMinWidth is the narrowest bar worth drawing. Fewer columns than this is
// not a shape, it is a smudge.
const pulseMinWidth = 24

// pulseBars renders the columns, left-padded so a young session grows into the
// row from the left instead of stretching to fill it.
//
// Stretching would be a lie of a particular kind: the same three actions would
// draw the same picture whether they happened over ten seconds or ten minutes.
func pulseBars(columns []state.Slot, width int) string {
	peak := state.Peak(columns)

	var b strings.Builder
	for _, column := range columns {
		if column.Count == 0 {
			// Quiet time is drawn as quiet. A baseline tick here would make
			// "nothing happened" look like "something small happened".
			b.WriteByte(' ')
			continue
		}
		b.WriteString(verbStyle(column.Kind).Render(string(pulseLevel(column.Count, peak))))
	}
	if pad := width - len(columns); pad > 0 {
		b.WriteString(strings.Repeat(" ", pad))
	}
	return b.String()
}

// pulseLevel maps a count onto the eighths, so one action is always visible and
// the busiest column is always full height.
func pulseLevel(count, peak int) rune {
	if count <= 0 {
		return ' '
	}
	if peak <= 1 {
		// Nothing was busier than anything else, so there is no shape to
		// scale. Drawn at the floor a quiet session reads as an empty row
		// rather than an even one, which is the opposite of what it says.
		return pulseLevels[2]
	}
	step := (count - 1) * (len(pulseLevels) - 1) / (peak - 1)
	return pulseLevels[clamp(step, 0, len(pulseLevels)-1)]
}

// pulseSpan names the stretch of time the row covers, which is what stops the
// shape from being unreadable: without it the same picture could be a minute
// or an afternoon.
func (m Model) pulseSpan() string {
	from, _, ok := m.st.Agent.Pulse.Span()
	if !ok {
		return ""
	}
	return styleDim.Render(from.Format("15:04") + " → now")
}
