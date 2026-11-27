package mpegts

import (
	"bytes"
	"reflect"
	"testing"
)

func TestPATRoundTrip(t *testing.T) {
	progs := []Program{{Number: 1, PMTPID: 0x1000}, {Number: 2, PMTPID: 0x1001}}
	pkt, err := SectionPacket(0, 5, PATSection(7, progs))
	if err != nil {
		t.Fatal(err)
	}
	s, err := SectionInPacket(pkt)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParsePAT(s)
	if err != nil || !reflect.DeepEqual(got, progs) {
		t.Fatalf("ParsePAT = %v, %v", got, err)
	}
	if pkt[3]&0x0f != 5 {
		t.Errorf("CC = %d, want 5", pkt[3]&0x0f)
	}
}

func TestPMTRoundTrip(t *testing.T) {
	pmt := &PMT{
		ProgramNumber: 3, Version: 9, PCRPID: 0x100, ProgramInfo: []byte{0x0e, 0x03, 1, 2, 3},
		Streams: []ElementaryStream{
			{Type: 0x1b, PID: 0x100},
			TeletextStream(0x101, []DescriptorPage{{Language: "swe", Type: TypeSubtitle, Magazine: 8, Page: 0x88}}),
		},
	}
	pkt, err := SectionPacket(0x1000, 0, pmt.Section())
	if err != nil {
		t.Fatal(err)
	}
	s, err := SectionInPacket(pkt)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParsePMT(s)
	if err != nil || !reflect.DeepEqual(got, pmt) {
		t.Fatalf("ParsePMT = %+v, %v\nwant %+v", got, err, pmt)
	}
	// Type 0x02 (subtitle) in the top 5 bits, magazine 8 coded as 0.
	if want := []byte{0x56, 5, 's', 'w', 'e', 0x10, 0x88}; !bytes.Equal(got.Streams[1].Info, want) {
		t.Errorf("teletext descriptor = % x, want % x", got.Streams[1].Info, want)
	}
	pkt[20] ^= 1
	if _, err := SectionInPacket(pkt); err == nil {
		t.Error("CRC error not detected")
	}
}

func TestPCRRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	m := NewMuxer(&buf, DefaultConfig())
	for _, v := range []uint64{0, 1, 299, 300, 27_000_000*3600 + 123, pcrWrap - 1} {
		buf.Reset()
		if err := m.WritePCR(v); err != nil {
			t.Fatal(err)
		}
		if got, ok := PCR(buf.Bytes()); !ok || got != v {
			t.Errorf("PCR(%d) = %d, %v", v, got, ok)
		}
		if got, _ := PCR(withPCR(buf.Bytes(), v/2)); got != v/2 {
			t.Errorf("withPCR(%d) = %d", v/2, got)
		}
	}
}
