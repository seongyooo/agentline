package tui

import (
	"io"
	"os/exec"
	"strings"
	"sync"
	"unicode"
)

// Alert gets the user's attention when the agent stops and needs an answer.
//
// This is the only thing AgentLine interrupts anyone for, and it is the reason
// the rest of the UI can be quiet: if being blocked reaches you on its own,
// nothing else has to be watched.
//
// It rides out on the frame the way the caret does, because Bubble Tea owns
// the terminal and a second writer would race the renderer for it. Two escapes
// go out, both ignored by terminals that do not know them:
//
//   - BEL, which every terminal understands, though many only flash
//   - OSC 9, a real desktop notification in Windows Terminal and iTerm2
//
// A terminal that does neither — over a bare ssh session, say — is what
// NotifyCommand is for.
type Alert struct {
	// NotifyCommand is run with the alert text as its last argument, for
	// terminals that raise no notification of their own. Empty runs nothing.
	NotifyCommand []string

	mu      sync.Mutex
	pending string
	armed   bool
}

// Raise asks for attention once. Raising again before the frame goes out
// replaces the text rather than queueing: what is wanted is the current
// question, not a backlog of them.
func (a *Alert) Raise(text string) {
	a.mu.Lock()
	a.pending, a.armed = alertText(text), true
	a.mu.Unlock()

	a.run(text)
}

// run hands the alert to an external notifier, off the UI goroutine so a slow
// or missing command cannot stall a frame.
func (a *Alert) run(text string) {
	if len(a.NotifyCommand) == 0 {
		return
	}
	command := append(append([]string{}, a.NotifyCommand...), text)
	go func() {
		// Nothing is reported if it fails. A notifier that does not work is
		// not worth taking over the screen to complain about, and the on-screen
		// state says the same thing anyway.
		_ = exec.Command(command[0], command[1:]...).Run()
	}()
}

// take reports a pending alert and clears it.
func (a *Alert) take() (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	text, armed := a.pending, a.armed
	a.pending, a.armed = "", false
	return text, armed
}

// Writer wraps the terminal so a raised alert goes out after the next frame.
func (a *Alert) Writer(out io.Writer) io.Writer { return &alertWriter{out: out, alert: a} }

type alertWriter struct {
	out   io.Writer
	alert *Alert
}

// Write passes the frame through, then rings.
//
// The count returned is what the caller wrote. The escapes are this writer's
// own business, and counting them would look like a short write to a caller
// that does not know they exist.
func (w *alertWriter) Write(p []byte) (int, error) {
	n, err := w.out.Write(p)
	if err != nil {
		return n, err
	}

	text, armed := w.alert.take()
	if !armed {
		return n, nil
	}
	if _, err := io.WriteString(w.out, "\a\x1b]9;"+text+"\a"); err != nil {
		return n, err
	}
	return n, nil
}

// alertMaxLen bounds the notification body. A desktop notification truncates
// anyway, and the useful half is at the front.
const alertMaxLen = 120

// alertText makes a string safe to put inside an escape sequence.
//
// Anything that could end the sequence early or start another one is dropped,
// so an alert built from the agent's own words cannot smuggle escapes into the
// terminal. The text is not ours: it comes from whatever the agent asked about.
func alertText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\a' || r == 0x1b || r == '\\':
			continue
		case unicode.IsControl(r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
		if b.Len() >= alertMaxLen {
			break
		}
	}
	return strings.TrimSpace(b.String())
}
