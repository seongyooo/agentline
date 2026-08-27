// Package state holds AgentView's application state and the reducer that
// advances it from normalized events. It contains no rendering logic.
package state

import (
	"strings"
	"time"

	"github.com/seonl/agentview/internal/events"
	"github.com/seonl/agentview/internal/git"
	"github.com/seonl/agentview/internal/project"
)

// maxActivity bounds in-memory history; the UI shows far fewer.
const maxActivity = 50

// ActionKind is the observable category of what the agent is doing.
type ActionKind string

const (
	ActionIdle ActionKind = "idle"
	// ActionWorking covers observable work with no more specific action yet:
	// the agent reported that it is busy but has not touched a file or run a
	// command. It is a summary of reported status, never inferred detail.
	ActionWorking  ActionKind = "working"
	ActionReading  ActionKind = "reading"
	ActionEditing  ActionKind = "editing"
	ActionCreating ActionKind = "creating"
	ActionDeleting ActionKind = "deleting"
	ActionRunning  ActionKind = "running"
	ActionWaiting  ActionKind = "waiting"
	ActionDone     ActionKind = "done"
	ActionFailed   ActionKind = "failed"
)

// Action is a single observable agent action.
type Action struct {
	Kind   ActionKind
	Target string
	At     time.Time
}

// ActivityLevel expresses how recently a path was touched.
type ActivityLevel int

const (
	Inactive ActivityLevel = iota
	Modified
	Recent
	Current
)

// Decay thresholds for ActivityLevel.
const (
	currentWindow = 30 * time.Second
	recentWindow  = 5 * time.Minute
	modifiedWFor  = 30 * time.Minute
)

// AgentState is what the agent is doing.
type AgentState struct {
	Agent    string
	Status   events.Status
	Now      Action
	Mission  string
	Next     string
	Activity []Action

	// Reply is the last thing the agent said, kept so the user can tell a
	// turn landed. AgentView shows a line of it, never the conversation.
	Reply string

	// missionPinned marks a mission the user set explicitly, which observed
	// prompts must not overwrite.
	missionPinned bool
}

// ProjectState is where the agent is working.
type ProjectState struct {
	Root           string
	Tree           *project.Node
	ActivityByPath map[string]time.Time

	// Git is the last repository snapshot. Its zero value means there is no
	// repository, or none has been read yet.
	Git git.Status
}

// State is the single source of truth handed to the renderer.
type State struct {
	Agent   AgentState
	Project ProjectState

	// observed records whether anything has ever been heard from an agent.
	// It is not the same as having activity to show: a prompt sets the
	// mission and the status without logging an action.
	observed bool
}

// Observed reports whether any agent event has been applied. The UI uses it to
// tell "nothing is wired up" apart from "the agent has nothing to report".
func (s *State) Observed() bool { return s.observed }

// New returns an empty state rooted at the given directory.
func New(root string) *State {
	return &State{
		Agent:   AgentState{Agent: "—", Status: events.StatusWaiting, Now: Action{Kind: ActionIdle}},
		Project: ProjectState{Root: root, ActivityByPath: map[string]time.Time{}},
	}
}

// Apply advances the state by one event. Invalid events are ignored so that a
// single malformed event cannot corrupt state or crash the application.
func (s *State) Apply(e events.Event) {
	if !e.Valid() {
		return
	}
	s.observed = true
	s.Agent.Agent = e.Source

	switch e.Type {
	case events.FileRead:
		s.recordFile(ActionReading, e)
	case events.FileEdit:
		s.recordFile(ActionEditing, e)
	case events.FileCreate:
		s.recordFile(ActionCreating, e)
	case events.FileDelete:
		s.recordFile(ActionDeleting, e)

	case events.CommandStart:
		s.setNow(Action{Kind: ActionRunning, Target: e.Command, At: e.Timestamp})
		s.Agent.Status = events.StatusWorking

	case events.CommandEnd:
		// NOW must move off "Running": the command is no longer running.
		kind := ActionDone
		if e.Failed || (e.ExitCode != nil && *e.ExitCode != 0) {
			kind = ActionFailed
			s.Agent.Status = events.StatusError
		}
		s.setNow(Action{Kind: kind, Target: e.Command, At: e.Timestamp})

	case events.AgentStatus:
		s.Agent.Status = e.Status
		switch e.Status {
		case events.StatusWaiting, events.StatusNeedsInput:
			s.setNow(Action{Kind: ActionWaiting, At: e.Timestamp})
		case events.StatusDone:
			s.setNow(Action{Kind: ActionDone, At: e.Timestamp})
		case events.StatusWorking:
			s.markWorking(e.Timestamp)
		}

	case events.AgentError:
		s.Agent.Status = events.StatusError
		s.setNow(Action{Kind: ActionFailed, Target: e.Message, At: e.Timestamp})

	case events.UserPrompt:
		s.setMission(e.Message)
		s.Agent.Reply = "" // the previous answer is about to be superseded
		s.markWorking(e.Timestamp)

	case events.AgentReply:
		s.Agent.Reply = headline(e.Message)
	}
}

