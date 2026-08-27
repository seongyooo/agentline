// Package claude adapts Claude Code's hook events into AgentView's normalized
// event model.
//
// Everything Claude-specific — hook names, tool names, payload shapes — is
// confined to this package. Nothing outside it may depend on these types.
//
// The payload shapes here were captured from a real session rather than copied
// from documentation; see docs/hook-spike.md for the transcript and for the
// four gaps this adapter works around.
package claude

// Hook event names AgentView reacts to. Claude Code emits many more; the rest
// are ignored rather than guessed at.
const (
	hookPreToolUse         = "PreToolUse"
	hookPostToolUse        = "PostToolUse"
	hookPostToolUseFailure = "PostToolUseFailure"
	hookUserPromptSubmit   = "UserPromptSubmit"
	hookStop               = "Stop"
	hookSessionEnd         = "SessionEnd"
)

// Tool names. The shell tool is not always called Bash: on Windows it arrives
// as PowerShell, and matching only "Bash" would silently drop every command
// event on that platform.
var shellTools = map[string]bool{
	"Bash":       true,
	"PowerShell": true,
}

// File tools, mapped to the kind of change they represent.
const (
	toolRead         = "Read"
	toolEdit         = "Edit"
	toolWrite        = "Write"
	toolNotebookEdit = "NotebookEdit"
)

// payload is the subset of a hook payload AgentView reads. Fields Claude Code
// sends that AgentView has no use for — transcript_path, effort, the assistant's
// message text — are deliberately not decoded: AgentView is not a conversation
// viewer.
type payload struct {
	HookEventName string `json:"hook_event_name"`
	SessionID     string `json:"session_id"`
	CWD           string `json:"cwd"`

	ToolName  string    `json:"tool_name"`
	ToolUseID string    `json:"tool_use_id"`
	ToolInput toolInput `json:"tool_input"`

	// Prompt is the instruction the user typed, delivered with
	// UserPromptSubmit. It is what MISSION is derived from.
	Prompt string `json:"prompt"`

	// Error carries a human-readable failure description on
	// PostToolUseFailure, e.g. "Exit code 3\n3". The numeric status is not
	// exposed as a field, so it is reported as a failure without a code.
	Error string `json:"error"`

	// IsInterrupt separates a user interruption from a genuine failure.
	IsInterrupt bool `json:"is_interrupt"`
}

// toolInput is the tool's arguments. Only the fields that say what the agent
// is doing are decoded; file contents and diffs are not.
type toolInput struct {
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
	Command      string `json:"command"`
	Description  string `json:"description"`

	// taskFields are set by the tools that keep the agent's task list, which
	// is where PROGRESS is counted from.
	taskFields
}

// path returns whichever path field the tool used.
func (t toolInput) path() string {
	if t.FilePath != "" {
		return t.FilePath
	}
	return t.NotebookPath
}
