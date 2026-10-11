package media

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/unxed/vtui"
)

// On a screen with no graphics protocol the picture is drawn in half blocks
// instead of an apology.
func TestImageViewDrawsHalfBlocksWithoutGraphics(t *testing.T) {
	scr := vtui.NewScreenBuf()
	scr.Writer = io.Discard
	scr.AllocBuf(80, 25)
	if scr.SupportsGraphics() {
		t.Skip("this screen has graphics")
	}
	iv := newTestImageView(t, 4, 8)
	for y := 0; y < 8; y++ {
		for x := 0; x < 4; x++ {
			iv.surface.SetPixel(x, y, 255, 0, 0, 255)
		}
	}
	iv.Show(scr)

	var dump bytes.Buffer
	scr.Dump(&dump)
	if strings.Contains(dump.String(), "cannot display images") {
		t.Fatal("the picture was apologised for")
	}
	if !strings.Contains(dump.String(), "▀") {
		t.Fatalf("no half blocks on the screen:\n%s", dump.String())
	}

	blank := newTestImageView(t, 4, 8)
	blank.surface = nil
	scr2 := vtui.NewScreenBuf()
	scr2.Writer = io.Discard
	scr2.AllocBuf(80, 25)
	blank.Show(scr2) // nothing to draw: no panic, and the apology is not owed either
}
