package vtui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/unxed/vtinput"
)

// fakeImageFrame mimics the f4 image viewer: it occupies the rows below the
// workspace tab strip and declares one sixel image at the first content row.
type fakeImageFrame struct {
	BaseFrame
	surf *ImageSurface
}

func (f *fakeImageFrame) Show(scr *ScreenBuf) {
	x1, y1, x2, y2 := f.GetPosition()
	scr.FillRect(x1, y1, x2, y2, ' ', 0x00FFFFFF)
	scr.Graphics().DrawImage("img", ImagePlacement{
		Surface: f.surf,
		Col:     x1, Row: y1, Cols: x2 - x1 + 1, Rows: y2 - y1 + 1,
	})
}
func (f *fakeImageFrame) ResizeConsole(w, h int) {
	f.SetPosition(0, FrameManager.WorkspaceTopInset(), w-1, h-1)
}
func (f *fakeImageFrame) ProcessKey(*vtinput.InputEvent) bool   { return false }
func (f *fakeImageFrame) ProcessMouse(*vtinput.InputEvent) bool { return false }
func (f *fakeImageFrame) HandleCommand(int, any) bool           { return false }
func (f *fakeImageFrame) GetTitle() string                      { return "img" }
func (f *fakeImageFrame) GetType() FrameType                    { return TypeUser }

func TestRenderPhaseSixelDoesNotCoverTabRow(t *testing.T) {
	scr := NewScreenBuf()
	var out bytes.Buffer
	scr.Writer = &out
	scr.AllocBuf(80, 25)
	scr.Graphics().SetProtocol(GraphicsSixel)
	scr.Graphics().SetCellSize(8, 16)

	FrameManager.Init(scr)
	oldMode := FrameManager.WorkspaceTabMode
	FrameManager.WorkspaceTabMode = WorkspaceTabsAlways
	t.Cleanup(func() { FrameManager.WorkspaceTabMode = oldMode })

	FrameManager.Push(NewDesktop())
	img := &fakeImageFrame{surf: NewImageSurface(100, 100)}
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.surf.SetPixel(x, y, 200, 30, 30, 255)
		}
	}
	img.ResizeConsole(80, 25)
	FrameManager.Push(img)

	FrameManager.renderPhase()
	scr.Flush()

	raw := out.String()
	idx := strings.Index(raw, "\x1bP0;1;8q")
	if idx < 0 {
		t.Fatalf("no sixel DCS emitted:\n%q", raw)
	}
	before := raw[:idx]
	if !strings.HasSuffix(before, "\x1b[2;1H") {
		t.Fatalf("sixel cursor must be at row 2 (below tabs); tail=%q", before[max(0, len(before)-40):])
	}
}
