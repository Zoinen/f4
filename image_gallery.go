package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// The grid of thumbnails behind F12. A tile is measured in cells and gives
// its bottom row to the file name. The numbers are a compromise: small enough
// that an ordinary terminal shows a directory as a grid, large enough that an
// Exif thumbnail is still recognisable.
const (
	imageTileCols = 18
	imageTileRows = 9
)

var (
	imageTileNameAttr   = vtui.SetRGBBoth(0, 0xC0C0C0, 0x101010)
	imageTileCursorAttr = vtui.SetRGBBoth(0, 0x101010, 0xC0C0C0)
	imageTilePickedAttr = vtui.SetRGBBoth(0, 0xFFFF00, 0x101010)
)

// imageGallery is the state of the grid: where the cursor is, which row of
// tiles is the first one on screen, and the thumbnails that have arrived so
// far. It knows nothing about files — the viewer owns the list of pictures.
type imageGallery struct {
	cursor int
	top    int
	cols   int
	rows   int
	thumbs map[string]*vtui.ImageSurface
	asked  map[string]bool
}

// layout works out how many tiles fit into the window.
func (g *imageGallery) layout(cols, rows int) {
	g.cols = cols / imageTileCols
	g.rows = rows / imageTileRows
	if g.cols < 1 {
		g.cols = 1
	}
	if g.rows < 1 {
		g.rows = 1
	}
}

// step is how far one press of the up or down arrow moves.
func (g *imageGallery) step() int {
	if g.cols < 1 {
		return 1
	}
	return g.cols
}

// page is how many rows of tiles one press of PgUp or PgDn moves.
func (g *imageGallery) page() int {
	if g.rows < 1 {
		return 1
	}
	return g.rows
}

// move walks the grid and stops at both ends rather than wrapping, so that it
// stays obvious where the directory begins and where it ends.
func (g *imageGallery) move(delta, total int) {
	if total <= 0 {
		return
	}
	g.cursor += delta
	if g.cursor < 0 {
		g.cursor = 0
	}
	if g.cursor >= total {
		g.cursor = total - 1
	}
}

// scrollTo brings the row the cursor sits on into view.
func (g *imageGallery) scrollTo(idx, total int) {
	cols, rows := g.step(), g.page()
	row := idx / cols
	if row < g.top {
		g.top = row
	}
	if row >= g.top+rows {
		g.top = row - rows + 1
	}
	if last := (total + cols - 1) / cols; g.top > last-rows {
		g.top = last - rows
	}
	if g.top < 0 {
		g.top = 0
	}
}

// ToggleGallery switches between one picture and the grid. The grid opens on
// the picture that was on screen, and leaving it by anything but Enter leaves
// that same picture on screen.
func (iv *ImageView) ToggleGallery() {
	// A grid and a slide show both want to own which picture is current,
	// and there is nothing to divide that ownership with.
	iv.stopSlideShow()
	if iv.gal != nil {
		iv.gal = nil
		return
	}
	cursor := iv.index
	if cursor < 0 {
		cursor = 0
	}
	iv.gal = &imageGallery{
		cursor: cursor,
		cols:   1,
		rows:   1,
		thumbs: make(map[string]*vtui.ImageSurface),
		asked:  make(map[string]bool),
	}
}

// galleryPath is the picture under the grid cursor.
func (iv *ImageView) galleryPath() string {
	if iv.gal == nil || iv.gal.cursor < 0 || iv.gal.cursor >= len(iv.siblings) {
		return ""
	}
	return iv.siblings[iv.gal.cursor]
}

// SetSelection replaces the set of picked pictures without telling anybody:
// it is how the panel hands over what it had picked before the viewer opened.
func (iv *ImageView) SetSelection(picked map[string]bool) {
	iv.selected = make(map[string]bool, len(picked))
	for path := range picked {
		iv.selected[path] = true
	}
}

// SetSelected picks or unpicks one picture and passes the news on, so that the
// panel underneath ends up with the same files selected.
func (iv *ImageView) SetSelected(path string, on bool) {
	if path == "" {
		return
	}
	if iv.selected == nil {
		iv.selected = make(map[string]bool)
	}
	if on {
		iv.selected[path] = true
	} else {
		delete(iv.selected, path)
	}
	if iv.OnSelect != nil {
		iv.OnSelect(path, on)
	}
}

// requestThumb decodes one thumbnail off the drawing path. PreviewSync is
// cheap on a picture that is already known and reads only the header on one
// that is not, but cheap is not free, and a screenful of tiles would
// otherwise mean a screenful of reads on every frame.
func (iv *ImageView) requestThumb(path string) {
	g := iv.gal
	if g == nil || g.asked[path] {
		return
	}
	g.asked[path] = true

	v := iv.vfs
	vtui.RunAsync(func(ctx *vtui.TaskContext) {
		res, ok := ImagePipe.PreviewSync(ctx.Context, v, path)
		if !ok {
			// No thumbnail inside the file: decoding it whole is the only
			// way this tile will ever show anything.
			res = ImagePipe.LoadSync(ctx.Context, v, path)
			if res.Err != nil {
				return
			}
		}
		surface := res.Surface
		ctx.RunOnUI(func() {
			if iv.gal != nil && surface.Valid() {
				iv.gal.thumbs[path] = surface
			}
		})
	})
}

