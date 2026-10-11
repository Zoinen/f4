package editor

import (
	"bytes"
	"testing"
)

func TestSaveAsEOLKeepCoverageBatch28(t *testing.T) {
	if got := saveAsEOLBytes(saveAsEOLKeep); got != nil {
		t.Fatalf("keep EOL bytes = %q, want nil", got)
	}
}

func TestSaveAsEOLDosCoverageBatch28(t *testing.T) {
	if got := saveAsEOLBytes(saveAsEOLDos); !bytes.Equal(got, []byte("\r\n")) {
		t.Fatalf("DOS EOL bytes = %q, want CRLF", got)
	}
}

func TestSaveAsEOLUnixCoverageBatch28(t *testing.T) {
	if got := saveAsEOLBytes(saveAsEOLUnix); !bytes.Equal(got, []byte("\n")) {
		t.Fatalf("Unix EOL bytes = %q, want LF", got)
	}
}

func TestSaveAsEOLMacCoverageBatch28(t *testing.T) {
	if got := saveAsEOLBytes(saveAsEOLMac); !bytes.Equal(got, []byte("\r")) {
		t.Fatalf("Mac EOL bytes = %q, want CR", got)
	}
}

func TestConvertLineEndingsKeepsEmptyTargetCoverageBatch28(t *testing.T) {
	input := []byte("a\r\nb\nc\rd")
	if got := convertLineEndings(input, nil); !bytes.Equal(got, input) || &got[0] != &input[0] {
		t.Fatalf("empty target changed input: %q", got)
	}
}

func TestConvertLineEndingsToUnixCoverageBatch28(t *testing.T) {
	if got := convertLineEndings([]byte("a\r\nb\rc\nd"), []byte("\n")); !bytes.Equal(got, []byte("a\nb\nc\nd")) {
		t.Fatalf("Unix conversion = %q", got)
	}
}

func TestConvertLineEndingsToDosCoverageBatch28(t *testing.T) {
	if got := convertLineEndings([]byte("a\nb\rc\nd"), []byte("\r\n")); !bytes.Equal(got, []byte("a\r\nb\r\nc\r\nd")) {
		t.Fatalf("DOS conversion = %q", got)
	}
}

func TestCodepageWritesBOMCoverageBatch28(t *testing.T) {
	for _, id := range []int{1200, 1201, 12000, 12001} {
		if !codepageWritesBOM(id) {
			t.Errorf("codepage %d should write BOM", id)
		}
	}
	if codepageWritesBOM(65001) {
		t.Error("UTF-8 should be controlled separately")
	}
}

func TestIsUnicodeCodepageCoverageBatch28(t *testing.T) {
	for _, id := range []int{65001, 1200, 1201, 12000, 12001} {
		if !isUnicodeCodepage(id) {
			t.Errorf("codepage %d should be Unicode", id)
		}
	}
	if isUnicodeCodepage(1251) {
		t.Error("Windows-1251 should not be treated as Unicode")
	}
}

func TestStripEncodedBOMCoverageBatch28(t *testing.T) {
	for _, tc := range []struct {
		id   int
		data []byte
		want []byte
	}{
		{1200, []byte{0xff, 0xfe, 'A'}, []byte{'A'}},
		{1201, []byte{0xfe, 0xff, 'A'}, []byte{'A'}},
		{12000, []byte{0xff, 0xfe, 0, 0, 'A'}, []byte{'A'}},
		{12001, []byte{0, 0, 0xfe, 0xff, 'A'}, []byte{'A'}},
	} {
		if got := stripEncodedBOM(tc.data, tc.id); !bytes.Equal(got, tc.want) {
			t.Errorf("stripEncodedBOM(%d) = %x, want %x", tc.id, got, tc.want)
		}
	}
}
