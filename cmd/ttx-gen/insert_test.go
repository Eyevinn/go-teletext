package main

import (
	"bytes"
	"math/bits"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Eyevinn/go-teletext/internal/tstest"
	"github.com/Eyevinn/go-teletext/mpegts"
)

// rowTexts returns the 7-bit text of all display rows in a TS, for Latin pages.
func rowTexts(b []byte) string {
	var sb strings.Builder
	var pes []byte
	flushPES := func() {
		for i := 46; i+46 <= len(pes); i += 46 {
			if pes[i] != mpegts.DataUnitSubtitle {
				continue
			}
			for _, c := range pes[i+6 : i+46] {
				if c := bits.Reverse8(c) & 0x7f; c >= 0x20 {
					sb.WriteByte(c)
				}
			}
			sb.WriteByte('\n')
		}
	}
	for off := 0; off < len(b); off += mpegts.PacketSize {
		p := b[off : off+mpegts.PacketSize]
		if mpegts.PacketPID(p) != 0x101 {
			continue
		}
		if mpegts.PayloadStart(p) {
			flushPES()
			pes = nil
		}
		pes = append(pes, p[4:]...)
	}
	flushPES()
	return sb.String()
}

func TestInsert(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.ts")
	c := tstest.Default(5 * time.Second) // first PCR at 10 s
	if err := os.WriteFile(in, tstest.Synth(c), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		clock, stamp string
		extra        []string
	}{
		{"wall", "Svenska 12:00:00.040 (1)", nil},
		{"pts", "Svenska 00:00:10.080 (1)", nil},
		{"wall", "Svenska 12:00:02.040 (2)", []string{"-muxrate", "2100000"}},
		{"wall", "Svenska 12:00:02.040 (2)", []string{"-heartbeat-kind", "stuffing"}},
	} {
		out := filepath.Join(dir, "out-"+tc.clock+".ts")
		args := append([]string{appName, "-i", in, "-o", out, "-langs", "swe", "-clock", tc.clock,
			"-start", "2026-11-20T12:00:00Z"}, tc.extra...)
		if err := run(args, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		if len(tc.extra) > 1 && tc.extra[0] == "-muxrate" {
			// 5 s at 2.1 Mbit/s, from the first PCR.
			if got, want := len(b)/188, 5*2_100_000/8/188; got < want-20 || got > want+20 {
				t.Errorf("-muxrate: %d packets, want about %d", got, want)
			}
		}
		if !strings.Contains(rowTexts(b), tc.stamp) {
			t.Errorf("-clock %s %v: no row %q in\n%s", tc.clock, tc.extra, tc.stamp, rowTexts(b))
		}
		if len(tc.extra) > 0 {
			continue
		}
		// One teletext PES per 40 ms of input clock from the first tick after
		// the first PCR (10.0015 s), with PTS = tick + 40 ms.
		pes := readPES(t, b)
		if len(pes) != 124 {
			t.Errorf("%d teletext PES, want 124", len(pes))
		}
		for i, p := range pes {
			if want := int64(10_080+40*i) - 12*3600*1000; p.ms != want {
				t.Fatalf("PES %d PTS %d ms, want %d", i, p.ms, want)
			}
		}
	}
}
