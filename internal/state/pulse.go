package state

import "time"

// Pulse is the shape of a session over time: how much the agent did, when, and
// what kind of thing it was.
//
// It counts rather than remembers. The activity log keeps the last fifty
// actions and drops the rest, which is right for a log and useless for a
// picture of a session — the interesting part of "what happened while I was
// away" is usually older than fifty actions. Buckets cost a few bytes each and
// never fall off the front.
//
// Nothing here is estimated. A bucket's height is the number of actions
// observed in it, and its kind is the most notable one that landed there.
type Pulse struct {
	// start is when the first action arrived, which is what every bucket is
	// counted from.
	start time.Time
	slots []Slot
}

// Slot is one bucket of the session: how many actions landed in it, and the
// one worth colouring it by.
type Slot struct {
	Count int
	Kind  ActionKind
}

// pulseBucket is how much time one bucket covers.
//
// Small enough that a burst of work does not collapse into one column, and
// small enough that the aggregation done at render time has something to work
// with. Long sessions are handled by combining buckets, not by making them
// coarser as they age, which would make the same session look different
// depending on when it was looked at.
const pulseBucket = 5 * time.Second

// pulseMaxSlots bounds the memory a very long session can use. At five seconds
// a slot this is a little over three hours, after which the oldest are dropped
// and the pulse becomes a window rather than the whole session.
const pulseMaxSlots = 2400

// Add records one observed action.
func (p *Pulse) Add(kind ActionKind, at time.Time) {
	if at.IsZero() {
		return
	}
	if p.start.IsZero() {
		p.start = at
	}
	// Clocks are not guaranteed to move forward — hooks arrive out of order,
	// and an agent's timestamps are its own. Anything before the start counts
	// into the first slot rather than being dropped or moving the origin.
	index := 0
	if at.After(p.start) {
		index = int(at.Sub(p.start) / pulseBucket)
	}

	for len(p.slots) <= index {
		p.slots = append(p.slots, Slot{})
	}
	if n := len(p.slots) - pulseMaxSlots; n > 0 {
		p.slots = p.slots[n:]
		p.start = p.start.Add(time.Duration(n) * pulseBucket)
		index -= n
	}

	slot := &p.slots[index]
	slot.Count++
	if notability(kind) > notability(slot.Kind) {
		slot.Kind = kind
	}
}

// Columns aggregates the session into n columns, so the whole of it fits
// whatever width it is drawn in and a longer session compresses rather than
// scrolling out of view.
func (p *Pulse) Columns(n int) []Slot {
	if n <= 0 || len(p.slots) == 0 {
		return nil
	}
	if len(p.slots) <= n {
		out := make([]Slot, len(p.slots))
		copy(out, p.slots)
		return out
	}

	// Ceiling division, so the last column is the short one rather than the
	// tail being dropped.
	per := (len(p.slots) + n - 1) / n
	out := make([]Slot, 0, n)

	for i := 0; i < len(p.slots); i += per {
		merged := Slot{}
		for _, slot := range p.slots[i:min(i+per, len(p.slots))] {
			merged.Count += slot.Count
			if notability(slot.Kind) > notability(merged.Kind) {
				merged.Kind = slot.Kind
			}
		}
		out = append(out, merged)
	}
	return out
}

// Span is the stretch of time the pulse covers, and whether it covers any.
func (p *Pulse) Span() (from, to time.Time, ok bool) {
	if len(p.slots) == 0 {
		return time.Time{}, time.Time{}, false
	}
	return p.start, p.start.Add(time.Duration(len(p.slots)) * pulseBucket), true
}

// Peak is the busiest column of n, which is what the heights are drawn against.
func Peak(columns []Slot) int {
	peak := 0
	for _, c := range columns {
		peak = max(peak, c.Count)
	}
	return peak
}

// notability ranks action kinds by how much they deserve to colour a column
// they share with others.
//
// A failure outranks everything: a bucket that contains one is worth finding
// again, and it is the thing someone scanning a session is looking for. Writes
// outrank reads for the same reason — they are what changed the project.
func notability(k ActionKind) int {
	switch k {
	case ActionFailed:
		return 5
	case ActionAsking, ActionWaiting:
		return 4
	case ActionDeleting, ActionWriting, ActionEditing, ActionCreating:
		return 3
	case ActionRunning:
		return 2
	case ActionReading:
		return 1
	}
	return 0
}
