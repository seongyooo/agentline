package agent

import (
	"errors"
	"time"

	"context"

	"github.com/seongyooo/agentline/internal/events"
)

// MockName is the source name mock events carry. It is deliberately not
// "claude-code": the header shows this name, and it must be obvious at a
// glance that no real agent is attached.
const MockName = "mock"

// ErrNoPaths is returned when a Mock has no files to act on.
var ErrNoPaths = errors.New("mock: no paths to replay")

// Mock replays a scripted session over real files so the event pipeline and
// the UI can be exercised without an agent. It covers every state transition
// the reducer handles: working, running a command, done, and waiting.
var _ Source = (*Mock)(nil)

type Mock struct {
	// Paths are real, root-relative files the script pretends to work on.
	Paths []string

	// Interval is the delay between events. Zero emits as fast as the
	// consumer reads, which is what tests want.
	Interval time.Duration

	// Loop restarts the script instead of closing the channel.
	Loop bool
}

func (m *Mock) Name() string { return MockName }

// Events replays the script until ctx is cancelled or, when Loop is false, the
// script runs out.
func (m *Mock) Events(ctx context.Context) (<-chan events.Event, error) {
	if len(m.Paths) == 0 {
		return nil, ErrNoPaths
	}
	script := m.script()

	out := make(chan events.Event)
	go func() {
		defer close(out)
		for {
			for _, e := range script {
				e.Timestamp = time.Now()
				select {
				case out <- e:
				case <-ctx.Done():
					return
				}
				if !sleep(ctx, m.Interval) {
					return
				}
			}
			if !m.Loop {
				return
			}
		}
	}()
	return out, nil
}

// script builds one pass of a plausible session.
func (m *Mock) script() []events.Event {
	exitOK := 0
	const command = "go test ./..."

	script := []events.Event{status(events.StatusWorking)}
	for i, p := range m.Paths {
		kind := events.FileEdit
		if i == 0 {
			kind = events.FileRead // a session usually reads before it writes
		}
		script = append(script, events.Event{Type: kind, Path: p, Source: MockName})
	}

	return append(script,
		events.Event{Type: events.CommandStart, Command: command, Source: MockName},
		events.Event{Type: events.CommandEnd, Command: command, ExitCode: &exitOK, Source: MockName},
		status(events.StatusDone),
		status(events.StatusWaiting),
	)
}

func status(s events.Status) events.Event {
	return events.Event{Type: events.AgentStatus, Status: s, Source: MockName}
}

// sleep waits for d, reporting false if ctx was cancelled first.
func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}

	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
