package claude

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/seonl/agentview/internal/events"
)

// SourceName identifies this adapter in the events it produces.
const SourceName = "claude-code"

// translator converts hook payloads into normalized events. It is pure and
// carries no I/O, so the mapping can be tested without an agent or a server.
type translator struct {
	root string // absolute project root, used to relativize paths
	now  func() time.Time
}

func newTranslator(root string) *translator {
	return &translator{root: root, now: time.Now}
}

// translate maps one hook payload to zero or more normalized events. Hook
// events AgentView has no meaning for produce nothing, rather than a guess.
func (t *translator) translate(p payload) []events.Event {
	switch p.HookEventName {
	case hookUserPromptSubmit:
		// The user just gave the agent work; it is now busy.
		return []events.Event{t.status(events.StatusWorking)}

	case hookPreToolUse:
		// Only commands are announced before they run, because a command is
		// slow enough that NOW should say it is running. File tools are
		// reported once they succeed: PreToolUse only means "about to", and a
		// tool can still be denied — announcing a write that never happened
		// would be inventing activity.
		if shellTools[p.ToolName] {
			return []events.Event{t.event(events.CommandStart, func(e *events.Event) {
				e.Command = p.ToolInput.Command
			})}
		}
		return nil

	case hookPostToolUse:
		return t.toolFinished(p, false)

	case hookPostToolUseFailure:
		return t.toolFinished(p, true)

	case hookStop, hookSessionEnd:
		// The turn ended, so the agent is waiting for the user. It is not
		// reported as DONE: whether the mission is complete is not observable.
		return []events.Event{t.status(events.StatusWaiting)}
	}
	return nil
}

// toolFinished maps a completed tool call, successful or not.
func (t *translator) toolFinished(p payload, failed bool) []events.Event {
	if shellTools[p.ToolName] {
		return []events.Event{t.event(events.CommandEnd, func(e *events.Event) {
			e.Command = p.ToolInput.Command
			e.Failed = failed
			if failed {
				e.Message = firstLine(p.Error)
			}
		})}
	}

	if failed {
		// A failed file tool changed nothing, so no file event is emitted.
		return []events.Event{t.event(events.AgentError, func(e *events.Event) {
			e.Message = firstLine(p.Error)
			e.Path = t.relative(p.ToolInput.path())
		})}
	}

	kind, ok := fileEventType(p.ToolName)
	if !ok {
		return nil // a tool AgentView has no file meaning for
	}
	rel := t.relative(p.ToolInput.path())
	if rel == "" {
		return nil // outside the project, or no path reported
	}
	return []events.Event{t.event(kind, func(e *events.Event) { e.Path = rel })}
}

// fileEventType maps a tool name to the kind of file change it makes.
func fileEventType(tool string) (events.Type, bool) {
	switch tool {
	case toolRead:
		return events.FileRead, true
	case toolEdit, toolNotebookEdit:
		return events.FileEdit, true
	case toolWrite:
		return events.FileCreate, true
	}
	return "", false
}

// relative converts an absolute hook path to a root-relative slash path.
//
// Hook payloads carry full OS paths, while the event model and the project
// tree use root-relative slash paths. Anything outside the project returns
// empty so it is dropped rather than displayed under a path it does not have.
func (t *translator) relative(p string) string {
	if p == "" || t.root == "" {
		return ""
	}

	rel, err := filepath.Rel(t.root, p)
	if err != nil {
		return ""
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return ""
	}
	return rel
}

func (t *translator) event(kind events.Type, set func(*events.Event)) events.Event {
	e := events.Event{Type: kind, Timestamp: t.now(), Source: SourceName}
	set(&e)
	return e
}

func (t *translator) status(s events.Status) events.Event {
	return events.Event{
		Type:      events.AgentStatus,
		Status:    s,
		Timestamp: t.now(),
		Source:    SourceName,
	}
}

// firstLine trims a multi-line error to its first line, which is the part
// worth showing in a one-line UI.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
