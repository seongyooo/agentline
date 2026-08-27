package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/seongyooo/agentline/internal/project"
	"github.com/seongyooo/agentline/internal/state"
)

// A frame is worth two columns and two rows of a large terminal and is not
// worth them on a small one. §21 allows the decoration only on that condition.
func TestFramesTurnThemselvesOff(t *testing.T) {
	tests := []struct {
		w, h  int
		boxed bool
	}{
		{100, 30, true},
		{boxMinWidth, boxMinHeight, true},
		{boxMinWidth - 1, 30, false},
		{100, boxMinHeight - 1, false},
		{72, 24, false},
	}

	for _, tc := range tests {
		if got := computeLayout(tc.w, tc.h).Boxed; got != tc.boxed {
			t.Errorf("%dx%d: boxed = %v, want %v", tc.w, tc.h, got, tc.boxed)
		}
	}
}

// The frame must cost the panel exactly what the layout says it costs, at
// every size, or a column ends up drawn past its own edge.
func TestFramedFrameIsExact(t *testing.T) {
	for _, w := range []int{88, 100, 132, 200} {
		for _, h := range []int{26, 30, 44} {
			st := state.New("/proj")
			st.Project.Tree = project.MockTree()

			model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: w, Height: h})
			out := model.(Model).View()

			lines := strings.Split(out, "\n")
			if len(lines) != h {
				t.Errorf("%dx%d: %d rows", w, h, len(lines))
			}
			for i, line := range lines {
				if got := ansi.StringWidth(line); got != w {
					t.Errorf("%dx%d line %d: width %d", w, h, i, got)
				}
			}
		}
	}
}

// A frame carries its panel's name, so the heading inside must go: printing it
// twice, one line apart, is what the frame was supposed to save.
func TestFramedPanelsDoNotRepeatTheirName(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()

	model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	out := ansi.Strip(model.(Model).View())

	for _, name := range []string{"PROJECT", "ACTIVITY"} {
		if got := strings.Count(out, name); got != 1 {
			t.Errorf("%q appears %d times, want once", name, got)
		}
	}
}
