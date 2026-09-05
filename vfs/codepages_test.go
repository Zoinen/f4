package vfs

import (
	"github.com/unxed/localecp"

	"context"
	"io"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCodepages_Basic(t *testing.T) {
	if _, ok := FindCodepage(65001); !ok {
		t.Error("Expected UTF-8 to be found")
	}

	cp, ok := DetectBOM([]byte{0xEF, 0xBB, 0xBF, 'a'})
	if !ok || cp != 65001 {
		t.Errorf("BOM detection failed: got %d, %v", cp, ok)
	}

	testStr := "Привет"
	encoded, err := EncodeBytes([]byte(testStr), 1251)
	if err != nil {
		t.Fatalf("Failed to encode CP1251: %v", err)
	}
	decoded, err := DecodeBytes(encoded, 1251)
	if err != nil {
		t.Fatalf("Failed to decode CP1251: %v", err)
	}
	if string(decoded) != testStr {
		t.Errorf("Roundtrip failed: expected %q, got %q", testStr, string(decoded))
	}
}

func TestCodepages_StripUTF8BOM(t *testing.T) {
	withBOM := []byte{0xEF, 0xBB, 0xBF, 't', 'e', 'x', 't'}
	if !HasUTF8BOM(withBOM) {
		t.Fatal("UTF-8 BOM was not detected")
	}
	if got := string(StripUTF8BOM(withBOM)); got != "text" {
		t.Fatalf("stripped text = %q, want %q", got, "text")
	}

	withoutBOM := []byte("text")
	if HasUTF8BOM(withoutBOM) {
		t.Fatal("plain UTF-8 was reported as BOM-marked")
	}
	stripped := StripUTF8BOM(withoutBOM)
	if &stripped[0] != &withoutBOM[0] {
		t.Fatal("stripping a missing BOM allocated or copied the input")
	}
}

func TestCodepages_GetSystemEncoding(t *testing.T) {
	oem := GetSystemOEMEncoding()
	ansi := GetSystemANSIEncoding()
	if SystemANSICodepage() <= 0 || SystemOEMCodepage() <= 0 {
		t.Fatalf("system codepage IDs must be real positive codepages: ANSI=%d OEM=%d", SystemANSICodepage(), SystemOEMCodepage())
	}
	if NormalizeCodepageID(legacySystemANSI) != SystemANSICodepage() || NormalizeCodepageID(legacySystemOEM) != SystemOEMCodepage() {
		t.Error("legacy system codepage IDs were not migrated")
	}

	if oem != localecp.OEMEncoding {
		t.Errorf("expected OEM encoding %v, got %v", localecp.OEMEncoding, oem)
	}
	if ansi != localecp.ANSIEncoding {
		t.Errorf("expected ANSI encoding %v, got %v", localecp.ANSIEncoding, ansi)
	}
	if cp, ok := FindCodepage(SystemANSICodepage()); !ok || cp.Enc != localecp.ANSIEncoding {
		t.Error("system ANSI menu entry does not use localecp.ANSIEncoding")
	}
	if cp, ok := FindCodepage(SystemOEMCodepage()); !ok || cp.Enc != localecp.OEMEncoding {
		t.Error("system OEM menu entry does not use localecp.OEMEncoding")
	}
}

func TestCodepages_IconvCodepages(t *testing.T) {
	var iconvCP Codepage
	for _, cp := range AvailableCodepages {
		if cp.group == codepageIconv {
			iconvCP = cp
			break
		}
	}
	if iconvCP.Enc == nil {
		t.Skip("iconv is not available")
	}

	const sample = "iconv codepage probe"
	encoded, err := EncodeBytes([]byte(sample), iconvCP.ID)
	if err != nil {
		t.Fatalf("encode through %s: %v", iconvCP.Name, err)
	}
	decoded, err := DecodeBytes(encoded, iconvCP.ID)
	if err != nil {
		t.Fatalf("decode through %s: %v", iconvCP.Name, err)
	}
	if string(decoded) != sample {
		t.Errorf("iconv roundtrip = %q, want %q", decoded, sample)
	}
}

func TestCodepages_DetectEncoding(t *testing.T) {
	data := []byte("hello")
	if cp := DetectEncoding(data, true, 1251); cp != 65001 {
		t.Errorf("Expected UTF-8 detection, got %d", cp)
	}

	invalidData := []byte{0xC0, 0xAF}
	if cp := DetectEncoding(invalidData, true, 1251); cp != 1251 {
		t.Errorf("Expected fallback to 1251, got %d", cp)
	}

	encodeSample := func(cp int, texts ...string) []byte {
		for _, text := range texts {
			encoded, err := EncodeBytes([]byte(text), cp)
			if err == nil && !utf8.Valid(encoded) {
				return encoded
			}
		}
		return nil
	}
	ansi := encodeSample(SystemANSICodepage(),
		"Привет, тестовый текст 123\n",
		"“Café et déjà vu” 123\n",
	)
	if ansi != nil {
		if cp := DetectEncoding(ansi, true, 65001); cp != SystemANSICodepage() {
			t.Errorf("Expected ANSI detection, got %d", cp)
		}
	} else {
		t.Log("system ANSI codepage has no non-UTF-8 sample")
	}
	oem := encodeSample(SystemOEMCodepage(),
		"Привет, тестовый текст 123\n",
		"Café déjà été à côté de Noël 123\n",
	)
	if oem != nil {
		if cp := DetectEncoding(oem, true, 65001); cp != SystemOEMCodepage() {
			t.Errorf("Expected OEM detection, got %d", cp)
		}
	} else {
		t.Log("system OEM codepage has no non-UTF-8 sample")
	}

	if cp := DetectEncoding(data, false, 1251); cp != 1251 {
		t.Errorf("Expected fallback when autodetect is false, got %d", cp)
	}
}

func TestCodepages_DetectEncoding_ExplicitLegacyCodepages(t *testing.T) {
	// The host locale is intentionally irrelevant here. These are the
	// encodings that used to be missed when Linux reported unrelated ANSI/OEM
	// codepages (for example, C.UTF-8 -> Windows-1252/CP437).
	for _, text := range []string{
		"Привет, это достаточно длинный русский текст для определения кодировки.\n" +
			"Вторая строка содержит числа 123 и знаки препинания.\n",
		"Привет, это достаточно длинный русский текст для проверки кодировки.\n" +
			"Вторая строка содержит числа 123 и знаки препинания.\n",
	} {
		for _, want := range []int{1251, 866, 20866} {
			raw, err := EncodeBytes([]byte(text), want)
			if err != nil {
				t.Fatalf("encode codepage %d: %v", want, err)
			}
			if utf8.Valid(raw) {
				t.Fatalf("codepage %d sample unexpectedly remained valid UTF-8", want)
			}
			got := DetectEncoding(raw, true, 65001)
			decoded, err := DecodeBytes(raw, got)
			if err != nil || string(decoded) != text {
				t.Errorf("DetectEncoding(%d) = %d for %q, which decodes as %q; want the original text", want, got, text, string(decoded))
			}
		}
	}
}

func TestCodepages_GetCodepageDecoderEncoder(t *testing.T) {
	dec, enc := GetCodepageDecoderEncoder("65001")
	if dec != nil || enc != nil {
		t.Error("Expected nil dec/enc for UTF-8")
	}

	decEmpty, encEmpty := GetCodepageDecoderEncoder("")
	if decEmpty != nil || encEmpty != nil {
		t.Error("Expected nil dec/enc for empty codepage")
	}

	decAnsi, encAnsi := GetCodepageDecoderEncoder(strconv.Itoa(SystemANSICodepage()))
	if decAnsi == nil || encAnsi == nil {
		t.Error("Expected valid dec/enc for System ANSI")
	}

	decOem, encOem := GetCodepageDecoderEncoder(strconv.Itoa(SystemOEMCodepage()))
	if decOem == nil || encOem == nil {
		t.Error("Expected valid dec/enc for System OEM")
	}

	dec1251, enc1251 := GetCodepageDecoderEncoder("1251")
	if dec1251 == nil || enc1251 == nil {
		t.Error("Expected valid dec/enc for CP1251")
	}
}

func TestCodepages_GetNextFastSwitchCodepage(t *testing.T) {
	tests := []struct {
		current int
		want    int
	}{
		{65001, SystemANSICodepage()},
		{SystemANSICodepage(), SystemOEMCodepage()},
		{SystemOEMCodepage(), 65001},
		{99999, 65001},
	}
	for _, tt := range tests {
		if got := GetNextFastSwitchCodepage(tt.current); got != tt.want {
			t.Errorf("GetNextFastSwitchCodepage(%d) = %d, want %d", tt.current, got, tt.want)
		}
	}
}

func TestCodepages_MemoryReadAtCloser(t *testing.T) {
	m := &MemoryReadAtCloser{Data: []byte("payload")}

	buf := make([]byte, 7)
	n, err := m.ReadAt(context.Background(), buf, 0)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if n != 7 || string(buf) != "payload" {
		t.Errorf("Expected 'payload', got %q", string(buf))
	}

	n2, err2 := m.ReadAt(context.Background(), buf, 10)
	if n2 != 0 || err2 != io.EOF {
		t.Errorf("Expected EOF at index 10, got n=%d, err=%v", n2, err2)
	}

	n3, err3 := m.Read(context.Background(), buf)
	if n3 != 0 || err3 != io.EOF {
		t.Errorf("Expected standard Read to return EOF, got n=%d, err=%v", n3, err3)
	}
}
func TestCodepages_DisplayCodepageName(t *testing.T) {
	tests := []struct {
		id   int
		want string
	}{
		{65001, "UTF-8"},
		{1200, "UTF-16 (Little endian)"},
		{20866, "KOI8-R (Cyrillic)"},
		{99999, "99999"},
	}
	for _, tt := range tests {
		if got := DisplayCodepageName(tt.id); got != tt.want {
			t.Errorf("DisplayCodepageName(%d) = %q, want %q", tt.id, got, tt.want)
		}
	}
	if got := DisplayCodepageName(SystemANSICodepage()); got != "ANSI" {
		t.Errorf("DisplayCodepageName(System ANSI) = %q, want ANSI", got)
	}
	if got := DisplayCodepageName(SystemOEMCodepage()); got != "OEM" {
		t.Errorf("DisplayCodepageName(System OEM) = %q, want OEM", got)
	}
	if got := DisplayCodepageName(legacySystemANSI); got != "ANSI" {
		t.Errorf("DisplayCodepageName(legacy ANSI) = %q, want ANSI", got)
	}
}

func TestCodepages_BuildCodepageMenuItems(t *testing.T) {
	items, currIdx := BuildCodepageMenuItems(SystemANSICodepage(), false)
	if len(items) == 0 {
		t.Fatal("BuildCodepageMenuItems returned empty items")
	}

	// First item should be Auto-detect
	if !strings.Contains(items[0].Text, "Auto-detect") {
		t.Errorf("Expected first item to be Auto-detect, got %q", items[0].Text)
	}

	// Should contain group headers
	hasSystem, hasUnicode, hasOther := false, false, false
	for _, item := range items {
		if item.Separator {
			if strings.Contains(item.Text, "System") {
				hasSystem = true
			}
			if strings.Contains(item.Text, "Unicode") {
				hasUnicode = true
			}
			if strings.Contains(item.Text, "Other") {
				hasOther = true
			}
		}
	}

	if !hasSystem || !hasUnicode || !hasOther {
		t.Errorf("Missing group headers: System=%v, Unicode=%v, Other=%v", hasSystem, hasUnicode, hasOther)
	}

	// Selected index should point to System ANSI
	if currIdx <= 0 || currIdx >= len(items) {
		t.Errorf("Invalid selected index %d", currIdx)
	}
}

func TestCodepages_BuildCodepageMenuItems_IncludesExplicitLegacyCodepages(t *testing.T) {
	items, _ := BuildCodepageMenuItems(65001, false)
	seen := make(map[int]bool)
	for _, item := range items {
		if id, ok := item.UserData.(int); ok {
			seen[id] = true
		}
	}
	for _, id := range []int{1251, 866} {
		if !seen[id] {
			t.Errorf("manual codepage menu does not contain explicit codepage %d", id)
		}
	}
}

func TestCodepages_BuildCodepageMenuItems_AutoDetect(t *testing.T) {
	items, currIdx := BuildCodepageMenuItems(SystemANSICodepage(), true)
	if len(items) == 0 {
		t.Fatal("Empty menu items")
	}
	if currIdx != 0 {
		t.Errorf("Expected active index to be 0 (Auto-detect), got %d", currIdx)
	}
	if !strings.Contains(items[0].Text, "√") {
		t.Errorf("Expected Auto-detect to have a checkmark, got %q", items[0].Text)
	}
}

func TestCodepages_SafeDecodeEncodeErrors(t *testing.T) {
	_, err := DecodeBytes([]byte("test"), 99999)
	if err == nil {
		t.Error("Expected error for unsupported decode CP ID, got nil")
	}

	_, err = EncodeBytes([]byte("test"), 99999)
	if err == nil {
		t.Error("Expected error for unsupported encode CP ID, got nil")
	}
}
