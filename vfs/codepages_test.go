package vfs

import (
	"github.com/unxed/localecp"

	"context"
	"io"
	"strings"
	"testing"
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

func TestCodepages_GetSystemEncoding(t *testing.T) {
	oem := GetSystemOEMEncoding()
	ansi := GetSystemANSIEncoding()

	if oem == nil {
		t.Fatal("expected GetSystemOEMEncoding to return non-nil encoding")
	}
	if ansi == nil {
		t.Fatal("expected GetSystemANSIEncoding to return non-nil encoding")
	}

	if oem != localecp.OEMEncoding {
		t.Errorf("expected OEM encoding %v, got %v", localecp.OEMEncoding, oem)
	}
	if ansi != localecp.ANSIEncoding {
		t.Errorf("expected ANSI encoding %v, got %v", localecp.ANSIEncoding, ansi)
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

	if cp := DetectEncoding(data, false, 1251); cp != 1251 {
		t.Errorf("Expected fallback when autodetect is false, got %d", cp)
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

	decAnsi, encAnsi := GetCodepageDecoderEncoder("11111")
	if decAnsi == nil || encAnsi == nil {
		t.Error("Expected valid dec/enc for System ANSI")
	}

	decOem, encOem := GetCodepageDecoderEncoder("22222")
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
		{65001, 11111},
		{11111, 22222},
		{22222, 65001},
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
		{11111, "ANSI"},
		{22222, "OEM"},
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
}

func TestCodepages_BuildCodepageMenuItems(t *testing.T) {
	items, currIdx := BuildCodepageMenuItems(11111, false)
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
func TestCodepages_BuildCodepageMenuItems_AutoDetect(t *testing.T) {
	items, currIdx := BuildCodepageMenuItems(11111, true)
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
