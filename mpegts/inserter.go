package mpegts

import (
	"bytes"
	"fmt"
	"io"
	"math"
)

// pcrWrap is the range of the PCR in 27 MHz units (33-bit base * 300).
const pcrWrap = (1 << 33) * 300

// InserterConfig configures an Inserter.
type InserterConfig struct {
	// TeletextPID is the PID for the teletext stream; 0 picks the PID after
	// the highest PID of the program's streams when the PMT is read.
	TeletextPID   uint16
	Pages         []DescriptorPage
	ProgramNumber uint16 // program to add teletext to; 0 = the first program in the PAT
	// ReplaceNull sends teletext packets in the place of null packets, so a
	// constant bitrate input keeps its bitrate and every other packet keeps
	// its position. A teletext packet waits for a null packet until shortly
	// before its PTS; then it is inserted, and a later null packet is dropped
	// instead. Without null packets the output bitrate grows by the teletext.
	ReplaceNull bool
	// MuxRate, when not 0, makes the output constant bitrate at this rate
	// (bit/s). Every packet goes in the output slot nearest after its input
	// clock time, empty slots get null packets (input null packets are
	// dropped) and the PCR of the program is restamped with the clock of its
	// slot. A PCR on another PID, as of another program, keeps its time base
	// and is moved by the delay of its packet. It must exceed the input
	// bitrate by the teletext bitrate.
	MuxRate uint64
}

// maxLateness is how far the constant bitrate output may fall behind the input
// clock before the mux rate is considered too low.
const maxLateness = 27_000_000 / 2

// nullWaitGuard is how long before its PTS a teletext packet stops waiting for
// a null packet to replace.
const nullWaitGuard = 27_000_000 / 200

// InserterStats counts what an Inserter has done.
type InserterStats struct {
	Packets       uint64 // input packets
	Inserted      uint64 // teletext packets written
	NullsReplaced uint64 // null packets replaced or dropped in exchange
	NoNullInTime  uint64 // teletext packets inserted because no null packet came in time (ReplaceNull)
	NullsAdded    uint64 // null packets written to keep a constant bitrate (MuxRate)
	MaxLate       uint64 // largest delay of a packet behind its input clock, 27 MHz (MuxRate)
	firstPCR      uint64
	firstPCRIdx   uint64
	lastPCR       uint64
	lastPCRIdx    uint64
}

// InputBitrate is the input bitrate in bit/s measured between the first and
// the last PCR, or 0 before two PCRs.
func (s InserterStats) InputBitrate() float64 {
	if s.lastPCR <= s.firstPCR {
		return 0
	}
	return float64(s.lastPCRIdx-s.firstPCRIdx) * PacketSize * 8 / (float64(s.lastPCR-s.firstPCR) / 27e6)
}

// Inserter adds a teletext stream to an existing transport stream. It adds
// the teletext PID to the PMT of one program and keeps a stream clock from
// the PCR of that program, interpolated between PCRs by packet count, so that
// teletext PES can be inserted on time with PTS that follow the input.
//
// For every input packet, in order, call Read, then WritePES for any teletext
// that is due at the returned clock, then Forward.
type Inserter struct {
	w     io.Writer
	cfg   InserterConfig
	ttx   *Muxer
	Stats InserterStats

	pmtPID    uint16
	havePMT   bool
	pcrPID    uint16
	pmtIn     []byte // last input PMT section
	pmtOut    []byte // its rewritten version
	pcrRaw    uint64
	pcrWraps  uint64
	havePCR   bool
	lastPCR   uint64 // unwrapped
	lastIdx   uint64
	perPacket float64 // 27 MHz ticks per packet, 0 until two PCRs have been seen
	nullDebt  uint64
	queue     []queued // teletext packets waiting for a null packet (ReplaceNull)
	nullAt    uint64   // clock of the last null packet, or of the first clock

	cur        uint64 // clock of the current packet
	haveClock  bool
	jumped     bool // the clock of the current packet jumped
	pesBuf     bytes.Buffer
	slot, next float64 // MuxRate: 27 MHz ticks per packet, and the clock of the next slot
	cbrStarted bool
}

// NewInserter returns an Inserter that writes to w.
func NewInserter(w io.Writer, cfg InserterConfig) *Inserter {
	in := &Inserter{w: w, cfg: cfg}
	if cfg.TeletextPID != 0 {
		in.ttx = NewMuxer(&in.pesBuf, Config{TeletextPID: cfg.TeletextPID})
	}
	if cfg.MuxRate > 0 {
		in.slot = 27e6 * PacketSize * 8 / float64(cfg.MuxRate)
	}
	return in
}

