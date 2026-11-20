// Command astisub-teletext generates one page per language with the
// go-teletext packages and decodes it with go-astisub's
// ReadFromTeletext. It compares the result with what a Level 1 decoder
// should show, and reports panics instead of crashing.
//
//	go run ./astisub-teletext                 # group 0 languages, no X/28: all should match
//	go run ./astisub-teletext -designation x28 # X/28/0 makes go-astisub panic
//	go run ./astisub-teletext -langs pol,rus   # need X/28: panic (auto mode)
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/Eyevinn/go-teletext"
	"github.com/Eyevinn/go-teletext/mpegts"
	"github.com/asticode/go-astisub"
)

func main() {
	langs := flag.String("langs", "eng,swe,fin,dan,nor,hun,ger,fre,ita,spa,por,cze,slo", "languages")
	designation := flag.String("designation", "auto", "auto, x28, m29 or none")
	flag.Parse()
	log.SetOutput(io.Discard) // go-astisub logs the PID it picks

	mode := map[string]teletext.DesignationMode{
		"auto": teletext.DesignationAuto, "x28": teletext.DesignationX28,
		"m29": teletext.DesignationM29, "none": teletext.DesignationNone,
	}[*designation]
	for _, code := range strings.Split(*langs, ",") {
		l, ok := teletext.LookupLanguage(code)
		if !ok {
			fmt.Printf("%s: unknown\n", code)
			continue
		}
		addr := teletext.PageAddr{Magazine: 8, Page: 0x88}
		enc := teletext.NewEncoder(l.Charset, true)
		lines := []teletext.Line{{Text: l.Sample}, {Text: l.Name + " 12:00:00.000"}}
		pg, err := teletext.ComposePage(addr, enc, lines, teletext.PageOptions{Level15: true, Designation: mode})
		if err != nil {
			fmt.Printf("%s: %v\n", code, err)
			continue
		}
		ts := stream(l, addr, pg, enc)

		var want []string
		for _, line := range lines {
			want = append(want, level1(enc, line.Text))
		}
		got, err := decode(ts)
		status := "OK  "
		switch {
		case err != nil:
			status, got = "FAIL", err.Error()
		case got != strings.Join(want, " / "):
			status = "DIFF"
		}
		fmt.Printf("%s %-4s %-34s got:  %s\n", status, code, l.Charset.Name, got)
		if status == "DIFF" {
			fmt.Printf("     %-4s %-34s want: %s\n", "", "", strings.Join(want, " / "))
		}
	}
}

// level1 is what a Level 1 decoder shows: G0 codes only, X/26 ignored.
func level1(enc *teletext.Encoder, s string) string {
	cells, _ := enc.Encode(s)
	g0 := enc.Charset().G0Table()
	var out []rune
	for _, c := range cells {
		out = append(out, g0[c.Code-0x20])
	}
	return string(out)
}

func stream(l teletext.Language, addr teletext.PageAddr, pg *teletext.Page, enc *teletext.Encoder) []byte {
	var buf bytes.Buffer
	cfg := mpegts.DefaultConfig()
	cfg.Pages = []mpegts.DescriptorPage{
		{Language: l.Code, Type: mpegts.TypeSubtitle, Magazine: addr.Magazine, Page: addr.Page},
	}
	m := mpegts.NewMuxer(&buf, cfg)
	unit := func(p teletext.Packet) []byte { return mpegts.DataUnit(mpegts.DataUnitSubtitle, true, 0, p.Bytes()) }
	must(m.WritePSI())
	var head [][]byte
	if pg.M29 != nil {
		head = append(head, unit(*pg.M29))
	}
	head = append(head, unit(pg.Header))
	must(m.WritePES(mpegts.TeletextPES(90000, head)))
	var body [][]byte
	for _, p := range pg.Body {
		body = append(body, unit(p))
	}
	must(m.WritePES(mpegts.TeletextPES(90000+3600, body)))
	clear, _ := teletext.ComposePage(addr, enc, nil, teletext.PageOptions{})
	must(m.WritePES(mpegts.TeletextPES(90000+144000, [][]byte{unit(clear.Header)})))
	must(m.WritePES(mpegts.TeletextPES(90000+180000, nil)))
	return buf.Bytes()
}

func decode(ts []byte) (s string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	subs, err := astisub.ReadFromTeletext(bytes.NewReader(ts), astisub.TeletextOptions{Page: 888})
	if err != nil {
		return "", err
	}
	if len(subs.Items) != 1 {
		return "", fmt.Errorf("%d items", len(subs.Items))
	}
	var lines []string
	for _, l := range subs.Items[0].Lines {
		var parts []string
		for _, it := range l.Items {
			parts = append(parts, it.Text)
		}
		lines = append(lines, strings.Join(parts, " "))
	}
	return strings.Join(lines, " / "), nil
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
