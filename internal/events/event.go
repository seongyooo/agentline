// Package events defines the normalized, provider-neutral event model.
//
// Agent adapters translate provider-specific output into these events.
// Nothing downstream of an adapter may depend on provider-specific formats.
package events

import "time"

// Type identifies what an agent observably did.
type Type string

const (
	FileRead     Type = "file_read"
	FileEdit     Type = "file_edit"
	FileCreate   Type = "file_create"
	FileDelete   Type = "file_delete"
	CommandStart Type = "command_start"
	CommandEnd   Type = "command_end"
	AgentStatus  Type = "agent_status"
	AgentError   Type = "agent_error"

	// UserPrompt is the instruction the user gave the agent. It carries the
	// prompt text in Message and is what MISSION is derived from.
	UserPrompt Type = "user_prompt"

	// AgentReply is what the agent said back, in Message. AgentView shows
	// only enough of it to know the turn landed; it is not a transcript.
	AgentReply Type = "agent_reply"

	// TaskProgress reports the agent's own task list, in Done and Total.
	// It is a count of what the agent said it planned and finished — never
	// an estimate of how complete the work is.
	TaskProgress Type = "task_progress"
)

// Status is the agent's observable lifecycle state.
type Status string

const (
	StatusWorking    Status = "working"
	StatusWaiting    Status = "waiting"
	StatusNeedsInput Status = "needs_input"
	StatusDone       Status = "done"
	StatusError      Status = "error"
)

// Event is a single observed agent action. Fields not relevant to a given
// Type are left zero; adapters must not populate a field they cannot observe.
type Event struct {
	Type      Type      `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`

	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
	Status  Status `json:"status,omitempty"`

	// Message is free text whose meaning depends on Type: the failure
	// description for an error, the instruction for a UserPrompt.
	Message string `json:"message,omitempty"`

	// Failed marks a command that did not succeed. It is separate from
	// ExitCode because some agents report only that a command failed, without
	// a numeric status; AgentView records what it observed and does not
	// invent a code it was never given.
	Failed bool `json:"failed,omitempty"`

	// ExitCode is set only when the agent actually reported one.
	ExitCode *int `json:"exit_code,omitempty"`

	// Done and Total carry a TaskProgress count.
	Done  int `json:"done,omitempty"`
	Total int `json:"total,omitempty"`
}

// Valid reports whether the event carries the fields its Type requires.
// Malformed events are dropped rather than allowed to corrupt state.
func (e Event) Valid() bool {
	if e.Source == "" || e.Timestamp.IsZero() {
		return false
	}
	switch e.Type {
	case FileRead, FileEdit, FileCreate, FileDelete:
		return e.Path != ""
	case CommandStart, CommandEnd:
		return e.Command != ""
	case AgentStatus:
		switch e.Status {
		case StatusWorking, StatusWaiting, StatusNeedsInput, StatusDone, StatusError:
			return true
		}
		return false
	case AgentError:
		return true
	case UserPrompt, AgentReply:
		return e.Message != ""
	case TaskProgress:
		// A count of nothing says nothing, and Done may not exceed Total.
		return e.Total > 0 && e.Done >= 0 && e.Done <= e.Total
	}
	return false
}
