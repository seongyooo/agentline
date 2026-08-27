package state

import (
	"time"

	"github.com/seongyooo/agentline/internal/events"
)

// Spinning is an agent that is working and not getting anywhere: the same file
// rewritten over and over, the same command failing the same way, minutes of
// activity that touches nothing it has not already touched.
//
// It is the failure mode that costs the most and shows the least. The agent's
// own terminal cannot show it — a scrollback has no memory of what it already
// said — so nobody finds out until the bill arrives. AgentLine has the one
// thing needed to see it: every action, with its target and its time.
//
// What is reported here is counted, never judged. "Valve.cs edited six times"
// is an observation; "the agent is stuck" is a conclusion, and a refactor and
// a loop look identical from the outside. The counts go on screen and the
// person reading them decides which one this is.
type Spin struct {
	// Since is when the earliest counted action happened.
	Since time.Time

	// Findings are what is repeating, most telling first.
	Findings []Finding
}

// Finding is one repetition, with the evidence for it.
type Finding struct {
	Kind   FindingKind
	Target string
	Count  int
}

// FindingKind names a pattern. The UI turns these into sentences; state counts
// them and stops there.
type FindingKind string

const (
	// RepeatedEdit is one file written again and again.
	RepeatedEdit FindingKind = "repeated_edit"

	// RepeatedFailure is work that keeps failing, whatever it is.
	RepeatedFailure FindingKind = "repeated_failure"

	// RepeatedCommand is the same command run again and again.
	RepeatedCommand FindingKind = "repeated_command"

	// NoNewGround is activity that never reaches anything it has not already
	// reached. On its own it is the weakest signal — a long edit to one file
	// looks the same — so it is only ever reported beside another.
	NoNewGround FindingKind = "no_new_ground"
)

// Thresholds. Set where a person looking at the log would start to wonder,
// rather than where a loop is provably a loop: the screen states the count and
// lets them judge, so being early is cheap and being late is not.
const (
	spinWindow = 5 * time.Minute

	spinEdits    = 4  // writes to one file
	spinFailures = 3  // failures of any kind
	spinCommands = 3  // runs of one command
	spinActions  = 10 // actions before "no new ground" means anything
)

// detectSpin reports what the recent log is repeating, or nil when it is not.
func detectSpin(actions []Action, now time.Time) *Spin {
	recent, earlier := splitAt(actions, now.Add(-spinWindow))
	if len(recent) == 0 {
		return nil
	}

	findings := append(repeatedTargets(recent), failureCount(recent)...)
	// Weakest last, and never alone: on its own it cannot tell a loop from a
	// long piece of work on one file.
	if len(findings) > 0 {
		if ground := noNewGround(recent, earlier); ground != nil {
			findings = append(findings, *ground)
		}
	}
	if len(findings) == 0 {
		return nil
	}
	return &Spin{Since: recent[0].At, Findings: findings}
}

// splitAt divides the log at a moment, since both halves are needed: what has
// been happening, and what had already been reached before it started.
func splitAt(actions []Action, at time.Time) (recent, earlier []Action) {
	for i, a := range actions {
		if !a.At.Before(at) {
			return actions[i:], actions[:i]
		}
	}
	return nil, actions
}

// repeatedTargets counts writes to one file and runs of one command.
func repeatedTargets(actions []Action) []Finding {
	edits := map[string]int{}
	commands := map[string]int{}

	for _, a := range actions {
		switch a.Kind {
		case ActionEditing, ActionWriting, ActionCreating:
			if a.Target != "" {
				edits[a.Target]++
			}
		case ActionRunning:
			if a.Target != "" {
				commands[a.Target]++
			}
		}
	}

	var findings []Finding
	findings = append(findings, over(edits, spinEdits, RepeatedEdit)...)
	return append(findings, over(commands, spinCommands, RepeatedCommand)...)
}

// over returns the entries at or past a threshold, worst first.
func over(counts map[string]int, threshold int, kind FindingKind) []Finding {
	var findings []Finding
	for target, count := range counts {
		if count >= threshold {
			findings = append(findings, Finding{Kind: kind, Target: target, Count: count})
		}
	}
	sortFindings(findings)
	return findings
}

// failureCount reports repeated failure, which is worth saying even when each
// one was a different thing: three failures in five minutes is the pattern,
// not the individual failures.
func failureCount(actions []Action) []Finding {
	count := 0
	for _, a := range actions {
		if a.Kind == ActionFailed {
			count++
		}
	}
	if count < spinFailures {
		return nil
	}
	return []Finding{{Kind: RepeatedFailure, Count: count}}
}

// noNewGround reports a stretch that never reached anything it had not already
// reached before the stretch began.
func noNewGround(recent, earlier []Action) *Finding {
	if len(recent) < spinActions {
		return nil
	}

	seen := map[string]bool{}
	for _, a := range earlier {
		if a.Target != "" {
			seen[a.Target] = true
		}
	}
	for _, a := range recent {
		if a.Target != "" && !seen[a.Target] {
			return nil // it got somewhere new
		}
	}
	return &Finding{Kind: NoNewGround, Count: len(recent)}
}

// sortFindings orders by count, then by target, so the same evidence is always
// listed the same way and a redraw does not shuffle it.
func sortFindings(findings []Finding) {
	for i := 1; i < len(findings); i++ {
		for j := i; j > 0; j-- {
			a, b := findings[j-1], findings[j]
			if a.Count > b.Count || (a.Count == b.Count && a.Target <= b.Target) {
				break
			}
			findings[j-1], findings[j] = b, a
		}
	}
}

// spinRearm is how much worse a finding already waved away has to get before
// it is worth raising again. Without it, dismissing puts the same evidence
// back on the screen on the next action.
const spinRearm = 2

// Refresh recomputes what is derived from the log rather than reported.
//
// It is called after every event and on the clock, because spinning is partly
// a fact about time passing: an agent that stops doing anything new stops
// spinning without a single further event arriving to say so.
func (s *State) Refresh(now time.Time) {
	// Only work in progress can be going nowhere. A turn that has ended is
	// not spinning, it is finished, and a question the agent is blocked on
	// outranks anything AgentLine worked out on its own.
	if s.Agent.Status != events.StatusWorking || s.Agent.Ask != nil {
		s.Agent.Spin = nil
		clear(s.dismissed) // a new turn is a new situation
		return
	}

	spin := detectSpin(s.trace, now)
	if spin == nil {
		s.Agent.Spin = nil
		return
	}
	if spin.Findings = s.stillWorthSaying(spin.Findings); len(spin.Findings) == 0 {
		s.Agent.Spin = nil
		return
	}
	s.Agent.Spin = spin
}

// DismissSpin takes the current evidence off the screen and remembers it, so
// it comes back only if the repetition gets worse.
func (s *State) DismissSpin() {
	if s.Agent.Spin == nil {
		return
	}
	for _, f := range s.Agent.Spin.Findings {
		s.dismissed[findingKey(f)] = f.Count
	}
	s.Agent.Spin = nil
}

// stillWorthSaying drops what the user has already seen and waved away.
func (s *State) stillWorthSaying(findings []Finding) []Finding {
	kept := findings[:0]
	for _, f := range findings {
		if at, seen := s.dismissed[findingKey(f)]; seen && f.Count < at+spinRearm {
			continue
		}
		kept = append(kept, f)
	}
	return kept
}

func findingKey(f Finding) string { return string(f.Kind) + "\x00" + f.Target }
