package tui

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// An input method draws the syllable it is composing at the terminal's cursor.
// Bubble Tea parks that cursor at the corner of the screen, which is where
// Korean was appearing until each syllable was finished.
func TestCaretFollowsTheTypedText(t *testing.T) {
	caret := &Caret{}

	m, _ := sendable(t)
	m = m.WithCaret(caret)
	m = focusPromptKey(m)
	m = typeText(m, "안녕")
	m.View()

	col, row, ok := caret.position()
	if !ok {
		t.Fatal("no caret while typing")
	}
	if row != m.height {
		t.Errorf("row = %d, want the prompt row %d", row, m.height)
	}
	// Two Hangul syllables are four cells, after the two-cell prompt marker.
	if want := ansi.StringWidth("> 안녕") + 1; col != want {
		t.Errorf("col = %d, want %d", col, want)
	}
}

// The caret must be measured in cells, or it lands mid-syllable and the input
// method composes in the wrong column.
func TestCaretIsMeasuredInCells(t *testing.T) {
	caret := &Caret{}

	m, _ := sendable(t)
	m = focusPromptKey(m.WithCaret(caret))

	for _, text := range []string{"hi", "안녕하세요", "hi 안녕 there"} {
		m.input.SetValue(text)
		m.View()

		col, _, ok := caret.position()
		if !ok {
			t.Fatalf("no caret for %q", text)
		}
		if want := ansi.StringWidth("> "+text) + 1; col != want {
			t.Errorf("%q: col = %d, want %d", text, col, want)
		}
	}
}

// With focus elsewhere there is nothing being typed, so the cursor is left
// where the renderer put it.
func TestCaretHiddenWhenNotTyping(t *testing.T) {
	caret := &Caret{}

	m, _ := sendable(t)
	m = focusPromptKey(m.WithCaret(caret))
	m.View()
	if _, _, ok := caret.position(); !ok {
		t.Fatal("no caret while typing")
	}

	m, _ = key(m, tea.KeyEsc)
	m.View()
	if _, _, ok := caret.position(); ok {
		t.Error("caret still placed after leaving the prompt")
	}
}

// The writer puts the cursor back after the frame the renderer parked it in.
func TestWriterRepositionsTheCursorAfterEachFrame(t *testing.T) {
	caret := &Caret{}
	caret.Set(12, 30)

	var out bytes.Buffer
	frame := "some frame" + ansi.CursorPosition(0, 30)

	n, err := caret.Writer(&out).Write([]byte(frame))
	if err != nil {
		t.Fatal(err)
	}
	// The caller hears about its own bytes, not the writer's extra escape.
	if n != len(frame) {
		t.Errorf("reported %d bytes written, want %d", n, len(frame))
	}
	if got := out.String(); !strings.HasSuffix(got, ansi.CursorPosition(12, 30)) {
		t.Errorf("frame does not end at the caret: %q", got)
	}
}

// Nothing is appended when there is no caret to place.
func TestWriterLeavesTheCursorAloneWhenHidden(t *testing.T) {
	caret := &Caret{}
	caret.Hide()

	var out bytes.Buffer
	if _, err := caret.Writer(&out).Write([]byte("frame")); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "frame" {
		t.Errorf("wrote %q, want the frame unchanged", got)
	}
}

// A model with no caret must render exactly as before.
func TestNoCaretIsHarmless(t *testing.T) {
	m, _ := sendable(t)
	m = focusPromptKey(m)

	if out := m.View(); out == "" {
		t.Error("view broke without a caret")
	}
}

func TestCaretRejectsPositionsOffTheScreen(t *testing.T) {
	caret := &Caret{}
	caret.Set(5, 5)

	caret.Set(0, 5)
	if _, _, ok := caret.position(); ok {
		t.Error("a zero column was accepted")
	}
}
