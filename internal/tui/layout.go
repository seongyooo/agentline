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

// activityChrome counts the activity panel's own rule and label rows.
const activityChrome = 2

// computeLayout derives the layout for a terminal size.
func computeLayout(width, height int) Layout {
	l := Layout{Width: width, Height: height}

	l.TwoColumn = width >= twoColumnMinWidth
	// The tree is low priority: it only appears when there is a column to
	// spare, never stacked above the mission in a narrow terminal.
	l.ShowTree = l.TwoColumn
	if l.ShowTree {
		l.TreeWidth = clamp(width*2/5, minTreeWidth, maxTreeWidth)
	}

	free := height - chromeRows
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

// ActivityHeight is the rows the activity panel occupies, chrome included.
func (l Layout) ActivityHeight() int {
	if l.ActivityRows == 0 {
		return 0
	}
	return l.ActivityRows + activityChrome
}

// MissionWidth is the columns available to the mission/now/next panel.
func (l Layout) MissionWidth() int {
	if !l.TwoColumn {
		return l.Width
	}
	return l.Width - l.TreeWidth - dividerWidth
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
