package teletext

import (
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// G0Set identifies a G0 primary character set (EN 300 706 clause 15.6).
type G0Set int

// G0 sets supported by the encoder. Arabic and Hebrew are not supported yet:
// both need right-to-left reordering, and Arabic also needs contextual shaping.
const (
	G0Latin G0Set = iota
	G0CyrillicSerbianCroatian
	G0CyrillicRussianBulgarian
	G0CyrillicUkrainian
	G0Greek
)

// NationalSubset identifies a Latin national option sub-set (table 36).
type NationalSubset int

// National option sub-sets.
const (
	SubsetNone NationalSubset = iota
	SubsetCzechSlovak
	SubsetEnglish
	SubsetEstonian
	SubsetFrench
	SubsetGerman
	SubsetItalian
	SubsetLettishLithuanian
	SubsetPolish
	SubsetPortugueseSpanish
	SubsetRomanian
	SubsetSerbianCroatianSlovenian
	SubsetSwedishFinnishHungarian
	SubsetTurkish
)

// nationalPositions are the G0 codes replaced by a national option sub-set.
var nationalPositions = [13]byte{0x23, 0x24, 0x40, 0x5B, 0x5C, 0x5D, 0x5E, 0x5F, 0x60, 0x7B, 0x7C, 0x7D, 0x7E}

// Charset is one entry of EN 300 706 table 32: a default G0 set plus, for
// Latin, a national option sub-set.
type Charset struct {
	Name   string
	Group  byte // table 32, triplet 1 bits 11-14
	Option byte // table 32, triplet 1 bits 8-10 = header bits C12 C13 C14 (C12 is the MSB)
	G0     G0Set
	Subset NationalSubset
}

// Designation returns the 7-bit value of triplet 1 bits 8-14 in X/28/0
// Format 1 and M/29/0.
func (c Charset) Designation() byte { return c.Group<<3 | c.Option&7 }

// NeedsDesignation reports whether the page header bits alone are ambiguous
// for this charset. Decoders without X/28 and M/29 support assume group 0.
func (c Charset) NeedsDesignation() bool { return c.Group != 0 }

// Charsets of table 32 that the encoder supports.
var (
	CharsetEnglish                  = latin("English", 0, 0b000, SubsetEnglish)
	CharsetGerman                   = latin("German", 0, 0b001, SubsetGerman)
	CharsetSwedishFinnishHungarian  = latin("Swedish/Finnish/Hungarian", 0, 0b010, SubsetSwedishFinnishHungarian)
	CharsetItalian                  = latin("Italian", 0, 0b011, SubsetItalian)
	CharsetFrench                   = latin("French", 0, 0b100, SubsetFrench)
	CharsetPortugueseSpanish        = latin("Portuguese/Spanish", 0, 0b101, SubsetPortugueseSpanish)
	CharsetCzechSlovak              = latin("Czech/Slovak", 0, 0b110, SubsetCzechSlovak)
	CharsetPolish                   = latin("Polish", 1, 0b000, SubsetPolish)
	CharsetTurkish                  = latin("Turkish", 2, 0b110, SubsetTurkish)
	CharsetSerbianCroatianSlovenian = latin("Serbian/Croatian/Slovenian", 3, 0b101, SubsetSerbianCroatianSlovenian)
	CharsetRomanian                 = latin("Romanian", 3, 0b111, SubsetRomanian)
	CharsetEstonian                 = latin("Estonian", 4, 0b010, SubsetEstonian)
	CharsetLettishLithuanian        = latin("Lettish/Lithuanian", 4, 0b011, SubsetLettishLithuanian)
	CharsetCyrillicSerbianCroatian  = nonLatin("Cyrillic Serbian/Croatian", 4, 0b000, G0CyrillicSerbianCroatian)
	CharsetCyrillicRussianBulgarian = nonLatin("Cyrillic Russian/Bulgarian", 4, 0b100, G0CyrillicRussianBulgarian)
	CharsetCyrillicUkrainian        = nonLatin("Cyrillic Ukrainian", 4, 0b101, G0CyrillicUkrainian)
	CharsetGreek                    = nonLatin("Greek", 6, 0b111, G0Greek)
)

// latin returns a Latin G0 charset with a national option sub-set.
func latin(name string, group, option byte, subset NationalSubset) Charset {
	return Charset{Name: "Latin " + name, Group: group, Option: option, G0: G0Latin, Subset: subset}
}

// nonLatin returns a charset whose G0 set has no national option sub-set.
func nonLatin(name string, group, option byte, g0 G0Set) Charset {
	return Charset{Name: name, Group: group, Option: option, G0: g0, Subset: SubsetNone}
}

// G0Table returns the 96 characters (codes 0x20-0x7F) that a decoder shows
// for this charset, with the national option sub-set applied.
func (c Charset) G0Table() [96]rune {
	var t [96]rune
	switch c.G0 {
	case G0CyrillicSerbianCroatian:
		t = g0CyrillicSerbianCroatianTable
	case G0CyrillicRussianBulgarian:
		t = g0CyrillicRussianBulgarianTable
	case G0CyrillicUkrainian:
		t = g0CyrillicUkrainianTable
	case G0Greek:
		t = g0GreekTable
	default:
		t = g0LatinTable
	}
	if c.G0 == G0Latin && c.Subset != SubsetNone {
		for i, r := range nationalSubsetTable[c.Subset] {
			t[nationalPositions[i]-0x20] = r
		}
	}
	return t
}

// aliases map a Unicode letter to the one the reference tables (zvbi, and so
// go-astisub) use for the same teletext glyph. See docs/research.md.
var aliases = map[rune]rune{
	'Ż': 'Ƶ', // Polish: Z with dot above, zvbi decodes the glyph as Z with stroke
	'Ă': 'Ǎ', // Romanian: spec bitmaps draw breve and caron alike; zvbi uses caron
	'ă': 'ǎ',
	'Ș': 'Ş', // Romanian comma-below letters use the cedilla glyphs
	'ș': 'ş',
	'Ț': 'Ţ',
	'ț': 'ţ',
	'Đ': 'Ð', // Croatian D with stroke; zvbi decodes as eth
	'đ': 'ð',
	'ę': 'ȩ',    // Lithuanian e with ogonek; zvbi decodes as e with cedilla
	'₺': 0xE800, // Turkish lira; zvbi uses a private-use code point
}

// diacriticModes maps a combining mark to the X/26 mode that adds the
// diacritical mark at G2 position 0x40+n (mode = 0x10+n).
var diacriticModes = map[rune]byte{
	'̀': 0x11, // grave
	'́': 0x12, // acute
	'̂': 0x13, // circumflex
	'̃': 0x14, // tilde
	'̄': 0x15, // macron
	'̆': 0x16, // breve
	'̇': 0x17, // dot above
	'̈': 0x18, // diaeresis
	'̣': 0x19, // dot below (G2 0x49; glyph uncertain in zvbi)
	'̊': 0x1A, // ring above
	'̧': 0x1B, // cedilla
	'̦': 0x1B, // comma below, approximated by cedilla
	'̋': 0x1D, // double acute
	'̨': 0x1E, // ogonek
	'̌': 0x1F, // caron
}

// level1Fallback gives the character a Level 1 decoder shows for G2 letters
// that have no decomposition.
var level1Fallback = map[rune]byte{
	'Æ': 'A', 'æ': 'a', 'Ø': 'O', 'ø': 'o', 'Œ': 'O', 'œ': 'o', 'ß': 's',
	'Ł': 'L', 'ł': 'l', 'Ð': 'D', 'ð': 'd', 'Þ': 'T', 'þ': 't', 'Ħ': 'H',
	'ħ': 'h', 'ı': 'i', 'Ŀ': 'L', 'ŀ': 'l', 'Ŧ': 'T', 'ŧ': 't', 'Ŋ': 'N',
	'ŋ': 'n', 'Ĳ': 'I', 'ĳ': 'i',
}

// Enhancement is an X/26 column address triplet that replaces the Level 1
// character of a cell for Level 1.5 decoders.
type Enhancement struct {
	Mode, Data byte
}

// Cell is one character position of a display row.
type Cell struct {
	Code byte         // G0 code 0x20-0x7F sent in the Level 1 row
	X26  *Enhancement // Level 1.5 replacement, or nil
}

// Encoder maps Unicode text to teletext cells for one charset.
type Encoder struct {
	cs      Charset
	level15 bool
	g0      map[rune]byte
	g2      map[rune]byte
}

// NewEncoder returns an encoder for cs. With level15 set, characters that are
// not in the G0 set are sent as X/26 enhancements (Latin only, per clause E.10).
func NewEncoder(cs Charset, level15 bool) *Encoder {
	e := &Encoder{cs: cs, level15: level15, g0: map[rune]byte{}, g2: map[rune]byte{}}
	for i, r := range cs.G0Table() {
		if _, dup := e.g0[r]; !dup {
			e.g0[r] = byte(0x20 + i)
		}
	}
	if cs.G0 == G0Latin {
		for i, r := range g2LatinTable {
			if r == ' ' || r == 0xA0 {
				continue
			}
			if _, dup := e.g2[r]; !dup {
				e.g2[r] = byte(0x20 + i)
			}
		}
	}
	return e
}

// Charset returns the encoder's charset.
func (e *Encoder) Charset() Charset { return e.cs }

// Encode maps s to cells. Characters that cannot be represented are sent as
// '?' and returned in missing.
func (e *Encoder) Encode(s string) (cells []Cell, missing []rune) {
	for _, r := range norm.NFC.String(s) {
		c, ok := e.encodeRune(r)
		if !ok {
			missing = append(missing, r)
			c = Cell{Code: '?'}
		}
		cells = append(cells, c)
	}
	return cells, missing
}

func (e *Encoder) encodeRune(r rune) (Cell, bool) {
	if code, ok := e.g0[r]; ok {
		return Cell{Code: code}, true
	}
	if a, ok := aliases[r]; ok {
		if code, ok := e.g0[a]; ok {
			return Cell{Code: code}, true
		}
	}
	if e.cs.G0 != G0Latin {
		return Cell{}, false
	}
	// Decomposable letter: base letter plus one diacritical mark.
	base, mark := decompose(r)
	isASCIILetter := (base >= 'A' && base <= 'Z') || (base >= 'a' && base <= 'z')
	if mode, ok := diacriticModes[mark]; ok && isASCIILetter {
		c := Cell{Code: byte(base)}
		if e.level15 {
			c.X26 = &Enhancement{Mode: mode, Data: byte(base)}
		}
		return c, true
	}
	if code, ok := e.g2[r]; ok {
		fb, ok := level1Fallback[r]
		if !ok {
			fb = '?'
		}
		c := Cell{Code: fb}
		if e.level15 {
			c.X26 = &Enhancement{Mode: ModeG2Character, Data: code}
		}
		return c, true
	}
	return Cell{}, false
}

// decompose returns the base letter and the single combining mark of r, or
// (r, 0) when r does not decompose into exactly two runes.
func decompose(r rune) (base, mark rune) {
	d := norm.NFD.String(string(r))
	if utf8.RuneCountInString(d) != 2 {
		return r, 0
	}
	base, n := utf8.DecodeRuneInString(d)
	mark, _ = utf8.DecodeRuneInString(d[n:])
	return base, mark
}
