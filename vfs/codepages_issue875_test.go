package vfs

import (
	"bytes"
	"testing"
)

// The text attached to issue #875, in the two spellings its samples use: the
// single-byte files were saved with a hyphen because they predate the Unicode
// ones, which use an em dash. Encoding it here reproduces those files byte for
// byte, so the tests below run on exactly what was reported.
const (
	issue875Hyphen = "Go (часто также golang) - компилируемый многопоточный язык программирования, \r\n" +
		"разработанный внутри компании Google]. Использует объектно-ориентированный \r\n" +
		"(структурный) стиль с поддержкой функциональных элементов."
	issue875EmDash = "Go (часто также golang) — компилируемый многопоточный язык программирования, \r\n" +
		"разработанный внутри компании Google]. Использует объектно-ориентированный \r\n" +
		"(структурный) стиль с поддержкой функциональных элементов."
)

// issue875Samples returns the six files from the issue, keyed by their names
// there. The viewer and editor tests build the same set, so the two layers are
// exercised on identical bytes.
func issue875Samples(t *testing.T) map[string][]byte {
	t.Helper()

	encode := func(text string, cp int) []byte {
		raw, err := EncodeBytes([]byte(text), cp)
		if err != nil {
			t.Fatalf("encode sample in codepage %d: %v", cp, err)
		}
		return raw
	}
	// EncodeBytes writes the byte-order mark for UTF-16, which is what
	// utf16_le_bom.txt is; utf16_le.txt is the same file without it.
	utf16BOM := encode(issue875EmDash, 1200)

	return map[string][]byte{
		"ansi_1251.txt":    encode(issue875Hyphen, 1251),
		"oem_866.txt":      encode(issue875Hyphen, 866),
		"utf16_le_bom.txt": utf16BOM,
		"utf16_le.txt":     utf16BOM[2:],
		"utf8_bom.txt":     append([]byte("\ufeff"), issue875EmDash...),
		"utf8.txt":         []byte(issue875EmDash),
	}
}

// Every sample from the issue has to come back as the codepage it was written
// in. Detection used to answer the system ANSI codepage for the CP1251 file
// and UTF-8 for the byte-order-mark-less UTF-16 one.
func TestCodepages_Issue875_DetectsEverySample(t *testing.T) {
	want := map[string]int{
		"ansi_1251.txt":    1251,
		"oem_866.txt":      866,
		"utf16_le_bom.txt": 1200,
		"utf16_le.txt":     1200,
		"utf8_bom.txt":     65001,
		"utf8.txt":         65001,
	}
	text := map[string]string{
		"ansi_1251.txt":    issue875Hyphen,
		"oem_866.txt":      issue875Hyphen,
		"utf16_le_bom.txt": issue875EmDash,
		"utf16_le.txt":     issue875EmDash,
		"utf8_bom.txt":     issue875EmDash,
		"utf8.txt":         issue875EmDash,
	}

	for name, raw := range issue875Samples(t) {
		got := DetectEncoding(raw, true, 65001)
		if got != want[name] {
			t.Errorf("DetectEncoding(%s) = %d, want %d", name, got, want[name])
		}
		decoded, err := DecodeBytes(StripUTF8BOM(raw), got)
		if err != nil {
			t.Errorf("decode %s as %d: %v", name, got, err)
			continue
		}
		// UTF-16 keeps its marker as U+FEFF once decoded; it is the text
		// after it that has to match.
		if string(bytes.TrimPrefix(decoded, []byte("\ufeff"))) != text[name] {
			t.Errorf("%s decoded as %d does not round-trip", name, got)
		}
	}
}

// Cyrillic text is readable under several codepages at once, and the tie used
// to go to the system ANSI codepage: whatlanggo scores CP1251 Russian read as
// Windows-1252 a confident "German", which outranked the correct decoding
// because the two were measured on different scales.
func TestCodepages_DetectsCyrillicOverSystemANSI(t *testing.T) {
	const text = "Это обычный русский текст в кодировке кириллицы для проверки автоопределения.\r\n" +
		"Вторая строка содержит числа 123 и знаки препинания, а также немного латиницы.\r\n"

	for _, want := range []int{1251, 866, 20866} {
		raw, err := EncodeBytes([]byte(text), want)
		if err != nil {
			t.Fatalf("encode codepage %d: %v", want, err)
		}
		got := DetectEncoding(raw, true, 65001)
		decoded, err := DecodeBytes(raw, got)
		if err != nil || string(decoded) != text {
			t.Errorf("DetectEncoding of CP%d text = %d, which decodes as %q", want, got, string(decoded))
		}
	}
}

