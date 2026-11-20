// Command astisub-stl writes and reads EBU STL files with go-astisub to show
// the gaps described in ../../docs/research.md (character mapping, extension
// blocks, comment flag, other character code tables).

package main

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/asticode/go-astisub"
)

func item(start, end time.Duration, lines ...string) *astisub.Item {
	it := &astisub.Item{StartAt: start, EndAt: end}
	for _, l := range lines {
		it.Lines = append(it.Lines, astisub.Line{Items: []astisub.LineItem{{Text: l}}})
	}
	return it
}

func roundtrip(name string, s *astisub.Subtitles) []byte {
	var buf bytes.Buffer
	if err := s.WriteToSTL(&buf); err != nil {
		fmt.Println(name, "write error:", err)
		return nil
	}
	b := buf.Bytes()
	r, err := astisub.ReadFromSTL(bytes.NewReader(b), astisub.STLOptions{})
	if err != nil {
		fmt.Println(name, "read error:", err)
		return b
	}
	fmt.Printf("%-22s GSI lang=%q country=%q DSC=%q CCT=%q -> ", name, b[14:16], b[274:277], b[11:12], b[12:14])
	for _, it := range r.Items {
		fmt.Printf("[%v-%v %q] ", it.StartAt, it.EndAt, it.String())
	}
	fmt.Println()
	return b
}

func main() {
	// 1. Character round trips
	s := astisub.NewSubtitles()
	s.Metadata = &astisub.Metadata{Language: astisub.LanguageNorwegian}
	s.Items = []*astisub.Item{
		item(time.Second, 2*time.Second, "Pris: $5 eller 4 €", "Åsa på Øya – “hei”…"),
		item(3*time.Second, 4*time.Second, "Ж Привет мир"),
		item(5*time.Second, 6*time.Second, "Ελληνικά"),
	}
	b := roundtrip("chars(norwegian)", s)
	fmt.Printf("   TTI1 text bytes: % x\n", bytes.TrimRight(b[1024+16:1024+128], "\x8f"))
	fmt.Printf("   TTI2 text bytes: % x\n", bytes.TrimRight(b[1024+128+16:1024+256], "\x8f"))

	// 2. Swedish metadata -> what language code?
	s2 := astisub.NewSubtitles()
	s2.Metadata = &astisub.Metadata{Language: "swedish"}
	s2.Items = []*astisub.Item{item(time.Second, 2*time.Second, "Hej")}
	roundtrip("lang(swedish)", s2)

	// 3. Long text > 112 bytes
	s3 := astisub.NewSubtitles()
	lower, upper := strings.Repeat("abcdefghij", 6), strings.Repeat("ABCDEFGHIJ", 6)
	s3.Items = []*astisub.Item{item(time.Second, 2*time.Second, lower, upper)}
	roundtrip("long(120 chars)", s3)

	// Base a hand-made file on the writer's GSI
	gsi := append([]byte(nil), b[:1024]...)
	tti := func(sn int, ebn, cf byte, tci, tco [4]byte, text []byte) []byte {
		t := make([]byte, 128)
		t[1], t[2], t[3], t[4] = byte(sn), byte(sn>>8), ebn, 0
		copy(t[5:9], tci[:])
		copy(t[9:13], tco[:])
		t[13], t[14], t[15] = 20, 2, cf
		for i := 16; i < 128; i++ {
			t[i] = 0x8f
		}
		copy(t[16:], text)
		return t
	}
	read := func(name string, f []byte) {
		r, err := astisub.ReadFromSTL(bytes.NewReader(f), astisub.STLOptions{})
		if err != nil {
			fmt.Printf("%-22s read error: %v\n", name, err)
			return
		}
		fmt.Printf("%-22s %d items: ", name, len(r.Items))
		for _, it := range r.Items {
			fmt.Printf("[%v-%v %q] ", it.StartAt, it.EndAt, it.String())
		}
		fmt.Println()
	}
	in, out := [4]byte{0, 0, 1, 0}, [4]byte{0, 0, 2, 0}

	// 4. DSC=1 (teletext) without start-box codes
	f := append([]byte(nil), gsi...)
	f[11] = '1'
	f = append(f, tti(1, 0xff, 0, in, out, []byte("No start box here"))...)
	read("dsc1-no-startbox", f)
	// with start boxes
	f = append([]byte(nil), gsi...)
	f[11] = '1'
	f = append(f, tti(1, 0xff, 0, in, out, []byte("\x0d\x0b\x0bWith start box\x0a\x0a"))...)
	read("dsc1-startbox", f)

	// 5. Extension blocks: SN 1 split over EBN 0x00 and 0xFF
	f = append([]byte(nil), gsi...)
	f[11] = '0'
	f = append(f, tti(1, 0x00, 0, in, out, []byte("First half of a long subtitle"))...)
	f = append(f, tti(1, 0xff, 0, [4]byte{}, [4]byte{}, []byte(" and the second half"))...)
	read("extension-blocks", f)

	// 6. Comment flag
	f = append([]byte(nil), gsi...)
	f[11] = '0'
	f = append(f, tti(1, 0xff, 1, in, out, []byte("Translator note: not for air"))...)
	read("comment-flag=1", f)

	// 7. Other character code tables
	for _, cct := range []string{"01", "02", "03", "04"} {
		f = append([]byte(nil), gsi...)
		f[11] = '0'
		copy(f[12:14], cct)
		f = append(f, tti(1, 0xff, 0, in, out, []byte{0xb6, 0xe0, 0xd8, 0xd2, 0xd5, 0xe2})...) // "Привет" in ISO 8859-5
		read("CCT "+cct, f)
	}
}
