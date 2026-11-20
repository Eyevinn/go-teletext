package teletext

import (
	"fmt"
	"strconv"
)

// Packet is one teletext packet: magazine M (1-8), packet number Y (0-31) and
// the 40 data bytes that follow the packet address, in line order.
type Packet struct {
	Magazine byte
	Y        byte
	Data     [40]byte
}

// Bytes returns the 42 bytes of the packet after the framing code:
// the two Hamming 8/4 magazine and packet address bytes, then the data.
func (p Packet) Bytes() []byte {
	m := p.Magazine & 7 // magazine 8 is coded as 0
	b := make([]byte, 0, 42)
	b = append(b, Ham84(m|(p.Y&1)<<3), Ham84(p.Y>>1))
	return append(b, p.Data[:]...)
}

// PageAddr is a page number: magazine 1-8 and a two-digit page number
// coded as BCD, so page 888 is {Magazine: 8, Page: 0x88}.
type PageAddr struct {
	Magazine byte
	Page     byte
}

// ParsePage parses a decimal page number such as "888".
func ParsePage(s string) (PageAddr, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 100 || n > 899 {
		return PageAddr{}, fmt.Errorf("teletext: page %q is not in 100-899", s)
	}
	return PageAddr{Magazine: byte(n / 100), Page: byte(n/10%10)<<4 | byte(n%10)}, nil
}

func (a PageAddr) String() string { return fmt.Sprintf("%d%02x", a.Magazine, a.Page) }

// HeaderControl holds the page header control bits (EN 300 706 clause 9.3.1.3).
type HeaderControl struct {
	Subcode         uint16 // S1-S4, 13 bits
	ErasePage       bool   // C4
	Newsflash       bool   // C5
	Subtitle        bool   // C6
	SuppressHeader  bool   // C7
	UpdateIndicator bool   // C8
	InterruptedSeq  bool   // C9
	InhibitDisplay  bool   // C10
	MagazineSerial  bool   // C11
	NationalOption  byte   // C12 C13 C14, as the 3-bit value of table 32 (C12 is the MSB)
}

func b2u(v bool) byte {
	if v {
		return 1
	}
	return 0
}

// HeaderPacket builds packet X/0. text is shown in columns 8-39 of row 0 and
// should only use characters that exist in every G0 set (digits, space, ':').
func HeaderPacket(a PageAddr, c HeaderControl, text string) Packet {
	p := Packet{Magazine: a.Magazine, Y: 0}
	s := c.Subcode
	p.Data[0] = Ham84(a.Page & 0xf)
	p.Data[1] = Ham84(a.Page >> 4)
	p.Data[2] = Ham84(byte(s))
	p.Data[3] = Ham84(byte(s>>4)&0x7 | b2u(c.ErasePage)<<3)
	p.Data[4] = Ham84(byte(s >> 7))
	p.Data[5] = Ham84(byte(s>>11)&0x3 | b2u(c.Newsflash)<<2 | b2u(c.Subtitle)<<3)
	c7c10 := b2u(c.SuppressHeader) | b2u(c.UpdateIndicator)<<1 | b2u(c.InterruptedSeq)<<2 | b2u(c.InhibitDisplay)<<3
	p.Data[6] = Ham84(c7c10)
	o := c.NationalOption
	p.Data[7] = Ham84(b2u(c.MagazineSerial) | (o>>2&1)<<1 | (o>>1&1)<<2 | (o&1)<<3)
	for i := 8; i < 40; i++ {
		ch := byte(' ')
		if j := i - 8; j < len(text) {
			ch = text[j]
		}
		p.Data[i] = OddParity(ch)
	}
	return p
}

// FillerHeader returns a header for page FF of the magazine. EN 300 706
// clause A.1 reserves page mFF for time filling and as a terminator: it carries
// no data, ends the transmission of the previous page in the magazine (in
// parallel mode) and does not change what is displayed. It makes a good
// heartbeat during periods without subtitles.
func FillerHeader(magazine byte) Packet {
	return HeaderPacket(PageAddr{Magazine: magazine, Page: 0xFF}, HeaderControl{SuppressHeader: true}, "")
}

// RowPacket builds a display packet X/1-X/25 from 40 G0 codes and spacing attributes.
func RowPacket(magazine, row byte, codes [40]byte) Packet {
	p := Packet{Magazine: magazine, Y: row}
	for i, c := range codes {
		p.Data[i] = OddParity(c)
	}
	return p
}

// Triplet is the 18-bit content of an X/26 triplet: address (6 bits),
// mode (5 bits) and data (7 bits), see EN 300 706 clause 12.3.
type Triplet struct {
	Address, Mode, Data byte
}

func (t Triplet) value() uint32 {
	return uint32(t.Address&0x3f) | uint32(t.Mode&0x1f)<<6 | uint32(t.Data&0x7f)<<11
}

// X/26 triplet constants.
const (
	ModeSetActivePosition = 0x04 // row address group
	ModeTermination       = 0x1f // row address group
	ModeG2Character       = 0x0f // column address group
	ModeG0NoDiacritic     = 0x10 // column address group
	// ModeG0NoDiacritic + n (n = 1..15) adds the diacritical mark at G2 position 0x40+n.
)

// TerminationTriplet marks the end of local enhancement data with no local
// objects following (clause 12.3.3, mode 11111, data 111).
var TerminationTriplet = Triplet{Address: 0x3f, Mode: ModeTermination, Data: 0x07}

// RowAddress returns the X/26 address that selects display row r (1-24).
func RowAddress(r byte) byte {
	if r == 24 {
		return 40
	}
	return 40 + r
}

// X26Packet builds packet X/26/designation from up to 13 triplets. Unused
// triplets are filled with the termination marker as clause 12.3.3 requires.
func X26Packet(magazine, designation byte, ts []Triplet) Packet {
	p := Packet{Magazine: magazine, Y: 26}
	p.Data[0] = Ham84(designation)
	for i := 0; i < 13; i++ {
		t := TerminationTriplet
		if i < len(ts) {
			t = ts[i]
		}
		h := Ham2418(t.value())
		copy(p.Data[1+3*i:], h[:])
	}
	return p
}

// DesignationPacket builds X/28/0 Format 1 (y = 28) or M/29/0 (y = 29) that
// designates the default G0/G2 sets and national option of table 32.
// secondG0 is the 7-bit second G0 designation of table 33; 0x7f means none.
//
// Draft limitation: the colour map (CLUT 2 and 3), default screen and row
// colours and side panels are left at zero, see docs/design.md.
func DesignationPacket(magazine, y byte, designation, secondG0 byte) Packet {
	p := Packet{Magazine: magazine, Y: y}
	p.Data[0] = Ham84(0) // designation code 0
	// Triplet 1: bits 1-4 page function (0 = LOP), 5-7 page coding (0 = 7-bit
	// odd parity), 8-14 default G0/G2 designation, 15-18 second G0 bits 1-4.
	t1 := uint32(designation&0x7f)<<7 | uint32(secondG0&0x0f)<<14
	// Triplet 2: bits 1-3 second G0 bits 5-7; side panels off; CLUT data 0.
	t2 := uint32(secondG0>>4) & 0x7
	h := Ham2418(t1)
	copy(p.Data[1:], h[:])
	h = Ham2418(t2)
	copy(p.Data[4:], h[:])
	for i := 2; i < 13; i++ {
		h = Ham2418(0)
		copy(p.Data[1+3*i:], h[:])
	}
	return p
}
