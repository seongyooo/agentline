package codex

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

	"github.com/seongyooo/agentline/internal/agent"
	"github.com/seongyooo/agentline/internal/events"
)

// Binary is the Codex executable AgentLine launches.
const Binary = "codex"

const (
	// maxLine bounds one line of output. A completed command carries its
	// aggregated output, which can be large.
	maxLine = 8 << 20

	// buffer is how many events may queue while the UI is busy.
	buffer = 256
)

// ErrNotRunning is returned when there is no live session to act on.
var ErrNotRunning = errors.New("codex: session is not running")

var _ agent.Source = (*Stream)(nil)

// Stream runs one Codex turn per prompt and reports what it did.
//
// Unlike Claude Code, `codex exec` is a one-shot: it takes a prompt, works,
// and exits. Continuity comes from resuming the thread it reports at the
// start, so AgentLine keeps that id and hands it back on the next prompt.
// The effect for the user is the same — a conversation that remembers — but
// there is no long-lived process to own.
type Stream struct {
	// Root is the project the session runs in.
	Root string

	// Bin overrides the executable, for tests.
	Bin string

	// Args are extra arguments passed to the agent.
	Args []string

	translator *translator

	mu       sync.Mutex
	out      chan events.Event
	started  bool
	closed   bool
	threadID string
	running  *exec.Cmd

	// working is whether a turn is still doing something, which is not the
	// same as the process being alive: Codex reports the turn finished and
	// then takes a moment to exit. Refusing a prompt in that window would
	// reject the one the user types the instant the turn appears to end.
	working bool
}

// NewStream returns a Stream for a project root.
func NewStream(root string) *Stream {
	return &Stream{Root: root, translator: newTranslator(root)}
}

func (s *Stream) Name() string { return SourceName }

// Events opens the stream. Nothing runs until a prompt is sent, because
// `codex exec` needs one to start.
func (s *Stream) Events(ctx context.Context) (<-chan events.Event, error) {
	s.mu.Lock()
	s.out = make(chan events.Event, buffer)
	s.started = true
	out := s.out
	s.mu.Unlock()

	go s.shutdown(ctx)
	return out, nil
}

// Send runs a turn with the given prompt.
//
// A turn already in flight is left to finish rather than interrupted: two
// Codex processes working in the same directory would race over the files.
func (s *Stream) Send(prompt string) error {
	s.mu.Lock()
	if !s.started || s.closed {
		s.mu.Unlock()
		return ErrNotRunning
	}
	if s.working {
		s.mu.Unlock()
		return errors.New("codex: a turn is already running")
	}
	s.working = true
	thread := s.threadID
	s.mu.Unlock()

	bin := s.Bin
	if bin == "" {
		bin = Binary
	}

	args := []string{"exec", "--json"}
	if thread != "" {
		// Continue where the last turn left off.
		args = append(args, "resume", thread)
	}
	args = append(args, s.Args...)
	args = append(args, prompt)

	cmd := exec.Command(bin, args...)
	cmd.Dir = s.Root

	// A turn that never starts must release the flag, or the session would
	// refuse every prompt from then on.
	fail := func(err error) error {
		s.mu.Lock()
		s.working = false
		s.mu.Unlock()
		return err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fail(fmt.Errorf("codex stdout: %w", err))
	}
	if err := cmd.Start(); err != nil {
		return fail(fmt.Errorf("start %s: %w", bin, err))
	}

	s.mu.Lock()
	s.running = cmd
	s.mu.Unlock()

	s.send(s.translator.prompt(prompt))
	go s.consume(cmd, stdout)
	return nil
}

// consume reads the turn's output until the process ends.
func (s *Stream) consume(cmd *exec.Cmd, stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64<<10), maxLine)

	for scanner.Scan() {
		line := scanner.Bytes()
		s.rememberThread(line)

		for _, e := range s.translator.translateLine(line) {
			// The turn is over once it says so, which is before the process
			// has finished exiting. Releasing here is what lets the next
			// prompt be accepted the moment the turn appears to end.
			if e.Type == events.AgentStatus && e.Status == events.StatusWaiting {
				s.mu.Lock()
				s.working = false
				s.mu.Unlock()
			}
			s.send(e)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("codex stream: %v", err)
	}
	cmd.Wait()

	s.mu.Lock()
	s.running = nil
	// A turn that died without saying it had finished still has to release
	// the flag, or the session would accept nothing further.
	s.working = false
	s.mu.Unlock()
}

// rememberThread notes the thread id so the next prompt can continue it.
func (s *Stream) rememberThread(line []byte) {
	var e struct {
		Type     string `json:"type"`
		ThreadID string `json:"thread_id"`
	}
	if err := json.Unmarshal(line, &e); err != nil {
		return
	}
	if e.Type != eventThreadStarted || e.ThreadID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.threadID == "" {
		s.threadID = e.ThreadID
	}
}

// Restart forgets the thread, so the next prompt starts a conversation that
// carries none of the previous one's context.
func (s *Stream) Restart() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrNotRunning
	}
	s.threadID = ""
	s.translator = newTranslator(s.Root)
	return nil
}

// shutdown ends any running turn when ctx is cancelled and closes the stream.
func (s *Stream) shutdown(ctx context.Context) {
	<-ctx.Done()

	s.mu.Lock()
	s.closed = true
	cmd := s.running
	s.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
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
		log.Printf("codex stream: dropped %s; consumer is behind", e.Type)
	}
}
