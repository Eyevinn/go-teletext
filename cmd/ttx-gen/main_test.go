package main

import (
	"bytes"
	"fmt"
	"math/bits"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

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

// pesSummary is the PTS and the page headers of one teletext PES.
type pesSummary struct {
	pts     uint64
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
		pts := uint64(x[9]>>1&7)<<30 | uint64(x[10])<<22 | uint64(x[11]>>1)<<15 | uint64(x[12])<<7 | uint64(x[13]>>1)
		s := pesSummary{pts: pts}
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
	start := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	for i, p := range pes {
		ms := mpegts.TimeOfPTS(p.pts, start).Sub(start).Milliseconds()
		if want := int64(80 + 40*i); ms != want {
			t.Fatalf("PES %d at %d ms, want %d", i, ms, want)
		}
		paused := ms >= 4080 && ms < 8080
		hasPage := false
		for _, h := range p.headers {
			if !strings.HasSuffix(h, "FF") {
				hasPage = true
			}
		}
		if paused && (hasPage || p.rows > 0) {
			t.Errorf("cue data at %d ms, during the pause", ms)
		}
		if !hasPage && p.rows == 0 && strings.Join(p.headers, " ") != "8FF 7FF" {
			t.Errorf("heartbeat at %d ms has headers %v, want [8FF 7FF]", ms, p.headers)
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

// The clock counts from the Unix epoch: it steps 40 ms per tick over midnight
// and over the 33-bit wrap, and each cue shows the time of its header's PTS.
func TestEpochClock(t *testing.T) {
	const pcrWrap = (1 << 33) * 300
	wrap := int64(18812) << 33 / 90000 // s; the wrap is 0.9 s later, on 2026-11-24
	for _, tc := range []struct {
		name  string
		start time.Time
		wraps int
	}{
		{"midnight", time.Date(2026, 11, 20, 23, 59, 58, 0, time.UTC), 0},
		{"wrap", time.Unix(wrap-2, 0).UTC(), 1},
	} {
		out := filepath.Join(t.TempDir(), "out.ts")
		start := tc.start.Format(time.RFC3339)
		args := []string{appName, "-o", out, "-duration", "6s", "-langs", "swe", "-start", start}
		if err := run(args, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		var pcrs []uint64
		for off := 0; off < len(b); off += mpegts.PacketSize {
			if pcr, ok := mpegts.PCR(b[off : off+mpegts.PacketSize]); ok {
				pcrs = append(pcrs, pcr)
			}
		}
		wraps := 0
		for i := 1; i < len(pcrs); i++ {
			if d := (pcrs[i] + pcrWrap - pcrs[i-1]) % pcrWrap; d != 40*27000 {
				t.Fatalf("%s: PCR %d steps %d", tc.name, i, d)
			}
			if pcrs[i] < pcrs[i-1] {
				wraps++
			}
		}
		if len(pcrs) != 150 || wraps != tc.wraps {
			t.Errorf("%s: %d PCR with %d wraps, want 150 with %d", tc.name, len(pcrs), wraps, tc.wraps)
		}

		headerTimes := map[string]bool{}
		for i, p := range readPES(t, b) {
			at := mpegts.TimeOfPTS(p.pts, tc.start)
			if want := tc.start.Add(time.Duration(80+40*i) * time.Millisecond); !at.Equal(want) {
				t.Fatalf("%s: PES %d at %v, want %v", tc.name, i, at, want)
			}
			if strings.Join(p.headers, " ") == "888" {
				headerTimes[at.Format("15:04:05.000")] = true
			}
		}
		stamps := regexp.MustCompile(`Svenska (\S+) \(`).FindAllStringSubmatch(rowTexts(b), -1)
		if len(stamps) != 3 {
			t.Errorf("%s: %d cues, want 3", tc.name, len(stamps))
		}
		for _, s := range stamps {
			if !headerTimes[s[1]] {
				t.Errorf("%s: cue shows %s, which is not the PTS of a page header", tc.name, s[1])
			}
		}
	}
}
