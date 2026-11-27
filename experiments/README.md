# Experiments

A separate Go module, so that go-astisub is not a dependency of go-teletext.
It uses go-astisub v0.45.0, whose teletext and STL code is the same as
`master` at `8dc2cdb`. It backs the findings in
[`../docs/research.md`](../docs/research.md).

## astisub-teletext

Builds one page per language with this repository's packages and decodes it
with go-astisub `ReadFromTeletext`. "want" is what a Level 1 decoder shows
(go-astisub ignores X/26).

```
$ go run ./astisub-teletext
OK   eng  Latin English                      got:  Tea for two costs £2½ today / English 12:00:00.000
OK   swe  Latin Swedish/Finnish/Hungarian    got:  Hej på dig, Åsa! Öl är gött. / Svenska 12:00:00.000
OK   fin  Latin Swedish/Finnish/Hungarian    got:  Hyvää päivää, Äiti! Öljyä / Suomi 12:00:00.000
OK   dan  Latin Swedish/Finnish/Hungarian    got:  Rodgrod med flode på Aro / Dansk 12:00:00.000
OK   nor  Latin Swedish/Finnish/Hungarian    got:  Blåbarsyltetoy på Oya / Norsk 12:00:00.000
OK   hun  Latin Swedish/Finnish/Hungarian    got:  Gyor osz, arvizturo / Magyar 12:00:00.000
OK   ger  Latin German                       got:  Grüße aus Köln, Äpfel & Öl / Deutsch 12:00:00.000
OK   fre  Latin French                       got:  Où êtes-vous ? Ca va très bien / Français 12:00:00.000
OK   ita  Latin Italian                      got:  Perché è così? Città più bella / Italiano 12:00:00.000
OK   spa  Latin Portuguese/Spanish           got:  ¿Qué tal? ¡Año nuevo, señor! / Español 12:00:00.000
OK   por  Latin Portuguese/Spanish           got:  Açao e coraçao, pao e limao / Portugues 12:00:00.000
OK   cze  Latin Czech/Slovak                 got:  Příliš žluťoučký kůn / Ceština 12:00:00.000
OK   slo  Latin Czech/Slovak                 got:  Dakujem, lúbim ťa / Slovenčina 12:00:00.000

$ go run ./astisub-teletext -designation x28 -langs swe,ger
FAIL swe  Latin Swedish/Finnish/Hungarian    got:  panic: runtime error: invalid memory address or nil pointer dereference
FAIL ger  Latin German                       got:  panic: runtime error: invalid memory address or nil pointer dereference

$ go run ./astisub-teletext -designation none -langs pol,tur,rus
DIFF pol  Latin Polish                       got:  Za¼#¾↑ g—‖l@ ja÷$ / Polski 12:00:00.000
                                             want: Zażółć gęślą jaźń / Polski 12:00:00.000
DIFF tur  Latin Turkish                      got:  Aůaú, čstanbul, áiáe, gšzel / Tšrkúe 12:00:00.000
                                             want: Ağaç, İstanbul, şişe, güzel / Türkçe 12:00:00.000
DIFF rus  Cyrillic Russian/Bulgarian         got:  Syeâx ve ôtih mqgkih bulok / Russkij 12:00:00.000
                                             want: Съешь же этих мягких булок / Русский 12:00:00.000
```

The last run shows why table 32 designation exists: with only the header
bits, a decoder that assumes group 0 shows the English or Czech sub-set.

## astisub-stl

Writes and reads EBU STL with go-astisub. Output on 2026-11-20:

```
$ go run ./astisub-stl
chars(norwegian)       GSI lang="1E" country="   " DSC="1" CCT="00" -> [1s-2s "Pris: ¤5 eller 4 ← - Åsa på Øya  “hei”&"] [3s-4s "@825B <8@"] [5s-6s "»»·½’”±́"]
lang(swedish)          GSI lang="0F" country="   " DSC="1" CCT="00" -> [1s-2s "Hej"]
long(120 chars)        GSI lang="0F" country="FRA" DSC="1" CCT="00" -> [1s-2s "abcdefghij...ABCDEFGHI"]   (last character cut)
dsc1-no-startbox       1 items: [1s-2s ""]
dsc1-startbox          1 items: [1s-2s "With start box"]
extension-blocks       2 items: [1s-2s "First half of a long subtitle"] [0s-0s "and the second half"]
comment-flag=1         1 items: [1s-2s "Translator note: not for air"]
CCT 01                 read error: ... table doesn't exist for character code table 12337
CCT 02..04             (same)
```

Input was `"Pris: $5 eller 4 €"`, `"Åsa på Øya – “hei”…"`, `"Ж Привет мир"` and `"Ελληνικά"`.
