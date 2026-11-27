package teletext

import "testing"

func lang(t *testing.T, code string) Language {
	t.Helper()
	l, ok := LookupLanguage(code)
	if !ok {
		t.Fatalf("no preset %s", code)
	}
	return l
}

func TestParsePage(t *testing.T) {
	a, err := ParsePage("888")
	if err != nil || a != (PageAddr{8, 0x88}) {
		t.Fatalf("ParsePage(888) = %v, %v", a, err)
	}
	if _, err := ParsePage("900"); err == nil {
		t.Error("page 900 accepted")
	}
}

func TestComposeLevel1Rows(t *testing.T) {
	l := lang(t, "swe")
	lines := []Line{{Text: "Hej"}, {Text: "på dig", Colour: Yellow}}
	pg, err := ComposePage(PageAddr{8, 0x88}, NewEncoder(l.Charset, true), lines, PageOptions{Level15: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(pg.Body) != 2 || pg.Body[0].Y != 20 || pg.Body[1].Y != 22 {
		t.Fatalf("rows = %+v", pg.Body)
	}
	row := pg.Body[1].Data
	var codes []byte
	for _, b := range row {
		if b != OddParity(b&0x7f) {
			t.Fatalf("parity error in %#02x", b)
		}
		codes = append(codes, b&0x7f)
	}
	want := "\x0d\x03\x0b\x0bp\x7d dig\x0a\x0a"
	start := (40 - len(want)) / 2
	if got := string(codes[start : start+len(want)]); got != want {
		t.Errorf("row 22 = %q, want %q", got, want)
	}
	// Header: C4 erase, C6 subtitle, C7 suppress header.
	if v, _ := UnHam84(pg.Header.Data[3]); v&8 == 0 {
		t.Error("C4 not set")
	}
	if v, _ := UnHam84(pg.Header.Data[5]); v&8 == 0 {
		t.Error("C6 not set")
	}
	if v, _ := UnHam84(pg.Header.Data[6]); v&1 == 0 {
		t.Error("C7 not set")
	}
}

func TestComposeDesignation(t *testing.T) {
	l := lang(t, "rus")
	pg, err := ComposePage(PageAddr{8, 0x88}, NewEncoder(l.Charset, true), []Line{{Text: l.Sample}}, PageOptions{})
	if err != nil {
		t.Fatal(err)
	}
	x28 := pg.Body[0]
	if x28.Y != 28 {
		t.Fatalf("first body packet is X/%d, want X/28", x28.Y)
	}
	t1, ok := UnHam2418([3]byte{x28.Data[1], x28.Data[2], x28.Data[3]})
	if !ok {
		t.Fatal("triplet 1 does not decode")
	}
	if t1&0x7f != 0 {
		t.Errorf("page function/coding = %#x, want 0", t1&0x7f)
	}
	if got := byte(t1 >> 7 & 0x7f); got != 4<<3|0b100 {
		t.Errorf("designation = %07b, want 0100100 (group 4, Cyrillic-2)", got)
	}

	// Group 0 charsets need no designation in auto mode.
	l = lang(t, "swe")
	pg, _ = ComposePage(PageAddr{8, 0x88}, NewEncoder(l.Charset, true), []Line{{Text: "x"}}, PageOptions{})
	if pg.Body[0].Y == 28 {
		t.Error("X/28 sent for a group 0 charset")
	}
	// M/29 mode puts the packet before the header instead.
	l = lang(t, "pol")
	opt := PageOptions{Designation: DesignationM29}
	pg, _ = ComposePage(PageAddr{8, 0x88}, NewEncoder(l.Charset, true), []Line{{Text: "x"}}, opt)
	if pg.M29 == nil || pg.M29.Y != 29 || pg.Body[0].Y == 28 {
		t.Error("M/29 mode did not produce M/29/0 only")
	}
}

func TestComposeX26(t *testing.T) {
	l := lang(t, "dan")
	enc := NewEncoder(l.Charset, true)
	pg, err := ComposePage(PageAddr{8, 0x88}, enc, []Line{{Text: "Ærø"}}, PageOptions{Level15: true})
	if err != nil {
		t.Fatal(err)
	}
	x26 := pg.Body[0]
	if x26.Y != 26 {
		t.Fatalf("first body packet is X/%d, want X/26", x26.Y)
	}
	var ts []Triplet
	for i := 0; i < 13; i++ {
		v, ok := UnHam2418([3]byte{x26.Data[1+3*i], x26.Data[2+3*i], x26.Data[3+3*i]})
		if !ok {
			t.Fatalf("triplet %d does not decode", i)
		}
		ts = append(ts, Triplet{Address: byte(v & 0x3f), Mode: byte(v >> 6 & 0x1f), Data: byte(v >> 11)})
	}
	start := byte((40 - 7 - 3) / 2) // prefix 0d 07 0b 0b, suffix 0a 0a
	col := start + 4
	want := []Triplet{
		{Address: 40 + 22, Mode: ModeSetActivePosition},
		{Address: col, Mode: ModeG2Character, Data: 0x61},     // Æ
		{Address: col + 2, Mode: ModeG2Character, Data: 0x79}, // ø
		TerminationTriplet,
	}
	for i, w := range want {
		if ts[i] != w {
			t.Errorf("triplet %d = %+v, want %+v", i, ts[i], w)
		}
	}
	for i := len(want); i < 13; i++ {
		if ts[i] != TerminationTriplet {
			t.Errorf("unused triplet %d = %+v, want termination marker", i, ts[i])
		}
	}
}

func TestFillerHeader(t *testing.T) {
	h := FillerHeader(8)
	units, _ := UnHam84(h.Data[0])
	tens, _ := UnHam84(h.Data[1])
	if h.Magazine != 8 || h.Y != 0 || units != 0xF || tens != 0xF {
		t.Errorf("filler header is %d/%d page %X%X, want 8/0 page FF", h.Magazine, h.Y, tens, units)
	}
	if c4, _ := UnHam84(h.Data[3]); c4&8 != 0 {
		t.Error("filler header erases the page")
	}
}

func TestComposeTooLong(t *testing.T) {
	long := []Line{{Text: "0123456789012345678901234567890123456789"}}
	_, err := ComposePage(PageAddr{8, 0x88}, NewEncoder(CharsetEnglish, true), long, PageOptions{})
	if err == nil {
		t.Error("40 character line accepted")
	}
}
