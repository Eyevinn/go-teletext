package main

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	teletext "github.com/Eyevinn/go-teletext"
	"github.com/Eyevinn/go-teletext/mpegts"
)

// pageState is one language page.
type pageState struct {
	lang teletext.Language
	addr teletext.PageAddr
	enc  *teletext.Encoder
	slot int // order within the magazine; pages of one magazine are sent one after another
}

// schedule decides what teletext to send on each 40 ms tick. The generator
// and the inserter share it; they differ only in where the clock comes from.
type schedule struct {
	o           *options
	ps          []*pageState
	slots       int
	periodTicks int
	cueTicks    int
	hbTicks     int
	opts        teletext.PageOptions
	cueNr       int
	lastPES     int
	warned      map[string]bool
}

func newSchedule(o *options) (*schedule, error) {
	modes := map[string]teletext.DesignationMode{
		"auto": teletext.DesignationAuto, "x28": teletext.DesignationX28,
		"m29": teletext.DesignationM29, "none": teletext.DesignationNone,
	}
	mode, ok := modes[o.designation]
	if !ok {
		return nil, fmt.Errorf("unknown -designation %q", o.designation)
	}
	ps, slots, err := pageStates(o)
	if err != nil {
		return nil, err
	}
	s := &schedule{
		o:           o,
		ps:          ps,
		slots:       slots,
		periodTicks: int(o.period / tick),
		cueTicks:    int(o.cue / tick),
		hbTicks:     int(o.heartbeat / tick),
		opts:        teletext.PageOptions{SingleHeight: o.singleHeight, Designation: mode, Level15: o.level15},
		warned:      map[string]bool{},
	}
	if s.periodTicks <= 0 || s.cueTicks < 2*slots || s.cueTicks >= s.periodTicks {
		return nil, fmt.Errorf("need 2*%d ticks <= cue (%v) < period (%v); pages of one magazine are sent 80 ms apart",
			slots, o.cue, o.period)
	}
	if o.heartbeatKind != "filler" && o.heartbeatKind != "stuffing" {
		return nil, fmt.Errorf("unknown -heartbeat-kind %q", o.heartbeatKind)
	}
	if (o.cuesOn > 0) != (o.cuesOff > 0) {
		return nil, errors.New("-cues-on and -cues-off must be given together")
	}
	return s, nil
}

// restart begins a new cue cycle at tick 0, for example after a clock jump.
func (s *schedule) restart() { s.lastPES = 0 }

// descriptorPages returns the teletext descriptor entries for the pages.
func (s *schedule) descriptorPages() []mpegts.DescriptorPage {
	typ := byte(mpegts.TypeSubtitle)
	if s.o.hearingImpaired {
		typ = mpegts.TypeHearingImpaired
	}
	var dps []mpegts.DescriptorPage
	for _, p := range s.ps {
		dps = append(dps, mpegts.DescriptorPage{
			Language: p.lang.Code, Type: typ, Magazine: p.addr.Magazine, Page: p.addr.Page,
		})
	}
	return dps
}

// tick returns the data units of the PES packets to send on tick n, counted
// from the start of the cue cycle. stamp gives the time to print in a cue
// whose page header is sent on tick m. A heartbeat of the stuffing kind is a
// PES with no data units, returned as a nil entry.
func (s *schedule) tick(n int, stamp func(m int) time.Time) ([][][]byte, error) {
	phase := n % s.periodTicks
	cycle := n - phase
	// Whole periods are skipped during a pause, so a cue and its clear stay together.
	o := s.o
	paused := o.cuesOn > 0 && time.Duration(cycle)*tick%(o.cuesOn+o.cuesOff) >= o.cuesOn

	var units [][]byte
	switch {
	case paused:
	case phase < 2*s.slots: // even: headers of slot phase/2; odd: their bodies 40 ms later (clause B.1)
		if phase == 0 {
			s.cueNr++
		}
		for _, p := range s.ps {
			if p.slot != phase/2 {
				continue
			}
			pg, err := s.compose(p, stamp(cycle+2*p.slot))
			if err != nil {
				return nil, err
			}
			if phase%2 == 0 {
				if pg.M29 != nil {
					units = append(units, unit(*pg.M29))
				}
				units = append(units, unit(pg.Header))
			} else {
				for _, pk := range pg.Body {
					units = append(units, unit(pk))
				}
			}
		}
	case phase == s.cueTicks: // clear all pages
		for _, p := range s.ps {
			pg, err := teletext.ComposePage(p.addr, p.enc, nil, s.opts)
			if err != nil {
				return nil, err
			}
			units = append(units, unit(pg.Header))
		}
	}

	var pes [][][]byte
	// EN 300 472: at most 16 lines per field, so 32 data units per frame.
	for len(units) > 0 {
		k := min(len(units), 32)
		pes = append(pes, units[:k])
		units = units[k:]
	}
	if len(pes) > 0 {
		s.lastPES = n
	} else if s.hbTicks > 0 && n-s.lastPES >= s.hbTicks {
		pes = append(pes, s.heartbeat())
		s.lastPES = n
	}
	return pes, nil
}

