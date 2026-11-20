package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/Eyevinn/go-teletext/mpegts"
)

const (
	ptsMask = 1<<33 - 1
	tick27  = uint64(tick / time.Microsecond * 27) // one tick in 27 MHz units
)

// insert copies an input transport stream and adds the teletext stream to it.
// Ticks follow the input's PCR, so teletext PTS are in the input's time base.
// The time printed in a cue is the wall-clock time of its PTS (-clock wall),
// mapped from the wall-clock time at the first PCR, or the PTS itself (-clock pts).
func insert(o *options, s *schedule, r io.Reader, paceFile bool, w io.Writer, flush func() error) error {
	ins := mpegts.NewInserter(w, mpegts.InserterConfig{
		TeletextPID:   uint16(o.pid),
		Pages:         s.descriptorPages(),
		ProgramNumber: uint16(o.program),
		ReplaceNull:   o.replaceNull,
		MuxRate:       o.muxRate,
	})
	start, err := startTime(o)
	if err != nil {
		return err
	}
	ptsOffset27 := uint64(o.ptsOffset / time.Microsecond * 27)

	var (
		anchored bool
		clock0   uint64    // clock at tick 0
		wall0    time.Time // wall-clock time at clock0
		n        int       // next tick
		paceFrom uint64    // clock at the last sleep
	)
	stamp := func(m int) time.Time {
		t := clock0 + uint64(m)*tick27 + ptsOffset27
		if o.clock == "pts" {
			pts := t / 300 & ptsMask
			return time.Unix(0, 0).UTC().Add(time.Duration(pts) * time.Second / 90000)
		}
		return wall0.Add(time.Duration(t-clock0) * time.Microsecond / 27)
	}

	br := bufio.NewReaderSize(r, 1<<16)
	pkt := make([]byte, mpegts.PacketSize)
	resynced := false
	for {
		if err := readPacket(br, pkt, &resynced); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return err
		}
		c, ok, err := ins.Read(pkt)
		if err != nil {
			return err
		}
		if ok {
			if !anchored || ins.Jumped() {
				if anchored {
					log.Printf("input clock jumped to %.3f s, restarting the cue cycle", float64(c)/27e6)
					s.restart()
				}
				// Ticks are on a 40 ms grid of the input clock. -start is the
				// wall-clock time of the first tick; else the time now is that of this PCR.
				clock0 = (c + tick27 - 1) / tick27 * tick27
				wall0 = start
				if o.start == "" {
					wall0 = time.Now().Add(time.Duration(clock0-c) * time.Microsecond / 27)
				}
				n, paceFrom, anchored = 0, c, true
			}
			wrote := false
			for clock0+uint64(n)*tick27 <= c {
				if o.duration > 0 && time.Duration(n)*tick >= o.duration {
					return finishInsert(ins, flush)
				}
				pes, err := s.tick(n, stamp)
				if err != nil {
					return err
				}
				pts := (clock0 + uint64(n)*tick27 + ptsOffset27) / 300 & ptsMask
				for _, units := range pes {
					if err := ins.WritePES(mpegts.TeletextPES(pts, units)); err != nil {
						return err
					}
					wrote = true
				}
				n++
			}
			if paceFile && c-paceFrom >= tick27/4 {
				time.Sleep(time.Until(wall0.Add(time.Duration(c-clock0) * time.Microsecond / 27)))
				paceFrom = c
			}
			if o.live && wrote {
				if err := flush(); err != nil {
					return err
				}
			}
		}
		if err := ins.Forward(pkt); err != nil {
			return err
		}
	}
	if !anchored {
		return errors.New("no PCR found in the input")
	}
	return finishInsert(ins, flush)
}

func finishInsert(ins *mpegts.Inserter, flush func() error) error {
	if err := ins.Flush(); err != nil {
		return err
	}
	st := ins.Stats
	log.Printf("teletext PID %#x", ins.TeletextPID())
	log.Printf("input: %d packets, %.0f bit/s; teletext packets inserted: %d; null packets replaced: %d, added: %d",
		st.Packets, st.InputBitrate(), st.Inserted, st.NullsReplaced, st.NullsAdded)
	if st.NullsReplaced == 0 && st.NullsAdded == 0 && st.Inserted > 0 {
		log.Printf("the input has no null packets: the teletext packets were added to it; " +
			"-muxrate gives constant bitrate output")
	}
	if st.NoNullInTime > 0 && st.NullsReplaced > 0 {
		log.Printf("%d teletext packets found no null packet before their PTS and were inserted, "+
			"moving later packets; a larger -pts-offset lets them wait longer, -muxrate avoids it", st.NoNullInTime)
	}
	if st.MaxLate > 0 {
		log.Printf("constant bitrate output: packets up to %.2f ms after their input time", float64(st.MaxLate)/27e3)
	}
	return flush()
}

// readPacket reads the next TS packet, skipping bytes until a sync byte if needed.
func readPacket(r *bufio.Reader, pkt []byte, resynced *bool) error {
	if _, err := io.ReadFull(r, pkt); err != nil {
		return err
	}
	for pkt[0] != 0x47 {
		if !*resynced {
			log.Printf("input lost TS sync, searching for the next sync byte")
			*resynced = true
		}
		i := 1
		for i < len(pkt) && pkt[i] != 0x47 {
			i++
		}
		copy(pkt, pkt[i:])
		if _, err := io.ReadFull(r, pkt[len(pkt)-i:]); err != nil {
			return err
		}
	}
	return nil
}

// openInput opens -i: a file, "-" for stdin, or udp://host:port to listen on,
// which joins the group when host is a multicast address. paceFile is true for
// a file, which can be read faster than real time.
func openInput(spec string) (r io.Reader, closeIn func(), paceFile bool, err error) {
	switch {
	case spec == "-":
		return os.Stdin, func() {}, false, nil
	case strings.HasPrefix(spec, "udp://"):
		addr, err := net.ResolveUDPAddr("udp", strings.TrimPrefix(spec, "udp://"))
		if err != nil {
			return nil, nil, false, err
		}
		var conn *net.UDPConn
		if addr.IP != nil && addr.IP.IsMulticast() {
			conn, err = net.ListenMulticastUDP("udp", nil, addr)
		} else {
			conn, err = net.ListenUDP("udp", addr)
		}
		if err != nil {
			return nil, nil, false, err
		}
		return &udpReader{conn: conn, buf: make([]byte, 65536)}, func() { _ = conn.Close() }, false, nil
	default:
		f, err := os.Open(spec)
		if err != nil {
			return nil, nil, false, err
		}
		return f, func() { _ = f.Close() }, true, nil
	}
}

// udpReader reads the datagrams of a UDP socket as one byte stream.
type udpReader struct {
	conn *net.UDPConn
	buf  []byte
	data []byte
}

func (u *udpReader) Read(p []byte) (int, error) {
	for len(u.data) == 0 {
		n, err := u.conn.Read(u.buf)
		if err != nil {
			return 0, fmt.Errorf("reading UDP input: %w", err)
		}
		u.data = u.buf[:n]
	}
	n := copy(p, u.data)
	u.data = u.data[n:]
	return n, nil
}
