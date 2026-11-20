package teletext

import (
	"math/bits"
	"math/rand"
	"testing"
)

// The Hamming 8/4 code words as published in telxcc, zvbi and others.
var ham84Reference = [16]byte{
	0x15, 0x02, 0x49, 0x5e, 0x64, 0x73, 0x38, 0x2f,
	0xd0, 0xc7, 0x8c, 0x9b, 0xa1, 0xb6, 0xfd, 0xea,
}

func TestHam84MatchesReference(t *testing.T) {
	for v, want := range ham84Reference {
		if got := Ham84(byte(v)); got != want {
			t.Errorf("Ham84(%d) = %#02x, want %#02x", v, got, want)
		}
	}
}

func TestUnHam84CorrectsAndDetects(t *testing.T) {
	for v := byte(0); v < 16; v++ {
		c := Ham84(v)
		if d, ok := UnHam84(c); !ok || d != v {
			t.Fatalf("UnHam84(%#02x) = %d,%v", c, d, ok)
		}
		for i := 0; i < 8; i++ {
			if d, ok := UnHam84(c ^ 1<<i); !ok || d != v {
				t.Errorf("single error bit %d in %d not corrected", i, v)
			}
			for j := i + 1; j < 8; j++ {
				if _, ok := UnHam84(c ^ 1<<i ^ 1<<j); ok {
					t.Errorf("double error bits %d,%d in %d not detected", i, j, v)
				}
			}
		}
	}
}

// telxcc24 is the Hamming 24/18 decoder used by telxcc, CCExtractor and
// shaka-packager (es_parser_teletext.cc), ported as an independent oracle.
func telxcc24(value uint32) (uint32, bool) {
	result := value
	var test uint8
	for i := uint8(0); i < 23; i++ {
		test ^= uint8((result>>i)&1) * (i + 0x21)
	}
	test ^= uint8((result>>0x17)&1) * 0x20
	if test&0x1f != 0x1f {
		if test&0x20 == 0x20 {
			return 0, false
		}
		result ^= 1 << (0x1e - test)
	}
	return (result&0x000004)>>2 | (result&0x000070)>>3 | (result&0x007f00)>>4 | (result&0x7f0000)>>5, true
}

func TestHam2418AllValues(t *testing.T) {
	for d := uint32(0); d < 1<<18; d++ {
		h := Ham2418(d)
		w := uint32(h[0]) | uint32(h[1])<<8 | uint32(h[2])<<16
		if bits.OnesCount32(w)%2 != 1 {
			t.Fatalf("Ham2418(%#x) does not have odd parity", d)
		}
		if got, ok := UnHam2418(h); !ok || got != d {
			t.Fatalf("UnHam2418(Ham2418(%#x)) = %#x,%v", d, got, ok)
		}
		if got, ok := telxcc24(w); !ok || got != d {
			t.Fatalf("telxcc decode of Ham2418(%#x) = %#x,%v", d, got, ok)
		}
	}
}

func TestHam2418Errors(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for n := 0; n < 20000; n++ {
		d := uint32(r.Intn(1 << 18))
		h := Ham2418(d)
		i := r.Intn(24)
		s := h
		s[i/8] ^= 1 << (i % 8)
		if got, ok := UnHam2418(s); !ok || got != d {
			t.Fatalf("single error at bit %d of %#x: got %#x,%v", i, d, got, ok)
		}
		j := (i + 1 + r.Intn(23)) % 24
		s[j/8] ^= 1 << (j % 8)
		if _, ok := UnHam2418(s); ok {
			t.Fatalf("double error at bits %d,%d of %#x not detected", i, j, d)
		}
	}
}

func TestOddParity(t *testing.T) {
	for c := byte(0); c < 0x80; c++ {
		p := OddParity(c)
		if bits.OnesCount8(p)%2 != 1 || p&0x7f != c {
			t.Errorf("OddParity(%#02x) = %#02x", c, p)
		}
	}
}
