package tui

import "testing"

func TestLayoutFillsExactHeight(t *testing.T) {
	for _, h := range []int{12, 16, 20, 24, 30, 40, 60} {
		for _, w := range []int{40, 60, 72, 100, 160} {
			l := computeLayout(w, h)
			total := chromeRows + l.BodyHeight + l.ActivityHeight()
			if total != h {
				t.Errorf("%dx%d: rows total %d, want %d", w, h, total, h)
			}
			if l.BodyHeight < minBodyHeight {
				t.Errorf("%dx%d: body %d rows, want >= %d", w, h, l.BodyHeight, minBodyHeight)
			}
		}
	}
}

func TestTwoColumnOnlyWhenWide(t *testing.T) {
	if l := computeLayout(twoColumnMinWidth-1, 40); l.TwoColumn || l.ShowTree {
		t.Error("narrow terminal should stack and hide the tree")
	}
	if l := computeLayout(twoColumnMinWidth, 40); !l.TwoColumn || !l.ShowTree {
		t.Error("wide terminal should use two columns with a tree")
	}
}

func TestColumnsFitWidth(t *testing.T) {
	for _, w := range []int{72, 80, 100, 200} {
		l := computeLayout(w, 30)
		if got := l.TreeWidth + dividerWidth + l.MissionWidth(); got != w {
			t.Errorf("width %d: columns total %d", w, got)
		}
		if l.MissionWidth() < minTreeWidth {
			t.Errorf("width %d: mission column only %d wide", w, l.MissionWidth())
		}
	}
}

// Section 20 ranks NEXT below the activity log, so a shrinking terminal must
// drop NEXT before it drops activity.
func TestNextIsDroppedBeforeActivity(t *testing.T) {
	var nextGone, activityGone int
	for h := 60; h >= minHeight; h-- {
		l := computeLayout(100, h)
		if !l.ShowNext && nextGone == 0 {
			nextGone = h
		}
		if l.ActivityRows == 0 && activityGone == 0 {
			activityGone = h
		}
	}
	if nextGone == 0 {
		t.Fatal("NEXT was never dropped")
	}
	if activityGone != 0 && activityGone >= nextGone {
		t.Errorf("activity dropped at h=%d, before NEXT at h=%d", activityGone, nextGone)
	}
}

func TestActivityLogIsNeverVestigial(t *testing.T) {
	for h := minHeight; h <= 60; h++ {
		if rows := computeLayout(100, h).ActivityRows; rows != 0 && rows < minActivityLog {
			t.Errorf("height %d: activity log has %d rows", h, rows)
		}
	}
}
