// Package state implements the NSIGII stateless protocol and here-and-now
// space-time model.
//
// Stateless means: the protocol does not expire via the standard
// major.minor.patch versioning scheme.  An artefact can always be integrated
// into any ecosystem regardless of when it was encoded.
//
// Here-and-Now 2×3 matrix (from the NSIGII spec):
//
//	         | SPACE                | TIME                 | STILLNESS
//	─────────┼──────────────────────┼──────────────────────┼────────────────
//	HERE/NOW | present_in_space     | present_in_time      | stillness
//	         | _then_time           | _then_space          |
//	─────────┼──────────────────────┼──────────────────────┼────────────────
//	THERE/   | there_in_space       | there_in_time        | stillness_past
//	THEN     | _then_time (past)    | _then_space (past)   |
//
// The observer-consumer model: σ = 1 / (observer × consumer)
// where consumer drives suffering (the actor) and observer is independent.
package state

import "fmt"

// SpaceTimeState encodes a single cell of the here-and-now matrix.
type SpaceTimeState int

const (
	// HERE / NOW row
	PresentInSpaceThenTime SpaceTimeState = iota // here, present, space-first
	PresentInTimeThenSpace                       // here, present, time-first
	HereNowStillness                             // here, stillness (equilibrium)

	// THERE / THEN row
	ThereInSpaceThenTime // past, space-first
	ThereInTimeThenSpace // past, time-first
	ThereStillnessPast   // past stillness

	// FUTURE
	InTimeForAllSpace // future: time-first, space for all time
)

func (s SpaceTimeState) String() string {
	switch s {
	case PresentInSpaceThenTime:
		return "present_in_space_then_time"
	case PresentInTimeThenSpace:
		return "present_in_time_then_space"
	case HereNowStillness:
		return "here_now_stillness"
	case ThereInSpaceThenTime:
		return "there_in_space_then_time"
	case ThereInTimeThenSpace:
		return "there_in_time_then_space"
	case ThereStillnessPast:
		return "there_stillness_past"
	case InTimeForAllSpace:
		return "in_time_for_all_space"
	default:
		return "unknown"
	}
}

// HereNowMatrix is the 2×3 space-time reference grid.
// Row 0 = HERE/NOW,  Row 1 = THERE/THEN.
// Columns 0..2 = SPACE, TIME, STILLNESS.
type HereNowMatrix [2][3]SpaceTimeState

// NewHereNowMatrix returns the canonical NSIGII here-and-now matrix.
func NewHereNowMatrix() HereNowMatrix {
	return HereNowMatrix{
		{PresentInSpaceThenTime, PresentInTimeThenSpace, HereNowStillness},
		{ThereInSpaceThenTime, ThereInTimeThenSpace, ThereStillnessPast},
	}
}

// StatelessProtocol manages the current space-time state of the codec.
type StatelessProtocol struct {
	Matrix  HereNowMatrix
	Current SpaceTimeState
	// ObserverWeight and ConsumerWeight for the σ = 1/(O×C) model
	ObserverWeight  float64
	ConsumerWeight  float64
}

// NewStatelessProtocol initialises the protocol at HERE/NOW/SPACE.
func NewStatelessProtocol() *StatelessProtocol {
	return &StatelessProtocol{
		Matrix:         NewHereNowMatrix(),
		Current:        PresentInSpaceThenTime,
		ObserverWeight: 1.0,
		ConsumerWeight: 1.0,
	}
}

// Advance moves the protocol one step forward through the matrix.
// Order:  PresentSpace → PresentTime → HereNowStillness →
//         TherePast… → InTimeForAllSpace → wrap to PresentSpace
func (p *StatelessProtocol) Advance() SpaceTimeState {
	next := (p.Current + 1) % 7
	p.Current = next
	return p.Current
}

// Regress moves the protocol one step backward (rollback support).
func (p *StatelessProtocol) Regress() SpaceTimeState {
	if p.Current == 0 {
		p.Current = InTimeForAllSpace
	} else {
		p.Current--
	}
	return p.Current
}

// SufferingIndex computes σ = N×K / R (need × constant / resources).
// A low resource value pushes σ toward ∞; high resources collapse it to 0.
// This is the NSIGII "encoding suffering into silicon" operator.
func SufferingIndex(need, constant, resources float64) float64 {
	if resources <= 0 {
		return 1<<53 - 1 // near-infinity sentinel
	}
	return (need * constant) / resources
}

// ObserverConsumerRatio computes 1 / (observer × consumer).
// A high consumer load with a low observer produces a large ratio (distress).
func (p *StatelessProtocol) ObserverConsumerRatio() float64 {
	denom := p.ObserverWeight * p.ConsumerWeight
	if denom <= 0 {
		return 1<<53 - 1
	}
	return 1.0 / denom
}

// PrintMatrix displays the 2×3 here-and-now matrix.
func (p *StatelessProtocol) PrintMatrix() {
	fmt.Println("┌─────────────────────────────────────────────────────────┐")
	fmt.Println("│  NSIGII Here-And-Now 2×3 Matrix                         │")
	fmt.Println("├────────────────┬────────────────────┬───────────────────┤")
	fmt.Println("│                │ SPACE              │ TIME              │")
	fmt.Println("├────────────────┼────────────────────┼───────────────────┤")
	fmt.Printf( "│ HERE / NOW     │ %-18s │ %-17s │\n",
		p.Matrix[0][0], p.Matrix[0][1])
	fmt.Printf( "│ THERE / THEN   │ %-18s │ %-17s │\n",
		p.Matrix[1][0], p.Matrix[1][1])
	fmt.Println("└────────────────┴────────────────────┴───────────────────┘")
	fmt.Printf("  Current state: %s\n", p.Current)
}
