// Package agent defines the seam between a coding agent and AgentLine.
//
// Each backend — Claude Code, and later others — implements Source by
// translating whatever it emits into the normalized event model. Nothing
// downstream of a Source is allowed to know which backend produced an event,
// so provider-specific formats stay inside their adapter.
package agent

import (
	"context"

	"github.com/seongyooo/agentline/internal/events"
)

// Source produces normalized events from a running agent.
//
// Events returns a channel that the Source owns and closes when it is
// exhausted or ctx is cancelled. A Source reports setup failures — an agent
// that is not running, a socket it cannot open — as an error from Events;
// problems encountered later are dropped or logged rather than returned, so a
// single bad event never takes the UI down.
type Source interface {
	// Name identifies the backend, e.g. "claude-code". It appears in the
	// header, so it must be honest about what is being observed.
	Name() string

	// Events starts the source and streams until ctx is done.
	Events(ctx context.Context) (<-chan events.Event, error)
}
