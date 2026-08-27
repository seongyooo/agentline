package tui

import (
	"io"
	"sync/atomic"

	"github.com/charmbracelet/x/ansi"
)

// Caret tracks where the text cursor belongs and puts the terminal's cursor
// there after each frame.
//
// Bubble Tea v1 parks the terminal cursor at the start of the last line after
// every render, and offers no way to place it elsewhere. An input method draws
// the syllable it is composing at that cursor, so typing Korean showed the
// character at the far left of the screen until it was finished, at which
// point it jumped to where the text actually was.
//
// Wrapping the program's output lets the position be corrected: the escape the
// renderer writes to park the cursor is followed by one that moves it to the
// caret. Bubble Tea v2 has real cursor support and this can go when the
// migration is worth it.
type Caret struct {
	// pos packs a one-based row and column into one word, so the render
	// goroutine can read what the update goroutine wrote without a lock.
	// Zero means the caret is hidden.
	pos atomic.Uint64
}

// Set records where the caret belongs, in one-based screen coordinates.
func (c *Caret) Set(col, row int) {
	if col < 1 || row < 1 {
		c.Hide()
		return
	}
	c.pos.Store(uint64(row)<<32 | uint64(uint32(col)))
}

// Hide stops placing the cursor, leaving it wherever the renderer parked it.
func (c *Caret) Hide() { c.pos.Store(0) }

// position reports the caret, and whether there is one.
func (c *Caret) position() (col, row int, ok bool) {
	packed := c.pos.Load()
	if packed == 0 {
		return 0, 0, false
	}
	return int(uint32(packed)), int(packed >> 32), true
}

// Writer wraps the terminal so each frame ends with the cursor at the caret.
func (c *Caret) Writer(out io.Writer) io.Writer { return &caretWriter{out: out, caret: c} }

type caretWriter struct {
	out   io.Writer
	caret *Caret
}

// Write passes the frame through and re-places the cursor after it.
//
// The count returned is what the caller wrote, not what reached the terminal:
// the extra escape is this writer's own business, and reporting it would look
// like a short write to a caller that has no idea it exists.
func (w *caretWriter) Write(p []byte) (int, error) {
	n, err := w.out.Write(p)
	if err != nil {
		return n, err
	}

	col, row, ok := w.caret.position()
	if !ok {
		return n, nil
	}
	if _, err := io.WriteString(w.out, ansi.CursorPosition(col, row)); err != nil {
		return n, err
	}
	return n, nil
}
