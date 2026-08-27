package claude

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/seonl/agentview/internal/events"
)

// streamEvent is one line of Claude Code's streaming JSON output. Only the
// fields AgentView acts on are decoded; token counts, thinking blocks, and
// rate-limit notices are read past.
type streamEvent struct {
	Type    string        `json:"type"`
	Subtype string        `json:"subtype"`
	Message streamMessage `json:"message"`
	IsError bool          `json:"is_error"`
}

type streamMessage struct {
	Content []streamBlock `json:"content"`
}

// streamBlock is one content block of a message.
type streamBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`

	// Set on tool_use.
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Input toolInput `json:"input"`

	// Set on tool_result.
	ToolUseID string `json:"tool_use_id"`
	IsError   bool   `json:"is_error"`
}

// streamTranslator converts streaming output into normalized events.
//
// Unlike the hook translator it has to remember things: a tool_result names
// only the id of the call it answers, so what the agent actually did is known
// only by pairing it with the earlier tool_use.
type streamTranslator struct {
	*translator
	pending map[string]streamBlock
	tasks   *taskList
}

func newStreamTranslator(root string) *streamTranslator {
	return &streamTranslator{
		translator: newTranslator(root),
		pending:    map[string]streamBlock{},
		tasks:      newTaskList(),
	}
}

// translateLine maps one line of output to zero or more normalized events.
func (t *streamTranslator) translateLine(line []byte) []events.Event {
	var e streamEvent
	if err := json.Unmarshal(line, &e); err != nil {
		return nil // a line AgentView cannot read is not one it should guess at
	}

	switch e.Type {
	case "assistant":
		return t.assistant(e)
	case "user":
		return t.toolResults(e)
	case "result":
		// The turn is over, so the agent is waiting for the user again. It is
		// not reported as DONE: whether the mission is complete is not
		// something the transcript says.
		return []events.Event{t.status(events.StatusWaiting)}
	case "system":
		if e.Subtype == "init" {
			return []events.Event{t.status(events.StatusWorking)}
		}
	}
	return nil
}

// assistant handles what the agent said and the tools it is about to run.
func (t *streamTranslator) assistant(e streamEvent) []events.Event {
	var out []events.Event

	for _, block := range e.Message.Content {
		switch block.Type {
		case "text":
			if text := strings.TrimSpace(block.Text); text != "" {
				out = append(out, t.event(events.AgentReply, func(ev *events.Event) {
					ev.Message = text
				}))
			}

		case "tool_use":
			// Remembered so the matching result can say what was done.
			t.pending[block.ID] = block

			// The agent's own task list is the only honest source of
			// progress, so its bookkeeping calls are read as they go by.
			if done, total, ok := t.tasks.observe(block.Name, block.Input.taskFields); ok {
				out = append(out, t.event(events.TaskProgress, func(ev *events.Event) {
					ev.Done, ev.Total = done, total
				}))
			}

			// Only commands are announced before they run: a command is slow
			// enough that NOW should say it is running, while a file tool is
			// reported once it has actually succeeded.
			if shellTools[block.Name] {
				out = append(out, t.event(events.CommandStart, func(ev *events.Event) {
					ev.Command = block.Input.Command
				}))
			}
		}
	}
	return out
}

// toolResults handles the outcome of tool calls, paired with what started them.
func (t *streamTranslator) toolResults(e streamEvent) []events.Event {
	var out []events.Event

	for _, block := range e.Message.Content {
		if block.Type != "tool_result" {
			continue
		}
		call, ok := t.pending[block.ToolUseID]
		if !ok {
			continue // a result for a call this translator never saw
		}
		delete(t.pending, block.ToolUseID)

		out = append(out, t.finished(call, block.IsError)...)
	}
	return out
}

// finished maps a completed tool call to its event.
func (t *streamTranslator) finished(call streamBlock, failed bool) []events.Event {
	if shellTools[call.Name] {
		return []events.Event{t.event(events.CommandEnd, func(ev *events.Event) {
			ev.Command = call.Input.Command
			ev.Failed = failed
		})}
	}

	kind, ok := fileEventType(call.Name)
	if !ok {
		return nil // a tool with no file meaning
	}
	rel := t.relative(call.Input.path())
	if rel == "" {
		return nil // outside the project, or no path reported
	}

	if failed {
		// A failed file tool changed nothing, so no file event is emitted.
		return []events.Event{t.event(events.AgentError, func(ev *events.Event) {
			ev.Path = rel
			ev.Message = "tool failed: " + call.Name
		})}
	}
	return []events.Event{t.event(kind, func(ev *events.Event) { ev.Path = rel })}
}

// prompt is the event for an instruction the user submitted through AgentView.
func (t *streamTranslator) prompt(text string) events.Event {
	return events.Event{
		Type:      events.UserPrompt,
		Message:   text,
		Timestamp: time.Now(),
		Source:    SourceName,
	}
}
