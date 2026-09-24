package viewer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type recordingColorizer struct {
	context []string
	lines   []WindowLine
	closed  bool
}

func (r *recordingColorizer) Request(context []string, lines []WindowLine) {
	r.context, r.lines = context, lines
}

func (r *recordingColorizer) LineAttrs(offset int64, text string) []uint64 {
	attrs := make([]uint64, len([]rune(text)))
	for i := range attrs {
		attrs[i] = 0x4200 + uint64(i)
	}
	return attrs
}

func (r *recordingColorizer) Close() { r.closed = true }

// The viewer hands the colorizer the logical lines on screen with the lines
// above as context, and paints each cell with its rune's colour, wrapped rows
// included.
func TestViewerHighlightsTheLinesOnScreen(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("zero\none\ntwo abcdefgh\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldAuto, oldDefault := config.App.ViewerAutodetectCodePage, config.App.ViewerDefaultCodePage
	config.App.ViewerAutodetectCodePage, config.App.ViewerDefaultCodePage = false, 65001
	t.Cleanup(func() { config.App.ViewerAutodetectCodePage, config.App.ViewerDefaultCodePage = oldAuto, oldDefault })

	rec := &recordingColorizer{}
	old := NewWindowColorizer
	NewWindowColorizer = func(string, string, uint64, func()) WindowColorizer { return rec }
	t.Cleanup(func() { NewWindowColorizer = old })

	vv, err := NewViewerView(context.Background(), vfs.NewOSVFS(root), path)
	if err != nil {
		t.Fatalf("NewViewerView: %v", err)
	}
	// The backend loads through UI tasks; run them until the file is there.
	deadline := time.After(2 * time.Second)
	for {
		if _, err := vv.Backend.ReadAt(0, int(vv.Backend.Size())); err == nil {
			break
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("the file did not load")
		case <-time.After(5 * time.Millisecond):
		}
	}
	vv.WrapMode = true
	vv.TopOffset = 5 // "one"
	vv.SetPosition(0, 0, 9, 6)
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(10, 7)
	vv.renderText(scr, 8, 4)

	if len(rec.context) != 1 || rec.context[0] != "zero" {
		t.Errorf("context %q, want the line above", rec.context)
	}
	if len(rec.lines) < 2 || rec.lines[0] != (WindowLine{Offset: 5, Text: "one"}) || rec.lines[1] != (WindowLine{Offset: 9, Text: "two abcdefgh"}) {
		t.Fatalf("lines %+v", rec.lines)
	}
	// Row 2 is the wrapped continuation of "two abcdefgh": its first cell is
	// the rune after the first row's eight.
	cont := scr.GetCell(vv.X1, vv.Y1+1+2)
	if cont.Attributes != 0x4200+8 {
		t.Errorf("continuation cell %q attr %#x, want the ninth rune's colour", rune(cont.Char), cont.Attributes)
	}
	vv.Close()
	if !rec.closed {
		t.Error("the colorizer was not closed with the viewer")
	}
}
