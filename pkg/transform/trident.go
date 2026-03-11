// Trident implements the 3-channel verification architecture.
//
// Three channels with loopback addressing:
//
//	Channel 0 — TRANSMITTER  127.0.0.1  (1/3)  WRITE   → encodes
//	Channel 1 — RECEIVER     127.0.0.2  (2/3)  READ    → decodes
//	Channel 2 — VERIFIER     127.0.0.3  (3/3)  EXECUTE → validates
//
// The Verifier uses Discriminant Flash: Δ = b² - 4ac
//
//	Δ > 0  → ORDER     (two real roots — coherent signal)
//	Δ = 0  → CONSENSUS (flash point — perfect balance)
//	Δ < 0  → CHAOS     (complex roots — repair triggered)
//
// Wheel positions: Transmitter=0°, Receiver=120°, Verifier=240°
package transform

import (
	"math"
)

// TridentState represents the discriminant-derived state of a channel.
type TridentState int

const (
	StateOrder     TridentState = iota // Δ > 0: coherent
	StateConsensus                     // Δ = 0: flash point
	StateChaos                         // Δ < 0: repair needed
)

func (s TridentState) String() string {
	switch s {
	case StateOrder:
		return "ORDER"
	case StateConsensus:
		return "CONSENSUS"
	default:
		return "CHAOS"
	}
}

// RWXFlags mirrors Unix-style permissions for channel access.
const (
	RWXRead    uint8 = 0x04
	RWXWrite   uint8 = 0x02
	RWXExecute uint8 = 0x01
	RWXFull    uint8 = 0x07
)

// ChannelID names the three trident channels.
type ChannelID uint8

const (
	ChannelTransmitter ChannelID = 0
	ChannelReceiver    ChannelID = 1
	ChannelVerifier    ChannelID = 2
)

// TridentResult carries the outcome of a full 3-channel verification pass.
type TridentResult struct {
	Data         []byte
	State        TridentState
	RWXFlags     uint8
	WheelDeg     int     // 0, 120, or 240
	Discriminant float64 // Δ = b² - 4ac
	Verified     bool
	Polarity     byte // '+' or '-'
}

// RunTrident executes the full TRANSMIT → RECEIVE → VERIFY pipeline.
// Returns a TridentResult describing the final state.
func RunTrident(data []byte) TridentResult {
	// Channel 0: TRANSMITTER — apply write-side encoding
	transmitted := transmit(data)

	// Channel 1: RECEIVER — read-side echo
	received := receive(transmitted)

	// Channel 2: VERIFIER — discriminant flash check
	a, b, c := bipartiteConsensusParams(received)
	delta := b*b - 4*a*c

	var state TridentState
	var rwx uint8
	var wheelDeg int

	switch {
	case delta > 0:
		state = StateOrder
		rwx = RWXFull
		wheelDeg = 120
	case delta == 0:
		state = StateConsensus
		rwx = RWXFull
		wheelDeg = 240
	default:
		state = StateChaos
		// Apply enzyme repair on CHAOS: XOR adjacent bytes to attempt correction
		received = enzymeRepair(received)
		rwx = RWXRead
		wheelDeg = 0
	}

	polarity := PolaritySign(received)

	return TridentResult{
		Data:         received,
		State:        state,
		RWXFlags:     rwx,
		WheelDeg:     wheelDeg,
		Discriminant: delta,
		Verified:     state != StateChaos,
		Polarity:     polarity,
	}
}

// DiscriminantState computes only the state classification for raw bytes.
// Useful for a quick parity check without running the full pipeline.
func DiscriminantState(data []byte) TridentState {
	a, b, c := bipartiteConsensusParams(data)
	delta := b*b - 4*a*c
	switch {
	case delta > 0:
		return StateOrder
	case delta == 0:
		return StateConsensus
	default:
		return StateChaos
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────────────

// transmit is the Channel 0 operation — simple pass-through with WRITE flag.
// The actual encoding is handled by the isomorphic layer; here we only
// enforce the channel boundary.
func transmit(data []byte) []byte {
	out := make([]byte, len(data))
	copy(out, data)
	return out
}

// receive is the Channel 1 operation — bipartite order check.
// Even-indexed bytes set ORDER, odd-indexed bytes set CHAOS.
// The result is a parity-adjusted copy.
func receive(data []byte) []byte {
	out := make([]byte, len(data))
	for i, b := range data {
		if i%2 == 0 {
			out[i] = b          // ORDER lane
		} else {
			out[i] = b ^ 0x01  // CHAOS lane: flip LSB
		}
	}
	return out
}

// bipartiteConsensusParams maps byte content entropy to quadratic
// discriminant parameters A=1, B∈[0,4], C=1.
//
//	consensus → 1.0: B = 4 → Δ = 12 (ORDER)
//	consensus → 0.5: B = 2 → Δ =  0 (CONSENSUS flash point)
//	consensus → 0.0: B = 0 → Δ = -4 (CHAOS)
func bipartiteConsensusParams(data []byte) (a, b, c float64) {
	if len(data) == 0 {
		return 1, 0, 1
	}
	var setBits int
	for _, by := range data {
		for v := by; v != 0; v >>= 1 {
			setBits += int(v & 1)
		}
	}
	totalBits := len(data) * 8
	density := float64(setBits) / float64(totalBits)
	// Map density to consensus in [0,1] via sin correction
	wheelPos := 240.0 // Verifier at 240°
	correction := math.Sin(wheelPos * math.Pi / 180.0)
	consensus := math.Abs(density+correction) / 2.0
	if consensus > 1.0 {
		consensus = 1.0
	}
	return 1.0, consensus * 4.0, 1.0
}

// enzymeRepair applies a simple XOR repair (ENZYME_REPAIR equivalent):
// each byte is XORed with its predecessor to attempt error correction.
func enzymeRepair(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	out := make([]byte, len(data))
	copy(out, data)
	for i := 1; i < len(out); i++ {
		out[i] ^= out[i-1]
	}
	return out
}
