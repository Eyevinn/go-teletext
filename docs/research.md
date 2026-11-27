# Research: teletext and EBU STL support, and existing generators

Research done 2026-11-20 against:

- go-astisub `master` at `8dc2cdb` (teletext and STL code identical to release v0.45.0)
- shaka-packager `main` at `323ac9edf1`
- the specifications in [`../specs`](../specs) (fetch with `specs/fetch.sh`)

Statements marked **verified** were reproduced with code in
[`../experiments`](../experiments) or with the tests in this repository.

## 1. Specifications

| Spec | Version | Content |
|---|---|---|
| ETSI EN 300 706 | V1.2.1 (2003-04), 156 pp | Enhanced Teletext: packets, Hamming coding, presentation levels, character sets |
| ETSI EN 300 472 | V1.4.1 (2017-04) | Teletext in DVB: PES and data unit format |
| ETSI EN 301 775 | V1.2.1 | VBI data in DVB (superset of EN 300 472, data_identifier 0x10-0x1F) |
| ETSI EN 300 468 | (not downloaded) | DVB SI: teletext descriptor 0x56 |
| EBU Tech 3264 | 1991, 31 pp | EBU subtitling data exchange format (`.stl`) |

### 1.1 Teletext presentation levels (EN 300 706 clause 10)

- **Level 1**: 40x25 cells. Rows X/1-X/25 carry 7-bit G0 codes with odd parity.
  Codes 0x00-0x1F are spacing attributes (colour, double height, start/end box).
  The page header bits C12-C14 pick one of 8 **national option sub-sets**:
  13 G0 positions (0x23 0x24 0x40 0x5B-0x60 0x7B-0x7E) are replaced by national letters.
- **Level 1.5**: adds
  - **X/26** triplets that put a G2 character, or a G0 letter with a diacritical
    mark from G2, on a given row/column. This is how letters that are in no
    national sub-set reach the screen (Hungarian ő ű, Danish and Norwegian Æ Ø,
    Portuguese ã õ, ...). The Level 1 row carries a fallback letter.
  - **X/28/0 Format 1** (page) and **M/29/0** (magazine) that designate the
    default G0/G2 sets and national sub-set via table 32. Without them the C12-C14
    bits are ambiguous: the same 3 bits mean English in group 0, Polish in
    group 1, Cyrillic in group 4, and so on.
- **Level 2.5 / 3.5**: full X/26 (non-spacing attributes, G1/G3 mosaics),
  colour look-up tables, side panels, objects (POP/GPOP), downloadable
  characters (DRCS). Rarely used for subtitles.

Table 32 (default G0/G2 designation, triplet 1 bits 8-14) as used by the generator:

| Group (bits 14-11) | C12 C13 C14 | Character set |
|---|---|---|
| 0000 | 000 / 001 / 010 / 011 / 100 / 101 / 110 | Latin: English / German / Swedish-Finnish-Hungarian / Italian / French / Portuguese-Spanish / Czech-Slovak |
| 0001 | 000 | Latin Polish (other values as group 0) |
| 0010 | 110 | Latin Turkish (other values as group 0) |
| 0011 | 101 / 111 | Latin Serbian-Croatian-Slovenian / Romanian |
| 0100 | 000 / 010 / 011 / 100 / 101 | Cyrillic-1 Serbian / Estonian / Lettish-Lithuanian / Cyrillic-2 Russian-Bulgarian / Cyrillic-3 Ukrainian |
| 0110 | 110 / 111 | Latin Turkish / Greek |
| 1000, 1010 | ... | Latin with Arabic G2, Arabic, Hebrew |

Note the bit order: table 32 writes C12 as the MSB, while the header byte
carries C11 C12 C13 C14 as bits 0..3. Both decoders compared below index their
tables with `C12 | C13<<1 | C14<<2`, which is consistent but reversed relative
to the table.

### 1.2 DVB carriage (EN 300 472)