// markWorking records that the agent is busy.
//
// Every path that learns the agent is working goes through here, so NOW can
// never contradict the header: reporting WORKING above an Idle NOW is exactly
// the inconsistency this prevents. A specific action already on screen is more
// informative than the summary and is left alone.
func (s *State) markWorking(at time.Time) {
	s.Agent.Status = events.StatusWorking
	if s.Agent.Now.Kind == ActionIdle {
		s.setNow(Action{Kind: ActionWorking, At: at})
	}
}

// PinMission sets a mission the user supplied. Observed prompts will not
// overwrite it: an explicit instruction outranks anything derived.
func (s *State) PinMission(mission string) {
	s.Agent.Mission = mission
	s.Agent.missionPinned = mission != ""
}

// setMission derives the mission from a user prompt.
//
// The prompt is the user's own statement of intent, so this is a summary of
// something observed, not an inference about it — no model is consulted. The
// most recent prompt wins, because an older instruction the user has since
// moved on from would describe work that is no longer happening.
func (s *State) setMission(prompt string) {
	if s.Agent.missionPinned {
		return
	}
	if mission := headline(prompt); mission != "" {
		s.Agent.Mission = mission
	}
}

// headline reduces a prompt to its first line with whitespace collapsed. It
// only trims; it never rewrites the user's words.
func headline(prompt string) string {
	line := prompt
	if i := strings.IndexAny(line, "\r\n"); i >= 0 {
		line = line[:i]
	}
	return strings.Join(strings.Fields(line), " ")
}

func (s *State) recordFile(kind ActionKind, e events.Event) {
	s.setNow(Action{Kind: kind, Target: e.Path, At: e.Timestamp})
	s.Agent.Status = events.StatusWorking
	s.Project.ActivityByPath[e.Path] = e.Timestamp
}

// setNow updates the current action and records it. Repeating the action
// already on screen is not new activity, so it is not logged again: several
// hooks can report the same state, and the log should show what changed.
func (s *State) setNow(a Action) {
	repeat := s.Agent.Now.Kind == a.Kind && s.Agent.Now.Target == a.Target
	s.Agent.Now = a
	if !repeat {
		s.push(a)
	}
}

func (s *State) push(a Action) {
	s.Agent.Activity = append(s.Agent.Activity, a)
	if n := len(s.Agent.Activity); n > maxActivity {
		s.Agent.Activity = s.Agent.Activity[n-maxActivity:]
	}
}

// Recent returns up to n most recent actions, newest last.
func (s *State) Recent(n int) []Action {
	if len(s.Agent.Activity) <= n {
		return s.Agent.Activity
	}
	return s.Agent.Activity[len(s.Agent.Activity)-n:]
}

// NodeLevel reports a node's activity level. A directory inherits the highest
// level among its descendants, so an active file makes its ancestors visible.
func (s *State) NodeLevel(n *project.Node, now time.Time) ActivityLevel {
	if n == nil {
		return Inactive
	}
	level := s.LevelAt(n.Path, now)
	for _, c := range n.Children {
		if l := s.NodeLevel(c, now); l > level {
			level = l
		}
	}
	return level
}

// LevelAt reports how active a path is relative to now, decaying over time.
func (s *State) LevelAt(path string, now time.Time) ActivityLevel {
	at, ok := s.Project.ActivityByPath[path]
	if !ok {
		return Inactive
	}
	switch age := now.Sub(at); {
	case age < currentWindow:
		return Current
	case age < recentWindow:
		return Recent
	case age < modifiedWFor:
		return Modified
	default:
		return Inactive
	}
}
