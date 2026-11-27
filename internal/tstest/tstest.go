// Package tstest builds synthetic transport streams for tests.
package tstest

import (
	"time"

	"github.com/Eyevinn/go-teletext/mpegts"
)

// Config describes a synthetic constant bitrate stream with PAT, PMT, one
// "video" PID that carries the PCR, and null packets.
type Config struct {
	Duration    time.Duration
	Bitrate     int // bit/s
	NullEvery   int // every NullEvery-th packet is a null packet; 0 = none
	PCRInterval time.Duration
	StartPCR    uint64 // 27 MHz
	PMTPID      uint16
	VideoPID    uint16
}

// Default returns a 2 Mbit/s stream of d with a PCR every 40 ms and no null packets.
func Default(d time.Duration) Config {
	return Config{Duration: d, Bitrate: 2_000_000, PCRInterval: 40 * time.Millisecond,
		StartPCR: 10 * 27_000_000, PMTPID: 0x1000, VideoPID: 0x100}
}

// Slot is the duration of one packet at the bitrate, in 27 MHz units.
func (c Config) Slot() float64 { return 27e6 * mpegts.PacketSize * 8 / float64(c.Bitrate) }

// Synth returns the stream. Packet k is at clock StartPCR + k*Slot. The
// payload of a video packet holds its index in bytes 4-7, so tests can check
// order and content.
func Synth(c Config) []byte {
	pat := mpegts.PATSection(1, []mpegts.Program{{Number: 1, PMTPID: c.PMTPID}})
	pmt := (&mpegts.PMT{ProgramNumber: 1, PCRPID: c.VideoPID,
		Streams: []mpegts.ElementaryStream{{Type: 0x1b, PID: c.VideoPID}}}).Section()
	slot := c.Slot()
	n := int(float64(c.Duration/time.Microsecond) * 27 / slot)
	psiEvery := uint64(100 * time.Millisecond / time.Microsecond * 27)
	pcrEvery := uint64(c.PCRInterval / time.Microsecond * 27)
	var out []byte
	var nextPSI, nextPCR uint64
	cc := map[uint16]byte{}
	for k := 0; k < n; k++ {
		t := c.StartPCR + uint64(float64(k)*slot)
		var pkt []byte
		switch {
		case t >= nextPSI+c.StartPCR:
			pkt, _ = mpegts.SectionPacket(0, cc[0], pat)
			cc[0]++
			// The PMT follows in the next packet.
			out = append(out, pkt...)
			k++
			pkt, _ = mpegts.SectionPacket(c.PMTPID, cc[c.PMTPID], pmt)
			cc[c.PMTPID]++
			nextPSI += psiEvery
		case c.NullEvery > 0 && k%c.NullEvery == 0:
			pkt = make([]byte, mpegts.PacketSize)
			pkt[0], pkt[1], pkt[2], pkt[3] = 0x47, 0x1f, 0xff, 0x10
		default:
			pkt = make([]byte, mpegts.PacketSize)
			pid := c.VideoPID
			pkt[0], pkt[1], pkt[2] = 0x47, byte(pid>>8), byte(pid)
			off := 4
			if t >= nextPCR+c.StartPCR {
				pkt[3] = 0x30 | cc[pid]&0xf
				pkt[4], pkt[5] = 7, 0x10
				base, ext := t/300, t%300
				pkt[6], pkt[7], pkt[8], pkt[9] = byte(base>>25), byte(base>>17), byte(base>>9), byte(base>>1)
				pkt[10], pkt[11] = byte(base<<7)|0x7e|byte(ext>>8), byte(ext)
				off = 12
				nextPCR += pcrEvery
			} else {
				pkt[3] = 0x10 | cc[pid]&0xf
			}
			cc[pid]++
			pkt[off], pkt[off+1], pkt[off+2], pkt[off+3] = byte(k>>24), byte(k>>16), byte(k>>8), byte(k)
		}
		out = append(out, pkt...)
	}
	return out
}
