package teletext

import "strings"

// Language is a test-source preset: a charset plus text that exercises it.
type Language struct {
	Code    string   // ISO 639-2 code used in the teletext descriptor (B form, as common in DVB)
	Aliases []string // other accepted codes (ISO 639-2/T)
	Name    string   // name in the language itself, shown with the timestamp
	Charset Charset
	Sample  string // a sentence that uses the characters the charset is meant for
	Note    string // what the language tests
}

// Languages lists the presets. Samples avoid characters that no teletext set has
// (for example the euro sign, Russian ё and Ukrainian ґ).
var Languages = []Language{
	{
		Code: "eng", Name: "English", Charset: CharsetEnglish,
		Sample: "Tea for two costs £2½ today",
		Note:   "Level 1, English sub-set (£ ½)",
	},
	{
		Code: "swe", Name: "Svenska", Charset: CharsetSwedishFinnishHungarian,
		Sample: "Hej på dig, Åsa! Öl är gött.",
		Note:   "Level 1, Swedish/Finnish sub-set",
	},
	{
		Code: "fin", Name: "Suomi", Charset: CharsetSwedishFinnishHungarian,
		Sample: "Hyvää päivää, Äiti! Öljyä",
		Note:   "Level 1, Swedish/Finnish sub-set",
	},
	{
		Code: "dan", Name: "Dansk", Charset: CharsetSwedishFinnishHungarian,
		Sample: "Rødgrød med fløde på Ærø",
		Note:   "Level 1.5: Æ ø from G2 via X/26",
	},
	{
		Code: "nor", Name: "Norsk", Charset: CharsetSwedishFinnishHungarian,
		Sample: "Blåbærsyltetøy på Øya",
		Note:   "Level 1.5: æ ø Ø from G2 via X/26",
	},
	{
		Code: "hun", Name: "Magyar", Charset: CharsetSwedishFinnishHungarian,
		Sample: "Győr ősz, árvíztűrő",
		Note:   "Level 1.5: ő ű á í via X/26 diacritics",
	},
	{
		Code: "ger", Aliases: []string{"deu"}, Name: "Deutsch", Charset: CharsetGerman,
		Sample: "Grüße aus Köln, Äpfel & Öl",
		Note:   "Level 1, German sub-set",
	},
	{
		Code: "fre", Aliases: []string{"fra"}, Name: "Français", Charset: CharsetFrench,
		Sample: "Où êtes-vous ? Ça va très bien",
		Note:   "Level 1 + X/26 for Ç",
	},
	{
		Code: "ita", Name: "Italiano", Charset: CharsetItalian,
		Sample: "Perché è così? Città più bella",
		Note:   "Level 1, Italian sub-set",
	},
	{
		Code: "spa", Name: "Español", Charset: CharsetPortugueseSpanish,
		Sample: "¿Qué tal? ¡Año nuevo, señor!",
		Note:   "Level 1, Portuguese/Spanish sub-set",
	},
	{
		Code: "por", Name: "Português", Charset: CharsetPortugueseSpanish,
		Sample: "Ação e coração, pão e limão",
		Note:   "Level 1 + X/26 for ã õ ê",
	},
	{
		Code: "cze", Aliases: []string{"ces"}, Name: "Čeština", Charset: CharsetCzechSlovak,
		Sample: "Příliš žluťoučký kůň",
		Note:   "Level 1 + X/26 for Č ň",
	},
	{
		Code: "slo", Aliases: []string{"slk"}, Name: "Slovenčina", Charset: CharsetCzechSlovak,
		Sample: "Ďakujem, ľúbim ťa",
		Note:   "Level 1 + X/26 for Ď ľ",
	},
	{
		Code: "pol", Name: "Polski", Charset: CharsetPolish,
		Sample: "Zażółć gęślą jaźń",
		Note:   "table 32 group 1: needs X/28 or M/29",
	},
	{
		Code: "tur", Name: "Türkçe", Charset: CharsetTurkish,
		Sample: "Ağaç, İstanbul, şişe, güzel",
		Note:   "table 32 group 2: needs X/28 or M/29",
	},
	{
		Code: "hrv", Name: "Hrvatski", Charset: CharsetSerbianCroatianSlovenian,
		Sample: "Đurđevak, čađa i žuć",
		Note:   "table 32 group 3: needs X/28 or M/29",
	},
	{
		Code: "rum", Aliases: []string{"ron"}, Name: "Română", Charset: CharsetRomanian,
		Sample: "Știință și țară, mâine",
		Note:   "table 32 group 3: needs X/28 or M/29",
	},
	{
		Code: "est", Name: "Eesti", Charset: CharsetEstonian,
		Sample: "Õun, šokolaad, žürii",
		Note:   "table 32 group 4: needs X/28 or M/29",
	},
	{
		Code: "lav", Name: "Latviešu", Charset: CharsetLettishLithuanian,
		Sample: "Ģimene, ķēniņš, ļoti",
		Note:   "group 4 + X/26 for most Latvian letters",
	},
	{
		Code: "lit", Name: "Lietuvių", Charset: CharsetLettishLithuanian,
		Sample: "Ąžuolas, ūkis, įvykis",
		Note:   "table 32 group 4: needs X/28 or M/29",
	},
	{
		Code: "rus", Name: "Русский", Charset: CharsetCyrillicRussianBulgarian,
		Sample: "Съешь же этих мягких булок",
		Note:   "Cyrillic G0 option 2 via X/28 or M/29",
	},
	{
		Code: "bul", Name: "Български", Charset: CharsetCyrillicRussianBulgarian,
		Sample: "Щастието е път, не цел",
		Note:   "Cyrillic G0 option 2 via X/28 or M/29",
	},
	{
		Code: "ukr", Name: "Українська", Charset: CharsetCyrillicUkrainian,
		Sample: "Їжак їсть яблуко в Європі",
		Note:   "Cyrillic G0 option 3 via X/28 or M/29",
	},
	{
		Code: "srp", Name: "Српски", Charset: CharsetCyrillicSerbianCroatian,
		Sample: "Ђорђе, Љубав, Њива, Џак",
		Note:   "Cyrillic G0 option 1 via X/28 or M/29",
	},
	{
		Code: "gre", Aliases: []string{"ell"}, Name: "Ελληνικά", Charset: CharsetGreek,
		Sample: "Γειά σου, Κόσμε! Ψυχή",
		Note:   "Greek G0 via X/28 or M/29",
	},
}

// LookupLanguage finds a preset by ISO 639-2 code (B or T form).
func LookupLanguage(code string) (Language, bool) {
	code = strings.ToLower(code)
	for _, l := range Languages {
		if l.Code == code {
			return l, true
		}
		for _, a := range l.Aliases {
			if a == code {
				return l, true
			}
		}
	}
	return Language{}, false
}
