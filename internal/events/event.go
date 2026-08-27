// Package events defines the normalized, provider-neutral event model.
//
// Agent adapters translate provider-specific output into these events.
// Nothing downstream of an adapter may depend on provider-specific formats.
package events

import (
	"time"
)

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

	// AgentReply is what the agent said back, in Message. AgentLine shows
	// only enough of it to know the turn landed; it is not a transcript.
	AgentReply Type = "agent_reply"

	// FilePending marks a file the agent has said it is about to write. The
	// write has not happened yet and may still be refused, so it is shown as
	// claimed rather than done, and is cleared either way.
	FilePending Type = "file_pending"

	// SessionInfo reports what the agent told AgentLine about the session
	// itself: the model, and how much of a rate limit is used.
	SessionInfo Type = "session_info"

	// TaskProgress reports the agent's own task list, in Done and Total.
	// It is a count of what the agent said it planned and finished — never
	// an estimate of how complete the work is.
	TaskProgress Type = "task_progress"

	// PermissionAsk is the agent stopping to ask whether it may do something
	// it is not allowed to do unattended. It is the one event that is not a
	// report of the past: the agent is blocked until it is answered, which is
	// what makes it the only thing on screen worth interrupting someone for.
	PermissionAsk Type = "permission_ask"

	// PermissionAnswered closes an ask, whoever closed it — the user, a
	// timeout, or AgentLine shutting down. Ask carries the ID that was
	// answered so a late answer to an old ask cannot clear a new one.
	PermissionAnswered Type = "permission_answered"
)

// Task is one entry of the agent's own task list.
//
// The words are the agent's. AgentLine does not break work into steps — that
// is a judgement about the work, and §24 rules it out — but an agent that
// keeps a list has already made those judgements and is sending them. Reading
// them is observation; the alternative was throwing away the text and keeping
// the count, which is what happened until now.
type Task struct {
	// Text is what the agent said it would do, in the imperative.
	Text string `json:"text,omitempty"`

	// Doing is the agent's present-tense wording for the same task, which it
	// supplies so a UI can say what is happening rather than what is planned.
	Doing string `json:"doing,omitempty"`

	Done bool `json:"done,omitempty"`

	// Now marks the task the agent says it is on. There is normally exactly
	// one, but nothing is assumed: the agent decides, and it can decide none.
	Now bool `json:"now,omitempty"`
}

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
	// a numeric status; AgentLine records what it observed and does not
	// invent a code it was never given.
	Failed bool `json:"failed,omitempty"`

	// ExitCode is set only when the agent actually reported one.
	ExitCode *int `json:"exit_code,omitempty"`

	// Done and Total carry a TaskProgress count.
	Done  int `json:"done,omitempty"`
	Total int `json:"total,omitempty"`

	// Tasks is the agent's own list, when it reported one with its progress.
	// The count is a summary of this; both travel together so nothing has to
	// be recomputed from a number.
	Tasks []Task `json:"tasks,omitempty"`

	// Session carries a SessionInfo report.
	Session *Session `json:"session,omitempty"`

	// Ask carries a PermissionAsk, or the ID being closed on a
	// PermissionAnswered.
	Ask *Ask `json:"ask,omitempty"`
}

// Ask is a permission request the agent is blocked on.
//
// It holds what is needed to decide and nothing else. The provider's raw tool
// input stays inside the adapter that received it: answering means echoing it
// back unchanged, which is the adapter's business, and putting a
// provider-shaped payload in the neutral model is exactly what §13 forbids.
type Ask struct {
	// ID is opaque and is what an answer names. Only the adapter that issued
	// it knows what it means.
	ID string `json:"id"`

	// Tool is what the agent wants to use, in the provider's own naming, and
	// Title is that name as the provider would show it to a person.
	Tool  string `json:"tool,omitempty"`
	Title string `json:"title,omitempty"`

	// Target is the file or command the tool would act on, when there is one
	// that can be named.
	Target string `json:"target,omitempty"`

	// Reason is the agent's own explanation of why it is asking rather than
	// proceeding. It is shown verbatim: AgentLine does not paraphrase a
	// safety judgement it did not make.
	Reason string `json:"reason,omitempty"`

	// Mode is a permission mode the agent suggested would let this and
	// everything like it through. Empty when it suggested none.
	Mode string `json:"mode,omitempty"`

	// Allowed records how an ask was answered, on PermissionAnswered.
	Allowed bool `json:"allowed,omitempty"`
}

// Session is what the agent reported about the session it is running.
type Session struct {
	Model string `json:"model,omitempty"`

	// Limits are the usage windows the agent reported, keyed by name. There
	// is more than one - a five-hour window and a weekly one - and each is
	// reported separately, so they are kept side by side rather than
	// overwriting each other.
	Limits map[string]Limit `json:"limits,omitempty"`

	// Turns counts completed turns in this session.
	Turns int `json:"turns,omitempty"`

	// InputTokens is what the last turn sent, cached portion included. It
	// grows with the conversation, since a turn re-sends what came before.
	InputTokens int `json:"input_tokens,omitempty"`

	// OutputTokens is what the last turn generated.
	OutputTokens int `json:"output_tokens,omitempty"`

	// CostUSD is what the agent reported this session has cost so far, and
	// is zero when it reported nothing.
	CostUSD float64 `json:"cost_usd,omitempty"`

	// ContextWindow, ContextUsed and ContextPercent are how full the
	// context is, as the agent measures it. Only the agent knows the window
	// it is working against, so the share is asked for rather than worked
	// out from a token count and a guess at the limit.
	ContextWindow  int     `json:"context_window,omitempty"`
	ContextUsed    int     `json:"context_used,omitempty"`
	ContextPercent float64 `json:"context_percent,omitempty"`

	// Capabilities are what this session announced it supports.
	Capabilities Capabilities `json:"capabilities,omitzero"`
}

// Capabilities are what the agent reported it can do this session.
//
// They are read from the agent's own announcement rather than assumed, so a
// command it does not offer is never presented as though it exists.
type Capabilities struct {
	// PermissionMode is how tool calls are currently being approved.
	PermissionMode string `json:"permission_mode,omitempty"`

	// SlashCommands are the commands the agent said it accepts.
	SlashCommands []string `json:"slash_commands,omitempty"`
}

// Limit is how much of one usage window has been consumed.
type Limit struct {
	// Used is the share consumed, between 0 and 1.
	Used float64 `json:"used"`

	// ResetsAt is when the window rolls over, zero if not reported.
	ResetsAt time.Time `json:"resets_at,omitempty"`

	// Overage marks usage beyond the included allowance.
	Overage bool `json:"overage,omitempty"`
}

// Peak returns the most-consumed window, which is the one that will run out
// first and so the one worth warning about.
func (s *Session) Peak() (name string, limit Limit, ok bool) {
	for n, l := range s.Limits {
		if !ok || l.Used > limit.Used {
			name, limit, ok = n, l, true
		}
	}
	return name, limit, ok
}

// Valid reports whether the event carries the fields its Type requires.
// Malformed events are dropped rather than allowed to corrupt state.
func (e Event) Valid() bool {
	if e.Source == "" || e.Timestamp.IsZero() {
		return false
	}
	switch e.Type {
	case FileRead, FileEdit, FileCreate, FileDelete, FilePending:
		return e.Path != ""
	case SessionInfo:
		return e.Session != nil
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
	case PermissionAsk, PermissionAnswered:
		// Without an ID there is no way to answer it, or to know what an
		// answer closed.
		return e.Ask != nil && e.Ask.ID != ""
	}
	return false
}
