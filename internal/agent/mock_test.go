package agent

import (
	"context"
	"testing"
	"time"

	"github.com/seongyooo/agentline/internal/events"
)

func drain(t *testing.T, ch <-chan events.Event, limit int) []events.Event {
	t.Helper()

	var got []events.Event
	timeout := time.After(2 * time.Second)
	for len(got) < limit {
		select {
		case e, ok := <-ch:
			if !ok {
				return got
			}
			got = append(got, e)
		case <-timeout:
			t.Fatalf("timed out after %d events", len(got))
		}
	}
	return got
}

func TestMockRequiresPaths(t *testing.T) {
	if _, err := (&Mock{}).Events(context.Background()); err != ErrNoPaths {
		t.Errorf("err = %v, want %v", err, ErrNoPaths)
	}
}

func TestMockReplaysAFullSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := (&Mock{Paths: []string{"a.go", "b.go"}}).Events(ctx)
	if err != nil {
		t.Fatal(err)
	}

	got := drain(t, ch, 100) // the script ends, so this reads to close
	want := []events.Type{
		events.AgentStatus,  // working
		events.FileRead,     // a.go
		events.FileEdit,     // b.go
		events.CommandStart, //
		events.CommandEnd,   //
		events.AgentStatus,  // done
		events.AgentStatus,  // waiting
	}

	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Type != w {
			t.Errorf("event %d = %q, want %q", i, got[i].Type, w)
		}
	}
}

// Every emitted event must survive the reducer's validation, or the pipeline
// would silently drop it.
func TestMockEmitsOnlyValidEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, _ := (&Mock{Paths: []string{"a.go"}}).Events(ctx)
	for _, e := range drain(t, ch, 100) {
		if !e.Valid() {
			t.Errorf("invalid event: %+v", e)
		}
		if e.Source != MockName {
			t.Errorf("Source = %q, want %q", e.Source, MockName)
		}
		if e.Timestamp.IsZero() {
			t.Errorf("event %q has no timestamp", e.Type)
		}
	}
}

// The name must not impersonate a real agent: the header shows it, and the
// user has to be able to tell that nothing real is attached.
func TestMockNameIsNotARealAgent(t *testing.T) {
	if got := (&Mock{}).Name(); got != "mock" {
		t.Errorf("Name = %q, want %q", got, "mock")
	}
}

func TestMockClosesChannelWhenScriptEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, _ := (&Mock{Paths: []string{"a.go"}}).Events(ctx)
	drain(t, ch, 100)

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel still delivering after the script ended")
		}
	case <-time.After(time.Second):
		t.Error("channel was never closed")
	}
}

func TestMockStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := (&Mock{Paths: []string{"a.go"}, Interval: time.Hour, Loop: true}).Events(ctx)
	if err != nil {
		t.Fatal(err)
	}

	drain(t, ch, 1) // let it emit once, then block on the hour-long interval
	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("event delivered after cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Error("cancelled source did not close its channel")
	}
}

func TestMockLoops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, _ := (&Mock{Paths: []string{"a.go"}, Loop: true}).Events(ctx)

	// One pass is 6 events for a single path; reading more proves it restarts.
	got := drain(t, ch, 13)
	if got[0].Type != got[6].Type {
		t.Errorf("second pass starts with %q, want %q", got[6].Type, got[0].Type)
	}
}
