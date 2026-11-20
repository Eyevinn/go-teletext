package main

import (
	"io"
	"time"

	"github.com/Eyevinn/go-teletext/mpegts"
)

// generate writes a teletext-only stream. PCR and PTS are the UTC time of day
// (90 kHz), so the time printed in a cue equals the PTS of its page header.
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
		if err := mux.WritePCR(timeOfDay(now) * 300); err != nil {
			return err
		}
		pes, err := s.tick(n, stamp)
		if err != nil {
			return err
		}
		pts := timeOfDay(now.Add(o.ptsOffset)) & ptsMask
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

// timeOfDay returns the UTC time of day in 90 kHz units.
func timeOfDay(t time.Time) uint64 {
	y, m, d := t.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return uint64(t.Sub(midnight) * 90000 / time.Second)
}
