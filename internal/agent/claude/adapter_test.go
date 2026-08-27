package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/seongyooo/agentline/internal/agent"
	"github.com/seongyooo/agentline/internal/events"
)

var _ agent.Source = (*Adapter)(nil) // the adapter must fit the backend seam

// start runs an adapter on an ephemeral port and returns its stream and URL.
func start(t *testing.T) (*Adapter, <-chan events.Event, string) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	a := New(root, "127.0.0.1:0")
	stream, err := a.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return a, stream, fmt.Sprintf("http://%s%s", a.Addr, HookPath)
}

func post(t *testing.T, url string, body string) *http.Response {
	t.Helper()

	resp, err := http.Post(url, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp
}

func receive(t *testing.T, stream <-chan events.Event) events.Event {
	t.Helper()

	select {
	case e, ok := <-stream:
		if !ok {
			t.Fatal("stream closed before an event arrived")
		}
		return e
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for an event")
	}
	return events.Event{}
}

func TestAdapterDeliversTranslatedEvents(t *testing.T) {
	_, stream, url := start(t)

	if resp := post(t, url, readPayload); resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	got := receive(t, stream)
	if got.Type != events.FileRead || got.Path != "notes.md" {
		t.Errorf("got %+v, want a file_read of notes.md", got)
	}
}

// A hook runs inline with the agent's tool call, so a malformed payload must
// still get an immediate 200: it is AgentLine's problem, not the agent's.
func TestMalformedPayloadStillAnswersOK(t *testing.T) {
	_, stream, url := start(t)

	if resp := post(t, url, "{not json"); resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 even for a bad payload", resp.StatusCode)
	}

	select {
	case e := <-stream:
		t.Errorf("malformed payload produced an event: %+v", e)
	case <-time.After(100 * time.Millisecond):
	}

	// The adapter must still be usable afterwards.
	post(t, url, readPayload)
	if got := receive(t, stream); got.Type != events.FileRead {
		t.Errorf("adapter stopped working after a bad payload: %+v", got)
	}
}

// If the UI stalls, the adapter drops events rather than blocking the agent.
func TestFullBufferDropsInsteadOfBlockingTheAgent(t *testing.T) {
	a, _, url := start(t)

	// Never read the stream, so the buffer fills and then overflows.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < buffer+50; i++ {
			post(t, url, readPayload)
		}
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("posting blocked: a stalled consumer must not stall the agent")
	}

	if a.Dropped() == 0 {
		t.Error("Dropped() = 0, want the overflow to be counted, not silent")
	}
}

func TestCancellationClosesTheStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	a := New(root, "127.0.0.1:0")
	stream, err := a.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	select {
	case _, ok := <-stream:
		if ok {
			t.Error("stream delivered an event after cancellation")
		}
	case <-time.After(5 * time.Second):
		t.Error("cancelled adapter never closed its stream")
	}
}

// A port already in use must be reported, not silently produce no events.
func TestPortInUseIsReported(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	first := New(root, "127.0.0.1:0")
	if _, err := first.Events(ctx); err != nil {
		t.Fatal(err)
	}

	second := New(root, first.Addr)
	if _, err := second.Events(ctx); err == nil {
		t.Error("binding an occupied port succeeded; the clash must be reported")
	}
}

func TestGetIsRejected(t *testing.T) {
	_, _, url := start(t)

	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
}

// The generated config must point at the address the adapter actually binds,
// so the two can never drift apart.
func TestHookSettingsPointAtTheListenAddress(t *testing.T) {
	raw, err := HookSettings("127.0.0.1:9999")
	if err != nil {
		t.Fatal(err)
	}

	var settings struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Type string `json:"type"`
				URL  string `json:"url"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatal(err)
	}

	for _, event := range hookEvents {
		entry, ok := settings.Hooks[event]
		if !ok {
			t.Errorf("%s not registered", event)
			continue
		}
		got := entry[0].Hooks[0]
		if got.Type != "http" {
			t.Errorf("%s type = %q, want http", event, got.Type)
		}
		if want := "http://127.0.0.1:9999" + HookPath; got.URL != want {
			t.Errorf("%s url = %q, want %q", event, got.URL, want)
		}
	}
}

// Settings generated with the defaults must work against a default adapter.
func TestDefaultHookSettingsMatchDefaultAddr(t *testing.T) {
	raw, err := HookSettings("")
	if err != nil {
		t.Fatal(err)
	}
	if want := "http://" + DefaultAddr + HookPath; !bytes.Contains(raw, []byte(want)) {
		t.Errorf("default settings do not point at %q:\n%s", want, raw)
	}
}
