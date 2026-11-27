package mpegts

import (
	"testing"
	"time"
)

func TestEpochClock(t *testing.T) {
	// The wrap period is 2^33 ticks of 90 kHz. Wrap 18812 starts on 2026-11-24.
	wrapStart := func(k int64) time.Time {
		n := k * ptsWrap
		return time.Unix(n/90000, n%90000*100000/9).UTC()
	}
	w := wrapStart(18812)
	if got := w.Format("2006-01-02"); got != "2026-11-24" {
		t.Fatalf("wrap 18812 starts %s", w.Format(time.RFC3339Nano))
	}

	if got := EpochPCR(time.Unix(0, 0)); got != 0 {
		t.Errorf("PCR at the epoch is %d", got)
	}
	if got := EpochPTS(time.Unix(1, 0)); got != 90000 {
		t.Errorf("PTS 1 s after the epoch is %d", got)
	}
	// The last 40 ms tick before the wrap and the first after it are 3600 PTS
	// ticks apart modulo 2^33, and PCR is 300 times PTS throughout.
	before := w.Truncate(40 * time.Millisecond)
	after := before.Add(40 * time.Millisecond)
	a, b := EpochPTS(before), EpochPTS(after)
	if b >= a || (b-a)&(ptsWrap-1) != 3600 {
		t.Errorf("PTS %d then %d across the wrap", a, b)
	}
	if got := EpochPCR(before); got != a*300 {
		t.Errorf("PCR %d, want %d", got, a*300)
	}

	// Midnight is not a step.
	day := time.Date(2026, 11, 20, 23, 59, 59, 960e6, time.UTC)
	if d := EpochPTS(day.Add(40*time.Millisecond)) - EpochPTS(day); d != 3600 {
		t.Errorf("PTS steps %d over midnight", d)
	}

	// TimeOfPTS finds the time from a clock that is up to 13 hours off.
	for _, tm := range []time.Time{
		time.Unix(0, 0).UTC(),
		day, before, after, w,
		time.Date(1969, 12, 31, 23, 59, 59, 40e6, time.UTC),
	} {
		tm = tm.Truncate(time.Millisecond)
		for _, off := range []time.Duration{0, 777 * time.Microsecond, 13 * time.Hour, -13 * time.Hour} {
			if got := TimeOfPTS(EpochPTS(tm), tm.Add(off)); !got.Equal(tm) {
				t.Errorf("TimeOfPTS(%v, off %v) = %v", tm, off, got)
			}
		}
		if got := TimeOfPTS(EpochPTS(tm), tm.Add(14*time.Hour)); got.Equal(tm) {
			t.Errorf("TimeOfPTS(%v) found the time from a clock 14 hours off", tm)
		}
	}
}
