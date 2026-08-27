// Package tui renders application state. It holds no event-processing,
// filesystem, or agent-specific logic; it delegates to the reducer and the
// project scanner and renders whatever they produce.
package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/seongyooo/agentline/internal/events"
	"github.com/seongyooo/agentline/internal/git"
	"github.com/seongyooo/agentline/internal/project"
	"github.com/seongyooo/agentline/internal/state"
)

// minWidth/minHeight are the smallest usable terminal; below this the UI
// refuses to render rather than showing an unreadable layout.
const (
	minWidth  = 40
	minHeight = 12
)

// decayInterval is how often the UI redraws with no new events, so activity
// markers age out of CURRENT and RECENT on their own. Without it a file edited
// hours ago would still look active while the agent sat idle.
const decayInterval = 2 * time.Second

// Git refresh pacing. The working tree changes far more slowly than the agent
// acts, and reading it costs a subprocess, so it is polled gently and bounded
// so a slow or wedged repository cannot pile up work.
const (
	gitInterval = 5 * time.Second
	gitTimeout  = 3 * time.Second
)

// eventMsg carries one normalized agent event into the message loop.
type eventMsg events.Event

// sourceClosedMsg reports that the event stream ended. The UI keeps showing
// the last observed state rather than blanking or inventing a new one.
type sourceClosedMsg struct{}

// decayMsg is the periodic redraw that lets activity age.
type decayMsg time.Time

// gitMsg carries a repository snapshot read off the UI goroutine.
type gitMsg git.Status

// Model is the Bubble Tea model. It renders a *state.State it mutates only
// through the reducer, plus the tree's expand/collapse flags.
type Model struct {
	st      *state.State
	scanner *project.Scanner
	stream  <-chan events.Event

	// hint explains where events are expected from. It is shown only until
	// the first one arrives, so an unwired setup is diagnosable instead of
	// looking like an idle agent.
	hint string

	// sender is set only when AgentLine owns the session; observing one
	// leaves the prompt field inert.
	sender Sender

	input   textinput.Model
	sendErr error

	focus          focusArea
	replyScroll    int
	activityScroll int

	// picker is an open choice, such as which model to run.
	picker picker

	// inspecting is the tree entry being looked at, which takes over the
	// mission column while it is set.
	inspecting *project.Node

	// caret places the terminal cursor, which is where an input method draws
	// the character it is composing. Nil leaves the cursor alone.
	caret *Caret

	// alert reaches the user when the agent stops and needs an answer. Nil
	// leaves the terminal quiet.
	alert *Alert

	tree   treeView
	width  int
	height int
}

// inputFocused reports whether typing goes to the prompt.
func (m Model) inputFocused() bool { return m.focus == focusPrompt }

// observedActivity reports whether anything has been heard from an agent.
//
// It asks the state, not the activity log: a prompt sets the mission and the
// status without logging an action, and counting log entries made the UI
// contradict itself — header WORKING and a filled MISSION above a NOW panel
// still claiming it had seen nothing.
func (m Model) observedActivity() bool {
	return m.st.Observed()
}

// New returns a Model rendering the given state.
//
// scanner may be nil, in which case directories cannot be expanded beyond what
// is already loaded. stream may be nil, in which case the UI is static.
func New(st *state.State, scanner *project.Scanner, stream <-chan events.Event) Model {
	return Model{st: st, scanner: scanner, stream: stream, input: newInput()}
}

// WithHint sets the message shown until the first event arrives.
func (m Model) WithHint(hint string) Model {
	m.hint = hint
	return m
}

// WithStream attaches the event source the UI listens to.
func (m Model) WithStream(stream <-chan events.Event) Model {
	m.stream = stream
	return m
}

// WithCaret lets the model place the terminal cursor, so an input method
// composes where the text is rather than at the corner of the screen.
func (m Model) WithCaret(caret *Caret) Model {
	m.caret = caret
	return m
}

// WithAlert lets AgentLine interrupt the user when the agent is blocked.
func (m Model) WithAlert(alert *Alert) Model {
	m.alert = alert
	return m
}

