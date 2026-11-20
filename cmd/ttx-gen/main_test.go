package main

import (
	"bytes"
	"fmt"
	"math/bits"
	"os"
	"path/filepath"
	"strings"
	"testing"

	teletext "github.com/Eyevinn/go-teletext"
	"github.com/Eyevinn/go-teletext/mpegts"
)

func TestRunFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.ts")
	args := []string{appName, "-o", out, "-duration", "4s", "-langs", "swe,rus,gre", "-start", "2026-11-20T12:00:00Z"}
	if err := run(args, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 || len(b)%mpegts.PacketSize != 0 {
		t.Fatalf("output is %d bytes", len(b))
	}
	// 4 s at 40 ms per tick: 100 PCR packets on the PCR PID.
	pcr := 0
	for off := 0; off < len(b); off += mpegts.PacketSize {
		if pid := uint16(b[off+1]&0x1f)<<8 | uint16(b[off+2]); pid == mpegts.DefaultConfig().PCRPID {
			pcr++
		}
	}
	if pcr != 100 {
		t.Errorf("%d PCR packets, want 100", pcr)
	}
}

// pesSummary is the PTS (ms after the start) and the page headers of one teletext PES.
type pesSummary struct {
	ms      int64
	headers []string // e.g. "888", "8FF"
	rows    int
}

// readPES splits the teletext PID of a TS into PES packets and summarises them.
func readPES(t *testing.T, b []byte) []pesSummary {
	t.Helper()
	var pes [][]byte
	for off := 0; off < len(b); off += mpegts.PacketSize {
		p := b[off : off+mpegts.PacketSize]
		if uint16(p[1]&0x1f)<<8|uint16(p[2]) != mpegts.DefaultConfig().TeletextPID {
			continue
		}
		if p[1]&0x40 != 0 {
			pes = append(pes, nil)
		}
		pes[len(pes)-1] = append(pes[len(pes)-1], p[4:]...)
	}
	unham := func(b byte) byte {
		v, ok := teletext.UnHam84(bits.Reverse8(b))
		if !ok {
			t.Fatalf("Hamming error in %#x", b)
		}
		return v
	}
	var out []pesSummary
	for _, x := range pes {
		pts := int64(x[9]>>1&7)<<30 | int64(x[10])<<22 | int64(x[11]>>1)<<15 | int64(x[12])<<7 | int64(x[13]>>1)
		s := pesSummary{ms: (pts - 12*3600*90000) / 90}
		for i := 46; i+46 <= len(x); i += 46 {
			if x[i] != mpegts.DataUnitSubtitle {
				continue
			}
			mrag := unham(x[i+4]) | unham(x[i+5])<<4
			mag, y := mrag&7, mrag>>3
			if mag == 0 {
				mag = 8
			}
			if y == 0 {
				s.headers = append(s.headers, fmt.Sprintf("%d%X%X", mag, unham(x[i+7]), unham(x[i+6])))
			} else {
				s.rows++
			}
		}
		out = append(out, s)
	}
	return out
}

func TestHeartbeatAndPause(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.ts")
	args := []string{appName, "-o", out, "-duration", "12s", "-langs", "swe,fin",
		"-cues-on", "4s", "-cues-off", "4s", "-start", "2026-11-20T12:00:00Z"}
	if err := run(args, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	pes := readPES(t, b)
	// One PES per 40 ms tick: cue data or a heartbeat.
	if len(pes) != 300 {
		t.Fatalf("%d PES, want 300", len(pes))
	}
	for i, p := range pes {
		if want := int64(80 + 40*i); p.ms != want {
			t.Fatalf("PES %d at %d ms, want %d", i, p.ms, want)
		}
		paused := p.ms >= 4080 && p.ms < 8080
		hasPage := false
		for _, h := range p.headers {
			if !strings.HasSuffix(h, "FF") {
				hasPage = true
			}
		}
		if paused && (hasPage || p.rows > 0) {
			t.Errorf("cue data at %d ms, during the pause", p.ms)
		}
		if !hasPage && p.rows == 0 && strings.Join(p.headers, " ") != "8FF 7FF" {
			t.Errorf("heartbeat at %d ms has headers %v, want [8FF 7FF]", p.ms, p.headers)
		}
	}
}

func TestRunList(t *testing.T) {
	var buf bytes.Buffer
	if err := run([]string{appName, "-list"}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Svenska") {
		t.Errorf("-list output lacks Svenska:\n%s", buf.String())
	}
}

func TestRunErrors(t *testing.T) {
	for _, args := range [][]string{
		{appName, "-langs", "xxx"},
		{appName, "-designation", "foo"},
		{appName, "-cue", "3s"},
		{appName, "-duration", "0"},
		{appName, "-heartbeat-kind", "foo"},
		{appName, "-cues-on", "4s"},
	} {
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Errorf("%v: no error", args[1:])
		}
	}
}
