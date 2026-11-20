package mpegts

import (
	"errors"
	"fmt"
)

// NullPID is the PID of null packets.
const NullPID = 0x1fff

// ErrSectionSpansPackets is returned for a PSI section that does not fit in
// one TS packet. Such sections are not supported yet.
var ErrSectionSpansPackets = errors.New("mpegts: PSI section spans more than one TS packet")

// PacketPID returns the PID of a TS packet.
func PacketPID(pkt []byte) uint16 { return uint16(pkt[1]&0x1f)<<8 | uint16(pkt[2]) }

// PayloadStart reports whether payload_unit_start_indicator is set.
func PayloadStart(pkt []byte) bool { return pkt[1]&0x40 != 0 }

// payload returns the payload of a TS packet, after any adaptation field.
func payload(pkt []byte) []byte {
	afc := pkt[3] >> 4 & 3
	if afc&1 == 0 {
		return nil
	}
	off := 4
	if afc&2 != 0 {
		off += 1 + int(pkt[4])
	}
	if off >= PacketSize {
		return nil
	}
	return pkt[off:]
}

// PCR returns the program clock reference (27 MHz) of a packet, if it has one.
func PCR(pkt []byte) (uint64, bool) {
	if pkt[3]&0x20 == 0 || pkt[4] < 7 || pkt[5]&0x10 == 0 {
		return 0, false
	}
	base := uint64(pkt[6])<<25 | uint64(pkt[7])<<17 | uint64(pkt[8])<<9 | uint64(pkt[9])<<1 | uint64(pkt[10])>>7
	ext := uint64(pkt[10]&1)<<8 | uint64(pkt[11])
	return base*300 + ext, true
}

// Discontinuity reports whether the adaptation field has discontinuity_indicator set.
func Discontinuity(pkt []byte) bool {
	return pkt[3]&0x20 != 0 && pkt[4] > 0 && pkt[5]&0x80 != 0
}

// SectionInPacket returns the PSI section that starts in a packet with
// payload_unit_start_indicator set. The CRC is checked.
func SectionInPacket(pkt []byte) ([]byte, error) {
	if !PayloadStart(pkt) {
		return nil, errors.New("mpegts: no section starts in this packet")
	}
	p := payload(pkt)
	if len(p) < 1 || 1+int(p[0])+3 > len(p) {
		return nil, ErrSectionSpansPackets
	}
	s := p[1+int(p[0]):]
	l := 3 + (int(s[1]&0x0f)<<8 | int(s[2]))
	if l > len(s) {
		return nil, ErrSectionSpansPackets
	}
	s = s[:l]
	if CRC32(s) != 0 {
		return nil, errors.New("mpegts: PSI section CRC error")
	}
	return s, nil
}

// SectionPacket puts a PSI section in one TS packet with continuity counter cc.
func SectionPacket(pid uint16, cc byte, section []byte) ([]byte, error) {
	if 1+len(section) > PacketSize-4 {
		return nil, ErrSectionSpansPackets
	}
	pkt := make([]byte, PacketSize)
	pkt[0], pkt[1], pkt[2], pkt[3] = 0x47, 0x40|byte(pid>>8)&0x1f, byte(pid), 0x10|cc&0x0f
	pkt[4] = 0 // pointer_field
	copy(pkt[5:], section)
	for i := 5 + len(section); i < PacketSize; i++ {
		pkt[i] = 0xff
	}
	return pkt, nil
}