// WithSender makes the prompt field live, submitting to the given session.
func (m Model) WithSender(sender Sender) Model {
	m.sender = sender
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(waitForEvent(m.stream), decayTick(), readGit(m.st.Project.Root))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case eventMsg:
		event := events.Event(msg)
		m.applyEvent(event)

		// A turn just ended, so how full the context is has changed. Asking
		// now keeps the figure current without polling for it.
		if event.Type == events.AgentStatus && event.Status == events.StatusWaiting {
			return m, tea.Batch(waitForEvent(m.stream), m.askForContextUsage())
		}
		return m, waitForEvent(m.stream)

	case sourceClosedMsg:
		m.stream = nil
		return m, nil

	case decayMsg:
		// Nothing to change: returning re-renders, which is the point.
		return m, decayTick()

	case gitMsg:
		m.st.Project.Git = git.Status(msg)
		// Scheduled only once a read has landed, so a slow repository delays
		// the next look instead of stacking up overlapping subprocesses.
		return m, gitTick(m.st.Project.Root)

	case promptSentMsg:
		// A failed send is shown in the prompt bar rather than swallowed.
		m.sendErr = msg.err
		return m, nil

	case restartedMsg:
		m.sendErr = msg.err
		if msg.err == nil {
			// The new session carries none of the old context, so the
			// figures describing the old one no longer apply.
			m.st.ResetSession()
		}
		return m, nil

	case modelChangedMsg:
		// Recorded only once the session accepted it, and the agent reports
		// its own model on the next turn anyway.
		m.sendErr = msg.err
		if msg.err == nil {
			m.st.SetModel(msg.model)
		}
		return m, nil

	case modeChangedMsg:
		// Recorded only once the session accepted it, so the header never
		// shows a mode the agent is not actually in.
		m.sendErr = msg.err
		if msg.err == nil {
			m.st.SetPermissionMode(msg.mode)
		}
		return m, nil

	case tea.MouseMsg:
		return m.mouse(msg)

	case tea.KeyMsg:
		if m.inputFocused() {
			return m.typing(msg)
		}
		return m.navigating(msg)
	}

	m.syncScroll()
	return m, nil
}

// typing routes a key to the prompt field.
//
// While typing, only the keys that leave the field are intercepted: everything
// else is text, so "q" must not quit and the arrows must move the cursor
// rather than the tree.
func (m Model) typing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// An open choice takes the keys until it is answered, so the arrows move
	// the selection rather than the caret.
	if m.picker.open {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "up", "left":
			m.movePicker(-1)
		case "down", "right":
			m.movePicker(1)
		case "enter":
			return m, m.choosePicker()
		case "esc":
			m.closePicker()
		}
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "ctrl+n":
		return m, m.restartSession()
	case "shift+tab":
		return m, m.cyclePermissionMode()
	case "enter":
		return m, m.submitPrompt()

	case "tab":
		// Tab completes a command being typed, and only leaves the field
		// when there is nothing to complete.
		if m.completeSlash() {
			return m, nil
		}
		m.setFocus(focusTree)
		return m, nil

	case "esc":
		m.setFocus(focusTree)
		return m, nil
	}

	m.sendErr = nil // the user is composing again
	return m, m.updateInput(msg)
}

// navigating routes a key to the focused panel.
func (m Model) navigating(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	l := computeLayout(m.width, m.height)

	// A blocked agent takes the keys first. Nothing else on screen is waiting
	// on an answer, and every key spent navigating while it waits is the agent
	// sitting idle.
	if cmd, handled := m.answering(msg); handled {
		return m, cmd
	}

	switch msg.String() {
	// Esc is reserved for cancelling and closing popups (§22), so it is
	// deliberately not a quit key.
	case "q", "ctrl+c":
		return m, tea.Quit

	case "ctrl+n":
		return m, m.restartSession()

	case "shift+tab":
		return m, m.cyclePermissionMode()

	case "tab":
		return m, m.setFocus(m.cycle(m.focus, l))
	case "i":
		if m.canSend() {
			return m, m.setFocus(focusPrompt)
		}

	case "up":
		m.scrollFocused(-1, l)
	case "down":
		m.scrollFocused(1, l)
	case "pgup":
		m.scrollFocused(-l.ActivityRows-1, l)
	case "pgdown":
		m.scrollFocused(l.ActivityRows+1, l)

	// Expanding and collapsing only mean something in the tree.
	case "right":
		if m.focus == focusTree {
			m.expand()
			m.syncScroll()
		}
	case "left":
		if m.focus == focusTree {
			m.collapse()
			m.syncScroll()
		}
	case "enter":
		// Enter looks at the selected entry rather than toggling it, since
		// left and right already open and close directories.
		if m.focus == focusTree {
			m.inspecting = m.selected()
		}

	case "esc":
		m.inspecting = nil
	}
	return m, nil
}

