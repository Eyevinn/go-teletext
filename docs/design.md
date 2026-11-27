# Design of the teletext test generator

Goal: a live DVB teletext subtitle source for testing packagers and players,
with one page per language, timestamps in the text, and the character set
features that decoders handle differently.

## Layout

| Package | Role |
|---|---|
| `teletext` (repository root) | EN 300 706: Hamming 8/4 and 24/18, parity, packets (header, rows, X/26, X/28/M/29), character sets, Unicode encoder, page composer, language presets |
| `mpegts` | EN 300 472 data units and PES, minimal TS muxer (PAT, PMT with teletext descriptor, PCR) |
| `cmd/ttx-gen` | CLI: file or live (stdout/UDP) output, cue schedule |
| `experiments` | separate module: go-astisub as decoder oracle and STL reproducers |
| `internal` | build information for `-version` |

## Encoding a character

`teletext.Encoder.Encode` tries, in order:

1. the charset's G0 table with its national sub-set applied;
2. an alias, when zvbi uses another code point for the same glyph (Ż→Ƶ, Ă→Ǎ, Ș→Ş, Đ→Ð, ę→ȩ, ₺);
3. Latin only: a letter A-Z/a-z plus one diacritical mark → X/26 mode 0x11-0x1F,
   with the bare letter in the Level 1 row (EN 300 706 E.3, E.10);
4. Latin only: a G2 character → X/26 mode 0x0F, with a look-alike letter in the row (Æ→A, ø→o, ...);
5. otherwise `?`, and the rune is reported as missing.

With `-level15=false` steps 3-4 still give the fallback letter but no X/26.

## Page transmission

Per cue and page (clauses B.1, B.2, A.4):

1. PES at time *t*: optional M/29/0, then the header X/0 with C4 erase, C6 subtitle,
   C7 suppress header, C12-C14 from table 32, and a clock in the last 8 header bytes (E.14).
2. PES at *t*+40 ms: X/28/0 Format 1 (if the charset needs it), X/26/0.. in
   designation order, then the rows. Rows are double height (0x0D), colour,
   Start Box twice, text, End Box twice, centred; rows 20 and 22 for two lines.
3. At *t*+cue: a header with C4 erase and no rows clears the page.

Pages in different magazines are sent in the same PES; pages that share a
magazine are sent one after another, 80 ms apart, because in parallel mode a
header ends the previous page of its magazine. The default page numbers
(888, 788, ..., 188, 889, ...) give each language its own magazine.

## Timing

- PCR and PTS count from 1970-01-01T00:00:00Z and wrap with their 33 bits, every
  2^33 / 90000 s = 95443.718 s (26.5 hours): PTS = (90000 × Unix time) mod 2^33
  (`mpegts.EpochPTS`). The clock is continuous over midnight, and its only steps
  are the 33-bit wraps that receivers handle.
- The time printed in a cue, `Svenska 12:00:02.080 (2)`, is the time of the PTS
  of the header PES. A receiver gets it back as k × 2^33 / 90000 s + PTS / 90000 s
  after the epoch, where k is the number of wraps so far (18812 from 2026-11-24).
  Any clock within 13 hours of the right time gives k (`mpegts.TimeOfPTS`).
- PTS = transmission time + `-pts-offset` (default 40 ms, the EN 300 472 decoder retention).
- A 40 ms tick drives everything: PCR every tick (adaptation-field-only packets
  on their own PID), PAT/PMT every 80 ms, cue events on tick boundaries.

## Heartbeats and periods without subtitles

A live segmenter must release video, audio and subtitle segments together.
Subtitles are sparse: without teletext PES during a long gap, the segmenter
cannot tell whether a subtitle is still coming for the current segment. So
the stream should carry teletext PES with fresh PTS also when nothing changes.
shaka-packager, for example, turns every teletext PES without displayable rows
into a `TextHeartBeat` sample that drives its `TextChunker`.

`ttx-gen` sends a heartbeat PES on every tick with no other teletext PES, by
default every 40 ms, so the stream carries one teletext PES per frame like
teletext from a VBI source. `-heartbeat 1s` makes them sparser and
`-heartbeat 0` turns them off, to test segmenters without heartbeats.

`-heartbeat-kind` selects the content:

- `filler` (default): a page header for page mFF in each magazine in use.
  EN 300 706 clause A.1 reserves page FF for time filling and as a terminator:
  it carries no data, does not change the display, and in parallel mode ends
  the transmission of the previous page of the magazine.
- `stuffing`: only stuffing data units (id 0xFF).

`-cues-on 60s -cues-off 30s` sends cues for 60 s and then none for 30 s,
repeating, while the heartbeats go on. Whole periods are skipped, so a cue is
never left on screen during a pause.

## Inserting into a transport stream (`ttx-gen -i`)

`ttx-gen -i in.ts` (or `-i -` for stdin, `-i udp://host:port` to listen,
multicast included) copies a transport stream and adds the teletext to one
program (`-program`, default the first in the PAT). The work is done by
`mpegts.Inserter`:

- **PMT**: the teletext stream (type 0x06, teletext descriptor) is appended to
  the program's PMT, which keeps its version. The PID is `-pid`, or by default
  the one after the highest PID of the program (0x102 for ffmpeg's 0x100 video
  and 0x101 audio). The inserter stops if the input uses that PID.
