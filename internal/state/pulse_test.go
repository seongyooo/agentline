package state

import (
	"testing"
	"time"
)

func TestPulseCountsIntoBuckets(t *testing.T) {
	var p Pulse
	start := time.Now()

	p.Add(ActionReading, start)
	p.Add(ActionReading, start.Add(time.Second))
	p.Add(ActionEditing, start.Add(2*pulseBucket))

	columns := p.Columns(16)
	if len(columns) != 3 {
		t.Fatalf("columns = %d, want 3 buckets spanning the gap", len(columns))
	}
	if columns[0].Count != 2 {
		t.Errorf("first bucket = %d, want both actions in it", columns[0].Count)
	}
	// The gap has to survive. Quiet time is the part that says the agent was
	// stuck, or thinking, or waiting on something.
	if columns[1].Count != 0 {
		t.Errorf("the gap was filled in: %d", columns[1].Count)
	}
}

// A failure must survive being merged with whatever else shared its column,
// because it is the thing someone scanning a session is looking for.
func TestPulseKeepsTheMostNotableKind(t *testing.T) {
	var p Pulse
	start := time.Now()

	for i := 0; i < 20; i++ {
		p.Add(ActionReading, start.Add(time.Duration(i)*pulseBucket))
	}
	p.Add(ActionFailed, start.Add(10*pulseBucket))

	for _, column := range p.Columns(4) {
		if column.Kind == ActionFailed {
			return
		}
	}
	t.Error("the failure was lost when its column was merged with reads")
}

// The whole session has to fit whatever width it is drawn in, or the strip
// stops being a picture of the session and becomes a picture of its tail.
func TestPulseCompressesToFit(t *testing.T) {
	var p Pulse
	start := time.Now()

	total := 0
	for i := 0; i < 500; i++ {
		p.Add(ActionReading, start.Add(time.Duration(i)*pulseBucket))
		total++
	}

	columns := p.Columns(40)
	if len(columns) > 40 {
		t.Fatalf("columns = %d, want at most 40", len(columns))
	}

	counted := 0
	for _, c := range columns {
		counted += c.Count
	}
	if counted != total {
		t.Errorf("counted %d of %d actions; compression dropped some", counted, total)
	}
}

// Timestamps are the agent's, and agents are not required to have tidy clocks.
func TestPulseSurvivesOutOfOrderTimes(t *testing.T) {
	var p Pulse
	start := time.Now()

	p.Add(ActionReading, start)
	p.Add(ActionEditing, start.Add(-time.Hour))
	p.Add(ActionReading, time.Time{})

	columns := p.Columns(8)
	if len(columns) != 1 || columns[0].Count != 2 {
		t.Errorf("columns = %v, want the two dated actions in the first bucket", columns)
	}
}