- PES `stream_id` 0xBD, `PES_header_data_length` = 0x24, `PES_packet_length` = N*184-6,
  so a PES fills whole TS packets with no adaptation field
  (only `adaptation_field_control` 01 and 10 are allowed on the teletext PID).
- `data_identifier` 0x10-0x1F; data units of 46 bytes: id (0x02 non-subtitle,
  0x03 subtitle, 0xFF stuffing), length 0x2C, field parity + line offset
  (0 = undefined), framing code 0xE4, then the 42 bytes of the packet,
  **each byte bit-reversed** relative to EN 300 706's LSB-first numbering.
- Annex A: send a PTS; decoders may also present within 40 ms of arrival.
- EN 300 706 clause B.1: keep 20 ms between a header with C4 (erase) and the
  rows of that page. Clause B.2: order is header, X/27, X/28, X/26 (ascending
  designation), then X/1-X/25.

## 2. Teletext in go-astisub

Files: `teletext.go` (721 lines, reader only), `teletext_charsets.go`
(generated from zvbi's `test/unicode-out-ref.txt` by `internal/cmd/teletext`).
API: `ReadFromTeletext(r, TeletextOptions{Page, PID})`. There is no writer.

### 2.1 Coverage

| Feature | Level | go-astisub | shaka-packager (`es_parser_teletext.cc`) |
|---|---|---|---|
| Latin G0 + group 0 sub-sets via C12-C14 (en, de, sv/fi/hu, it, fr, pt/es, cs/sk) | 1 | yes, **verified** for 13 languages | yes |
| Other Latin sub-sets (pl, tr, ro, sr/hr/sl, et, lv/lt) | needs X/28/M/29 | tables present, unreachable | no |
| Cyrillic 1-3, Greek, Arabic, Hebrew G0 | needs X/28/M/29 | tables present, unreachable | no |
| X/26 G2 characters and diacritics | 1.5 | no (`// TODO Implement`, `teletext.go:315`) | yes (Hamming 24/18 decoded, modes 0x0F, 0x10, 0x11-0x1F) |
| X/28/0 Format 1, M/29/0 designation | 1.5 / 2.5 | **panics** (see 2.2) | ignored |
| ESC to second G0 set | 2.5 | no | no |
| Colours, double height/width/size, start/end box | 1 | yes | colours, double height, box, background (0x1D) |
| Row position, alignment | - | no | row number, left/centre/right heuristic |
| C4 erase page | 1 | ignored | ends the cue |
| Writing teletext | - | no | no |
| Tests | - | unit tests of row parsing only, no TS-level tests | TS-level unit tests |

### 2.2 Bugs found

1. **Nil pointer panic on X/28/0 and M/29/0** (**verified**, `experiments/astisub-teletext -designation x28`).
   `teletextCharacterDecoder.setTripletM29` / `setTripletX28` dereference
   `d.tripletM29` / `d.tripletX28` before they are ever set (`teletext.go:471`, `:479`).
   Any M/29/0 or M/29/4 on the subtitle magazine panics. X/28/0 panics when the
   low nibble of the raw first triplet byte is zero.
2. **Triplet not Hamming 24/18 decoded** (`teletext.go:422`, TODO in the code). Even
   with (1) fixed, `(triplet&0x3f80)>>10` reads the raw bytes, so the group is wrong.
3. **Table error**: group 0010, C12-C14 = 110 maps to Czech/Slovak instead of
   Turkish (`teletext_charsets.go:61`, spec table 32). Hidden behind (1).
4. **Charset is decided after the whole file is read**: `updateCharset` runs in
   `page.parse`, after demuxing, so the last X/28/M/29 seen would win for all pages.
5. **Retransmitted pages become duplicate cues** (**verified**): every header for the
   page ends the previous cue; C4 (erase) and C8 (update) are ignored. A page
   re-sent without erase (common on air) gives two identical items. From reading
   the code, add-on rows sent after a header without erase also lose the earlier rows.
6. Not streaming: the whole TS is demuxed before any page is parsed.

### 2.3 Character table notes (zvbi, and so go-astisub)

The tables match EN 300 706 except where the spec's 7x10 bitmaps are ambiguous
or zvbi picked an unusual code point. The generator maps the letters a language
actually uses onto these positions (see `aliases` in [`charset.go`](../charset.go)):

| Sub-set, position | zvbi / go-astisub | Language needs | Note |
|---|---|---|---|
| Polish 0x5B | Ƶ U+01B5 (Z with stroke) | Ż U+017B | |
| Romanian 0x5D / 0x7D | Ǎ ǎ (caron) | Ă ă (breve) | the spec bitmaps draw breve and caron alike (compare Turkish ğ) |
| Romanian 0x5C 0x60 0x7C 0x5E | Ş ţ ş Ţ (cedilla) | Ș ț (comma below) | traditional teletext glyphs |
| Serbian/Croatian 0x5D / 0x7D | Ð ð (eth) | Đ đ (d with stroke) | the spec bitmap at 0x7D is a d with stroke |
| Lettish/Lithuanian 0x5C | ȩ (e cedilla) | ę (e ogonek) | |
| Turkish 0x23 | U+E800 (private use) | ₺ / "TL" | Unicode has no TL ligature |

## 3. EBU STL (Tech 3264) in go-astisub

### 3.1 The format

A 1024-byte **GSI** block: code page number for the GSI text (437, 850, 860,
863, 865), disk format code `STL25.01` / `STL30.01`, display standard code
(blank, `0` open, `1` Level-1 teletext, `2` Level-2 teletext), character code
table `00`-`04` (Latin ISO 6937, Cyrillic 8859-5, Arabic 8859-6, Greek 8859-7,
Hebrew 8859-8), language code, titles, counts, max rows/columns, timecodes.

Then 128-byte **TTI** blocks: subtitle group, subtitle number, extension block
number (0x00-0xEF continuation, 0xFE user data, 0xFF last), cumulative status,
time code in/out, vertical position (teletext row 1-23), justification, comment
flag, 112-byte text field. In teletext mode the text field uses teletext
spacing attributes 0x00-0x1F; 0x8A is a line break, 0x8F is padding;
0x80-0x85 are italics/underline/boxing for open subtitles.

### 3.2 What works

`stl.go` (1179 lines) reads and writes all GSI fields, timecodes (25/30 fps,
start-of-programme offset with an ignore option), vertical position and
justification (mapped to WebVTT/TTML), italics/underline/boxing (open) and
teletext colours/double height (closed, through the teletext row parser).
The CCT 00 decode table matches Tech 3264 Appendix 2 (checked against the
rendered table), except 0xCC (non-spacing underline) which is missing.
Floating diacritics (0xC1-0xCF) are composed with NFC.

### 3.3 Gaps (all **verified** with `experiments/astisub-stl`)

| Gap | Result |
|---|---|
| CCT 01-04 | `table doesn't exist for character code table 12337` (hard error) |
| Writer character mapping | falls back to `byte(rune)`: `$`→`¤`, `€`→`←`, `…`→`&`, `–` (U+2013) → 0x13 (teletext "alpha yellow"), "Привет" → "@825B <8@", Greek → junk |
| Extension blocks | not joined: one subtitle becomes two items, the second at 0s-0s |
| Comment flag CF=1 | translator comments are output as subtitles |
| Cumulative status | ignored |
| Writer text > 112 bytes | silently truncated (no extension blocks written) |
| Teletext DSC without Start Box | the row parser only keeps text after 0x0B, so cues are empty |
| Writer Start Box | one 0x0B per row; EN 300 706 table 26 note: Start Box must be sent twice |
| Language code | only zh, en, fr, ja, no mapped; anything else is written as French `0F`; default country `FRA` |
| GSI code page | ignored: GSI text fields are copied as raw bytes |

### 3.4 STL to teletext

There is no conversion. STL teletext text fields are parsed with the same row
parser as teletext, but STL uses its own character table (ISO 6937 with
floating accents) while teletext uses G0 + national sub-set + X/26. A
converter has to go through Unicode, which is what `teletext.Encoder` does.

## 4. Existing teletext generators

Survey done 2026-11-20; repositories checked by reading their source.

### Go

| Project | License | What it does | Use |
|---|---|---|---|
| [yama6a/subtitle-toolbox-go](https://github.com/yama6a/subtitle-toolbox-go) `container/mpegts/writer_test.go` (266 lines) | MIT | test-only TS writer: PAT/PMT with 0x56, PES 0x24, Hamming 8/4 + 24/18, header bits, X/28/M/29 designation. The repo's reader (`teletext.go`) handles X/28/M/29 with Latin, Cyrillic, Greek; CLI `stbox` | code to compare with; `stbox` as a decoder oracle |
| [boomlinde/teletext](https://github.com/boomlinde/teletext) | none, 2016 | 42-byte rows for raspi-teletext, no PES/TS | ideas only |
| asticode/go-astisub | MIT | decoder only (section 2) | oracle for Level 1 |
| asticode/go-astits | MIT | muxer cannot pad `PES_header_data_length` to 0x24 and pads the last packet with an adaptation field, which EN 300 472 forbids for teletext (read in code, not tested) | not usable as is |

### Other languages

| Project | License | What it does |
|---|---|---|
| [vbit2](https://github.com/peterkvt80/vbit2) (C++, active) | HPND-style | live `.tti` page carousel with header clock, `--format ts --pid`; data unit 0x02 hard-coded, no PAT/PMT |
| [libzvbi](https://github.com/zapping-vbi/zvbi) (C, v0.2.46) | LGPL-2+ / GPL-2+ tools | `src/dvb_mux.c` EN 300 472/301 775 mux, Hamming/parity encoders, `test/decode`, `export`; best **decoder oracle**, also via ffmpeg `--enable-libzvbi` (`-txt_page 888 -txt_format text`) |
| [OpenCaster](https://github.com/aventuri/opencaster) (Python 2, 2014) | GPL-2+ | `EBUTeletext.py`, `txt2pes`, `srt2ebu.py` tutorial |
| [vhs-teletext](https://github.com/ali1234/vhs-teletext) (Python) | GPL-3 | T42 tools, `--ts PID` input, viewers |
| FFmpeg, GStreamer, GPAC, TSDuck, CCExtractor | - | decode or pass through only, no teletext encoder |

### Subtitle Edit ([SubtitleEdit/subtitleedit](https://github.com/SubtitleEdit/subtitleedit), MIT since SE5)

- `src/libse/SubtitleFormats/Ebu.cs` (2437 lines): EBU STL read/write with
  all five character code tables (ISO 6937 and ISO 8859-5/6/7/8).
- `src/libse/ContainerFormats/TransportStream/Teletext.cs` (934 lines, telxcc
  port): reads X/26, X/28, M/29; `TeletextTables.cs` has Latin, Cyrillic,
  Greek, Arabic, Hebrew G0 and G2.
- `ManzanitaTeletextWriter.cs` (959 lines) + `SubtitleFormats/DvbTeletext.cs`:
  writes a Manzanita `.dvbttx` file (XML index + PES payloads), **not a `.ts`**.
  Data unit 0x03, PES header 0x24, boxed double-height rows, X/26 for
  non-English Latin letters, Level 2.5 colour via X/28/0 + X/26. No Cyrillic,
  Greek, Arabic or Hebrew output.

### Gap

No open-source generator found that writes Cyrillic, Greek, Arabic or Hebrew
teletext from text, or that produces a live multi-language subtitle test TS.
This repository's draft covers Latin (all 13 sub-sets + X/26), Cyrillic and Greek.