// Read inspects an input packet: it follows the PAT and PMT and reads PCRs.
// It returns the stream clock at this packet in 27 MHz units, unwrapped so it
// keeps increasing across the 33-bit wrap; ok is false until the first PCR.
func (in *Inserter) Read(pkt []byte) (clock uint64, ok bool, err error) {
	if len(pkt) != PacketSize || pkt[0] != 0x47 {
		return 0, false, fmt.Errorf("mpegts: packet %d has no sync byte", in.Stats.Packets)
	}
	idx := in.Stats.Packets
	in.Stats.Packets++
	pid := PacketPID(pkt)
	if in.ttx != nil && pid == in.cfg.TeletextPID {
		return 0, false, fmt.Errorf("mpegts: the input uses the teletext PID %#x", pid)
	}
	switch {
	case pid == 0 && PayloadStart(pkt):
		if err := in.readPAT(pkt); err != nil {
			return 0, false, err
		}
	case in.pmtPID != 0 && pid == in.pmtPID && PayloadStart(pkt):
		if err := in.readPMT(pkt); err != nil {
			return 0, false, err
		}
	}
	in.jumped = false
	if in.havePMT && pid == in.pcrPID {
		if pcr, has := PCR(pkt); has {
			disc := Discontinuity(pkt)
			in.addPCR(pcr, idx, disc)
			return in.setClock(in.lastPCR, disc), true, nil
		}
	}
	if !in.havePCR {
		return 0, false, nil
	}
	perPacket := in.perPacket
	if perPacket == 0 {
		// Until two PCRs give the input rate, assume the output rate (0 when
		// keeping the input timing, which holds the clock at the last PCR).
		perPacket = in.slot
	}
	return in.setClock(in.lastPCR+uint64(perPacket*float64(idx-in.lastIdx)), false), true, nil
}

// maxClockStep is the largest step of the clock, forward or back, that is not
// a discontinuity.
const maxClockStep = 27_000_000

// setClock records the clock of the current packet. The clock between PCRs is
// estimated from the bitrate of the previous PCR interval, which overshoots
// when the bitrate drops, so a lower PCR holds the clock instead of moving it
// back. A discontinuity_indicator or a step of more than a second is a jump:
// the clock follows it and the constant bitrate timeline restarts.
func (in *Inserter) setClock(c uint64, discontinuity bool) uint64 {
	if in.haveClock {
		back := c < in.cur
		step := c - in.cur
		if back {
			step = in.cur - c
		}
		switch {
		case discontinuity || step > maxClockStep:
			in.jumped, in.cbrStarted = true, false
		case back:
			c = in.cur
		}
	}
	if !in.haveClock {
		in.nullAt = c
	}
	in.cur, in.haveClock = c, true
	return c
}

// Jumped reports whether the clock returned by the last Read jumped, because
// of a discontinuity_indicator or a step of more than a second.
func (in *Inserter) Jumped() bool { return in.jumped }

func (in *Inserter) readPAT(pkt []byte) error {
	s, err := SectionInPacket(pkt)
	if err != nil {
		return fmt.Errorf("PAT: %w", err)
	}
	progs, err := ParsePAT(s)
	if err != nil {
		return err
	}
	for _, p := range progs {
		if in.cfg.ProgramNumber == 0 || p.Number == in.cfg.ProgramNumber {
			if in.pmtPID != p.PMTPID {
				in.pmtPID, in.havePMT, in.pmtIn = p.PMTPID, false, nil
			}
			return nil
		}
	}
	return fmt.Errorf("mpegts: program %d not in PAT", in.cfg.ProgramNumber)
}

