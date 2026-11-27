package teletext

import (
	"testing"

	"golang.org/x/text/unicode/norm"
)

// combining marks for X/26 diacritic modes 0x11-0x1F (G2 0x41-0x4F).
var modeMarks = map[byte]rune{
	0x11: '̀', 0x12: '́', 0x13: '̂', 0x14: '̃', 0x15: '̄',
	0x16: '̆', 0x17: '̇', 0x18: '̈', 0x19: '̣', 0x1A: '̊',
	0x1B: '̧', 0x1D: '̋', 0x1E: '̨', 0x1F: '̌',
}

// decodeCells is what a Level 1.5 decoder would show.
func decodeCells(cs Charset, cells []Cell) string {
	g0 := cs.G0Table()
	var out []rune
	for _, c := range cells {
		switch {
		case c.X26 == nil:
			out = append(out, g0[c.Code-0x20])
		case c.X26.Mode == ModeG2Character:
			out = append(out, g2LatinTable[c.X26.Data-0x20])
		case c.X26.Mode == ModeG0NoDiacritic:
			out = append(out, g0LatinTable[c.X26.Data-0x20])
		default:
			composed := norm.NFC.String(string([]rune{g0LatinTable[c.X26.Data-0x20], modeMarks[c.X26.Mode]}))
			out = append(out, []rune(composed)...)
		}
	}
	return string(out)
}

// canonical applies the encoder aliases, for characters not in the charset's
// G0 set, so expected text compares with decoded text.
func canonical(cs Charset, s string) string {
	in := map[rune]bool{}
	for _, r := range cs.G0Table() {
		in[r] = true
	}
	var out []rune
	for _, r := range norm.NFC.String(s) {
		if a, ok := aliases[r]; ok && !in[r] {
			r = a
		}
		out = append(out, r)
	}
	return string(out)
}

func TestPresetsRoundTrip(t *testing.T) {
	for _, l := range Languages {
		for _, text := range []string{l.Sample, l.Name, "0123456789 :.()"} {
			enc := NewEncoder(l.Charset, true)
			cells, missing := enc.Encode(text)
			if len(missing) > 0 {
				t.Errorf("%s: %q has no code for %q", l.Code, text, string(missing))
				continue
			}
			if got, want := decodeCells(l.Charset, cells), canonical(l.Charset, text); got != want {
				t.Errorf("%s: decoded %q, want %q", l.Code, got, want)
			}
		}
	}
}

func TestLevel1Fallback(t *testing.T) {
	enc := NewEncoder(CharsetSwedishFinnishHungarian, false)
	cells, missing := enc.Encode("Győr Ærø")
	if len(missing) > 0 {
		t.Fatalf("missing %q", string(missing))
	}
	var got []byte
	for _, c := range cells {
		if c.X26 != nil {
			t.Fatalf("X/26 sent with Level 1.5 off: %+v", c)
		}
		got = append(got, c.Code)
	}
	// ő falls back to o, Æ to A and ø to o.
	if string(got) != "Gyor Aro" {
		t.Errorf("Level 1 codes = %q", got)
	}
}

// The Romanian sub-set has Î at 0x5E and î at 0x7E (EN 300 706 table 36), so
// both are plain Level 1 codes. Í is not in it and needs an X/26 accent.
func TestRomanianICircumflex(t *testing.T) {
	enc := NewEncoder(CharsetRomanian, true)
	cells, missing := enc.Encode("Îî")
	if len(missing) > 0 || len(cells) != 2 {
		t.Fatalf("%d cells, missing %q", len(cells), string(missing))
	}
	if cells[0] != (Cell{Code: 0x5E}) || cells[1] != (Cell{Code: 0x7E}) {
		t.Errorf("Î and î are %+v and %+v, want codes 0x5E and 0x7E without X/26", cells[0], cells[1])
	}
	cells, _ = enc.Encode("Í")
	if len(cells) != 1 || cells[0].Code != 'I' || cells[0].X26 == nil {
		t.Errorf("Í is %+v, want I with an X/26 acute accent", cells)
	}
	if got := decodeCells(CharsetRomanian, cells); got != "Í" {
		t.Errorf("Í decodes as %q", got)
	}
}

func TestNationalOptionHeaderBits(t *testing.T) {
	// Table 32 value 010 (Swedish/Finnish/Hungarian) is C12=0 C13=1 C14=0,
	// sent in header byte 7 as C11 C12 C13 C14 = bits 0..3.
	h := HeaderPacket(PageAddr{8, 0x88}, HeaderControl{NationalOption: 0b010}, "")
	if v, _ := UnHam84(h.Data[7]); v != 0b0100 {
		t.Errorf("C11-C14 nibble = %04b, want 0100", v)
	}
	h = HeaderPacket(PageAddr{8, 0x88}, HeaderControl{NationalOption: 0b100}, "")
	if v, _ := UnHam84(h.Data[7]); v != 0b0010 {
		t.Errorf("C11-C14 nibble for French = %04b, want 0010", v)
	}
}

func TestUnencodable(t *testing.T) {
	enc := NewEncoder(CharsetCyrillicRussianBulgarian, true)
	_, missing := enc.Encode("Ёж €")
	if string(missing) != "Ё€" {
		t.Errorf("missing = %q, want %q", string(missing), "Ё€")
	}
}
