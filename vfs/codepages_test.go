package vfs

import (
	"context"
	"io"
	"os"
	"testing"

	"golang.org/x/text/encoding/charmap"
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
	origLang := os.Getenv("LANG")
	origLcAll := os.Getenv("LC_ALL")
	defer func() {
		os.Setenv("LANG", origLang)
		os.Setenv("LC_ALL", origLcAll)
	}()

	os.Setenv("LC_ALL", "")

	os.Setenv("LANG", "ru_RU.UTF-8")
	oem := GetSystemOEMEncoding()
	ansi := GetSystemANSIEncoding()
	if oem != charmap.CodePage866 {
		t.Errorf("Expected CP866 for RU, got %v", oem)
	}
	if ansi != charmap.Windows1251 {
		t.Errorf("Expected CP1251 for RU, got %v", ansi)
	}

	os.Setenv("LANG", "cs_CZ.UTF-8")
	oemCS := GetSystemOEMEncoding()
	ansiCS := GetSystemANSIEncoding()
	if oemCS != charmap.CodePage852 {
		t.Errorf("Expected CP852 for CS, got %v", oemCS)
	}
	if ansiCS != charmap.Windows1250 {
		t.Errorf("Expected CP1250 for CS, got %v", ansiCS)
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