func (in *Inserter) readPMT(pkt []byte) error {
	s, err := SectionInPacket(pkt)
	if err != nil {
		return fmt.Errorf("PMT: %w", err)
	}
	if bytes.Equal(s, in.pmtIn) {
		return nil
	}
	p, err := ParsePMT(s)
	if err != nil {
		return err
	}
	if in.cfg.ProgramNumber != 0 && p.ProgramNumber != in.cfg.ProgramNumber {
		return nil
	}
	if in.cfg.TeletextPID == 0 {
		top := p.PCRPID
		for _, es := range p.Streams {
			top = max(top, es.PID)
		}
		if top >= NullPID-1 {
			return fmt.Errorf("mpegts: no free PID after %#x for teletext", top)
		}
		in.cfg.TeletextPID = top + 1
		in.ttx = NewMuxer(&in.pesBuf, Config{TeletextPID: in.cfg.TeletextPID})
	}
	if p.PCRPID == in.cfg.TeletextPID {
		return fmt.Errorf("mpegts: teletext PID %#x is the PCR PID of the input", in.cfg.TeletextPID)
	}
	for _, es := range p.Streams {
		if es.PID == in.cfg.TeletextPID {
			return fmt.Errorf("mpegts: teletext PID %#x is already used by the input", in.cfg.TeletextPID)
		}
	}
	p.Streams = append(p.Streams, TeletextStream(in.cfg.TeletextPID, in.cfg.Pages))
	out := p.Section()
	if 1+len(out) > PacketSize-4 {
		return fmt.Errorf("PMT with teletext: %w", ErrSectionSpansPackets)
	}
	if in.havePMT && p.PCRPID != in.pcrPID {
		in.havePCR, in.perPacket = false, 0
	}
	in.pmtIn, in.pmtOut, in.pcrPID, in.havePMT = s, out, p.PCRPID, true
	return nil
}

func (in *Inserter) addPCR(pcr, idx uint64, discontinuity bool) {
	if in.havePCR && pcr+pcrWrap/2 < in.pcrRaw {
		in.pcrWraps++
	}
	in.pcrRaw = pcr
	full := pcr + in.pcrWraps*pcrWrap
	// Across a jump, flagged or not (see setClock), the PCR difference is not
	// the duration of the packets in between, so it gives no bitrate.
	jump := discontinuity || full > in.lastPCR+maxClockStep || full+maxClockStep < in.lastPCR
	switch {
	case !in.havePCR || jump || full <= in.lastPCR || idx <= in.lastIdx:
		in.perPacket = 0
	default:
		in.perPacket = float64(full-in.lastPCR) / float64(idx-in.lastIdx)
	}
	if !in.havePCR || jump {
		in.Stats.firstPCR, in.Stats.firstPCRIdx = full, idx
	}
	in.Stats.lastPCR, in.Stats.lastPCRIdx = full, idx
	in.lastPCR, in.lastIdx, in.havePCR = full, idx, true
}

// TeletextPID returns the teletext PID, 0 until it has been picked.
func (in *Inserter) TeletextPID() uint16 { return in.cfg.TeletextPID }

// WritePES writes a teletext PES (see TeletextPES) before the current packet.
func (in *Inserter) WritePES(pes []byte) error {
	if in.ttx == nil {
		return fmt.Errorf("mpegts: teletext PID not picked yet, no PMT read")
	}
	in.pesBuf.Reset()
	if err := in.ttx.WritePES(pes); err != nil {
		return err
	}
	b := in.pesBuf.Bytes()
	in.Stats.Inserted += uint64(len(b) / PacketSize)
	deadline := in.cur
	if len(pes) >= 14 && pes[7]&0x80 != 0 {
		// Wait at most until shortly before the PTS: the PTS minus the clock,
		// modulo 2^33, is the time left.
		pts := uint64(pes[9]>>1&7)<<30 | uint64(pes[10])<<22 | uint64(pes[11]>>1)<<15 |
			uint64(pes[12])<<7 | uint64(pes[13]>>1)
		if left := (pts - in.cur/300) & (1<<33 - 1) * 300; left > nullWaitGuard && left < maxClockStep {
			deadline = in.cur + left - nullWaitGuard
		}
	}
	// Wait for a null packet while the input has had one in the last second
	// (or is in its first second); else the input has none to replace, and the
	// packets that still wait go out first, to keep the order.
	wait := in.cfg.ReplaceNull && in.cfg.MuxRate == 0 && in.cur < in.nullAt+maxClockStep
	if !wait {
		if err := in.flushDue(true); err != nil {
			return err
		}
	}
	for i := 0; i < len(b); i += PacketSize {
		pkt := b[i : i+PacketSize]
		if wait {
			in.queue = append(in.queue, queued{append([]byte(nil), pkt...), deadline})
			continue
		}
		if err := in.emit(pkt); err != nil {
			return err
		}
	}
	return nil
}

// queued is a teletext packet waiting for a null packet to replace.
type queued struct {
	pkt      []byte
	deadline uint64 // clock after which it is inserted instead
}