// showGallery paints the grid over the area the picture would have taken.
func (iv *ImageView) showGallery(scr *vtui.ScreenBuf) {
	g := iv.gal
	if g == nil || scr == nil {
		return
	}
	x1, y1, x2, y2 := iv.GetPosition()
	top := y1 + iv.barHeight()

	g.layout(x2-x1+1, y2-top+1)
	total := len(iv.siblings)
	if total == 0 {
		return
	}
	g.move(0, total)
	g.scrollTo(g.cursor, total)

	cw, ch := scr.Graphics().CellSize()
	if cw <= 0 || ch <= 0 {
		cw, ch = imageViewFallbackCellW, imageViewFallbackCellH
	}

	first := g.top * g.cols
	for slot := 0; slot < g.cols*g.rows; slot++ {
		idx := first + slot
		if idx >= total {
			break
		}
		iv.showTile(scr, idx,
			x1+(slot%g.cols)*imageTileCols,
			top+(slot/g.cols)*imageTileRows,
			cw, ch)
	}
}

// showTile paints one thumbnail and the caption under it.
func (iv *ImageView) showTile(scr *vtui.ScreenBuf, idx, col, row, cw, ch int) {
	path := iv.siblings[idx]
	name := filepath.Base(path)
	if iv.vfs != nil {
		name = iv.vfs.Base(path)
	}

	attr := imageTileNameAttr
	switch {
	case idx == iv.gal.cursor:
		attr = imageTileCursorAttr
	case iv.selected[path]:
		attr = imageTilePickedAttr
	}

	caption := runewidth.Truncate(" "+name, imageTileCols, "…")
	if w := runewidth.StringWidth(caption); w < imageTileCols {
		caption += strings.Repeat(" ", imageTileCols-w)
	}
	scr.Write(col, row+imageTileRows-1, vtui.StringToCharInfo(caption, attr))

	surface := iv.gal.thumbs[path]
	if !surface.Valid() {
		iv.requestThumb(path)
		return
	}
	if !scr.SupportsGraphics() {
		return
	}

	boxCols, boxRows := imageTileCols-2, imageTileRows-2
	w, h := vtui.FitInside(surface.Width, surface.Height, boxCols*cw, boxRows*ch)
	p := vtui.ImagePlacement{Surface: surface}
	p.Cols, p.Rows = cellsFor(w, cw, boxCols), cellsFor(h, ch, boxRows)
	p.Col = col + 1 + (boxCols-p.Cols)/2
	p.Row = row + (boxRows-p.Rows)/2
	scr.Graphics().DrawImage(fmt.Sprintf("%s#%d", iv.gfxKey, idx), p)
}

// galleryKey handles the grid. Anything it does not know falls through to the
// ordinary viewer keys.
func (iv *ImageView) galleryKey(e *vtinput.InputEvent) bool {
	g := iv.gal
	if g == nil {
		return false
	}
	if (e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed |
		vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0 {
		return false
	}
	total := len(iv.siblings)

	switch e.Char {
	case 'a', 'A':
		g.move(-1, total)
		return true
	case 'd', 'D', ' ':
		g.move(1, total)
		return true
	case 'w', 'W':
		g.move(-g.step(), total)
		return true
	case 's', 'S':
		g.move(g.step(), total)
		return true
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_F12, vtinput.VK_ESCAPE:
		iv.ToggleGallery()
		return true
	case vtinput.VK_RETURN:
		idx := g.cursor
		iv.ToggleGallery()
		iv.GoTo(idx)
		return true
	case vtinput.VK_LEFT:
		g.move(-1, total)
		return true
	case vtinput.VK_RIGHT:
		g.move(1, total)
		return true
	case vtinput.VK_UP:
		g.move(-g.step(), total)
		return true
	case vtinput.VK_DOWN:
		g.move(g.step(), total)
		return true
	case vtinput.VK_PRIOR:
		g.move(-g.step()*g.page(), total)
		return true
	case vtinput.VK_NEXT:
		g.move(g.step()*g.page(), total)
		return true
	case vtinput.VK_HOME:
		g.move(-total, total)
		return true
	case vtinput.VK_END:
		g.move(total, total)
		return true
	case vtinput.VK_INSERT:
		path := iv.galleryPath()
		iv.SetSelected(path, !iv.selected[path])
		g.move(1, total)
		return true
	case vtinput.VK_DELETE:
		iv.SetSelected(iv.galleryPath(), false)
		g.move(1, total)
		return true
	}
	return false
}
