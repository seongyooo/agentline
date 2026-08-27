package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/seongyooo/agentline/internal/agent"
	"github.com/seongyooo/agentline/internal/events"
)

// timeAfter is a variable so shutdown can be tested without waiting.
var timeAfter = time.After

// Binary is the Claude Code executable AgentLine launches.
const Binary = "claude"

// maxLine bounds one line of streaming output. Tool results can carry whole
// files, so the scanner needs far more than its default.
const maxLine = 8 << 20

// ErrNotRunning is returned when a prompt is submitted with no live session.
var ErrNotRunning = errors.New("claude: session is not running")

var _ agent.Source = (*Stream)(nil)

// Stream owns a Claude Code session and both observes and drives it.
//
// It launches the agent in streaming-JSON mode, which keeps one process alive
// across many turns, so prompts typed in AgentLine continue the same
// conversation. Everything AgentLine needs arrives in-band on stdout: no hook
// configuration to install, no port to listen on, and nothing left behind in
// the project when AgentLine exits.
type Stream struct {
	// Root is the project the session runs in.
	Root string

	// Bin overrides the executable, for tests.
	Bin string

	// Args are extra arguments passed to the agent.
	Args []string

	translator *streamTranslator

	mu     sync.Mutex
	stdin  io.WriteCloser
	cmd    *exec.Cmd
	out    chan events.Event
	closed bool
}

// NewStream returns a Stream for a project root.
func NewStream(root string) *Stream {
	return &Stream{Root: root, translator: newStreamTranslator(root)}
}

func (s *Stream) Name() string { return SourceName }

// Events starts the session and streams its activity until ctx is done.
//
// A missing executable is reported here rather than leaving a UI that looks
// live but never updates.
func (s *Stream) Events(ctx context.Context) (<-chan events.Event, error) {
	s.mu.Lock()
	s.out = make(chan events.Event, buffer)
	out := s.out
	s.mu.Unlock()

	if err := s.start(); err != nil {
		return nil, err
	}
	go s.shutdown(ctx)
	return out, nil
}

// start launches the agent and begins reading its output.
func (s *Stream) start() error {
	bin := s.Bin
	if bin == "" {
		bin = Binary
	}

	args := append([]string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		// Without this the agent silently refuses anything the current mode
		// does not already allow, and there is nothing to ask about.
		"--permission-prompt-tool", permissionPromptTool,
	}, s.Args...)

	cmd := exec.Command(bin, args...)
	cmd.Dir = s.Root

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("claude stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("claude stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", bin, err)
	}

	s.mu.Lock()
	s.stdin, s.cmd = stdin, cmd
	s.mu.Unlock()

	go s.consume(stdout)
	return nil
}

// waitOrKill ends a process, killing it if it does not stop on its own.
func waitOrKill(cmd *exec.Cmd) {
	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-timeAfter(shutdownGrace):
		cmd.Process.Kill()
		<-done
	}
}

// Send submits a prompt to the running session.
//
// The prompt is also emitted as an event, so it reaches MISSION by the same
// path as one observed from outside — the UI never learns about it separately.
func (s *Stream) Send(prompt string) error {
	line, err := json.Marshal(map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": []any{map[string]any{"type": "text", "text": prompt}},
		},
	})
	if err != nil {
		return fmt.Errorf("encode prompt: %w", err)
	}
	if err := s.write(line); err != nil {
		return err
	}

	s.send(s.translator.prompt(prompt))
	return nil
}

// write sends one line to the running session.
func (s *Stream) write(line []byte) error {
	s.mu.Lock()
	stdin, closed := s.stdin, s.closed
	s.mu.Unlock()

	if stdin == nil || closed {
		return ErrNotRunning
	}
	if _, err := stdin.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write to session: %w", err)
	}
	return nil
}

// Restart replaces the session with a fresh one.
//
// A session AgentLine owns is never compacted, so its context only grows and
// every further turn re-sends all of it. Starting over is what gets the cost
// per turn back down. The event stream is kept, so the UI does not have to be
// rebuilt around a new channel.
func (s *Stream) Restart() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrNotRunning
	}
	old, cmd := s.stdin, s.cmd
	s.stdin, s.cmd = nil, nil
	s.mu.Unlock()

	// Ending the old process first, so two sessions never run at once.
	if old != nil {
		old.Close()
	}
	if cmd != nil {
		waitOrKill(cmd)
	}

	s.translator = newStreamTranslator(s.Root)
	return s.start()
}

// consume reads streaming output until the process ends.
func (s *Stream) consume(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64<<10), maxLine)

	for scanner.Scan() {
		for _, e := range s.translator.translateLine(scanner.Bytes()) {
			s.send(e)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("claude stream: %v", err)
	}
}

// shutdown ends the session when ctx is cancelled and closes the stream once
// nothing can still be sent on it.
func (s *Stream) shutdown(ctx context.Context) {
	<-ctx.Done()

	// Closing stdin asks the agent to finish; killing is the fallback.
	s.mu.Lock()
	s.closed = true
	stdin, cmd := s.stdin, s.cmd
	s.mu.Unlock()

	// Answered before stdin closes, or the agent is left waiting on a
	// question that can no longer be delivered.
	for _, e := range s.denyOutstanding("AgentLine closed") {
		s.send(e)
	}

	if stdin != nil {
		stdin.Close()
	}
	if cmd != nil {
		waitOrKill(cmd)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	close(s.out)
	s.out = nil
}

// send queues an event without ever blocking the agent.
func (s *Stream) send(e events.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.out == nil {
		return
	}
	select {
	case s.out <- e:
	default:
		// The UI is behind. Dropping keeps the agent moving.
		log.Printf("claude stream: dropped %s; consumer is behind", e.Type)
	}
}
