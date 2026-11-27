package mpegts

import "time"

// ptsWrap is the range of PTS and of the PCR base in 90 kHz units. They wrap
// every 2^33 / 90000 s, which is about 26.5 hours (95443.718 s).
const ptsWrap = 1 << 33

// epoch27 returns the time since the Unix epoch in 27 MHz units, rounded down.
func epoch27(t time.Time) int64 {
	return t.Unix()*27_000_000 + int64(t.Nanosecond())*27/1000
}

// EpochPCR returns the PCR (27 MHz units) at t of a clock that started at 0 on
// 1970-01-01T00:00:00Z and has wrapped with the 33-bit PCR base ever since.
// The clock is continuous over midnight; its only steps are the wraps that
// every receiver handles. TimeOfPTS maps a time stamp back to the time.
func EpochPCR(t time.Time) uint64 {
	v := epoch27(t) % pcrWrap
	if v < 0 {
		v += pcrWrap
	}
	return uint64(v)
}

// EpochPTS returns the PTS (90 kHz units) at t on the clock of EpochPCR.
func EpochPTS(t time.Time) uint64 {
	return EpochPCR(t) / 300
}

// TimeOfPTS returns the time at which the clock of EpochPCR shows pts, to the
// 90 kHz tick. A PTS fits one instant in every wrap period of about 26.5 hours;
// the result is the one closest to near, so near must be within 13 hours of
// the answer.
func TimeOfPTS(pts uint64, near time.Time) time.Time {
	e := epoch27(near)
	n := e / 300
	if e%300 < 0 {
		n-- // round down before the epoch too
	}
	d := int64(pts&(ptsWrap-1)) - (n%ptsWrap+ptsWrap)%ptsWrap
	switch {
	case d > ptsWrap/2:
		d -= ptsWrap
	case d < -ptsWrap/2:
		d += ptsWrap
	}
	n += d
	sec, ticks := n/90000, n%90000
	if ticks < 0 {
		sec, ticks = sec-1, ticks+90000
	}
	return time.Unix(sec, ticks*100000/9).UTC()
}