// longSection builds a PSI section with section_syntax_indicator 1 and a CRC.
func longSection(tableID byte, ext uint16, version byte, body []byte) []byte {
	l := 5 + len(body) + 4
	s := []byte{tableID, 0xb0 | byte(l>>8), byte(l), byte(ext >> 8), byte(ext), 0xc1 | (version&0x1f)<<1, 0, 0}
	s = append(s, body...)
	crc := CRC32(s)
	return append(s, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
}

// Program is an entry of the PAT.
type Program struct {
	Number uint16
	PMTPID uint16
}

// ParsePAT returns the programs of a PAT section, without the NIT entry.
func ParsePAT(s []byte) ([]Program, error) {
	if len(s) < 12 || s[0] != 0x00 {
		return nil, errors.New("mpegts: not a PAT section")
	}
	var ps []Program
	for i := 8; i+4 <= len(s)-4; i += 4 {
		n := uint16(s[i])<<8 | uint16(s[i+1])
		if n != 0 {
			ps = append(ps, Program{Number: n, PMTPID: uint16(s[i+2]&0x1f)<<8 | uint16(s[i+3])})
		}
	}
	return ps, nil
}

// PATSection builds a PAT section.
func PATSection(transportStreamID uint16, programs []Program) []byte {
	var body []byte
	for _, p := range programs {
		body = append(body, byte(p.Number>>8), byte(p.Number), 0xe0|byte(p.PMTPID>>8), byte(p.PMTPID))
	}
	return longSection(0x00, transportStreamID, 0, body)
}

// ElementaryStream is an entry of the PMT.
type ElementaryStream struct {
	Type byte
	PID  uint16
	Info []byte // descriptors
}

// PMT is a program map table.
type PMT struct {
	ProgramNumber uint16
	Version       byte
	PCRPID        uint16
	ProgramInfo   []byte // descriptors
	Streams       []ElementaryStream
}

// ParsePMT parses a PMT section.
func ParsePMT(s []byte) (*PMT, error) {
	if len(s) < 16 || s[0] != 0x02 {
		return nil, errors.New("mpegts: not a PMT section")
	}
	p := &PMT{
		ProgramNumber: uint16(s[3])<<8 | uint16(s[4]),
		Version:       s[5] >> 1 & 0x1f,
		PCRPID:        uint16(s[8]&0x1f)<<8 | uint16(s[9]),
	}
	end := len(s) - 4
	pil := int(s[10]&0x0f)<<8 | int(s[11])
	if 12+pil > end {
		return nil, errors.New("mpegts: PMT program_info_length too long")
	}
	p.ProgramInfo = append([]byte(nil), s[12:12+pil]...)
	for i := 12 + pil; i < end; {
		if i+5 > end {
			return nil, errors.New("mpegts: truncated PMT stream entry")
		}
		l := int(s[i+3]&0x0f)<<8 | int(s[i+4])
		if i+5+l > end {
			return nil, fmt.Errorf("mpegts: PMT ES_info_length %d too long", l)
		}
		p.Streams = append(p.Streams, ElementaryStream{
			Type: s[i],
			PID:  uint16(s[i+1]&0x1f)<<8 | uint16(s[i+2]),
			Info: append([]byte(nil), s[i+5:i+5+l]...),
		})
		i += 5 + l
	}
	return p, nil
}

// Section builds the PMT section.
func (p *PMT) Section() []byte {
	pil := len(p.ProgramInfo)
	body := []byte{0xe0 | byte(p.PCRPID>>8), byte(p.PCRPID), 0xf0 | byte(pil>>8), byte(pil)}
	body = append(body, p.ProgramInfo...)
	for _, es := range p.Streams {
		l := len(es.Info)
		body = append(body, es.Type, 0xe0|byte(es.PID>>8), byte(es.PID), 0xf0|byte(l>>8), byte(l))
		body = append(body, es.Info...)
	}
	return longSection(0x02, p.ProgramNumber, p.Version, body)
}

// TeletextDescriptor builds the EN 300 468 teletext descriptor (tag 0x56).
func TeletextDescriptor(pages []DescriptorPage) []byte {
	var d []byte
	for _, p := range pages {
		lang := (p.Language + "   ")[:3]
		d = append(d, lang[0], lang[1], lang[2], p.Type<<3|p.Magazine&7, p.Page)
	}
	return append([]byte{0x56, byte(len(d))}, d...)
}

// TeletextStream returns the PMT entry for a teletext PID with these pages.
func TeletextStream(pid uint16, pages []DescriptorPage) ElementaryStream {
	return ElementaryStream{Type: 0x06, PID: pid, Info: TeletextDescriptor(pages)}
}
