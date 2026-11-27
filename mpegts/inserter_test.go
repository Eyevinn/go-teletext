package mpegts_test

import (
	"bytes"
	"math"
	"testing"
	"time"

	"github.com/Eyevinn/go-teletext/internal/tstest"
	"github.com/Eyevinn/go-teletext/mpegts"
)

const ttxPID = 0x101

var pages = []mpegts.DescriptorPage{{Language: "swe", Type: mpegts.TypeSubtitle, Magazine: 8, Page: 0x88}}

// run feeds in through an Inserter and writes a stuffing-only teletext PES
// every 40 ms of input clock, with a PTS 40 ms later, as ttx-gen does. It
// returns the output and the inserter.
func run(t *testing.T, in []byte, cfg mpegts.InserterConfig) ([]byte, *mpegts.Inserter) {
	t.Helper()
	return runOffset(t, in, cfg, 40*time.Millisecond)
}

// runOffset is run with PTS ptsOffset after the tick. Ticks restart at a clock jump.
func runOffset(t *testing.T, in []byte, cfg mpegts.InserterConfig, ptsOffset time.Duration) ([]byte, *mpegts.Inserter) {
	t.Helper()
	var out bytes.Buffer
	ins := mpegts.NewInserter(&out, cfg)
	var next uint64
	for off := 0; off < len(in); off += mpegts.PacketSize {
		pkt := in[off : off+mpegts.PacketSize]
		c, ok, err := ins.Read(pkt)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			if next == 0 || ins.Jumped() {
				next = c
			}
			for next <= c {
				pts := next/300 + uint64(ptsOffset/time.Microsecond)*9/100
				if err := ins.WritePES(mpegts.TeletextPES(pts, nil)); err != nil {
					t.Fatal(err)
				}
				next += 1_080_000
			}
		}
		if err := ins.Forward(pkt); err != nil {
			t.Fatal(err)
		}
	}
	if err := ins.Flush(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes(), ins
}

// teletextPTS returns the PTS of the teletext PES in a TS, in stream order,
// and fails on a continuity counter error on the teletext PID.
func teletextPTS(t *testing.T, b []byte) []uint64 {
	t.Helper()
	var pts []uint64
	for i, p := range split(b)[ttxPID] {
		if cc := p[3] & 0x0f; cc != byte(i)&0x0f {
			t.Fatalf("teletext packet %d has continuity counter %d", i, cc)
		}
		if mpegts.PayloadStart(p) {
			x := p[4:]
			pts = append(pts, uint64(x[9]>>1&7)<<30|uint64(x[10])<<22|uint64(x[11]>>1)<<15|
				uint64(x[12])<<7|uint64(x[13]>>1))
		}
	}
	return pts
}

// split returns the packets of a TS grouped by PID.
func split(b []byte) map[uint16][][]byte {
	m := map[uint16][][]byte{}
	for off := 0; off < len(b); off += mpegts.PacketSize {
		p := b[off : off+mpegts.PacketSize]
		m[mpegts.PacketPID(p)] = append(m[mpegts.PacketPID(p)], p)
	}
	return m
}

func checkPMT(t *testing.T, out []byte) {
	t.Helper()
	pmts := split(out)[0x1000]
	if len(pmts) == 0 {
		t.Fatal("no PMT in output")
	}
	for _, p := range pmts {
		s, err := mpegts.SectionInPacket(p)
		if err != nil {
			t.Fatal(err)
		}
		pmt, err := mpegts.ParsePMT(s)
		if err != nil {
			t.Fatal(err)
		}
		if len(pmt.Streams) != 2 || pmt.Streams[1].PID != ttxPID || pmt.Streams[1].Type != 0x06 {
			t.Fatalf("PMT streams = %+v", pmt.Streams)
		}
	}
}

