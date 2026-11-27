package main

import (
	"io"
	"time"

	"github.com/Eyevinn/go-teletext/mpegts"
)

// generate writes a teletext-only stream. PCR and PTS count from the Unix epoch
// and wrap with their 33 bits (mpegts.EpochPCR), so the clock is continuous over
// midnight. The time printed in a cue is the time of the PTS of its page header,
// which mpegts.TimeOfPTS gives back.
func generate(o *options, s *schedule, w io.Writer, flush func() error) error {
	cfg := mpegts.DefaultConfig()
	if o.pid != 0 {
		cfg.TeletextPID = uint16(o.pid)
	}
	cfg.Pages = s.descriptorPages()
	mux := mpegts.NewMuxer(w, cfg)

	t0, err := startTime(o)
	if err != nil {
		return err
	}
	t0 = t0.Truncate(tick).Add(tick)
	stamp := func(m int) time.Time { return t0.Add(time.Duration(m)*tick + o.ptsOffset) }

	for n := 0; o.duration <= 0 || time.Duration(n)*tick < o.duration; n++ {
		now := t0.Add(time.Duration(n) * tick)
		if o.live {
			time.Sleep(time.Until(now))
		}
		if n%2 == 0 { // PAT/PMT every 80 ms
			if err := mux.WritePSI(); err != nil {
				return err
			}
		}
		if err := mux.WritePCR(mpegts.EpochPCR(now)); err != nil {
			return err
		}
		pes, err := s.tick(n, stamp)
		if err != nil {
			return err
		}
		pts := mpegts.EpochPTS(now.Add(o.ptsOffset))
		for _, units := range pes {
			if err := mux.WritePES(mpegts.TeletextPES(pts, units)); err != nil {
				return err
			}
		}
		if o.live {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}
