// Package teletext builds EN 300 706 teletext packets for subtitle pages.
//
// Bit order: EN 300 706 numbers the bits of a byte in transmission order,
// least significant bit first (bit 1 = 0x01). All bytes produced by this
// package are in that "line order". EN 300 472 PES data units carry every
// byte bit-reversed; the mpegts package does that conversion.
package teletext

import "math/bits"

// Ham84 returns the Hamming 8/4 code word for the low nibble of d
// (EN 300 706 clause 8.2). Bits in transmission order: P1 D1 P2 D2 P3 D3 P4 D4.
func Ham84(d byte) byte {
	d1, d2, d3, d4 := d&1, d>>1&1, d>>2&1, d>>3&1
	p1 := 1 ^ d1 ^ d3 ^ d4
	p2 := 1 ^ d1 ^ d2 ^ d4
	p3 := 1 ^ d1 ^ d2 ^ d3
	p4 := 1 ^ p1 ^ d1 ^ p2 ^ d2 ^ p3 ^ d3 ^ d4
	return p1 | d1<<1 | p2<<2 | d2<<3 | p3<<4 | d3<<5 | p4<<6 | d4<<7
}

// UnHam84 decodes a Hamming 8/4 byte, correcting a single bit error.
// ok is false when the byte has a detectable double error.
func UnHam84(b byte) (d byte, ok bool) {
	for v := byte(0); v < 16; v++ {
		if bits.OnesCount8(Ham84(v)^b) <= 1 {
			return v, true
		}
	}
	return 0, false
}

// ham2418DataPos are the 1-based bit positions of D1..D18 within a triplet.
var ham2418DataPos = [18]uint{3, 5, 6, 7, 9, 10, 11, 12, 13, 14, 15, 17, 18, 19, 20, 21, 22, 23}

// Ham2418 returns the three Hamming 24/18 bytes for the 18 data bits of d
// (EN 300 706 clause 8.3). D1 is bit 0 of d.
func Ham2418(d uint32) [3]byte {
	var w uint32 // bit i-1 holds transmission bit i
	for i, p := range ham2418DataPos {
		w |= (d >> uint(i) & 1) << (p - 1)
	}
	// P1..P5 at positions 1, 2, 4, 8, 16: each makes an odd-parity group over
	// the positions whose index has that bit set (tests A-E of clause 8.3).
	for k := uint(0); k < 5; k++ {
		var x uint32 = 1
		for pos := uint(1); pos <= 23; pos++ {
			if pos&(1<<k) != 0 && pos != 1<<k {
				x ^= w >> (pos - 1) & 1
			}
		}
		w |= x << ((1 << k) - 1)
	}
	// P6 at position 24: odd parity over the whole triplet.
	if bits.OnesCount32(w)%2 == 0 {
		w |= 1 << 23
	}
	return [3]byte{byte(w), byte(w >> 8), byte(w >> 16)}
}

// UnHam2418 decodes a Hamming 24/18 triplet, correcting a single bit error.
// ok is false when a double error is detected.
func UnHam2418(t [3]byte) (d uint32, ok bool) {
	w := uint32(t[0]) | uint32(t[1])<<8 | uint32(t[2])<<16
	var syndrome uint
	for k := uint(0); k < 5; k++ {
		var x uint32
		for pos := uint(1); pos <= 23; pos++ {
			if pos&(1<<k) != 0 {
				x ^= w >> (pos - 1) & 1
			}
		}
		if x != 1 { // odd parity test failed
			syndrome |= 1 << k
		}
	}
	allOdd := bits.OnesCount32(w)%2 == 1
	switch {
	case syndrome == 0:
		// No error, or an error in P6 only.
	case allOdd:
		return 0, false // double error
	default:
		if syndrome > 23 {
			return 0, false
		}
		w ^= 1 << (syndrome - 1)
	}
	for i, p := range ham2418DataPos {
		d |= (w >> (p - 1) & 1) << uint(i)
	}
	return d, true
}

// OddParity returns the 7-bit code c with bit 8 set so the byte has odd parity
// (EN 300 706 clause 8.1).
func OddParity(c byte) byte {
	c &= 0x7f
	if bits.OnesCount8(c)%2 == 0 {
		c |= 0x80
	}
	return c
}
