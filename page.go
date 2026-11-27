package teletext

import (
	"fmt"
	"sort"
)

// Colour is a teletext alphanumeric colour spacing attribute (0x00-0x07).
type Colour byte

// Alphanumeric colours.
const (
	Black Colour = iota
	Red
	Green
	Yellow
	Blue
	Magenta
	Cyan
	White
)

// Spacing attributes used for subtitles (clause 12.2).
const (
	attrEndBox       = 0x0a
	attrStartBox     = 0x0b
	attrNormalSize   = 0x0c
	attrDoubleHeight = 0x0d
)

// DesignationMode selects how a non-default charset is signalled.
type DesignationMode int

// Designation modes.
const (
	DesignationAuto DesignationMode = iota // X/28/0 Format 1 when the charset needs it
	DesignationX28                         // always X/28/0 Format 1
	DesignationM29                         // M/29/0 (magazine-wide) when the charset needs it
	DesignationNone                        // header bits C12-C14 only
)

// Line is one subtitle line.
type Line struct {
	Text   string
	Colour Colour // text colour; the zero value (Black) is sent as White
}

// PageOptions controls the layout of a subtitle page.
type PageOptions struct {
	SingleHeight bool            // default is double height
	BottomRow    byte            // last row used; default 22 (double height) or 23
	Designation  DesignationMode // default DesignationAuto
	Level15      bool            // send X/26 enhancements
	HeaderText   string          // header row text, e.g. a clock "12:34:56"
}

// Page is a composed subtitle page, split the way it must be transmitted:
// the header first, then (at least 20 ms later, clause B.1) the body.
type Page struct {
	Addr    PageAddr
	Header  Packet
	Body    []Packet // X/28, X/26 in designation order, then X/1-X/25
	M29     *Packet  // magazine designation to send before the header, if any
	Missing []rune   // characters that could not be encoded
}

// ComposePage builds the packets of a subtitle page. An empty lines slice
// gives a page that only clears the screen.
func ComposePage(addr PageAddr, enc *Encoder, lines []Line, opt PageOptions) (*Page, error) {
	cs := enc.Charset()
	pg := &Page{Addr: addr}
	pg.Header = HeaderPacket(addr, HeaderControl{
		ErasePage:      true,
		Subtitle:       true,
		SuppressHeader: true,
		NationalOption: cs.Option,
	}, opt.HeaderText)

	needs := cs.NeedsDesignation()
	switch {
	case opt.Designation == DesignationX28 || (opt.Designation == DesignationAuto && needs):
		pg.Body = append(pg.Body, DesignationPacket(addr.Magazine, 28, cs.Designation(), 0x7f))
	case opt.Designation == DesignationM29 && needs:
		m := DesignationPacket(addr.Magazine, 29, cs.Designation(), 0x7f)
		pg.M29 = &m
	}
	if len(lines) == 0 {
		return pg, nil
	}

	double := !opt.SingleHeight
	step := byte(1)
	bottom := byte(23)
	if double {
		step, bottom = 2, 22
	}
	if opt.BottomRow != 0 {
		bottom = opt.BottomRow
	}
	if double && bottom > 22 {
		return nil, fmt.Errorf("teletext: double height row %d would overflow row 23", bottom)
	}
	first := int(bottom) - int(step)*(len(lines)-1)
	if first < 1 {
		return nil, fmt.Errorf("teletext: %d lines do not fit above row %d", len(lines), bottom)
	}

	type enh struct {
		row, col byte
		e        Enhancement
	}
	var enhs []enh
	var rows []Packet
	for i, l := range lines {
		row := byte(first + i*int(step))
		cells, missing := enc.Encode(l.Text)
		pg.Missing = append(pg.Missing, missing...)

		var prefix []byte
		if double {
			prefix = append(prefix, attrDoubleHeight)
		}
		col := l.Colour
		if col == Black {
			col = White
		}
		prefix = append(prefix, byte(col), attrStartBox, attrStartBox)
		suffix := []byte{attrEndBox, attrEndBox}
		maxText := 40 - len(prefix) - len(suffix)
		if len(cells) > maxText {
			return nil, fmt.Errorf("teletext: line %q is %d characters, max %d", l.Text, len(cells), maxText)
		}

		var codes [40]byte
		for j := range codes {
			codes[j] = ' '
		}
		start := (40 - len(prefix) - len(cells) - len(suffix)) / 2
		pos := start
		for _, b := range prefix {
			codes[pos] = b
			pos++
		}
		for _, c := range cells {
			codes[pos] = c.Code
			if c.X26 != nil {
				enhs = append(enhs, enh{row, byte(pos), *c.X26})
			}
			pos++
		}
		for _, b := range suffix {
			codes[pos] = b
			pos++
		}
		rows = append(rows, RowPacket(addr.Magazine, row, codes))
	}

	if opt.Level15 && len(enhs) > 0 {
		// Clause A.4: triplets in screen order, left to right, top to bottom.
		sort.SliceStable(enhs, func(i, j int) bool {
			if enhs[i].row != enhs[j].row {
				return enhs[i].row < enhs[j].row
			}
			return enhs[i].col < enhs[j].col
		})
		var ts []Triplet
		lastRow := byte(0)
		for _, x := range enhs {
			if x.row != lastRow {
				ts = append(ts, Triplet{Address: RowAddress(x.row), Mode: ModeSetActivePosition, Data: 0})
				lastRow = x.row
			}
			ts = append(ts, Triplet{Address: x.col, Mode: x.e.Mode, Data: x.e.Data})
		}
		ts = append(ts, TerminationTriplet)
		if n := (len(ts) + 12) / 13; n > 16 {
			return nil, fmt.Errorf("teletext: %d X/26 triplets need %d packets, max 16", len(ts), n)
		}
		// Clause B.2: X/26 packets in ascending designation order, before the rows.
		for d := 0; d*13 < len(ts); d++ {
			end := min((d+1)*13, len(ts))
			pg.Body = append(pg.Body, X26Packet(addr.Magazine, byte(d), ts[d*13:end]))
		}
	}
	pg.Body = append(pg.Body, rows...)
	return pg, nil
}