// flushDue inserts the queued packets whose deadline has passed, or all of
// them, and drops as many later null packets.
func (in *Inserter) flushDue(all bool) error {
	for len(in.queue) > 0 && (all || in.cur >= in.queue[0].deadline) {
		if _, err := in.w.Write(in.queue[0].pkt); err != nil {
			return err
		}
		in.queue = in.queue[1:]
		in.nullDebt++
		in.Stats.NoNullInTime++
	}
	return nil
}

// Flush writes teletext packets still waiting for a null packet. Call it at
// the end of the input.
func (in *Inserter) Flush() error { return in.flushDue(true) }

// emit writes a packet, in constant bitrate mode in the next free slot at or
// after the current clock.
func (in *Inserter) emit(pkt []byte) error {
	if in.cfg.MuxRate == 0 || !in.haveClock {
		_, err := in.w.Write(pkt)
		return err
	}
	t := float64(in.cur)
	if !in.cbrStarted {
		in.next, in.cbrStarted = t, true
	}
	for in.next+in.slot <= t {
		if _, err := in.w.Write(nullPacket); err != nil {
			return err
		}
		in.next += in.slot
		in.Stats.NullsAdded++
	}
	if late := in.next - t; late > 0 {
		in.Stats.MaxLate = max(in.Stats.MaxLate, uint64(late))
		if late > maxLateness {
			return fmt.Errorf("mpegts: -muxrate %d is too low: output is %.0f ms behind the input clock",
				in.cfg.MuxRate, late/27e3)
		}
	}
	if pcr, has := PCR(pkt); has {
		if PacketPID(pkt) == in.pcrPID {
			pkt = withPCR(pkt, uint64(in.next+0.5)%pcrWrap)
		} else {
			// Another time base, as of another program: keep it, and add the
			// time the packet moved, which can be up to one slot back.
			moved := int64(math.Floor(in.next - t + 0.5))
			pkt = withPCR(pkt, uint64(int64(pcr)+pcrWrap+moved)%pcrWrap)
		}
	}
	in.next += in.slot
	_, err := in.w.Write(pkt)
	return err
}

// nullPacket is a null packet; its continuity counter is not checked.
var nullPacket = func() []byte {
	p := make([]byte, PacketSize)
	p[0], p[1], p[2], p[3] = 0x47, 0x1f, 0xff, 0x10
	for i := 4; i < PacketSize; i++ {
		p[i] = 0xff
	}
	return p
}()

// withPCR returns a copy of pkt with its PCR set to pcr (27 MHz).
func withPCR(pkt []byte, pcr uint64) []byte {
	out := append([]byte(nil), pkt...)
	base, ext := pcr/300, pcr%300
	out[6] = byte(base >> 25)
	out[7] = byte(base >> 17)
	out[8] = byte(base >> 9)
	out[9] = byte(base >> 1)
	out[10] = byte(base<<7) | 0x7e | byte(ext>>8)
	out[11] = byte(ext)
	return out
}

// Forward writes an input packet: the PMT with the teletext stream added,
// nothing for a null packet that pays for an inserted packet, else unchanged.
func (in *Inserter) Forward(pkt []byte) error {
	pid := PacketPID(pkt)
	if pid == NullPID {
		in.nullAt = in.cur
	} else if err := in.flushDue(false); err != nil {
		return err
	}
	switch {
	case pid == NullPID && in.cfg.MuxRate > 0 && in.haveClock:
		return nil // null packets are regenerated at the output rate
	case pid == NullPID && len(in.queue) > 0:
		_, err := in.w.Write(in.queue[0].pkt)
		in.queue = in.queue[1:]
		in.Stats.NullsReplaced++
		return err
	case pid == NullPID && in.nullDebt > 0:
		in.nullDebt--
		in.Stats.NullsReplaced++
		return nil
	case in.havePMT && pid == in.pmtPID && PayloadStart(pkt) && in.isOurPMT(pkt):
		out, err := SectionPacket(pid, pkt[3]&0x0f, in.pmtOut)
		if err != nil {
			return err
		}
		return in.emit(out)
	}
	return in.emit(pkt)
}

// isOurPMT reports whether the packet carries the PMT section that was
// rewritten, and not the PMT of another program on the same PID.
func (in *Inserter) isOurPMT(pkt []byte) bool {
	s, err := SectionInPacket(pkt)
	return err == nil && bytes.Equal(s, in.pmtIn)
}