// compose builds the cue page for p, printing the time t.
func (s *schedule) compose(p *pageState, t time.Time) (*teletext.Page, error) {
	stamp := fmt.Sprintf("%s %s (%d)", p.lang.Name, t.Format("15:04:05.000"), s.cueNr)
	lines := []teletext.Line{
		{Text: p.lang.Sample, Colour: teletext.White},
		{Text: stamp, Colour: teletext.Yellow},
	}
	opts := s.opts
	opts.HeaderText = strings.Repeat(" ", 24) + t.Format("15:04:05")
	pg, err := teletext.ComposePage(p.addr, p.enc, lines, opts)
	if err != nil {
		return nil, err
	}
	if len(pg.Missing) > 0 && !s.warned[p.lang.Code] {
		s.warned[p.lang.Code] = true
		missing := make([]string, 0, len(pg.Missing))
		for _, r := range pg.Missing {
			missing = append(missing, string(r))
		}
		sort.Strings(missing)
		log.Printf("%s: no teletext code for %q, sent as '?'", p.lang.Code, strings.Join(missing, ""))
	}
	return pg, nil
}

// heartbeat returns the data units of a heartbeat PES: a time filling header
// (page mFF) for every magazine in use, or none, which gives a PES of stuffing
// data units only. Either way the PES has a PTS and no displayable rows, which a
// segmenter can use to close subtitle segments in step with video and audio.
func (s *schedule) heartbeat() [][]byte {
	if s.o.heartbeatKind != "filler" {
		return nil
	}
	var units [][]byte
	seen := map[byte]bool{}
	for _, p := range s.ps {
		if !seen[p.addr.Magazine] {
			seen[p.addr.Magazine] = true
			units = append(units, unit(teletext.FillerHeader(p.addr.Magazine)))
		}
	}
	return units
}

// pageStates resolves the languages and their pages, and returns the largest
// number of pages that share a magazine.
func pageStates(o *options) ([]*pageState, int, error) {
	var pageList []string
	if o.pages != "" {
		pageList = strings.Split(o.pages, ",")
	}
	var ps []*pageState
	perMag := map[byte]int{}
	for i, code := range strings.Split(o.langs, ",") {
		l, ok := teletext.LookupLanguage(strings.TrimSpace(code))
		if !ok {
			return nil, 0, fmt.Errorf("unknown language %q, see -list", code)
		}
		// Default: one magazine per language (888, 788, ... 188, then 889, 789, ...)
		// so pages in different magazines can be sent in parallel.
		p := fmt.Sprintf("%d%02d", 8-i%8, 88+i/8)
		if i < len(pageList) {
			p = strings.TrimSpace(pageList[i])
		}
		addr, err := teletext.ParsePage(p)
		if err != nil {
			return nil, 0, err
		}
		ps = append(ps, &pageState{
			lang: l, addr: addr, enc: teletext.NewEncoder(l.Charset, o.level15), slot: perMag[addr.Magazine],
		})
		perMag[addr.Magazine]++
	}
	slots := 0
	for _, n := range perMag {
		slots = max(slots, n)
	}
	return ps, slots, nil
}

func unit(p teletext.Packet) []byte {
	return mpegts.DataUnit(mpegts.DataUnitSubtitle, true, 0, p.Bytes())
}
