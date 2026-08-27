// Command fakeagent stands in for Claude Code so AgentLine can be exercised
// without spending anything.
//
// It speaks the same streaming-JSON protocol on the same pipes, so running
//
//	agentline -run -agent fakeagent
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
	"sync"
	"time"
)

func main() {
	// `codex exec` takes its prompt as an argument and speaks a different
	// protocol, so the stand-in has to be told which agent it is pretending
	// to be. AgentLine passes the same arguments it would pass the real one.
	if len(os.Args) > 1 && os.Args[1] == "exec" {
		codexTurn(os.Args[1:])
		return
	}

	agent := &agent{
		model:      "claude-opus-5-20260514",
		permission: "default",
		started:    time.Now(),
		answers:    make(chan bool, 1),
	}
	agent.run()
}

type agent struct {
	model      string
	permission string
	started    time.Time

	turns  int
	tokens int
	edits  int

	// answers carries permission decisions back from the reader to the turn
	// that is waiting on one. The real agent asks and blocks, so the
	// stand-in has to as well, or the state it is standing in for is one
	// AgentLine can never be driven into without spending money.
	answers chan bool
}

// run reads messages until the pipe closes, which is how AgentLine ends a
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
			Response struct {
				Response struct {
					Behavior string `json:"behavior"`
				} `json:"response"`
			} `json:"response"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "control_request":
			a.control(msg.RequestID, msg.Request.Subtype, msg.Request.Mode, msg.Request.Model)

		case "control_response":
			// An answer to something this agent asked. Delivered without
			// blocking: an answer nobody is waiting for is one that arrived
			// after the asker gave up, and the reader must keep reading.
			select {
			case a.answers <- msg.Response.Response.Behavior == "allow":
			default:
			}

		case "user":
			var prompt string
			if len(msg.Message.Content) > 0 {
				prompt = msg.Message.Content[0].Text
			}
			// Off the reader, so a turn that stops to ask something can still
			// hear the answer. AgentLine sends one prompt at a time, so there
			// is no queue to keep.
			go a.turn(prompt)
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
		// that is not a known family is refused, so AgentLine's error path
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

	// Most specific first. A prompt asking for a loop of tests contains both
	// words, and whichever case is written first is the one that answers.
	lower := strings.ToLower(prompt)
	switch {
	case strings.Contains(lower, "spin"), strings.Contains(lower, "loop"), strings.Contains(lower, "반복"), strings.Contains(lower, "뺑뺑이"):
		// Enough rounds to pass every threshold AgentLine counts against,
		// with room to spare so the panel is not a coin toss.
		steps = append(steps, a.spin(6)...)

	case strings.Contains(lower, "permission"), strings.Contains(lower, "ask"), strings.Contains(lower, "권한"):
		steps = append(steps, func() { a.askThenWrite() })

	case strings.Contains(lower, "fail"), strings.Contains(lower, "실패"):
		steps = append(steps, func() { a.runCommand("go test ./...", "Run the test suite", true) })

	case strings.Contains(lower, "test"), strings.Contains(lower, "테스트"):
		steps = append(steps, func() { a.runCommand("go test ./...", "Run the test suite", false) })

	case strings.Contains(lower, "readme"), strings.Contains(lower, "리드미"), strings.Contains(lower, "문서"):
		steps = append(steps, func() { a.writeScratch() })

	default:
		steps = append(steps, func() { a.editScratch() })
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

// scratchFile is the only file this program ever writes.
const scratchFile = "FAKE-NOTES.md"

// writeScratch creates the file, which is what makes it appear in the tree.
func (a *agent) writeScratch() {
	id := toolID()
	a.toolUse(id, "Write", map[string]any{
		"file_path":   abs(scratchFile),
		"description": "Write " + scratchFile,
	})

	pause() // long enough to see the entry arrive dimmed before it lands
	err := os.WriteFile(abs(scratchFile), []byte("# Notes\n\nWritten by fakeagent.\n"), 0o644)
	a.toolResult(id, err != nil)
}

// editScratch reports an edit, against the one file this program owns.
//
// It never writes to a file it did not create. Rewriting a real source file
// with its own contents would look harmless, but a process killed mid-write
// truncates it, and a thing you run to try out an interface has no business
// putting a repository at that risk.
func (a *agent) editScratch() {
	id := toolID()
	a.toolUse(id, "Edit", map[string]any{
		"file_path":   abs(scratchFile),
		"description": "Adjust " + scratchFile,
	})

	a.edits++
	err := os.WriteFile(abs(scratchFile),
		fmt.Appendf(nil, "# Notes\n\nWritten by fakeagent.\nEdits: %d\n", a.edits), 0o644)
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
		// Both wordings, the way the real agent is asked to supply them: the
		// imperative for the list and the present tense for what is in hand.
		text := planSteps[i%len(planSteps)]
		todos = append(todos, map[string]any{
			"status":     status,
			"content":    text.content,
			"activeForm": text.active,
		})
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

// announce sends the init event, which is how AgentLine learns the model, the
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
		if path == "." {
			// The root's own name is ".", which the check below would read
			// as a hidden directory and skip, taking the whole tree with it.
			return nil
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

// emitMu serialises output. Turns run off the reader now, so two goroutines
// can reach this, and a half-written line is a protocol error rather than a
// glitch.
var emitMu sync.Mutex

func emit(v map[string]any) {
	line, err := json.Marshal(v)
	if err != nil {
		return
	}
	emitMu.Lock()
	defer emitMu.Unlock()
	fmt.Println(string(line))
}

// askPermission asks AgentLine whether a tool call may go ahead, in the shape
// the real agent asks it, and waits for the answer.
//
// The payload is the one a live session was observed sending, minus the fields
// AgentLine does not read. Getting it wrong here would mean the stand-in
// exercises a protocol nothing speaks.
func (a *agent) askPermission(tool, description string, input map[string]any) bool {
	drain(a.answers)

	emit(map[string]any{
		"type":       "control_request",
		"request_id": toolID(),
		"request": map[string]any{
			"subtype":         "can_use_tool",
			"tool_name":       tool,
			"display_name":    tool,
			"input":           input,
			"description":     description,
			"decision_reason": "fakeagent was told to ask before doing this.",
			"permission_suggestions": []map[string]any{
				{"type": "setMode", "mode": "acceptEdits", "destination": "session"},
			},
			"tool_use_id": toolID(),
		},
	})

	// A stand-in must not hang forever waiting on a person who has walked
	// away. The real agent gives up too, and how AgentLine behaves when an
	// answer never comes is worth being able to try.
	select {
	case allowed := <-a.answers:
		return allowed
	case <-time.After(2 * time.Minute):
		return false
	}
}

// drain clears a stale answer, so one that arrived late for a previous ask
// cannot be mistaken for the answer to this one.
func drain(answers chan bool) {
	select {
	case <-answers:
	default:
	}
}

// askThenWrite is a write that stops to ask first, which is the only way to
// see the NEEDS YOU state without a real session.
func (a *agent) askThenWrite() {
	input := map[string]any{
		"file_path": abs(scratchFile),
		"content":   "# Notes\n\nWritten by fakeagent, with permission.\n",
	}
	if !a.askPermission("Write", scratchFile, input) {
		// Refused. The agent says so and carries on, which is what makes a
		// refusal an answer rather than a failure.
		a.runCommand("echo skipped", "Skip the write that was refused", false)
		return
	}
	a.writeScratch()
}

// spin does the same work over and over without getting anywhere, which is the
// thing SPINNING exists to notice. It is not a simulation of being stuck: it
// really does rewrite one file and really does fail the same command, so what
// AgentLine counts is what happened.
func (a *agent) spin(rounds int) []func() {
	steps := make([]func(), 0, rounds*2)
	for i := 0; i < rounds; i++ {
		steps = append(steps,
			func() { a.editScratch() },
			func() { a.runCommand("go test ./internal/valve", "Run the valve tests", true) },
		)
	}
	return steps
}

// planSteps are the things the stand-in claims to be doing. Canned, like its
// replies, but shaped the way a real task list is shaped so what AgentLine
// reads is what it would read from one.
var planSteps = []struct{ content, active string }{
	{"Read the project files", "Reading the project files"},
	{"Adjust the notes file", "Adjusting the notes file"},
	{"Run the build", "Running the build"},
	{"Check the result", "Checking the result"},
	{"Write up what changed", "Writing up what changed"},
}
