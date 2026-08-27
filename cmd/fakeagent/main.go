// Command fakeagent stands in for Claude Code so AgentView can be exercised
// without spending anything.
//
// It speaks the same streaming-JSON protocol on the same pipes, so running
//
//	agentview -run -agent fakeagent
//
// drives the real adapter, the real process handling, and the real control
// requests. Only the thinking is missing.
//
// It works on the files it is pointed at, reading and writing them for real,
// so the project tree and the activity log show the same things they would
// during a session that cost money.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func main() {
	agent := &agent{
		model:      "claude-opus-5-20260514",
		permission: "default",
		started:    time.Now(),
	}
	agent.run()
}

type agent struct {
	model      string
	permission string
	started    time.Time

	turns  int
	tokens int
}

// run reads messages until the pipe closes, which is how AgentView ends a
// session.
func (a *agent) run() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64<<10), 8<<20)

	for scanner.Scan() {
		var msg struct {
			Type    string `json:"type"`
			Message struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
			RequestID string `json:"request_id"`
			Request   struct {
				Subtype string `json:"subtype"`
				Mode    string `json:"mode"`
				Model   string `json:"model"`
			} `json:"request"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "control_request":
			a.control(msg.RequestID, msg.Request.Subtype, msg.Request.Mode, msg.Request.Model)
		case "user":
			var prompt string
			if len(msg.Message.Content) > 0 {
				prompt = msg.Message.Content[0].Text
			}
			a.turn(prompt)
		}
	}
}

// control answers a control request the way the real agent does.
func (a *agent) control(id, subtype, mode, model string) {
	fail := func(reason string) {
		emit(map[string]any{
			"type":     "control_response",
			"response": map[string]any{"subtype": "error", "request_id": id, "error": reason},
		})
	}

	switch subtype {
	case "set_permission_mode":
		if mode == "" {
			fail("set_permission_mode: mode must be a string")
			return
		}
		a.permission = mode

	case "set_model":
		// An empty model means "back to the session default", and anything
		// that is not a known family is refused, so AgentView's error path
		// can be exercised too.
		switch {
		case model == "":
			a.model = "claude-opus-5-20260514"
		case knownModel(model):
			a.model = model
		default:
			fail("set_model: unknown model " + model)
			return
		}

	case "get_context_usage":
		// Answered from the same running total the usage report uses, so
		// the two figures agree the way a real session's do.
		const window = 200_000
		emit(map[string]any{
			"type": "control_response",
			"response": map[string]any{
				"subtype": "success", "request_id": id,
				"response": map[string]any{
					"context_window_size":  window,
					"current_usage":        a.tokens,
					"used_percentage":      minFloat(float64(a.tokens)/window, 1),
					"remaining_percentage": maxFloat(1-float64(a.tokens)/window, 0),
				},
			},
		})
		return

	case "interrupt":
		// Nothing is running long enough here to interrupt.
	}

	emit(map[string]any{
		"type":     "control_response",
		"response": map[string]any{"subtype": "success", "request_id": id},
	})
	a.announce()
}

// turn plays out one exchange, doing real work on real files so the tree and
// the activity log behave as they would with an agent that costs money.
func (a *agent) turn(prompt string) {
	a.turns++
	a.announce()

	steps := a.plan(prompt)
	for i, step := range steps {
		a.progress(i, len(steps))
		step()
		pause()
	}
	a.progress(len(steps), len(steps))

	a.reply(prompt, len(steps))
	a.usage()
}

// plan turns the prompt into a sequence of things to do. What it picks up on
// is deliberately shallow: the point is to produce plausible activity, not to
// understand anything.
func (a *agent) plan(prompt string) []func() {
	files := projectFiles()
	steps := []func(){}

	if len(files) > 0 {
		steps = append(steps, func() { a.readFile(files[0]) })
	}
	if len(files) > 1 {
		steps = append(steps, func() { a.readFile(files[rand.Intn(len(files))]) })
	}

	lower := strings.ToLower(prompt)
	switch {
	case strings.Contains(lower, "fail"), strings.Contains(lower, "실패"):
		steps = append(steps, func() { a.runCommand("go test ./...", "Run the test suite", true) })

	case strings.Contains(lower, "test"), strings.Contains(lower, "테스트"):
		steps = append(steps, func() { a.runCommand("go test ./...", "Run the test suite", false) })

	case strings.Contains(lower, "readme"), strings.Contains(lower, "리드미"), strings.Contains(lower, "문서"):
		steps = append(steps, func() { a.writeFile("FAKE-NOTES.md", "# Notes\n\nWritten by fakeagent.\n") })

	default:
		if len(files) > 0 {
			steps = append(steps, func() { a.touchFile(files[0]) })
		}
		steps = append(steps, func() { a.runCommand("go build ./...", "Build the project", false) })
	}
	return steps
}

// readFile reports a read, and actually reads so the timing is not fictional.
func (a *agent) readFile(path string) {
	id := toolID()
	a.toolUse(id, "Read", map[string]any{"file_path": abs(path)})

	_, err := os.ReadFile(abs(path))
	a.toolResult(id, err != nil)
}

// writeFile creates a file, which is what makes it appear in the tree.
func (a *agent) writeFile(path, content string) {
	id := toolID()
	a.toolUse(id, "Write", map[string]any{
		"file_path":   abs(path),
		"description": "Write " + path,
	})

	pause() // long enough to see the entry arrive dimmed before it lands
	err := os.WriteFile(abs(path), []byte(content), 0o644)
	a.toolResult(id, err != nil)
}

// touchFile rewrites a file with its own contents, so an edit is reported
// without the file actually changing.
func (a *agent) touchFile(path string) {
	id := toolID()
	a.toolUse(id, "Edit", map[string]any{
		"file_path":   abs(path),
		"description": "Adjust " + filepath.Base(path),
	})

	content, err := os.ReadFile(abs(path))
	if err == nil {
		err = os.WriteFile(abs(path), content, 0o644)
	}
	a.toolResult(id, err != nil)
}

// runCommand reports a command without running one: the point is the shape of
// the activity, and running arbitrary commands from a test double is not.
func (a *agent) runCommand(command, description string, fail bool) {
	id := toolID()
	a.toolUse(id, shellTool(), map[string]any{
		"command":     command,
		"description": description,
	})

	pause()
	a.toolResult(id, fail)
}

func (a *agent) toolUse(id, name string, input map[string]any) {
	emit(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"role": "assistant",
			"content": []any{map[string]any{
				"type": "tool_use", "id": id, "name": name, "input": input,
			}},
		},
	})
}

func (a *agent) toolResult(id string, isError bool) {
	emit(map[string]any{
		"type": "user",
		"message": map[string]any{
			"role": "user",
			"content": []any{map[string]any{
				"type": "tool_result", "tool_use_id": id,
				"is_error": isError, "content": "output",
			}},
		},
	})
}

// progress reports the task list, which is where PROGRESS comes from.
func (a *agent) progress(done, total int) {
	if total == 0 {
		return
	}
	todos := make([]any, 0, total)
	for i := 0; i < total; i++ {
		status := "pending"
		if i < done {
			status = "completed"
		}
		todos = append(todos, map[string]any{"status": status})
	}

	a.toolUse(toolID(), "TodoWrite", map[string]any{"todos": todos})
}

func (a *agent) reply(prompt string, steps int) {
	text := fmt.Sprintf(
		"Done: %d steps for %q. This is fakeagent, so nothing was reasoned about — "+
			"the files were really read and written, but the words are canned. "+
			"Use it to exercise the interface without paying for a session.",
		steps, strings.TrimSpace(prompt))

	emit(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"role":    "assistant",
			"content": []any{map[string]any{"type": "text", "text": text}},
		},
	})
}

// announce sends the init event, which is how AgentView learns the model, the
// permission mode, and the commands on offer.
func (a *agent) announce() {
	cwd, _ := os.Getwd()

	emit(map[string]any{
		"type": "system", "subtype": "init",
		"session_id":     "fakeagent",
		"cwd":            cwd,
		"model":          a.model,
		"permissionMode": a.permission,
		"slash_commands": []string{"model", "compact", "clear", "context", "cost"},
	})
}

// usage ends the turn, reporting what it cost. The context grows the way a
// real session's does, since that is the thing worth watching.
//
// The limits go first and the result last: the result is what says the turn
// is over, so anything after it would arrive against a session the interface
// already considers idle.
func (a *agent) usage() {
	a.tokens += 12_000 + rand.Intn(8_000)

	emit(map[string]any{
		"type": "rate_limit_event",
		"rate_limit_info": map[string]any{
			"status":         "allowed",
			"rateLimitType":  "five_hour",
			"utilization":    minFloat(float64(a.turns)*0.07, 1),
			"resetsAt":       a.started.Add(5 * time.Hour).Unix(),
			"isUsingOverage": false,
		},
	})
	emit(map[string]any{
		"type": "rate_limit_event",
		"rate_limit_info": map[string]any{
			"status":         "allowed",
			"rateLimitType":  "seven_day",
			"utilization":    minFloat(float64(a.turns)*0.02, 1),
			"resetsAt":       a.started.Add(7 * 24 * time.Hour).Unix(),
			"isUsingOverage": false,
		},
	})

	emit(map[string]any{
		"type":           "result",
		"subtype":        "success",
		"is_error":       false,
		"total_cost_usd": float64(a.turns) * 0.11,
		"usage": map[string]any{
			"input_tokens":                1_200,
			"output_tokens":               600 + rand.Intn(400),
			"cache_read_input_tokens":     a.tokens,
			"cache_creation_input_tokens": 2_000,
		},
	})
}

// projectFiles lists a few real files to work on, so the tree lights up with
// paths the project actually has.
func projectFiles() []string {
	var found []string

	filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil || len(found) >= 12 {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasPrefix(name, ".") {
			found = append(found, filepath.ToSlash(path))
		}
		return nil
	})

	sort.Strings(found)
	return found
}

func abs(path string) string {
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, filepath.FromSlash(path))
}

// shellTool is the name the shell arrives under, which differs by platform.
func shellTool() string {
	if os.PathSeparator == '\\' {
		return "PowerShell"
	}
	return "Bash"
}

func knownModel(model string) bool {
	for _, family := range []string{"opus", "sonnet", "haiku", "fable"} {
		if strings.Contains(model, family) {
			return true
		}
	}
	return false
}

// pause is long enough to watch a step happen, short enough not to be tedious.
func pause() { time.Sleep(time.Duration(400+rand.Intn(500)) * time.Millisecond) }

func toolID() string { return fmt.Sprintf("fake-%d", rand.Int63()) }

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func emit(v map[string]any) {
	line, err := json.Marshal(v)
	if err != nil {
		return
	}
	fmt.Println(string(line))
}
