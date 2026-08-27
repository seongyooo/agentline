package tui

import "github.com/seongyooo/agentline/internal/project"

// treeView is the cursor and scroll position of the project panel. The tree
// itself lives in application state; only the viewing position is UI state.
type treeView struct {
	cursor int
	offset int
}

// window returns the slice of rows to draw and the scroll offset that keeps
// the cursor on screen.
func (v treeView) window(total, rows int) (start, end, offset int) {
	if rows <= 0 || total == 0 {
		return 0, 0, 0
	}

	offset = v.offset
	if offset > v.cursor {
		offset = v.cursor // cursor scrolled off the top
	}
	if bottom := v.cursor - rows + 1; offset < bottom {
		offset = bottom // cursor scrolled off the bottom
	}
	offset = clamp(offset, 0, max(total-rows, 0))

	return offset, min(offset+rows, total), offset
}

// rows is the currently visible tree rows.
func (m Model) rows() []project.Row {
	return project.Flatten(m.st.Project.Tree)
}

// selected returns the node under the cursor, or nil when the tree is empty.
func (m Model) selected() *project.Node {
	rows := m.rows()
	if len(rows) == 0 {
		return nil
	}
	return rows[clamp(m.tree.cursor, 0, len(rows)-1)].Node
}

// moveCursor moves the selection by delta, clamped to the tree.
func (m *Model) moveCursor(delta int) {
	total := len(m.rows())
	if total == 0 {
		return
	}
	m.tree.cursor = clamp(m.tree.cursor+delta, 0, total-1)
}

// expand opens the selected directory, loading it on first use.
func (m *Model) expand() {
	n := m.selected()
	if n == nil || !n.Dir || n.Placeholder {
		return
	}
	if m.scanner != nil {
		m.scanner.Load(n)
	}
	n.Expanded = true
}

// syncScroll clamps the scroll offset so the cursor stays visible. It runs
// after anything that changes the cursor or the number of visible rows.
func (m *Model) syncScroll() {
	if m.width < minWidth || m.height < minHeight {
		return
	}
	rows := computeLayout(m.width, m.height).BodyHeight - treeChrome
	_, _, m.tree.offset = m.tree.window(len(m.rows()), rows)
}

// collapse closes the selected directory, or jumps to its parent when the
// selection is already a leaf or a closed directory.
func (m *Model) collapse() {
	n := m.selected()
	if n == nil {
		return
	}
	if n.Dir && n.Expanded {
		n.Expanded = false
		return
	}
	m.selectParent(n)
}

// selectParent moves the cursor to the row that owns the given node.
func (m *Model) selectParent(n *project.Node) {
	rows := m.rows()
	for i := m.tree.cursor - 1; i >= 0; i-- {
		if isParent(rows[i].Node, n) {
			m.tree.cursor = i
			return
		}
	}
}

func isParent(candidate, child *project.Node) bool {
	if candidate == nil || !candidate.Dir {
		return false
	}
	for _, c := range candidate.Children {
		if c == child {
			return true
		}
	}
	return false
}

// toggle expands a closed directory and closes an open one.
func (m *Model) toggle() {
	n := m.selected()
	if n == nil || !n.Dir {
		return
	}
	if n.Expanded {
		n.Expanded = false
		return
	}
	m.expand()
}
