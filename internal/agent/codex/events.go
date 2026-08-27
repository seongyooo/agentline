// Package codex adapts the Codex CLI's event stream into AgentLine's
// normalized event model.
//
// Everything Codex-specific — event names, item shapes, flags — is confined to
// this package, the same way the Claude adapter confines its own. Nothing
// downstream may depend on these types.
//
// The schema is Codex's published one: `codex exec --json` writes a JSONL
// stream of thread events, each carrying an item that says what the agent did.
// It is better structured than most: a command reports its real exit code, and
// a file change says whether it added, updated or deleted.
package codex

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/seongyooo/agentline/internal/agent"
	"github.com/seongyooo/agentline/internal/events"
)

// SourceName identifies this adapter in the events it produces.
const SourceName = "codex"

// threadEvent is one line of the JSONL stream.
type threadEvent struct {
	Type  string     `json:"type"`
	Item  threadItem `json:"item"`
	Usage *usage     `json:"usage"`

	// Message carries a fatal stream error.
	Message string `json:"message"`

	// Error carries a failed turn.
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// threadItem is what the agent did. Only the fields AgentLine acts on are
// decoded; captured output and reasoning text are deliberately left out.
type threadItem struct {
	ID   string `json:"id"`
	Type string `json:"type"`

	// Set on command_execution.
	Command  string `json:"command"`
	ExitCode *int   `json:"exit_code"`
	Status   string `json:"status"`

	// Set on file_change.
	Changes []struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	} `json:"changes"`

	// Set on agent_message and error.
	Text    string `json:"text"`
	Message string `json:"message"`
}

type usage struct {
	InputTokens           int `json:"input_tokens"`
	CachedInputTokens     int `json:"cached_input_tokens"`
	CacheWriteInputTokens int `json:"cache_write_input_tokens"`
	OutputTokens          int `json:"output_tokens"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens"`
}

// Item and event names, kept here rather than spread through the code.
const (
	eventThreadStarted = "thread.started"
	eventTurnStarted   = "turn.started"
	eventTurnCompleted = "turn.completed"
	eventTurnFailed    = "turn.failed"
	eventItemStarted   = "item.started"
	eventItemCompleted = "item.completed"
	eventError         = "error"

	itemCommand    = "command_execution"
	itemFileChange = "file_change"
	itemAgentMsg   = "agent_message"
	itemError      = "error"
	statusFailed   = "failed"
	changeAdd      = "add"
	changeDelete   = "delete"
)

// translator converts Codex events into normalized ones. It is pure, so the
// mapping can be tested without running an agent.
type translator struct {
	root string
	now  func() time.Time
}

func newTranslator(root string) *translator {
	return &translator{root: root, now: time.Now}
}

// translateLine maps one line of the stream to zero or more events. A line
// AgentLine cannot read produces nothing rather than a guess.
func (t *translator) translateLine(line []byte) []events.Event {
	var e threadEvent
	if err := json.Unmarshal(line, &e); err != nil {
		return nil
	}

	switch e.Type {
	case eventThreadStarted, eventTurnStarted:
		return []events.Event{t.status(events.StatusWorking)}

	case eventItemStarted:
		return t.started(e.Item)

	case eventItemCompleted:
		return t.completed(e.Item)

	case eventTurnCompleted:
		// The turn ended, so the agent is waiting for the user again. It is
		// not reported as DONE: whether the work is finished is not something
		// the stream says.
		out := []events.Event{t.status(events.StatusWaiting)}
		if e.Usage != nil {
			out = append(out, t.event(events.SessionInfo, func(ev *events.Event) {
				ev.Session = &events.Session{
					// Cached input still counts towards the context the turn
					// carried, which is the number that says what it cost.
					InputTokens: e.Usage.InputTokens +
						e.Usage.CachedInputTokens + e.Usage.CacheWriteInputTokens,
					OutputTokens: e.Usage.OutputTokens,
				}
			}))
		}
		return out

	case eventTurnFailed:
		message := "the turn failed"
		if e.Error != nil {
			message = firstLine(e.Error.Message)
		}
		return []events.Event{t.failure(message)}

	case eventError:
		return []events.Event{t.failure(firstLine(e.Message))}
	}
	return nil
}

// started handles an item that has begun.
//
// Only commands are announced before they finish, because a command is slow
// enough that NOW should say it is running. A file change is reported once it
// has actually been applied — Codex emits the item only when the patch
// succeeds or fails, so there is nothing to claim early.
func (t *translator) started(item threadItem) []events.Event {
	if item.Type != itemCommand {
		return nil
	}
	return []events.Event{t.event(events.CommandStart, func(e *events.Event) {
		e.Command = item.Command
	})}
}

// completed handles an item that has reached a terminal state.
func (t *translator) completed(item threadItem) []events.Event {
	switch item.Type {
	case itemCommand:
		return []events.Event{t.event(events.CommandEnd, func(e *events.Event) {
			e.Command = item.Command
			e.Failed = item.Status == statusFailed
			// Codex reports the real exit code, so unlike the Claude adapter
			// there is a number to carry rather than only the fact of failure.
			e.ExitCode = item.ExitCode
			if e.ExitCode != nil && *e.ExitCode != 0 {
				e.Failed = true
			}
		})}

	case itemFileChange:
		return t.fileChanges(item)

	case itemAgentMsg:
		if text := strings.TrimSpace(item.Text); text != "" {
			return []events.Event{t.event(events.AgentReply, func(e *events.Event) {
				e.Message = text
			})}
		}

	case itemError:
		return []events.Event{t.failure(firstLine(item.Message))}
	}
	return nil
}

// fileChanges maps a patch to one event per file it touched.
func (t *translator) fileChanges(item threadItem) []events.Event {
	if item.Status == statusFailed {
		// A patch that failed changed nothing, so no file event is emitted.
		return []events.Event{t.failure("the patch failed")}
	}

	var out []events.Event
	for _, change := range item.Changes {
		rel := agent.RelativePath(t.root, change.Path)
		if rel == "" {
			continue // outside the project, or no path reported
		}

		kind := events.FileEdit
		switch change.Kind {
		case changeAdd:
			kind = events.FileCreate
		case changeDelete:
			kind = events.FileDelete
		}
		out = append(out, t.event(kind, func(e *events.Event) { e.Path = rel }))
	}
	return out
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

func (t *translator) failure(message string) events.Event {
	return t.event(events.AgentError, func(e *events.Event) { e.Message = message })
}

// prompt is the event for an instruction submitted through AgentLine.
func (t *translator) prompt(text string) events.Event {
	return events.Event{
		Type:      events.UserPrompt,
		Message:   text,
		Timestamp: t.now(),
		Source:    SourceName,
	}
}

// firstLine trims a message to its first line, for a one-line UI.
func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
