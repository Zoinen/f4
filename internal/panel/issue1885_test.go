package panel

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// issue1885Renderer draws nothing and claims to display images, which is all
// ScreenBuf.SupportsGraphics asks of a renderer.
type issue1885Renderer struct{}

func (issue1885Renderer) Render(buf, shadow []vtui.CharInfo, width, height int, forceRedraw bool) {}
func (issue1885Renderer) SetCursor(x, y int, visible bool, shape vtui.CursorShape)                {}
func (issue1885Renderer) SetPalette(palette *[256]uint32)                                         {}
func (issue1885Renderer) SetWindowTitle(title string)                                             {}
func (issue1885Renderer) Flush()                                                                  {}
func (issue1885Renderer) RenderGraphics(*vtui.GraphicsLayer, []vtui.CharInfo, []vtui.CharInfo, int, int, bool) {
}

// A picture in the quick view lies under whatever is painted after it: a
// dialog over it covers its cells, so no backend draws the picture over the
// dialog (#1885). The panels frame itself paints nothing over its picture.
func TestIssue1885_DialogCoversQuickViewPicture(t *testing.T) {
	for active := range 2 {
		t.Run([]string{"active-left", "active-right"}[active], func(t *testing.T) {
			issue1885DialogCoversQuickView(t, active)
		})
	}
}

func issue1885DialogCoversQuickView(t *testing.T, active int) {
	t.Cleanup(swapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	scr.Renderer = issue1885Renderer{}
	scr.Graphics().SetProtocol(vtui.GraphicsNative)
	scr.Graphics().SetCellSize(10, 20)
	vtui.FrameManager.Init(scr)
	theme.SetDefaultF4Palette()

	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for i := range img.Pix {
		img.Pix[i] = 0xC0
	}
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "photo.png"), buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	pf := NewPanelsFrame()
	t.Cleanup(pf.Close)
	pf.ResizeConsole(80, 25)
	pf.ShowPanels = true
	// Quick view takes the passive panel and previews the active one's
	// cursor, whichever side that is.
	pf.ActiveIdx = active
	passive := 1 - active
	src := pf.Panels[active].(*FileSystemPanel)
	src.Vfs = vfs.NewOSVFS(dir)
	src.ReadDirectory()
	waitForDirectoryLoads(t)
	testutil.DrainUITasks()
	for i, e := range src.Entries {
		if e.Name == "photo.png" {
			src.CursorIdx = i
		}
	}
	q := NewQuickViewPanel(src)
	q.SetPosition(pf.Panels[passive].GetPosition())
	pf.AltPanels[passive] = q

	paint := func(over vtui.Frame) []vtui.ImagePlacement {
		g := scr.Graphics()
		g.BeginFrame()
		pf.Show(scr)
		if over != nil {
			over.Show(scr)
		}
		g.EndFrame()
		list, _ := g.Snapshot(nil)
		return list
	}

	deadline := time.Now().Add(5 * time.Second)
	var list []vtui.ImagePlacement
	for list = paint(nil); len(list) == 0; list = paint(nil) {
		if time.Now().After(deadline) {
			t.Fatal("the quick view never declared its picture")
		}
		testutil.DrainUITasks()
		time.Sleep(10 * time.Millisecond)
	}
	if list[0].Covered.Any() {
		t.Fatal("the panels frame paints over its own quick view picture")
	}

	p := list[0]
	dlg := vtui.NewDialog(p.Col+p.Cols/2, p.Row+1, p.Col+p.Cols+10, p.Row+6, "Delete")
	list = paint(dlg)
	if len(list) != 1 {
		t.Fatalf("a partly covered picture must still be drawn natively, got %d placements", len(list))
	}
	m := list[0].Covered
	if !m.Covered(p.Col+p.Cols/2, p.Row+1) || !m.Covered(p.Col+p.Cols-1, p.Row+6) {
		t.Fatal("the dialog's cells over the picture must be covered")
	}
	if m.Covered(p.Col, p.Row) {
		t.Fatal("cells outside the dialog must stay uncovered")
	}

	if list = paint(nil); list[0].Covered.Any() {
		t.Fatal("closing the dialog must uncover the picture")
	}
}
