package tui

// Layout decides which panels are shown and how tall they are for a given
// terminal size. It encodes the priority order from IMPLEMENTATION.md §20:
// status > NOW > mission > project activity > activity log > tree > NEXT.
//
// It performs no rendering, so the collapsing rules can be tested directly.
type Layout struct {
	Width  int
	Height int

	TwoColumn bool
	ShowTree  bool
	ShowNext  bool
	ShowPulse bool

	// Boxed draws each panel in a frame with its name set into the top edge.
	// It costs two columns a panel and nothing in rows: the frame takes two,
	// and gives back the heading row and the spacer under it.
	Boxed bool

	TreeWidth    int // columns for the project panel, excluding the divider
	BodyHeight   int // rows for the mission/tree area
	ActivityRows int // activity entries; 0 hides the panel entirely
}

// Layout breakpoints.
const (
	twoColumnMinWidth = 72
	minTreeWidth      = 22
	maxTreeWidth      = 38
	dividerWidth      = 3 // " │ "

	// Rows the mission panel needs, mirroring the sections missionPanel
	// builds: MISSION(2) + NOW(3) + NEXT(2) plus a blank between each.
	bodyWithNext    = 9
	bodyWithoutNext = 6
	minBodyHeight   = bodyWithoutNext

	minActivityLog = 3 // a shorter log is noise; drop the panel instead
	maxActivityLog = 8
)

// chromeRows counts the always-present rows: header, the rule under it, the
// rule above the input, and the input line itself.
const chromeRows = 4

// pulseRows is the row the session strip costs when it is shown.
const pulseRows = 1

// pulseMinHeight is the terminal height below which the session strip is not
// worth a row. It is history, and history loses to everything that is about
// now — but on any ordinary terminal there is room for both.
const pulseMinHeight = 24

// headerRows is the header plus its rule, which the body starts after.
const headerRows = 2

// activityChrome counts the activity panel's own rule and label rows.
const activityChrome = 2

// computeLayout derives the layout for a terminal size.
func computeLayout(width, height int) Layout {
	l := Layout{Width: width, Height: height}

	l.Boxed = width >= boxMinWidth && height >= boxMinHeight
	l.TwoColumn = width >= twoColumnMinWidth
	// The tree is low priority: it only appears when there is a column to
	// spare, never stacked above the mission in a narrow terminal.
	l.ShowTree = l.TwoColumn
	if l.ShowTree {
		l.TreeWidth = clamp(width*2/5, minTreeWidth, maxTreeWidth)
	}

	free := height - chromeRows
	// The strip is the lowest-priority thing on the screen, taken out of the
	// budget before anything else is sized so it can never squeeze a panel.
	l.ShowPulse = height >= pulseMinHeight
	if l.ShowPulse {
		free -= pulseRows
	}
	l.ActivityRows = fitActivity(free)
	l.BodyHeight = free - l.ActivityHeight()
	// NEXT is the lowest-ranked panel, so it appears only once the body has
	// room for it on top of MISSION and NOW.
	l.ShowNext = l.BodyHeight >= bodyWithNext
	return l
}

// fitActivity sizes the activity log from the rows left over once the mission
// panel has what it needs. The log outranks only NEXT, so it gives up NEXT's
// rows first and disappears entirely rather than crowd out MISSION or NOW.
func fitActivity(free int) int {
	rows := clamp(free-bodyWithNext-activityChrome, 0, maxActivityLog)
	if rows >= minActivityLog {
		return rows
	}
	if rows = clamp(free-bodyWithoutNext-activityChrome, 0, maxActivityLog); rows >= minActivityLog {
		return rows
	}
	return 0
}

// PulseHeight is the rows the session strip occupies.
func (l Layout) PulseHeight() int {
	if !l.ShowPulse {
		return 0
	}
	return pulseRows
}

// ActivityHeight is the rows the activity panel occupies, chrome included.
func (l Layout) ActivityHeight() int {
	if l.ActivityRows == 0 {
		return 0
	}
	return l.ActivityRows + activityChrome
}

// MissionWidth is the columns the mission panel occupies, frame included.
func (l Layout) MissionWidth() int {
	if !l.TwoColumn {
		return l.Width
	}
	if l.Boxed {
		// Two frames side by side need no divider drawn between them; they
		// are already two edges apart.
		return l.Width - l.TreeWidth - boxedGap
	}
	return l.Width - l.TreeWidth - dividerWidth
}

// boxedGap is the space between two framed columns.
const boxedGap = 1

// MissionInner and TreeInner are the columns a panel's content actually gets,
// which is what every renderer measures against. Keeping the frame's cost in
// one place is what stops a panel from being drawn two cells too wide.
func (l Layout) MissionInner() int { return l.MissionWidth() - l.frame() }
func (l Layout) TreeInner() int    { return l.TreeWidth - l.frame() }

// PanelChrome is the rows a panel spends before its content: a frame's two
// edges, or a heading and the blank line under it.
func (l Layout) PanelChrome() int {
	if l.Boxed {
		return boxRows
	}
	return treeChrome
}

func (l Layout) frame() int {
	if l.Boxed {
		return boxSides
	}
	return 0
}

// FrameRows is what a frame costs a panel vertically, which is not what it
// costs horizontally — conflating the two silently shortened the mission
// column by the width of its own padding.
func (l Layout) FrameRows() int {
	if l.Boxed {
		return boxRows
	}
	return 0
}

// ColumnGap is the space between the two columns: a drawn divider, or the
// single cell two frames need between their edges.
func (l Layout) ColumnGap() int {
	if !l.TwoColumn {
		return 0
	}
	if l.Boxed {
		return boxedGap
	}
	return dividerWidth
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