func TestInserterReplacesNulls(t *testing.T) {
	c := tstest.Default(3 * time.Second)
	c.NullEvery = 10
	in := tstest.Synth(c)
	out, ins := run(t, in, mpegts.InserterConfig{TeletextPID: ttxPID, Pages: pages, ReplaceNull: true})
	checkPMT(t, out)
	if len(out) != len(in) {
		t.Errorf("output %d packets, input %d: nulls not replaced", len(out)/188, len(in)/188)
	}
	if ins.Stats.Inserted != ins.Stats.NullsReplaced || ins.Stats.Inserted < 70 {
		t.Errorf("stats %+v", ins.Stats)
	}
	// Teletext packets take the place of null packets: every other packet
	// keeps its position, so the PCR timing is unchanged.
	for k := 0; k < len(in)/188; k++ {
		p, q := in[k*188:(k+1)*188], out[k*188:(k+1)*188]
		if pid := mpegts.PacketPID(p); pid != mpegts.NullPID && pid != 0x1000 && !bytes.Equal(p, q) {
			t.Fatalf("packet %d (PID %#x) moved or changed", k, pid)
		}
	}
}

func TestInserterWithoutNulls(t *testing.T) {
	c := tstest.Default(3 * time.Second)
	in := tstest.Synth(c)
	out, ins := run(t, in, mpegts.InserterConfig{TeletextPID: ttxPID, Pages: pages, ReplaceNull: true})
	if got, want := len(out)/188, len(in)/188+int(ins.Stats.Inserted); got != want {
		t.Errorf("output %d packets, want %d", got, want)
	}
	if !packetsEqual(split(in)[0x100], split(out)[0x100], false) {
		t.Error("video packets changed")
	}
	// The clock is interpolated between PCRs: the input bitrate is measured exactly.
	if r := ins.Stats.InputBitrate(); math.Abs(r-float64(c.Bitrate)) > 1000 {
		t.Errorf("input bitrate %.0f, want %d", r, c.Bitrate)
	}
}

func TestInserterConstantBitrate(t *testing.T) {
	c := tstest.Default(3 * time.Second)
	in := tstest.Synth(c)
	const rate = 2_100_000
	out, ins := run(t, in, mpegts.InserterConfig{TeletextPID: ttxPID, Pages: pages, MuxRate: rate})
	checkPMT(t, out)
	if !packetsEqual(split(in)[0x100], split(out)[0x100], true) {
		t.Error("video packets changed other than PCR")
	}
	// Every PCR equals the clock of its slot at the new rate.
	slot := 27e6 * 188 * 8 / rate
	var first uint64
	firstIdx := -1
	for k := 0; k < len(out)/188; k++ {
		pcr, ok := mpegts.PCR(out[k*188 : (k+1)*188])
		if !ok {
			continue
		}
		if firstIdx < 0 {
			first, firstIdx = pcr, k
			continue
		}
		want := float64(first) + float64(k-firstIdx)*slot
		if math.Abs(float64(pcr)-want) > 1 {
			t.Fatalf("packet %d: PCR %d, want %.0f", k, pcr, want)
		}
	}
	// The output keeps pace with the input: the same duration at the new rate.
	inDur := float64(len(in)/188) * c.Slot()
	outDur := float64(len(out)/188-firstIdx) * slot
	if math.Abs(inDur-outDur) > 27e6*0.05 {
		t.Errorf("output lasts %.3f s, input %.3f s", outDur/27e6, inDur/27e6)
	}
	if ins.Stats.MaxLate > 27_000 { // 1 ms
		t.Errorf("packets up to %.2f ms late", float64(ins.Stats.MaxLate)/27e3)
	}
	// A rate below the input rate is refused.
	var sink bytes.Buffer
	low := mpegts.NewInserter(&sink, mpegts.InserterConfig{TeletextPID: ttxPID, Pages: pages, MuxRate: 1_500_000})
	var err error
	for off := 0; off < len(in) && err == nil; off += 188 {
		if _, _, err = low.Read(in[off : off+188]); err == nil {
			err = low.Forward(in[off : off+188])
		}
	}
	if err == nil {
		t.Error("too low -muxrate accepted")
	}
}