// The Western European cases the whole-fragment language check used to carry.
// They now rest on text quality alone: a word mixing scripts, or holding a
// capital in its middle, is what a wrong codepage produces.
func TestCodepages_DetectsWesternText(t *testing.T) {
	for _, text := range []string{
		"“Café et déjà vu” 123\n",
		"L'élève a réussi son contrôle de mathématiques après avoir étudié très tard.\r\n" +
			"Cet été, nous irons à la mer où la température dépasse trente degrés.\r\n",
		"Größe und Qualität der Fußgängerzone müssen überprüft werden.\r\n" +
			"Schöne Grüße aus München, wo die Häuser älter als dreihundert Jahre sind.\r\n",
	} {
		raw, err := EncodeBytes([]byte(text), 1252)
		if err != nil {
			t.Fatalf("encode %q: %v", text, err)
		}
		got := DetectEncoding(raw, true, 65001)
		decoded, err := DecodeBytes(raw, got)
		if err != nil || string(decoded) != text {
			t.Errorf("DetectEncoding of CP1252 %q = %d, which decodes as %q", text, got, string(decoded))
		}
	}
}

func TestCodepages_DetectUTF16WithoutBOM(t *testing.T) {
	const russian = "Это обычный русский текст в кодировке кириллицы для проверки.\r\n"
	const latin = "The quick brown fox jumps over the lazy dog, again and again.\r\n"

	for _, tc := range []struct {
		name string
		text string
		cp   int
	}{
		{"little endian Cyrillic", russian, 1200},
		{"big endian Cyrillic", russian, 1201},
		{"little endian Latin", latin, 1200},
		{"big endian Latin", latin, 1201},
	} {
		withBOM, err := EncodeBytes([]byte(tc.text), tc.cp)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		raw := withBOM[2:]
		if cp, ok := detectUTF16WithoutBOM(raw); !ok || cp != tc.cp {
			t.Errorf("%s: detectUTF16WithoutBOM = %d, %v; want %d, true", tc.name, cp, ok, tc.cp)
		}
		// UTF-16 Cyrillic is made of bytes below 0x80 and so passes
		// utf8.Valid: the check has to run before the UTF-8 one, or the
		// file is shown as UTF-8 control characters.
		if cp := DetectEncoding(raw, true, 65001); cp != tc.cp {
			t.Errorf("%s: DetectEncoding = %d, want %d", tc.name, cp, tc.cp)
		}
		if cp := DetectEncoding(raw, false, 65001); cp != 65001 {
			t.Errorf("%s: guessed %d with auto-detect off", tc.name, cp)
		}
	}
}

// Nothing that is not UTF-16 may be taken for it. Ordinary text has no NUL
// bytes at all, and a binary scatters them across both halves of a pair or
// pads with whole NUL characters.
func TestCodepages_DetectUTF16WithoutBOM_Negatives(t *testing.T) {
	cp1251, err := EncodeBytes([]byte("Это обычный русский текст в кодировке кириллицы.\r\n"), 1251)
	if err != nil {
		t.Fatal(err)
	}
	padded := make([]byte, 64)
	copy(padded, "report")
	elfLike := []byte{
		0x7F, 'E', 'L', 'F', 2, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		2, 0, 0x3E, 0, 1, 0, 0, 0, 0x40, 0x10, 0, 0, 0, 0, 0, 0,
		0x40, 0, 0, 0, 0, 0, 0, 0, 0x88, 0x21, 0, 0, 0, 0, 0, 0,
	}

	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"ascii", []byte("The quick brown fox jumps over the lazy dog.\r\n")},
		{"utf8 Cyrillic", []byte("Это обычный русский текст для проверки кодировки.\r\n")},
		{"cp1251", cp1251},
		{"NUL padded record", padded},
		{"ELF header", elfLike},
		{"too short", []byte("h\x00i\x00")},
	} {
		if cp, ok := detectUTF16WithoutBOM(tc.data); ok {
			t.Errorf("%s: mistaken for UTF-16 (%d)", tc.name, cp)
		}
	}
}

// A word whose letters come from two scripts, or one holding a capital after a
// lowercase letter, is what reading text with the wrong codepage produces.
// Neither costs a correctly decoded file anything.
func TestCodepages_LegacyTextScorePenalties(t *testing.T) {
	for _, tc := range []struct {
		name       string
		clean      string
		mojibake   string
		mustBeatBy int
	}{
		{"mixed scripts", "“Café et déjà vu”", "УCafщ et dщjр vuФ", 1},
		{"interior capital", "“Café et déjà vu”", "ôCafÚ et dÚjÓ vuö", 1},
	} {
		clean := legacyTextScore([]byte(tc.clean))
		mojibake := legacyTextScore([]byte(tc.mojibake))
		if clean-mojibake < tc.mustBeatBy {
			t.Errorf("%s: score %d for %q does not beat %d for %q",
				tc.name, clean, tc.clean, mojibake, tc.mojibake)
		}
	}

	// Words in different scripts are ordinary in one document; it is mixing
	// them inside a single word that is not.
	mixed := legacyTextScore([]byte("Это слово Latin и снова русский"))
	plain := legacyTextScore([]byte("Это слово просто и снова русский"))
	if mixed >= plain {
		t.Errorf("separate-script words were penalised: %d vs %d", mixed, plain)
	}
}
