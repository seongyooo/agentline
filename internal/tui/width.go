package tui

import "github.com/mattn/go-runewidth"

// Two width tables decide how this UI is drawn, and they must agree.
//
// The layout measures lines with x/ansi, which pins East Asian ambiguous
// characters to one cell. Bubble Tea's screen renderer measures the same lines
// with go-runewidth's default condition, which auto-detects an East Asian
// locale and then calls those characters two cells wide.
//
// AgentView is drawn almost entirely out of characters in that ambiguous set —
// the rules, the column divider, the activity markers, the ellipsis. In a
// Korean locale the renderer therefore believed every line was far wider than
// the layout had built it, its cursor bookkeeping drifted, and the output
// corrupted: typing Hangul pushed the last syllable to the left of the prompt
// marker and the bar appeared to run off the screen.
//
// Pinning the renderer to the same table settles it. One cell is also what
// terminals actually draw for these characters here, which is why the rules
// and dividers looked right while the text around them did not.
func init() {
	runewidth.DefaultCondition.EastAsianWidth = false
}
