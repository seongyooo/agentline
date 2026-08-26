// Package state holds AgentView's application state and the reducer that
// advances it from normalized events. It contains no rendering logic.
package state

import (
	"time"

	"github.com/seonl/agentview/internal/events"
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
}

// ProjectState is where the agent is working.
type ProjectState struct {
	Root           string
	Tree           *project.Node
	ActivityByPath map[string]time.Time
}

// State is the single source of truth handed to the renderer.
type State struct {
	Agent   AgentState
	Project ProjectState
}

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
		if e.ExitCode != nil && *e.ExitCode != 0 {
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
			// Only a summary: a specific action already on screen is more
			// informative and must not be overwritten by it.
			if s.Agent.Now.Kind == ActionIdle {
				s.setNow(Action{Kind: ActionWorking, At: e.Timestamp})
			}
		}

	case events.AgentError:
		s.Agent.Status = events.StatusError
		s.setNow(Action{Kind: ActionFailed, Target: e.Message, At: e.Timestamp})
	}
}

func (s *State) recordFile(kind ActionKind, e events.Event) {
	s.setNow(Action{Kind: kind, Target: e.Path, At: e.Timestamp})
	s.Agent.Status = events.StatusWorking
	s.Project.ActivityByPath[e.Path] = e.Timestamp
}

func (s *State) setNow(a Action) {
	s.Agent.Now = a
	s.push(a)
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
