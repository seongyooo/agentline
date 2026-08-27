package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/seongyooo/agentline/internal/events"
)

// DefaultAddr is where the adapter listens for hook deliveries. It binds to
// loopback only: hook payloads carry file paths and commands, and nothing
// leaves this machine.
const DefaultAddr = "127.0.0.1:8787"

// HookPath is the URL path hooks post to.
const HookPath = "/hook"

// buffer is how many events may queue while the UI is busy. Hooks run inline
// with the agent's tool calls, so the adapter must never block waiting for a
// reader; past this depth events are dropped instead.
const buffer = 256

// shutdownGrace bounds how long the server waits for in-flight deliveries.
const shutdownGrace = 2 * time.Second

// Adapter receives Claude Code hook deliveries over HTTP and emits normalized
// events. It implements agent.Source.
type Adapter struct {
	// Root is the absolute project root, used to relativize hook paths.
	Root string

	// Addr is the listen address. Empty means DefaultAddr. Once Events has
	// started, it holds the address actually bound, which differs from the
	// request when port 0 was used.
	Addr string

	translator *translator
	dropped    atomic.Int64

	// mu guards the send/close race: a handler may still be running when
	// shutdown is forced, and sending on a closed channel would panic.
	mu     sync.RWMutex
	out    chan events.Event
	closed bool
}

// New returns an Adapter for a project root.
func New(root, addr string) *Adapter {
	if addr == "" {
		addr = DefaultAddr
	}
	return &Adapter{Root: root, Addr: addr, translator: newTranslator(root)}
}

func (a *Adapter) Name() string { return SourceName }

// Dropped reports how many events were discarded because the consumer fell
// behind. It exists so the loss is visible rather than silent.
func (a *Adapter) Dropped() int64 { return a.dropped.Load() }

// Events starts the receiver and streams normalized events until ctx is done.
//
// It returns an error if the port cannot be bound — usually another AgentLine
// already listening — so the caller can report that instead of appearing to
// work while receiving nothing.
func (a *Adapter) Events(ctx context.Context) (<-chan events.Event, error) {
	listener, err := net.Listen("tcp", a.Addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", a.Addr, err)
	}
	// Record the address actually bound. With port 0 the OS chooses one, and
	// the hook config has to name the real port, not the request.
	a.Addr = listener.Addr().String()

	a.out = make(chan events.Event, buffer)
	mux := http.NewServeMux()
	mux.HandleFunc(HookPath, a.handle)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("claude adapter: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()

		// Shutdown waits for in-flight handlers, so after it returns no
		// handler can still be holding the channel.
		shutdown, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			server.Close()
		}

		a.mu.Lock()
		defer a.mu.Unlock()
		a.closed = true
		close(a.out)
	}()

	return a.out, nil
}

// handle answers the hook immediately and never blocks it.
//
// A hook runs inline with the agent's tool call, so anything slow here slows
// the agent down. The handler replies 200 with an empty body — Claude Code's
// "no decision" response — and only then queues the translated events.
func (a *Adapter) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var p payload
	err := json.NewDecoder(r.Body).Decode(&p)

	// Answer first, whatever happened: a malformed payload is AgentLine's
	// problem to log, never a reason to interfere with the agent's work.
	w.WriteHeader(http.StatusOK)

	if err != nil {
		log.Printf("claude adapter: malformed hook payload: %v", err)
		return
	}

	for _, e := range a.translator.translate(p) {
		a.send(e)
	}
}

// send queues an event without ever blocking the caller.
func (a *Adapter) send(e events.Event) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.closed {
		return // shutting down; the consumer is gone
	}
	select {
	case a.out <- e:
	default:
		// The UI is behind. Drop rather than stall the agent, and say so
		// instead of losing events silently.
		if n := a.dropped.Add(1); n == 1 || n%100 == 0 {
			log.Printf("claude adapter: dropped %d events; consumer is behind", n)
		}
	}
}
