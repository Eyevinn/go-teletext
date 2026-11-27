// Package mpegts writes a minimal MPEG-TS with one DVB teletext stream
// (EN 300 472 PES data, signalled with the EN 300 468 teletext descriptor).
package mpegts

import (
	"fmt"
	"io"
	"math/bits"
)

// PacketSize is the size of a TS packet.
const PacketSize = 188

// Data unit ids (EN 300 472 table 4).
const (
	DataUnitNonSubtitle = 0x02
	DataUnitSubtitle    = 0x03
	DataUnitStuffing    = 0xff
)

// Teletext types for the teletext descriptor (EN 300 468 table 94).
const (
	TypeSubtitle        = 0x02
	TypeHearingImpaired = 0x05
)

// DescriptorPage is one entry of the teletext descriptor.
type DescriptorPage struct {
	Language string // three-letter ISO 639-2 code
	Type     byte   // TypeSubtitle or TypeHearingImpaired
	Magazine byte   // 1-8
	Page     byte   // BCD, e.g. 0x88
}

// Config describes the transport stream.
type Config struct {
	TransportStreamID uint16
	ProgramNumber     uint16
	PMTPID            uint16
	TeletextPID       uint16
	PCRPID            uint16 // carries PCR-only packets; not listed as an elementary stream
	Pages             []DescriptorPage
}

// DefaultConfig returns PIDs that do not clash with common test sources.
func DefaultConfig() Config {
	return Config{TransportStreamID: 1, ProgramNumber: 1, PMTPID: 0x1000, TeletextPID: 0x0101, PCRPID: 0x0100}
}

// Muxer writes TS packets to w.
type Muxer struct {
	w   io.Writer
	cfg Config
	cc  map[uint16]byte
}

// NewMuxer returns a muxer that writes to w.
func NewMuxer(w io.Writer, cfg Config) *Muxer {
	return &Muxer{w: w, cfg: cfg, cc: map[uint16]byte{}}
}

// DataUnit wraps a 42-byte teletext packet (address + data, in line order)
// in an EN 300 472 data unit: id, length 0x2C, field parity/line offset,
// framing code and the bit-reversed packet. Line offset 0 means undefined.
func DataUnit(id byte, firstField bool, lineOffset byte, packet []byte) []byte {
	if len(packet) != 42 {
		panic(fmt.Sprintf("mpegts: teletext packet is %d bytes, want 42", len(packet)))
	}
	u := make([]byte, 0, 46)
	fp := byte(0)
	if firstField {
		fp = 1
	}
	u = append(u, id, 0x2c, 0xc0|fp<<5|lineOffset&0x1f, 0xe4)
	for _, b := range packet {
		u = append(u, bits.Reverse8(b))
	}
	return u
}

// stuffingUnit is a data unit with id 0xFF, used to fill the PES.
func stuffingUnit() []byte {
	u := make([]byte, 46)
	for i := range u {
		u[i] = 0xff
	}
	u[1] = 0x2c
	return u
}

// TeletextPES builds a PES packet per EN 300 472 clause 4.2: stream_id 0xBD,
// PES_header_data_length 0x24, PES_packet_length = N*184-6. The data units are
// padded with stuffing units so the PES fills whole TS packets.
func TeletextPES(pts uint64, units [][]byte) []byte {
	// 9 + 0x24 header bytes, 1 data_identifier and 46 bytes per unit must add
	// up to N*184, so 46+46n = 184N: n = 3, 7, 11, ...
	n := len(units)
	for (46+46*n)%184 != 0 {
		n++
	}
	for len(units) < n {
		units = append(units, stuffingUnit())
	}
	b := []byte{0x00, 0x00, 0x01, 0xbd, 0, 0, 0x84, 0x80, 0x24}
	b = append(b, encodePTS(pts)...)
	for len(b) < 9+0x24 {
		b = append(b, 0xff)
	}
	b = append(b, 0x10) // data_identifier: EBU data, EN 300 472 compliant
	for _, u := range units {
		b = append(b, u...)
	}
	l := len(b) - 6
	b[4], b[5] = byte(l>>8), byte(l)
	return b
}

