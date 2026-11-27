# Test stream with all languages

`scripts/all-langs-stream.sh` makes a transport stream for checking a player or
decoder by eye: H.264 video, AAC audio and a teletext subtitle page for every
language preset of `ttx-gen`. This page lists what each subtitle page shows.

```sh
scripts/all-langs-stream.sh                         # ttx-all-langs.ts, 60 s
scripts/all-langs-stream.sh m29.ts -designation m29 # options after the name go to ttx-gen
DURATION=10 scripts/all-langs-stream.sh short.ts
```

The script needs go, and ffmpeg with an H.264 encoder.

## What the stream contains

- A 1280x720 test pattern at 25 frames per second with a running clock in the
  picture, and a quiet tone.
- One teletext stream with 25 subtitle pages, one per language. The pages use
  the default designation (`-designation auto`) and double height rows unless
  other options are given.
- A new cue on every page every 2 s, shown for 1.6 s. A cue has two lines: the
  sample sentence of the language, and the name of the language with a time
  and a cue number, for example `Polski 00:00:02.080 (2)`.

## Pages and cue texts

`TestAllLangsTable` in `cmd/ttx-gen` checks this table against the presets.

| Page | Code | First line | Second line starts with | What it needs from the decoder |
|---|---|---|---|---|
| 888 | eng | Tea for two costs £2½ today | English | Level 1, English sub-set (£ ½) |
| 788 | swe | Hej på dig, Åsa! Öl är gött. | Svenska | Level 1, Swedish/Finnish sub-set |
| 688 | fin | Hyvää päivää, Äiti! Öljyä | Suomi | Level 1, Swedish/Finnish sub-set |
| 588 | dan | Rødgrød med fløde på Ærø | Dansk | Level 1.5: Æ ø from G2 via X/26 |
| 488 | nor | Blåbærsyltetøy på Øya | Norsk | Level 1.5: æ ø Ø from G2 via X/26 |
| 388 | hun | Győr ősz, árvíztűrő | Magyar | Level 1.5: ő ű á í via X/26 diacritics |
| 288 | ger | Grüße aus Köln, Äpfel & Öl | Deutsch | Level 1, German sub-set |
| 188 | fre | Où êtes-vous ? Ça va très bien | Français | Level 1 + X/26 for Ç |
| 889 | ita | Perché è così? Città più bella | Italiano | Level 1, Italian sub-set |
| 789 | spa | ¿Qué tal? ¡Año nuevo, señor! | Español | Level 1, Portuguese/Spanish sub-set |
| 689 | por | Ação e coração, pão e limão | Português | Level 1 + X/26 for ã õ ê |
| 589 | cze | Příliš žluťoučký kůň | Čeština | Level 1 + X/26 for Č ň |
| 489 | slo | Ďakujem, ľúbim ťa | Slovenčina | Level 1 + X/26 for Ď ľ |
| 389 | pol | Zażółć gęślą jaźń | Polski | table 32 group 1: needs X/28 or M/29 |
| 289 | tur | Ağaç, İstanbul, şişe, güzel | Türkçe | table 32 group 2: needs X/28 or M/29 |
| 189 | hrv | Đurđevak, čađa i žuć | Hrvatski | table 32 group 3: needs X/28 or M/29 |
| 890 | rum | Știință și țară, mâine | Română | table 32 group 3: needs X/28 or M/29 |
| 790 | est | Õun, šokolaad, žürii | Eesti | table 32 group 4: needs X/28 or M/29 |
| 690 | lav | Ģimene, ķēniņš, ļoti | Latviešu | group 4 + X/26 for most Latvian letters |
| 590 | lit | Ąžuolas, ūkis, įvykis | Lietuvių | table 32 group 4: needs X/28 or M/29 |
| 490 | rus | Съешь же этих мягких булок | Русский | Cyrillic G0 option 2 via X/28 or M/29 |
| 390 | bul | Щастието е път, не цел | Български | Cyrillic G0 option 2 via X/28 or M/29 |
| 290 | ukr | Їжак їсть яблуко в Європі | Українська | Cyrillic G0 option 3 via X/28 or M/29 |
| 190 | srp | Ђорђе, Љубав, Њива, Џак | Српски | Cyrillic G0 option 1 via X/28 or M/29 |
| 891 | gre | Γειά σου, Κόσμε! Ψυχή | Ελληνικά | Greek G0 via X/28 or M/29 |

What a decoder shows when it lacks a feature:

- **National sub-set** (the rows with "Level 1"): the plain letters are right
  and the national ones are replaced by those of another language or by
  brackets and other symbols.
- **X/26** (Level 1.5): the letter is shown without its accent, or as a similar
  plain letter: `Rodgrod` for `Rødgrød`, `Gyor` for `Győr`.
- **X/28 or M/29**: the page is decoded with the wrong character set. Latin
  pages get the national letters of another language, and Cyrillic and Greek
  pages show Latin letters.

Some teletext glyphs have no single agreed Unicode character. A decoder that
draws these differently is not wrong; libzvbi maps them like this:

- Romanian: `ş ţ` with a cedilla for `ș ț`, and `ǎ` with a caron for `ă`.
- Croatian: `Ð ð` for `Đ đ`.

## Timing

The stream is made with `-clock pts`: the time in a cue is the PTS of the PES
with its page header. The rows with the text come in the next PES, 40 ms
later, and the PTS of a video frame is about 21 ms after the time on its
clock. A cue therefore appears on the frame whose clock shows 40 ms more than
the time in the cue: the cue `Magyar 00:00:48.080 (25)` on the frame with
clock 00:00:48.120.

Pages that share a magazine are sent 80 ms apart, so their cues carry
different times. The rule above holds for each of them.

## VLC

Seen in VLC 3.0.24 on macOS:

- The subtitle track menu lists one track per language, 25 in all.
- Stepping frame by frame (the `E` key) does not update the subtitles: the cue
  that was on screen stays. The time a cue appears can therefore not be read
  off by stepping.

The result of a check in VLC is in the validation table of
[design.md](design.md#validation).
