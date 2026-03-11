// Package transform implements the NSIGII isomorphic transformation layer.
//
// "Isomorphic" means encode(decode(x)) = x and decode(encode(x)) = x for ALL
// data types — binary, text, fonts, email, HTML.  The transform is a keyed XOR
// with nibble conjugation and parity-axis normalisation.
//
// Key operators (from the NSIGII spec):
//
//	XOR  (⊕)   — remembers the state via exclusive power
//	Conjugate  — 0xF ⊕ nibble  (nibble-level reflection)
//	RightShift — normalise undefined states → 0 ("space holder")
//	LeftShift  — expand outward (yields undefined on overflow, handled by parity)
//	Parity     — even/odd axis check; rotation/reflection/translation/enlargement
package transform

// DeriveKey produces a repeating key from a UUID string.
// Any-length uuid → []byte key of at least 32 bytes.
func DeriveKey(uuid string) []byte {
	src := []byte(uuid)
	if len(src) == 0 {
		src = []byte("NSIGII-STATELESS-KEY")
	}
	// Expand to 64 bytes via position-shifted XOR accumulation
	key := make([]byte, 64)
	for i := range key {
		key[i] = src[i%len(src)] ^ byte(i*7+13)
	}
	return key
}

// Encode applies the isomorphic XOR transform.
// encode(decode(data, key), key) == data  (self-inverse).
func Encode(data, key []byte) []byte {
	if len(key) == 0 {
		return append([]byte(nil), data...)
	}
	out := make([]byte, len(data))
	for i, b := range data {
		k := key[i%len(key)]
		out[i] = b ^ ConjugateNibble(k)
	}
	return out
}

// Decode is identical to Encode — XOR is self-inverse.
// decode(encode(data, key), key) == data.
func Decode(data, key []byte) []byte {
	return Encode(data, key) // XOR is its own inverse
}

// ConjugateNibble performs nibble-level reflection: 0xF ⊕ (b & 0xF).
// The high nibble is preserved; only the low nibble is conjugated.
func ConjugateNibble(b byte) byte {
	return (b & 0xF0) | (0xF ^ (b & 0x0F))
}

// RightShiftNormalise shifts each byte right by 1, padding with 0.
// Undefined states collapse to 0 ("space placeholder" — decimal zero,
// binary zero; holds nothing but space).
func RightShiftNormalise(data []byte) []byte {
	out := make([]byte, len(data))
	for i, b := range data {
		out[i] = b >> 1
	}
	return out
}

// LeftShiftExpand shifts each byte left by 1.
// This expands outward; overflow bits become undefined (parity handles them).
func LeftShiftExpand(data []byte) []byte {
	out := make([]byte, len(data))
	for i, b := range data {
		out[i] = b << 1
	}
	return out
}

// ParityAxis computes the parity axis of a byte slice.
// Returns (evenCount, oddCount, parityByte).
//
//	parityByte 0x00 → pure even  (ORDER axis)
//	parityByte 0xFF → pure odd   (CHAOS axis)
//	otherwise  → mixed (CONSENSUS)
func ParityAxis(data []byte) (even, odd int, parityByte byte) {
	for _, b := range data {
		if b%2 == 0 {
			even++
		} else {
			odd++
		}
	}
	if len(data) == 0 {
		return 0, 0, 0
	}
	ratio := float64(odd) / float64(len(data))
	switch {
	case ratio < 0.1:
		parityByte = 0x00 // ORDER
	case ratio > 0.9:
		parityByte = 0xFF // CHAOS
	default:
		// Interpolate: consensus zone 0x01–0xFE
		parityByte = byte(ratio * 254)
	}
	return even, odd, parityByte
}

// BitFlipCheck detects unexpected bit flips by comparing two parity records.
// Returns true if the parity is consistent (no flips detected).
func BitFlipCheck(original, decoded []byte) bool {
	if len(original) != len(decoded) {
		return false
	}
	var diff int
	for i := range original {
		xored := original[i] ^ decoded[i]
		// Count set bits (Hamming weight)
		for xored != 0 {
			diff += int(xored & 1)
			xored >>= 1
		}
	}
	// Allow zero bit-flips for a clean isomorphic round-trip
	return diff == 0
}

// PolaritySign returns '+' for even-dominant data, '-' for odd-dominant.
// Mirrors the ROPEN polarity model (positive = ORDER, negative = CHAOS).
func PolaritySign(data []byte) byte {
	var even, odd int
	for _, b := range data {
		if b%2 == 0 {
			even++
		} else {
			odd++
		}
	}
	if even >= odd {
		return '+'
	}
	return '-'
}
