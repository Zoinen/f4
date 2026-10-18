package terminal

import (
	"strings"
	"testing"

	"github.com/unxed/vtui"
)

func TestGetAllLogANSIBytesPreservesCellColours(t *testing.T) {
	tv := NewTerminalView(8, 2)
	red := vtui.SetRGBBoth(DefaultTermAttr, 0xff0000, 0x000000)
	blue := vtui.SetRGBBoth(DefaultTermAttr, 0x0000ff, 0x101010)
	tv.PutChar('R', red)
	tv.PutChar('B', blue)

	got := string(tv.GetAllLogANSIBytes())
	for _, want := range []string{
		"\x1b[0;38;2;255;0;0;48;2;0;0;0mR",
		"\x1b[0;38;2;0;0;255;48;2;16;16;16mB",
		"\x1b[0m",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ANSI log %q does not contain %q", got, want)
		}
	}
	if plain := string(tv.GetAllLogBytes()); plain != "RB" {
		t.Fatalf("plain terminal log = %q, want RB", plain)
	}
}