- **Clock**: ticks follow the input's PCR, not the wall clock, so the teletext
  stays in sync with video and audio. Between PCRs the clock is estimated from
  the packet count and the bitrate of the previous PCR interval. With variable
  bitrate this estimate overshoots, so a lower PCR holds the clock instead of
  moving it back. A discontinuity_indicator or a step of more than a second is
  a jump: the cue cycle restarts (looping file inputs). Ticks are on a 40 ms grid
  of the PCR (multiples of 3600 in 90 kHz, like 25 fps video from ffmpeg).
- **PTS** = tick + `-pts-offset`, in the input's time base.
- **Time stamp in cues**: PCR and PTS have no absolute meaning, so
  `-clock wall` (default) maps them to wall-clock time: the wall-clock time when
  the first PCR was read (or `-start`, for the first tick) plus the PTS distance
  from it. This shows when the inserter scheduled the cue, which suits latency
  measurements. `-clock pts` prints the PTS itself.
- **Bitrate**, three cases:
  1. The input has null packets (constant bitrate): teletext packets take the
     place of null packets, so every other packet keeps its position and the
     PCR timing is untouched. A packet waits for a null packet until 5 ms
     before its PTS; then it is inserted and a later null packet is dropped.
     ffmpeg's `-muxrate` output has stretches of 150 ms without null packets
     during I-frames, so use `-pts-offset 200ms` with such inputs.
  2. The input has no null packets (none in the last second): the teletext
     packets are added, about 38 kbit/s for one PES per 40 ms. PCR values are
     kept, which suits software pipelines that time by PCR.
  3. `-muxrate R`: constant bitrate output at R bit/s. Every packet goes in the
     first output slot at or after its input time, empty slots get new null
     packets (input null packets are dropped), and the PCR of the program is
     restamped with the time of its slot. A PCR on another PID, as of another
     program in a multi-program stream, keeps its time base and is moved by the
     delay of its packet. PTS/DTS are not changed; packets end up at most a
     few slots later than in the input (reported as "packets up to x ms after
     their input time"). R must exceed the peak input rate; the inserter stops
     if the output falls more than 0.5 s behind.
- `-live` with a file input paces the output by the input's PCR, to stream a
  file in real time, for example to `-udp`.

Checked with ffmpeg-made H.264 + AAC streams, with and without `-muxrate`:
ffprobe lists the three streams with the teletext languages, an ffmpeg remux
reports no errors, there are no continuity counter errors, the PCR of constant
bitrate outputs deviates 0.000 ms from a straight line, and go-astisub decodes
the cues.

### Future: SMPTE 12M time codes

PCR and PTS are relative: their origin is arbitrary and they wrap after 26.5
hours. A SMPTE 12M time code can be absolute when the encoder takes it from
the time of day: H.264 pic_timing SEI (clock_timestamp), HEVC time code SEI or
the MPEG-2 GOP header time_code. A `-clock smpte` mode could print, in each
cue, the time code of the video frame with the same PTS. That would check
subtitle-to-video sync to the frame, independent of when the inserter ran.
Time codes carry no date, and 29.97 Hz streams use drop-frame counting.

## Validation

| Check | Status |
|---|---|
| Hamming 8/4 against the published table, single error correction, double error detection | unit test |
| Hamming 24/18: all 2^18 values against the telxcc decoder used by shaka-packager | unit test |
| Every preset encodes with no missing characters and decodes back (own Level 1.5 decoder) | unit test |
| X/26 triplet order, termination marker fill, X/28 designation bits | unit test |
| PES sizes N*184, header 0x24, no adaptation field on the teletext PID, CC, CRC | unit test |
| go-astisub decodes 13 group 0 languages exactly as the Level 1 view | `experiments/astisub-teletext` |
| ffprobe sees `dvb_teletext` with all languages and the expected start PTS | manual |

Still to do with independent decoders:

- libzvbi (zvbi `test/decode --ttx`, or ffmpeg built with `--enable-libzvbi`):
  X/26, X/28, Cyrillic, Greek.
- shaka-packager (X/26, heartbeat), Subtitle Edit (telxcc port with X/28),
  `stbox` from subtitle-toolbox-go, VLC, a real set-top box or TV.
- TSDuck `tsp -P analyze` / `tsanalyze` for PSI and timing.

## Limitations of the draft

- X/28/0 Format 1 and M/29/0 leave the CLUT 2/3 colour map, default screen and
  row colours at zero. Level 1.5 decoders ignore them; a Level 2.5 decoder may
  set CLUT 2 and 3 to black. Fill in the defaults of table 30 before relying on Level 2.5.
- No Arabic or Hebrew (right-to-left order, Arabic shaping), no second G0 set (ESC).
- No line wrapping: a line longer than 33 characters (double height) is an error.
- Without `-i` the stream has teletext only; use `-i` to add it to a stream with video.
- Line offset is 0 (undefined) and field parity is always 1; VBI re-insertion
  would need real line numbers, at most 16 lines per field.
- PSI fits one TS packet per section, which limits the descriptor to about 30 pages,
  and the inserter does not handle input PMT sections that span TS packets.

## Ideas

- Heartbeats with other content: headers of a non-subtitle page, or the
  subtitle page header again without C4, which some decoders treat as a new cue.
- Test patterns per decoder issue: page retransmission without erase,
  add-on rows, serial mode (C11), M/29 vs X/28 precedence, X/26/15,
  hearing impaired type 0x05, pages with subcodes.
- A "Level 1 only" variant per language next to the Level 1.5 page, to compare
  fallback and enhanced rendering side by side.
- STL input: read STL with a fixed reader (all CCTs, extension blocks) and
  play it out as teletext, giving the STL→teletext path go-astisub lacks.
