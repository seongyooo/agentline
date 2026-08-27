package claude

// fakeAgentSource is a stand-in for Claude Code that speaks the streaming
// protocol: it reads user messages on stdin and emits the same shape of
// output the real agent does, including the tool_use / tool_result pairing
// the translator depends on.
//
// It exists so the streaming path can be tested without an API call, and so a
// change in the real agent's protocol shows up as a failing test rather than
// as a UI that silently stops updating.
const fakeAgentSource = `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	cwd, _ := os.Getwd()
	notes := filepath.Join(cwd, "notes.md")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	for scanner.Scan() {
		var in struct {
			Message struct {
				Content []struct {
					Text string ` + "`json:\"text\"`" + `
				} ` + "`json:\"content\"`" + `
			} ` + "`json:\"message\"`" + `
		}
		if err := json.Unmarshal(scanner.Bytes(), &in); err != nil {
			continue
		}
		prompt := ""
		if len(in.Message.Content) > 0 {
			prompt = in.Message.Content[0].Text
		}

		emit(map[string]any{"type": "system", "subtype": "init", "session_id": "fake"})

		// Read a file: announced only by its result.
		emit(assistant([]any{
			toolUse("call-read", "Read", map[string]any{"file_path": notes}),
		}))
		emit(toolResult("call-read", false))

		// Run a command: announced before it runs, and again when it ends.
		failing := strings.Contains(prompt, "fail")
		emit(assistant([]any{
			toolUse("call-cmd", "Bash", map[string]any{"command": "go test ./..."}),
		}))
		emit(toolResult("call-cmd", failing))

		emit(assistant([]any{
			map[string]any{"type": "text", "text": "done: " + prompt},
		}))
		emit(map[string]any{"type": "result", "subtype": "success", "is_error": failing})
	}
}

func assistant(content []any) map[string]any {
	return map[string]any{
		"type":    "assistant",
		"message": map[string]any{"role": "assistant", "content": content},
	}
}

func toolUse(id, name string, input map[string]any) map[string]any {
	return map[string]any{"type": "tool_use", "id": id, "name": name, "input": input}
}

func toolResult(id string, isError bool) map[string]any {
	return map[string]any{
		"type": "user",
		"message": map[string]any{
			"role": "user",
			"content": []any{map[string]any{
				"type":        "tool_result",
				"tool_use_id": id,
				"is_error":    isError,
				"content":     "output",
			}},
		},
	}
}

func emit(v map[string]any) {
	line, _ := json.Marshal(v)
	fmt.Println(string(line))
}
`