func TestInserterPicksPID(t *testing.T) {
	in := tstest.Synth(tstest.Default(time.Second))
	_, ins := run(t, in, mpegts.InserterConfig{Pages: pages})
	if ins.TeletextPID() != 0x101 {
		t.Errorf("picked PID %#x, want 0x101 (after video 0x100)", ins.TeletextPID())
	}
}

func TestInserterPIDClash(t *testing.T) {
	in := tstest.Synth(tstest.Default(time.Second))
	var out bytes.Buffer
	ins := mpegts.NewInserter(&out, mpegts.InserterConfig{TeletextPID: 0x100, Pages: pages})
	var err error
	for off := 0; off < len(in) && err == nil; off += 188 {
		_, _, err = ins.Read(in[off : off+188])
	}
	if err == nil {
		t.Error("teletext PID equal to the video PID accepted")
	}
}

// packetsEqual compares packet lists; with ignorePCR the PCR bytes may differ.
func packetsEqual(a, b [][]byte, ignorePCR bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if ignorePCR {
			if _, ok := mpegts.PCR(x); ok {
				x, y = append([]byte(nil), x...), append([]byte(nil), y...)
				copy(x[6:12], make([]byte, 6))
				copy(y[6:12], make([]byte, 6))
			}
		}
		if !bytes.Equal(x, y) {
			return false
		}
	}
	return true
}

// Teletext packets keep their order when the inserter stops waiting for null
// packets: with a long PTS offset, packets are still queued at that moment.
func TestInserterOrderWithoutNulls(t *testing.T) {
	in := tstest.Synth(tstest.Default(3 * time.Second))
	cfg := mpegts.InserterConfig{TeletextPID: ttxPID, Pages: pages, ReplaceNull: true}
	out, _ := runOffset(t, in, cfg, 200*time.Millisecond)
	pts := teletextPTS(t, out)
	if len(pts) != 75 {
		t.Errorf("%d teletext PES, want 75", len(pts))
	}
	for i := 1; i < len(pts); i++ {
		if pts[i] != pts[i-1]+3600 {
			t.Fatalf("PES %d has PTS %d after %d", i, pts[i], pts[i-1])
		}
	}
}

// A forward PCR step without discontinuity_indicator is a jump: the clock
// follows it and does not run ahead between the two PCRs around it.
func TestInserterUnflaggedJump(t *testing.T) {
	a := tstest.Default(2 * time.Second) // PCR 10 s to 12 s
	b := a
	b.StartPCR = 22 * 27_000_000
	in := append(tstest.Synth(a), tstest.Synth(b)...)
	for _, rate := range []uint64{0, 2_100_000} {
		out, ins := run(t, in, mpegts.InserterConfig{TeletextPID: ttxPID, Pages: pages, MuxRate: rate})
		pts := teletextPTS(t, out)
		// 2 s of ticks from each part.
		if len(pts) < 99 || len(pts) > 101 {
			t.Errorf("rate %d: %d teletext PES, want 100", rate, len(pts))
		}
		for i, p := range pts {
			if s := float64(p) / 90000; s < 10 || s > 12.1 && s < 22 || s > 24.1 {
				t.Fatalf("rate %d: PES %d has PTS %.3f s", rate, i, s)
			}
		}
		if rate == 0 {
			continue
		}
		// 4 s at the mux rate, with no burst of null packets at the jump.
		if got, want := len(out)/188, int(4*rate/8/188); got < want-30 || got > want+30 {
			t.Errorf("rate %d: %d packets, want about %d", rate, got, want)
		}
		if r := ins.Stats.InputBitrate(); math.Abs(r-float64(a.Bitrate)) > 1000 {
			t.Errorf("rate %d: input bitrate %.0f, want %d", rate, r, a.Bitrate)
		}
	}
}