func encodePTS(pts uint64) []byte {
	pts &= 1<<33 - 1
	return []byte{
		0x21 | byte(pts>>29)&0x0e,
		byte(pts >> 22),
		0x01 | byte(pts>>14)&0xfe,
		byte(pts >> 7),
		0x01 | byte(pts<<1)&0xfe,
	}
}

// WritePES writes a teletext PES. Its length is a multiple of 184, so no
// adaptation field is needed (EN 300 472 only allows adaptation_field_control
// 01 and 10 on the teletext PID).
func (m *Muxer) WritePES(pes []byte) error {
	if len(pes)%184 != 0 {
		return fmt.Errorf("mpegts: PES length %d is not a multiple of 184", len(pes))
	}
	pid := m.cfg.TeletextPID
	for i := 0; i < len(pes); i += 184 {
		h := m.header(pid, i == 0, 0x10)
		if _, err := m.w.Write(append(h, pes[i:i+184]...)); err != nil {
			return err
		}
	}
	return nil
}

// WritePCR writes an adaptation-field-only packet with a PCR (27 MHz units).
func (m *Muxer) WritePCR(pcr uint64) error {
	pkt := make([]byte, PacketSize)
	pid := m.cfg.PCRPID
	pkt[0], pkt[1], pkt[2] = 0x47, byte(pid>>8)&0x1f, byte(pid)
	pkt[3] = 0x20 | m.cc[pid] // adaptation field only; CC is not incremented
	pkt[4] = 183
	pkt[5] = 0x10 // PCR flag
	base, ext := pcr/300&(1<<33-1), pcr%300
	pkt[6] = byte(base >> 25)
	pkt[7] = byte(base >> 17)
	pkt[8] = byte(base >> 9)
	pkt[9] = byte(base >> 1)
	pkt[10] = byte(base<<7) | 0x7e | byte(ext>>8)
	pkt[11] = byte(ext)
	for i := 12; i < PacketSize; i++ {
		pkt[i] = 0xff
	}
	_, err := m.w.Write(pkt)
	return err
}

// WritePSI writes the PAT and the PMT.
func (m *Muxer) WritePSI() error {
	c := m.cfg
	pat := PATSection(c.TransportStreamID, []Program{{Number: c.ProgramNumber, PMTPID: c.PMTPID}})
	pmt := &PMT{
		ProgramNumber: c.ProgramNumber,
		PCRPID:        c.PCRPID,
		Streams:       []ElementaryStream{TeletextStream(c.TeletextPID, c.Pages)},
	}
	for _, sec := range []struct {
		pid uint16
		s   []byte
	}{{0, pat}, {c.PMTPID, pmt.Section()}} {
		pkt, err := SectionPacket(sec.pid, m.cc[sec.pid], sec.s)
		if err != nil {
			return err
		}
		m.cc[sec.pid] = (m.cc[sec.pid] + 1) & 0xf
		if _, err := m.w.Write(pkt); err != nil {
			return err
		}
	}
	return nil
}

func (m *Muxer) header(pid uint16, pusi bool, afc byte) []byte {
	h := []byte{0x47, byte(pid>>8) & 0x1f, byte(pid), afc | m.cc[pid]}
	if pusi {
		h[1] |= 0x40
	}
	m.cc[pid] = (m.cc[pid] + 1) & 0xf
	return h
}

// CRC32 is the MPEG-2 CRC (polynomial 0x04C11DB7, no reflection).
func CRC32(b []byte) uint32 {
	c := uint32(0xffffffff)
	for _, x := range b {
		c ^= uint32(x) << 24
		for i := 0; i < 8; i++ {
			if c&0x80000000 != 0 {
				c = c<<1 ^ 0x04c11db7
			} else {
				c <<= 1
			}
		}
	}
	return c
}
