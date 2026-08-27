package claude

import (
	"strings"
	"time"

	"github.com/seongyooo/agentline/internal/agent"
	"github.com/seongyooo/agentline/internal/events"
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
// events AgentLine has no meaning for produce nothing, rather than a guess.
func (t *translator) translate(p payload) []events.Event {
	switch p.HookEventName {
	case hookUserPromptSubmit:
		// The prompt is the user's own statement of what they want, so it is
		// carried through verbatim; deriving a mission from it is the state
		// layer's job. A prompt event also means the agent is now busy.
		if p.Prompt != "" {
			return []events.Event{t.event(events.UserPrompt, func(e *events.Event) {
				e.Message = p.Prompt
			})}
		}
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
				e.Message = p.ToolInput.Description
			})}
		}
		// A write is announced as claimed, so the file shows up in the tree
		// while it is being written. The claim is dropped if the write is
		// refused or the turn ends without it landing.
		if writesFile(p.ToolName) {
			if rel := t.relative(p.ToolInput.path()); rel != "" {
				return []events.Event{t.event(events.FilePending, func(e *events.Event) {
					e.Path = rel
					e.Message = p.ToolInput.Description
				})}
			}
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
		return nil // a tool AgentLine has no file meaning for
	}
	rel := t.relative(p.ToolInput.path())
	if rel == "" {
		return nil // outside the project, or no path reported
	}
	return []events.Event{t.event(kind, func(e *events.Event) { e.Path = rel })}
}

// writesFile reports whether a tool changes a file, as opposed to only
// reading one. Only writes are worth announcing before they happen.
func writesFile(tool string) bool {
	switch tool {
	case toolEdit, toolWrite, toolNotebookEdit:
		return true
	}
	return false
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

// relative converts a path the agent reported into a root-relative one. The
// rule is shared with the other adapters so it cannot drift between them.
func (t *translator) relative(p string) string {
	return agent.RelativePath(t.root, p)
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
