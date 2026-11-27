### Added

- Package `teletext`: Hamming 8/4 and 24/18 coding, EN 300 706 packets (header, rows, X/26,
  X/28/0 Format 1, M/29/0, time filling header) and `ComposePage` for subtitle pages
- `Encoder` maps Unicode text to the Latin, Cyrillic and Greek teletext character sets, with X/26
  accents and G2 characters for letters outside G0; `Languages` has 25 language presets