// mouse routes a click or wheel movement to the panel under the pointer.
func (m Model) mouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.width < minWidth || m.height < minHeight {
		return m, nil
	}
	l := computeLayout(m.width, m.height)

	area, ok := m.focusAt(msg.X, msg.Y, l)
	if !ok {
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		// The wheel acts on whatever is under the pointer, which is what a
		// mouse user expects, and without stealing focus from elsewhere.
		if area == focusPrompt {
			return m, nil
		}
		focus := m.focus
		m.focus = area
		if msg.Button == tea.MouseButtonWheelUp {
			m.scrollFocused(-1, l)
		} else {
			m.scrollFocused(1, l)
		}
		m.focus = focus

	case tea.MouseButtonLeft:
		if msg.Action == tea.MouseActionPress {
			return m, m.setFocus(area)
		}
	}
	return m, nil
}

// applyEvent folds one event into the application state. The reducer owns
// every state change; the tree is opened so the touched file is visible,
// which is the whole point of the project panel.
func (m *Model) applyEvent(e events.Event) {
	if !e.Valid() {
		return // ignore malformed input without disturbing the selection
	}
	blocked := m.st.Agent.Ask != nil
	m.st.Apply(e)

	// Raised on the edge, not the state: an ask that is still unanswered is
	// still on screen, and ringing every time the clock redraws would train
	// the user to ignore the one time it matters.
	if !blocked && m.st.Agent.Ask != nil && m.alert != nil {
		m.alert.Raise(askAlert(m.st.Agent.Ask))
	}

	if e.Path != "" && m.scanner != nil {
		// Revealing inserts rows above the selection, so the cursor has to
		// follow the node the user chose rather than the index it sat at.
		m.keepingSelection(func() {
			m.showPath(e)
		})
	}
	m.syncScroll()
}

// showPath makes the file an event touched visible in the tree.
//
// The tree is a snapshot of one scan, so a file the agent just created is not
// in it. A completed write can be found by re-reading the directory; an
// announced one has nothing on disk yet, so its node is inserted directly and
// replaced by the real entry at the next refresh.
func (m *Model) showPath(e events.Event) {
	tree := m.st.Project.Tree

	if project.Find(tree, e.Path) == nil {
		if e.Type == events.FilePending {
			project.Insert(tree, e.Path)
		} else {
			m.scanner.RefreshParent(tree, e.Path)
		}
	}
	m.scanner.Reveal(tree, e.Path)
}

// keepingSelection runs a change that may insert or remove tree rows, then
// restores the cursor to whatever node was selected before.
func (m *Model) keepingSelection(change func()) {
	selected := m.selected()
	change()

	if selected == nil {
		return
	}
	for i, row := range m.rows() {
		if row.Node == selected {
			m.tree.cursor = i
			return
		}
	}
}

// decayTick schedules the next idle redraw.
func decayTick() tea.Cmd {
	return tea.Tick(decayInterval, func(t time.Time) tea.Msg { return decayMsg(t) })
}

// gitTick schedules the next repository read.
func gitTick(root string) tea.Cmd {
	return tea.Tick(gitInterval, func(time.Time) tea.Msg {
		return readGit(root)()
	})
}

// readGit reads the repository off the UI goroutine, bounded by a timeout so a
// wedged git cannot hold the interface. A directory that is not a repository
// yields the zero Status, which renders as no Git information at all.
func readGit(root string) tea.Cmd {
	if root == "" {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
		defer cancel()
		return gitMsg(git.Load(ctx, root))
	}
}

// waitForEvent blocks on the next event from the source. Bubble Tea runs it
// off the UI goroutine, and Update re-issues it to keep the stream pumping.
func waitForEvent(stream <-chan events.Event) tea.Cmd {
	if stream == nil {
		return nil
	}
	return func() tea.Msg {
		e, ok := <-stream
		if !ok {
			return sourceClosedMsg{}
		}
		return eventMsg(e)
	}
}
