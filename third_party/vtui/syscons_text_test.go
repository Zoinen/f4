package vtui

import (
	"regexp"
	"strings"
	"testing"
)

// Issue #91: syscons draws one cell per output byte (syscons_text.go), so a
// frame for it must contain nothing but ASCII, one byte per column, and no
// cursor resync is needed between cells.
func TestAnsiRenderer_SysconsWritesOneASCIIBytePerCell(t *testing.T) {
	oldConsole, oldSyscons := IsFreeBSDConsole, IsFreeBSDSyscons
	IsFreeBSDConsole, IsFreeBSDSyscons = true, true
	defer func() { IsFreeBSDConsole, IsFreeBSDSyscons = oldConsole, oldSyscons }()

	// The workspace tab row, a sorted column title and a symlink as f4 draws
	// them, then an accented letter, a wide character and a cluster.
	var cells []uint64
	for _, r := range " 1P / ─ sbin │+ Name ↑ → sys é" {
		cells = append(cells, uint64(r))
	}
	cells = append(cells, uint64('漢'), WideCharFiller, 'x', RegisterCluster("y\u0306"))
	const want = " 1P / - sbin |+ Name ^ > sys e??xy"

	w := len(cells)
	buf := make([]CharInfo, w)
	shadow := make([]CharInfo, w)
	for i, ch := range cells {
		buf[i] = CharInfo{Char: ch}
	}
	r := &AnsiRenderer{parent: &ScreenBuf{ColorProfile: ColorProfile16}}
	r.Render(buf, shadow, w, 1, true)
	got := r.frameOut.String()

	for i := 0; i < len(got); i++ {
		if got[i] >= 0x80 {
			t.Fatalf("byte %#x at %d in a syscons frame: %q", got[i], i, got)
		}
	}
	csi := regexp.MustCompile(`\x1b\[[0-9;?=]*[A-Za-z]`)
	if n := len(regexp.MustCompile(`\x1b\[[0-9;]*H`).FindAllString(got, -1)); n != 1 {
		t.Errorf("%d cursor positionings in one row, want 1 (no resync): %q", n, got)
	}
	if text := csi.ReplaceAllString(got, ""); text != want {
		t.Errorf("text\ngot  %q\nwant %q", text, want)
	}
	if len(want) != w {
		t.Fatalf("test setup: want is %d columns, the row %d", len(want), w)
	}
}

func TestAsciiStandIn(t *testing.T) {
	for r, want := range map[rune]byte{
		'a': 'a', '─': '-', '═': '=', '│': '|', '║': '|', '┌': '+', '╬': '+', '╟': '+',
		'┈': '-', '╳': 'X', '░': '.', '▒': ':', '▓': '#', '█': '#', '▲': '^', '▼': 'v',
		'⠋': '*', '\u00a0': ' ', '…': '.', 'ü': 'u', 'Ž': 'Z', 'Ж': '?', '漢': '?',
		'\u0301': '?', 0x85: '?',
	} {
		if got := asciiStandIn(r); got != want {
			t.Errorf("asciiStandIn(%U) = %q, want %q", r, got, want)
		}
	}
	for i := 0; i < len(boxDrawingASCII); i++ {
		if !strings.ContainsRune(`-=|+/\X`, rune(boxDrawingASCII[i])) {
			t.Errorf("boxDrawingASCII[%#x] = %q", i, boxDrawingASCII[i])
		}
	}
	if len(boxDrawingASCII) != 0x80 {
		t.Errorf("boxDrawingASCII has %d entries, want 128", len(boxDrawingASCII))
	}
}
