package vtui

import (
	"strings"
	"testing"
)

// SetPalette must pack every changed color into a single OSC 4 message
// (xterm accepts multiple ;index;spec pairs). Terminals that repaint per
// OSC message — iTerm2 among them — take multiple seconds to process 256
// individual messages, which froze startup and exit visibly.
func TestSetPaletteSingleOSC4Message(t *testing.T) {
	s := NewScreenBuf()
	r := s.Renderer.(*AnsiRenderer)

	var pal [256]uint32
	for i := range pal {
		pal[i] = uint32(i) * 0x010101
	}
	r.SetPalette(&pal)

	out := string(r.frameOut)
	if got := strings.Count(out, "\x1b]4"); got != 1 {
		t.Fatalf("want all changed colors in 1 OSC 4 message, got %d messages", got)
	}
	if pairs := strings.Count(out, ";rgb:"); pairs != 256 {
		t.Fatalf("want 256 color pairs, got %d", pairs)
	}
	if !strings.HasSuffix(out, "\x07") {
		t.Fatalf("OSC 4 message not BEL-terminated: %q", out[len(out)-16:])
	}

	// A second call with the same palette must emit nothing.
	r.frameOut.Reset()
	r.SetPalette(&pal)
	if r.frameOut.Len() != 0 {
		t.Fatalf("unchanged palette re-emitted %d bytes", r.frameOut.Len())
	}
}
