package tui

import tea "github.com/charmbracelet/bubbletea"

// focusArea is the panel the arrow keys and the mouse wheel act on.
//
// Several panels can scroll, so the keys have to belong to one of them at a
// time. Which one is shown in its heading, since a scroll that goes to the
// wrong panel is otherwise indistinguishable from one that does nothing.
type focusArea int

const (
	focusTree focusArea = iota
	focusReply
	focusActivity
	focusPrompt

	// focusInspect is not in the tab order: it is what the mission column
	// shows while an entry is being looked at, not a panel to cycle through.
	focusInspect
)

// cycle returns the next focus in tab order, skipping panels that are not on
// screen: tabbing onto something invisible would look like the key was
// swallowed.
func (m Model) cycle(from focusArea, l Layout) focusArea {
	order := []focusArea{focusTree, focusReply, focusActivity, focusPrompt}

	for i := 1; i <= len(order); i++ {
		next := order[(int(from)+i)%len(order)]
		if m.focusable(next, l) {
			return next
		}
	}
	return from
}

// focusable reports whether a panel can currently take focus.
func (m Model) focusable(area focusArea, l Layout) bool {
	switch area {
	case focusTree:
		return l.ShowTree
	case focusReply:
		return m.replyShown(l)
	case focusActivity:
		return l.ActivityRows > 0
	case focusPrompt:
		return m.canSend()
	}
	return false
}

// setFocus moves focus, taking the prompt's editing state with it.
//
// The terminal cursor is shown only while typing. It is what an input method
// composes against, and it doubles as the caret the user needs to see now that
// the field is drawn here rather than by the widget.
func (m *Model) setFocus(area focusArea) tea.Cmd {
	if m.focus == focusPrompt && area != focusPrompt {
		m.input.Blur()
		if m.caret != nil {
			m.caret.Hide()
		}
		m.focus = area
		return tea.HideCursor
	}
	m.focus = area

	if area == focusPrompt {
		return tea.Batch(m.input.Focus(), tea.ShowCursor)
	}
	return nil
}

// scrollFocused moves the focused panel by delta lines.
func (m *Model) scrollFocused(delta int, l Layout) {
	switch m.focus {
	case focusTree:
		m.moveCursor(delta)
		m.syncScroll()
	case focusReply:
		m.replyScroll = clamp(m.replyScroll+delta, 0, m.replyMaxScroll(l))
	case focusActivity:
		// The log's offset counts backwards from the newest entry, so moving
		// up the screen means moving further into the past.
		m.activityScroll = clamp(m.activityScroll-delta, 0, m.activityMaxScroll(l))
	}
}

// focusAt maps a click to the panel under it, so the mouse can move focus
// without the user having to tab there first.
func (m Model) focusAt(x, y int, l Layout) (focusArea, bool) {
	switch {
	case y >= m.height-1:
		if m.canSend() {
			return focusPrompt, true
		}
		return 0, false

	case y < headerRows:
		return 0, false
	}

	// Body rows, split into the tree column and the mission column.
	body := y - headerRows
	if body < l.BodyHeight {
		if l.ShowTree && x < l.TreeWidth {
			return focusTree, true
		}
		if m.replyLines() != nil && m.replyRegion(l).contains(body) {
			return focusReply, true
		}
		return 0, false
	}

	if l.ActivityRows > 0 {
		return focusActivity, true
	}
	return 0, false
}

// region is a span of body rows a panel occupies.
type region struct{ start, end int }

func (r region) contains(row int) bool { return row >= r.start && row < r.end }
