package viewer

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
)

func TestSemanticHexLineContract(t *testing.T) {
	got := semanticHexLine(0x2a, []byte{'A', 0, 0x7f, 0x80, 0xff})
	want := "000000002A: 41 00 7F 80 FF" + strings.Repeat(" ", 36) + "| A...."
	if got != want {
		t.Fatalf("hex line=%q, want %q", got, want)
	}
	if got := semanticHexLine(0x123456789a, nil); !strings.Contains(got, "123456789A:") || !strings.HasSuffix(got, "| ") {
		t.Fatalf("empty hex line=%q", got)
	}
}

func TestSemanticViewerLineLengthModes(t *testing.T) {
	oldTabSize := config.App.EditorTabSize
	t.Cleanup(func() { config.App.EditorTabSize = oldTabSize })

	tests := []struct {
		name     string
		data     string
		width    int
		wrap     bool
		tabSize  int
		wantLine int
		wantText int
	}{
		{name: "newline", data: "abc\ndef", width: 20, wrap: false, wantLine: 4, wantText: 3},
		{name: "carriage return", data: "a\rb\n", width: 20, wrap: false, wantLine: 4, wantText: 3},
		{name: "crlf only", data: "\r\nrest", width: 20, wrap: false, wantLine: 2, wantText: 0},
		{name: "hard width", data: "abcdef", width: 3, wrap: false, wantLine: 3, wantText: 3},
		{name: "soft wrap", data: "abcdef", width: 3, wrap: true, wantLine: 3, wantText: 3},
		{name: "wide rune", data: "ab界z", width: 4, wrap: true, wantLine: 5, wantText: 5},
		{name: "invalid utf8", data: string([]byte{'a', 0xff, 'b'}), width: 3, wrap: true, wantLine: 3, wantText: 3},
		{name: "custom tab", data: "a\tb", width: 4, wrap: true, tabSize: 2, wantLine: 3, wantText: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.App.EditorTabSize = tt.tabSize
			lineLen, textLen := semanticViewerLineLen([]byte(tt.data), tt.width, tt.wrap)
			if lineLen != tt.wantLine || textLen != tt.wantText {
				t.Fatalf("lineLen=%d textLen=%d, want %d/%d", lineLen, textLen, tt.wantLine, tt.wantText)
			}
		})
	}
}

func TestSemanticViewerLineLengthEmptyAndUnbounded(t *testing.T) {
	if lineLen, textLen := semanticViewerLineLen(nil, 10, true); lineLen != 0 || textLen != 0 {
		t.Fatalf("empty data=%d/%d", lineLen, textLen)
	}
	if lineLen, textLen := semanticViewerLineLen([]byte("abc"), 0, false); lineLen != 1 || textLen != 1 {
		t.Fatalf("zero-width hard line=%d/%d", lineLen, textLen)
	}
}
