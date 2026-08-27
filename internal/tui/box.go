package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Panels are drawn as boxes when there is width and height to spend on them,
// and as bare labelled blocks when there is not.
//
// §21 warns against excessive borders and it is right to: a frame around every
// small thing is how a terminal UI turns into a maze of lines. One frame per
// panel, with the panel's name set into its top edge, is the other thing —
// it is what makes a screen read as an application rather than as output that
// happened to be aligned. The rule is that the border must never cost content
// that would otherwise be shown, which is why it turns itself off rather than
// squeezing a panel below what it needs.
const (
	// boxSides is what a frame costs a panel horizontally: two edges and a
	// cell of air inside each of them. The padding is not decoration —
	// content pressed against a vertical rule is harder to read than the same
	// content with nothing around it at all.
	boxSides = 4
	boxRows  = 2 // the top and bottom edges

	// boxMinWidth is the narrowest terminal that can afford edges. Below it
	// the columns are already tight enough that two cells per panel is two
	// cells of a filename.
	boxMinWidth = 88

	// boxMinHeight is the shortest terminal that can afford them. Each panel
	// spends two rows, and a short terminal needs those rows for content.
	boxMinHeight = 26
)

// box draws lines inside a rounded frame with the title set into the top edge.
//
// width and height are the outer size, so a caller can lay boxes out without
// knowing what the border costs. Content is fitted to the inside, which means
// a box is exactly as tall and wide as it was asked to be, always.
func box(title string, lines []string, width, height int, focused bool) []string {
	if width < 4 || height < boxRows {
		return fit(lines, width, height)
	}

	inner := width - boxSides
	style := styleRule
	if focused {
		// The focused panel is outlined in the accent, which is the one place
		// colour is doing work no symbol is also doing — so the heading keeps
		// its marker for terminals that render none.
		style = styleFocus
	}

	out := make([]string, 0, height)
	out = append(out, style.Render(boxTop(title, width-2, focused)))

	body := fit(lines, inner, max(height-boxRows, 0))
	edge := style.Render("│")
	for _, line := range body {
		out = append(out, edge+" "+line+" "+edge)
	}

	return append(out, style.Render("╰"+strings.Repeat("─", width-2)+"╯"))
}

// boxTop is the top edge with the panel's name set into it.
func boxTop(title string, inner int, focused bool) string {
	if title == "" {
		return "╭" + strings.Repeat("─", inner) + "╮"
	}
	// Marked as well as coloured, so which panel the keys reach survives a
	// terminal that renders no colour — the same marker the bare headings use.
	if focused {
		title += " ◂"
	}

	label := " " + title + " "
	if ansi.StringWidth(label) > inner {
		return "╭" + strings.Repeat("─", inner) + "╮"
	}
	return "╭─" + label + strings.Repeat("─", inner-ansi.StringWidth(label)-1) + "╮"
}

// boxNote puts a short note on the bottom edge, which is where a scroll
// position belongs: it is about the box, not about what is in it.
func boxNote(bottom, note string, focused bool) string {
	if note == "" {
		return bottom
	}
	style := styleRule
	if focused {
		style = styleFocus
	}

	plain := ansi.Strip(bottom)
	label := " " + note + " "
	// The note sits in from the right corner. If it does not fit, the edge is
	// left plain rather than truncating something already this short.
	if len(plain) < ansi.StringWidth(label)+4 {
		return bottom
	}

	runes := []rune(plain)
	at := len(runes) - ansi.StringWidth(label) - 2
	return style.Render(string(runes[:at])) + styleDim.Render(label) + style.Render(string(runes[at+ansi.StringWidth(label):]))
}
