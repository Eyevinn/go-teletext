package mpegts

import (
	"bytes"
	"testing"
)

func TestTeletextPESSize(t *testing.T) {
	for n := 0; n <= 40; n++ {
		units := make([][]byte, n)
		for i := range units {
			units[i] = DataUnit(DataUnitSubtitle, true, 0, make([]byte, 42))
		}
		pes := TeletextPES(90000, units)
		if len(pes)%184 != 0 {
			t.Fatalf("%d units: PES is %d bytes", n, len(pes))
		}
		if pes[8] != 0x24 || pes[45] != 0x10 {
			t.Fatalf("%d units: header length %#x, data_identifier %#x", n, pes[8], pes[45])
		}
		if l := int(pes[4])<<8 | int(pes[5]); l != len(pes)-6 {
			t.Fatalf("PES_packet_length %d, want %d", l, len(pes)-6)
		}
	}
}

func TestDataUnit(t *testing.T) {
	pkt := make([]byte, 42)
	pkt[0] = 0x15 // Hamming 8/4 of 0 in line order
	u := DataUnit(DataUnitSubtitle, true, 0, pkt)
	if len(u) != 46 || u[0] != 0x03 || u[1] != 0x2c || u[2] != 0xe0 || u[3] != 0xe4 || u[4] != 0xa8 {
		t.Errorf("data unit header = % x", u[:5])
	}
}

func TestMuxer(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Pages = []DescriptorPage{{Language: "swe", Type: TypeSubtitle, Magazine: 8, Page: 0x88}}
	m := NewMuxer(&buf, cfg)
	for i := 0; i < 20; i++ {
		if err := m.WritePSI(); err != nil {
			t.Fatal(err)
		}
		if err := m.WritePCR(uint64(i) * 27000000); err != nil {
			t.Fatal(err)
		}
		if err := m.WritePES(TeletextPES(uint64(i)*90000, nil)); err != nil {
			t.Fatal(err)
		}
	}
	b := buf.Bytes()
	if len(b)%PacketSize != 0 {
		t.Fatalf("output is %d bytes", len(b))
	}
	last := map[uint16]int{}
	for off := 0; off < len(b); off += PacketSize {
		p := b[off : off+PacketSize]
		if p[0] != 0x47 {
			t.Fatalf("no sync byte at %d", off)
		}
		pid := uint16(p[1]&0x1f)<<8 | uint16(p[2])
		afc := p[3] >> 4 & 3
		cc := int(p[3] & 0xf)
		if pid == cfg.TeletextPID && afc != 1 {
			t.Fatalf("teletext packet with adaptation_field_control %02b", afc)
		}
		if afc&1 == 1 {
			if prev, ok := last[pid]; ok && cc != (prev+1)&0xf {
				t.Fatalf("PID %#x: CC %d after %d", pid, cc, prev)
			}
			last[pid] = cc
		}
		if pid == 0 || pid == cfg.PMTPID {
			l := int(p[6]&0x0f)<<8 | int(p[7])
			if CRC32(p[5:8+l]) != 0 {
				t.Fatalf("PID %#x: CRC error", pid)
			}
		}
	}
}
