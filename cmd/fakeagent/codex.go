package main

import (
	"os"
	"strings"
)

// The Codex side of the stand-in.
//
// `codex exec` is a one-shot: it takes a prompt as an argument, emits a JSONL
// stream of what it did, and exits. This plays out the same shape so the Codex
// adapter can be exercised — process handling, thread resumption and all —
// without an API call.

// codexTurn plays out one `codex exec --json` invocation.
func codexTurn(args []string) {
	prompt, resumed := codexArgs(args)

	// A resumed thread keeps its id, which is how AgentLine's conversation
	// continues rather than starting over each turn.
	thread := resumed
	if thread == "" {
		thread = "fake-thread-1"
	}
	emit(map[string]any{"type": "thread.started", "thread_id": thread})
	emit(map[string]any{"type": "turn.started"})

	lower := strings.ToLower(prompt)
	switch {
	case strings.Contains(lower, "fail"), strings.Contains(lower, "실패"):
		codexCommand("go test ./...", 1, "failed")

	case strings.Contains(lower, "test"), strings.Contains(lower, "테스트"):
		codexCommand("go test ./...", 0, "completed")

	case strings.Contains(lower, "readme"), strings.Contains(lower, "리드미"), strings.Contains(lower, "문서"):
		codexWrite()

	default:
		codexCommand("go build ./...", 0, "completed")
		codexEdit()
	}

	emit(map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id": "msg-1", "type": "agent_message",
			"text": "Done: " + strings.TrimSpace(prompt) +
				". This is fakeagent standing in for Codex, so the words are canned.",
		},
	})
	emit(map[string]any{
		"type": "turn.completed",
		"usage": map[string]any{
			"input_tokens": 1200, "cached_input_tokens": 24000,
			"cache_write_input_tokens": 2000, "output_tokens": 700,
			"reasoning_output_tokens": 100,
		},
	})
}

// codexArgs picks the prompt and any resumed thread out of the arguments,
// which arrive exactly as AgentLine would pass them to the real binary.
func codexArgs(args []string) (prompt, thread string) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "exec", "--json":
		case "resume":
			if i+1 < len(args) {
				thread = args[i+1]
				i++
			}
		default:
			prompt = args[i]
		}
	}
	return prompt, thread
}

func codexCommand(command string, exitCode int, status string) {
	emit(map[string]any{
		"type": "item.started",
		"item": map[string]any{
			"id": "cmd-1", "type": "command_execution",
			"command": command, "status": "in_progress",
		},
	})
	pause()
	emit(map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id": "cmd-1", "type": "command_execution",
			"command": command, "exit_code": exitCode, "status": status,
			"aggregated_output": "output",
		},
	})
}

// codexWrite creates the scratch file, which is what makes it appear in the
// tree. It is the only file this program ever writes.
func codexWrite() {
	pause()
	if err := os.WriteFile(abs(scratchFile), []byte("# Notes\n\nWritten by fakeagent.\n"), 0o644); err != nil {
		emit(map[string]any{"type": "error", "message": err.Error()})
		return
	}
	codexPatch(scratchFile, "add")
}

func codexEdit() {
	pause()
	if err := os.WriteFile(abs(scratchFile), []byte("# Notes\n\nEdited by fakeagent.\n"), 0o644); err != nil {
		emit(map[string]any{"type": "error", "message": err.Error()})
		return
	}
	codexPatch(scratchFile, "update")
}

// codexPatch reports a file change the way Codex does: absolute paths, and a
// kind that says whether the file was added, updated or deleted.
func codexPatch(path, kind string) {
	emit(map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id": "patch-1", "type": "file_change", "status": "completed",
			"changes": []any{map[string]any{"path": abs(path), "kind": kind}},
		},
	})
}
