package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

const (
	imageViewMinZoom = 0.05
	imageViewMaxZoom = 40.0

	// Terminal backends cannot always tell us how big a character cell is.
	// The exact numbers only affect the aspect ratio, so a common default is
	// good enough until the size can be queried.
	imageViewFallbackCellW = 8
	imageViewFallbackCellH = 16

	// imageViewPrefetchRadius is how many pictures on each side are decoded
	// before anybody asks to see them.
	imageViewPrefetchRadius = 2
)

func imageViewBackAttr() uint64 {
	return vtui.Palette[ColViewerText]
}

// imageOverlayAttr is what the info panel is written with. Resolve it at
// render time so a viewer that is already open follows a theme change.
func imageOverlayAttr() uint64 {
	return vtui.Palette[ColViewerStatus]
}

// ImageView shows a picture inside its workspace tab or across the whole
// screen when full-screen mode is enabled.
type ImageView struct {
	vtui.BaseFrame
	topBar *TopBar

	vfs     vfs.VFS
	path    string
	surface *vtui.ImageSurface
	decoder string
	gfxKey  string

	siblings []string
	index    int

	preview    bool
	loading    bool
	err        error
	loadGen    uint64
	actual     bool
	full       bool
	lastScale  float64
	zoom       float64
	panX, panY float64

	// How far the picture can still be moved along each axis, as of the last
	// frame: only drawing knows how large the window is. Zero means the
	// picture fits and there is nothing to pan.
	panMaxX, panMaxY float64

	// The last line of geometry that was written to the log, so that a
	// picture nobody is touching does not fill it.
	lastGeom string

	// Orientation chosen by the reader. The decoded picture stays in
	// surface; shown carries the turned and mirrored copy and is nil while
	// the picture is seen exactly as it was decoded.
	rotation     int
	flipH, flipV bool
	shown        *vtui.ImageSurface

	// The last console size, kept so that entering or leaving the whole
	// screen mode can lay the frame out again without waiting for a resize.
	conW, conH int

	overlay   bool
	fileSize  int64
	sizeKnown bool
	gal       *imageGallery
	selected  map[string]bool
	slideStop chan struct{}

	OnClose  func()
	OnSelect func(path string, selected bool)
}

// NewImageView loads and decodes the file. Decoding happens here rather than
// lazily so that a failure can still be reported as a normal open error.
func NewImageView(ctx context.Context, v vfs.VFS, path string) (*ImageView, error) {
	// A file that carries a thumbnail opens at once and sharpens when the
	// megapixels arrive; one that does not is waited for.
	res, ok := ImagePipe.PreviewSync(ctx, v, path)
	if !ok {
		res = ImagePipe.LoadSync(ctx, v, path)
		if res.Err != nil {
			return nil, res.Err
		}
	}
	surf, decoder := res.Surface, res.Decoder

	iv := &ImageView{
		vfs:     v,
		path:    path,
		surface: surf,
		decoder: decoder,
		preview: res.Preview,
		zoom:    1,
	}
	iv.gfxKey = fmt.Sprintf("f4.imageview:%p", iv)

	iv.index = -1
	iv.topBar = NewTopBar(
		func() string {
			var base string
			if v != nil {
				base = v.Base(iv.path)
			} else {
				base = filepath.Base(iv.path)
			}
			return " " + base
		},
		func() string {
			state := iv.decoder
			switch {
			case iv.err != nil:
				state = "error: " + iv.err.Error()
			case iv.loading:
				state += ", loading"
			case iv.preview:
				state += ", preview"
			}
			if iv.slideStop != nil {
				state += ", slideshow"
			}

			position := ""
			if iv.index >= 0 && len(iv.siblings) > 1 {
				position = fmt.Sprintf(" │ %d/%d", iv.index+1, len(iv.siblings))
			}

			position += iv.pickMark()

			scale := iv.lastScale
			if scale <= 0 {
				scale = iv.zoom
			}
			return fmt.Sprintf(" %dx%d │ %d%%%s │ %s ",
				iv.display().Width, iv.display().Height,
				int(scale*100+0.5), position, state)
		},
	)
	iv.topBar.GetAttr = iv.titleAttr
	iv.topBar.SetVisible(true)
	iv.SetCanFocus(true)
	iv.SetFocus(true)

	// What is on screen is a stand-in; ask for the real thing.
	iv.loading = res.Preview
	if res.Preview {
		gen := iv.loadGen
		ImagePipe.Load(v, path, func(full ImageResult) {
			iv.accept(gen, full)
		})
	}
	return iv, nil
}

// barHeight is how many rows the title bar takes from the picture.
func (iv *ImageView) barHeight() int {
	if iv.full {
		return 0
	}
	return 1
}

// SetSiblings tells the viewer which pictures stand next to this one, in the
// order the panel shows them.
func (iv *ImageView) SetSiblings(paths []string, index int) {
	iv.siblings = paths
	iv.index = index
	iv.prefetch()
}

// prefetch has the neighbours decoded while nobody is looking at them yet.
func (iv *ImageView) prefetch() {
	if iv.index < 0 || iv.index >= len(iv.siblings) {
		return
	}
	ImagePipe.Prefetch(iv.vfs, ImageNeighbourhood(iv.siblings, iv.index, imageViewPrefetchRadius))
}

// Step walks the siblings. It stops at the ends rather than wrapping around,
// so that it stays obvious where the directory begins and where it ends.
func (iv *ImageView) Step(delta int) {
	if len(iv.siblings) == 0 || iv.index < 0 {
		return
	}
	idx := iv.index + delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(iv.siblings) {
		idx = len(iv.siblings) - 1
	}
	iv.GoTo(idx)
}

// GoTo shows the sibling at the given position.
func (iv *ImageView) GoTo(idx int) {
	if idx < 0 || idx >= len(iv.siblings) || iv.siblings[idx] == iv.path {
		return
	}
	iv.index = idx
	iv.open(iv.siblings[idx])
}

// Reload decodes the file again, for a picture that has changed since it was
// put on screen.
func (iv *ImageView) Reload() {
	ImagePipe.Invalidate(iv.vfs, iv.path)
	iv.open(iv.path)
}

// open puts another picture on screen. One that is decoded already appears
// at once; otherwise the previous picture stays until the new one arrives,
// which is quieter than a flash of empty window.
func (iv *ImageView) open(path string) {
	iv.path = path
	iv.zoom = 1
	iv.panX, iv.panY = 0, 0
	iv.rotation, iv.flipH, iv.flipV = 0, false, false
	iv.fileSize, iv.sizeKnown = 0, false
	iv.shown = nil
	iv.err = nil
	iv.loadGen++
	gen := iv.loadGen
	iv.prefetch()
	if iv.overlay {
		iv.requestFileSize()
	}

	if res, ok := ImagePipe.Cached(iv.vfs, path); ok {
		iv.accept(gen, res)
		return
	}

	iv.loading = true
	v := iv.vfs
	// Read the pipeline here rather than inside the worker. The decode
	// outlives this call, and reading the global from the goroutine races
	// anything that reassigns ImagePipe while the picture is still arriving.
	// A decode also belongs to the pipeline it was started on: finishing
	// against a different one would answer with a picture nobody asked for.
	pipe := ImagePipe
	vtui.RunAsync(func(ctx *vtui.TaskContext) {
		if res, ok := pipe.PreviewSync(ctx.Context, v, path); ok {
			ctx.RunOnUI(func() { iv.accept(gen, res) })
		}
		res := pipe.LoadSync(ctx.Context, v, path)
		ctx.RunOnUI(func() { iv.accept(gen, res) })
	})
}

// accept takes a result unless the reader has moved on since asking for it.
func (iv *ImageView) accept(gen uint64, res ImageResult) {
	if gen != iv.loadGen {
		return
	}
	if res.Err != nil {
		iv.loading = false
		iv.err = res.Err
		vtui.DebugLog("IMAGE: %s: %v", res.Path, res.Err)
		return
	}
	iv.SetImage(res)
	iv.loading = res.Preview
}

// baseScale is what a zoom of one means: the picture fitted into the window,
// or its own pixels when the actual size is asked for.
func (iv *ImageView) baseScale(boxW, boxH int) float64 {
	img := iv.display()
	if !img.Valid() || img.Width <= 0 || iv.actual {
		return 1
	}
	fitW, _ := vtui.FitInside(img.Width, img.Height, boxW, boxH)
	if fitW <= 0 {
		return 1
	}
	return float64(fitW) / float64(img.Width)
}

// ToggleActualSize switches between the window and the picture itself
// deciding how large it is shown.
func (iv *ImageView) ToggleActualSize() {
	iv.actual = !iv.actual
	iv.zoom = 1
	iv.panX, iv.panY = 0, 0
}

// display is the picture the viewer works with: the turned and mirrored copy
// when the reader has changed the orientation, the decoded surface when they
// have not.
func (iv *ImageView) display() *vtui.ImageSurface {
	if iv.shown.Valid() {
		return iv.shown
	}
	return iv.surface
}

// rebuild bakes the current orientation into pixels. A backend can only ship
// a rectangle of pixels and place it on a grid of cells, so a turn cannot be
// expressed in the placement and has to be applied to the surface itself.
func (iv *ImageView) rebuild() {
	if iv.rotation == 0 && !iv.flipH && !iv.flipV {
		iv.shown = nil
		return
	}
	iv.shown = TransformSurface(iv.surface, iv.rotation, iv.flipH, iv.flipV)
}

// Rotate turns the picture clockwise by a multiple of ninety degrees.
func (iv *ImageView) Rotate(delta int) {
	// Mirroring is applied after the turn, and a mirror reverses the
	// direction of a turn, so with exactly one axis mirrored the stored
	// angle has to move the other way for the key to keep turning the
	// picture the reader actually sees.
	if iv.flipH != iv.flipV {
		delta = -delta
	}
	iv.rotation = ((iv.rotation+delta)%360 + 360) % 360
	iv.panX, iv.panY = 0, 0
	iv.rebuild()
}

// Flip mirrors the picture as it is seen, so it is applied after the turn.
func (iv *ImageView) Flip(horizontal, vertical bool) {
	if horizontal {
		iv.flipH = !iv.flipH
	}
	if vertical {
		iv.flipV = !iv.flipV
	}
	iv.panX, iv.panY = 0, 0
	iv.rebuild()
}

// SetImage replaces the picture on screen, keeping the viewer looking at the
// same part of it. Sizes differ between a thumbnail and the picture itself,
// so the panning is measured in the new picture's pixels.
func (iv *ImageView) SetImage(res ImageResult) {
	if res.Surface == nil || !res.Surface.Valid() {
		return
	}
	prev := iv.display()
	iv.surface = res.Surface
	iv.decoder = res.Decoder
	iv.preview = res.Preview
	iv.rebuild()
	if prev.Valid() && prev.Width > 0 {
		scale := float64(iv.display().Width) / float64(prev.Width)
		iv.panX *= scale
		iv.panY *= scale
	}
}

func (iv *ImageView) SetPosition(x1, y1, x2, y2 int) {
	iv.ScreenObject.SetPosition(x1, y1, x2, y2)
	if iv.topBar != nil {
		iv.topBar.SetPosition(x1, y1, x2, y1)
	}
}

// ResizeConsole lays the viewer out inside the active workspace tab. In the
// whole screen mode both the workspace tab row and the row that normally
// belongs to the key bar are taken by the picture.
func (iv *ImageView) ResizeConsole(w, h int) {
	iv.conW, iv.conH = w, h
	top := vtui.FrameManager.WorkspaceTopInset()
	bottom := h - 2
	if iv.full {
		top = 0
		bottom = h - 1
	}
	iv.SetPosition(0, top, w-1, bottom)
}

// SetZoom applies a new zoom factor, 1 meaning "fit into the window".
func (iv *ImageView) SetZoom(z float64) {
	if z < imageViewMinZoom {
		z = imageViewMinZoom
	}
	if z > imageViewMaxZoom {
		z = imageViewMaxZoom
	}
	iv.zoom = z
}

// Pan moves the visible region by a step of one twentieth of the image.
func (iv *ImageView) Pan(dx, dy int) {
	img := iv.display()
	if !img.Valid() {
		return
	}
	stepX := float64(img.Width) / 20
	stepY := float64(img.Height) / 20
	if stepX < 1 {
		stepX = 1
	}
	if stepY < 1 {
		stepY = 1
	}
	iv.panX += float64(dx) * stepX
	iv.panY += float64(dy) * stepY
	if iv.panX < 0 {
		iv.panX = 0
	}
	if iv.panY < 0 {
		iv.panY = 0
	}
}

func (iv *ImageView) clampPan(visW, visH int) {
	img := iv.display()
	maxX := float64(img.Width - visW)
	maxY := float64(img.Height - visH)
	if maxX < 0 {
		maxX = 0
	}
	if maxY < 0 {
		maxY = 0
	}
	if iv.panX > maxX {
		iv.panX = maxX
	}
	if iv.panY > maxY {
		iv.panY = maxY
	}
	if iv.panX < 0 {
		iv.panX = 0
	}
	if iv.panY < 0 {
		iv.panY = 0
	}
}

// placementFor computes where and how the picture should appear. While it
// fits, the placement is centred and shows the whole surface; once it is
// zoomed past the window, the placement fills the window and the source
// rectangle is cropped and panned instead.
func (iv *ImageView) placementFor(scr *vtui.ScreenBuf) (vtui.ImagePlacement, bool) {
	img := iv.display()
	if scr == nil || !img.Valid() {
		return vtui.ImagePlacement{}, false
	}

	x1, y1, x2, y2 := iv.GetPosition()
	top := y1 + iv.barHeight()
	cols := x2 - x1 + 1
	rows := y2 - top + 1
	if cols <= 0 || rows <= 0 {
		return vtui.ImagePlacement{}, false
	}

	cw, ch := scr.Graphics().CellSize()
	if cw <= 0 || ch <= 0 {
		cw, ch = imageViewFallbackCellW, imageViewFallbackCellH
	}

	boxW := cols * cw
	boxH := rows * ch
	scale := iv.baseScale(boxW, boxH) * iv.zoom
	if scale <= 0 {
		return vtui.ImagePlacement{}, false
	}
	iv.lastScale = scale

	dispW := int(float64(img.Width)*scale + 0.5)
	dispH := int(float64(img.Height)*scale + 0.5)
	if dispW < 1 {
		dispW = 1
	}
	if dispH < 1 {
		dispH = 1
	}

	p := vtui.ImagePlacement{Surface: img}
	if iv.overlay {
		// A negative z index asks the terminal to keep the picture under the
		// glyphs but still over the cell background, which is what makes the
		// info panel readable without hiding the picture behind a box.
		p.ZIndex = -1
	}

	if dispW <= boxW && dispH <= boxH {
		iv.panX, iv.panY = 0, 0
		iv.panMaxX, iv.panMaxY = 0, 0
		p.Cols, p.Rows = cellsFor(dispW, cw, cols), cellsFor(dispH, ch, rows)
		p.Col = x1 + (cols-p.Cols)/2
		p.Row = top + (rows-p.Rows)/2
		return p, true
	}

	visW := int(float64(boxW) / scale)
	visH := int(float64(boxH) / scale)
	if visW > img.Width {
		visW = img.Width
	}
	if visH > img.Height {
		visH = img.Height
	}
	if visW < 1 {
		visW = 1
	}
	if visH < 1 {
		visH = 1
	}
	iv.panMaxX = float64(img.Width - visW)
	iv.panMaxY = float64(img.Height - visH)
	if iv.panMaxX < 0 {
		iv.panMaxX = 0
	}
	if iv.panMaxY < 0 {
		iv.panMaxY = 0
	}
	iv.clampPan(visW, visH)

	shownW := int(float64(visW)*scale + 0.5)
	shownH := int(float64(visH)*scale + 0.5)
	if shownW > boxW {
		shownW = boxW
	}
	if shownH > boxH {
		shownH = boxH
	}

	p.Cols, p.Rows = cellsFor(shownW, cw, cols), cellsFor(shownH, ch, rows)
	p.Col = x1 + (cols-p.Cols)/2
	p.Row = top + (rows-p.Rows)/2
	p.SrcX, p.SrcY = int(iv.panX), int(iv.panY)
	p.SrcW, p.SrcH = visW, visH
	return p, true
}

// arrow is what the four arrow keys do. An axis the picture cannot be moved
// along at all has no panning to offer, so the key walks the directory
// instead; an axis that can be panned is panned, and the walking is left to
// space, PgUp and PgDn. Panning to the edge and then jumping to the next
// picture was the other candidate and was refused: it reads as a slip of the
// finger. The letters w, a, s and d pan whatever happens, so a reader moving
// a zoomed picture never has to think about which of the two an arrow means.
func (iv *ImageView) arrow(dx, dy int) {
	if dx != 0 && iv.panMaxX > 0 {
		iv.Pan(dx, 0)
		return
	}
	if dy != 0 && iv.panMaxY > 0 {
		iv.Pan(0, dy)
		return
	}
	if dx < 0 || dy < 0 {
		iv.Step(-1)
		return
	}
	iv.Step(1)
}

// pickMark is what the title bar says about the picture being selected. The
// colour says it as well, but a mark survives a terminal nobody has set the
// colours of.
func (iv *ImageView) pickMark() string {
	if iv.selected[iv.path] {
		return " *"
	}
	return ""
}

// titleAttr colours the title bar. A picked picture gets the colour the grid
// gives a picked tile, so that the two views say the same thing the same way.
// Zero leaves the palette in charge.
func (iv *ImageView) titleAttr() uint64 {
	if iv.selected[iv.path] {
		return imageTilePickedAttr()
	}
	return 0
}

// logGeometry records what the layout worked out to, once per change. It is
// here because a strip of background nobody asked for is a question about
// numbers — how many rows the frame has, how many the picture takes, how
// large a cell is — and about how many pictures the graphics layer holds at
// that moment, which is a different fault with the same symptom.
func (iv *ImageView) logGeometry(scr *vtui.ScreenBuf, p vtui.ImagePlacement) {
	img := iv.display()
	if scr == nil || !img.Valid() {
		return
	}
	x1, y1, x2, y2 := iv.GetPosition()
	cw, ch := scr.Graphics().CellSize()
	line := fmt.Sprintf(
		"console=%dx%d frame=%d,%d..%d,%d bar=%d cell=%dx%d img=%dx%d scale=%.4f place=%d,%d %dx%d src=%d,%d %dx%d z=%d layer=%d",
		iv.conW, iv.conH, x1, y1, x2, y2, iv.barHeight(), cw, ch,
		img.Width, img.Height, iv.lastScale,
		p.Col, p.Row, p.Cols, p.Rows, p.SrcX, p.SrcY, p.SrcW, p.SrcH, p.ZIndex,
		scr.Graphics().Len())
	if line == iv.lastGeom {
		return
	}
	iv.lastGeom = line
	vtui.DebugLog("IMAGE_GEOM: %s", line)
}

func cellsFor(pixels, cellSize, limit int) int {
	n := (pixels + cellSize - 1) / cellSize
	if n > limit {
		n = limit
	}
	if n < 1 {
		n = 1
	}
	return n
}

// SetFullScreen gives the rows of the title and key bars to the picture. The
// key bar is drawn by the frame manager rather than by the frame, and
// ScreenObject.Show makes an object visible whether it wants to be or not, so
// hiding it cannot be done locally and has to be asked for centrally.
func (iv *ImageView) SetFullScreen(on bool) {
	if iv.full == on {
		return
	}
	iv.full = on
	vtui.FrameManager.HideBars = on
	if iv.conW > 0 && iv.conH > 0 {
		iv.ResizeConsole(iv.conW, iv.conH)
	}
}

// ToggleOverlay shows or hides the panel that describes the picture.
func (iv *ImageView) ToggleOverlay() {
	iv.overlay = !iv.overlay
	if iv.overlay {
		iv.requestFileSize()
	}
}

// requestFileSize asks the file system how big the file is. Stat can be a
// network round trip on a remote file system, so it happens off the drawing
// path, once, and only for a reader who has actually opened the overlay.
func (iv *ImageView) requestFileSize() {
	if iv.sizeKnown || iv.vfs == nil {
		return
	}
	v, path, gen := iv.vfs, iv.path, iv.loadGen
	vtui.RunAsync(func(ctx *vtui.TaskContext) {
		item, err := v.Stat(ctx.Context, path)
		if err != nil {
			return
		}
		ctx.RunOnUI(func() {
			if gen == iv.loadGen {
				iv.fileSize, iv.sizeKnown = item.Size, true
			}
		})
	})
}

// imageOrientationLabel names a turn and a mirroring, and says nothing at all
// about a picture that is seen exactly as it was decoded.
func imageOrientationLabel(rotation int, flipH, flipV bool) string {
	var parts []string
	if rotation != 0 {
		parts = append(parts, fmt.Sprintf("%d°", rotation))
	}
	if flipH {
		parts = append(parts, "mirror H")
	}
	if flipV {
		parts = append(parts, "mirror V")
	}
	return strings.Join(parts, ", ")
}

// overlayLines is what the info panel has to say about the picture.
func (iv *ImageView) overlayLines() []string {
	name := filepath.Base(iv.path)
	if iv.vfs != nil {
		name = iv.vfs.Base(iv.path)
	}

	img := iv.display()
	scale := iv.lastScale
	if scale <= 0 {
		scale = iv.zoom
	}

	size := "unknown size"
	if iv.sizeKnown {
		size = formatSize(iv.fileSize)
	}

	lines := []string{
		name,
		fmt.Sprintf("%dx%d", img.Width, img.Height),
		size,
		iv.decoder,
		fmt.Sprintf("%d%%", int(scale*100+0.5)),
	}
	if label := imageOrientationLabel(iv.rotation, iv.flipH, iv.flipV); label != "" {
		lines = append(lines, label)
	}
	return lines
}

// drawOverlay writes the info panel over the left edge of the picture.
func (iv *ImageView) drawOverlay(scr *vtui.ScreenBuf) {
	lines := iv.overlayLines()
	if scr == nil || len(lines) == 0 {
		return
	}
	x1, y1, x2, y2 := iv.GetPosition()
	top := y1 + iv.barHeight()

	width := 0
	for _, s := range lines {
		if w := runewidth.StringWidth(s); w > width {
			width = w
		}
	}
	width += 2
	if limit := x2 - x1 + 1; width > limit {
		width = limit
	}
	if width <= 0 {
		return
	}

	for i, line := range lines {
		row := top + i
		if row > y2 {
			break
		}
		text := runewidth.Truncate(" "+line, width, "…")
		if w := runewidth.StringWidth(text); w < width {
			text += strings.Repeat(" ", width-w)
		}
		scr.Write(x1, row, vtui.StringToCharInfo(text, imageOverlayAttr()))
	}
}

func (iv *ImageView) Show(scr *vtui.ScreenBuf) {
	iv.ScreenObject.Show(scr)
	if iv.topBar != nil {
		iv.topBar.SetVisible(!iv.full)
		if !iv.full {
			iv.topBar.Show(scr)
		}
	}

	x1, y1, x2, y2 := iv.GetPosition()
	top := y1 + iv.barHeight()
	scr.FillRect(x1, top, x2, y2, ' ', imageViewBackAttr())
	if iv.gal != nil {
		iv.showGallery(scr)
		return
	}

	p, ok := iv.placementFor(scr)
	if !ok {
		return
	}
	if !scr.SupportsGraphics() {
		msg := "This backend cannot display images."
		x := x1 + (x2-x1+1-len(msg))/2
		if x < x1 {
			x = x1
		}
		scr.Write(x, (top+y2)/2, vtui.StringToCharInfo(msg, imageViewBackAttr()))
		return
	}
	scr.Graphics().DrawImage(iv.gfxKey, p)
	iv.logGeometry(scr, p)
	if iv.overlay {
		iv.drawOverlay(scr)
	}
}

func (iv *ImageView) ProcessKey(e *vtinput.InputEvent) bool {
	if e == nil || !e.KeyDown {
		return false
	}
	if iv.gal != nil && iv.galleryKey(e) {
		return true
	}

	ctrl := (e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
	if ctrl {
		switch e.VirtualKeyCode {
		case vtinput.VK_R:
			iv.Reload()
			return true
		case vtinput.VK_F:
			iv.SetFullScreen(!iv.full)
			return true
		case vtinput.VK_I:
			iv.ToggleOverlay()
			return true
		case vtinput.VK_S:
			iv.ToggleSlideShow()
			return true
		}
		return false
	}

	alt := (e.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0
	if alt {
		switch e.Char {
		case '>', '.':
			iv.Flip(true, false)
			return true
		case '<', ',':
			iv.Flip(false, true)
			return true
		}
		return false
	}

	switch e.Char {
	case '+', '=', 'e', 'E':
		iv.SetZoom(iv.zoom * 1.25)
		return true
	case '-', '_', 'q', 'Q':
		iv.SetZoom(iv.zoom / 1.25)
		return true
	case '*', '0':
		iv.ToggleActualSize()
		return true
	case '>', '.':
		iv.Rotate(90)
		return true
	case '<', ',':
		iv.Rotate(-90)
		return true
	case ' ':
		iv.Step(1)
		return true
	case 'f', 'F':
		iv.SetFullScreen(!iv.full)
		return true
	case 'i', 'I':
		iv.ToggleOverlay()
		return true
	case 'a', 'A':
		iv.Pan(-1, 0)
		return true
	case 'd', 'D':
		iv.Pan(1, 0)
		return true
	case 'w', 'W':
		iv.Pan(0, -1)
		return true
	case 's', 'S':
		iv.Pan(0, 1)
		return true
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_ESCAPE, vtinput.VK_F10:
		iv.Close()
		return true
	case vtinput.VK_NEXT:
		iv.Step(1)
		return true
	case vtinput.VK_PRIOR, vtinput.VK_BACK:
		iv.Step(-1)
		return true
	case vtinput.VK_HOME:
		iv.GoTo(0)
		return true
	case vtinput.VK_END:
		iv.GoTo(len(iv.siblings) - 1)
		return true
	case vtinput.VK_TAB:
		iv.ToggleActualSize()
		return true
	case vtinput.VK_F12:
		iv.ToggleGallery()
		return true
	case vtinput.VK_INSERT:
		iv.SetSelected(iv.path, !iv.selected[iv.path])
		iv.Step(1)
		return true
	case vtinput.VK_DELETE:
		iv.SetSelected(iv.path, false)
		iv.Step(1)
		return true
	case vtinput.VK_LEFT:
		iv.arrow(-1, 0)
		return true
	case vtinput.VK_RIGHT:
		iv.arrow(1, 0)
		return true
	case vtinput.VK_UP:
		iv.arrow(0, -1)
		return true
	case vtinput.VK_DOWN:
		iv.arrow(0, 1)
		return true
	}
	return false
}

func (iv *ImageView) HandleCommand(cmd int, args any) bool {
	if cmd == vtui.CmClose {
		iv.Close()
		return true
	}
	if handleWorkspaceForkCommand(cmd, args) {
		return true
	}
	return iv.BaseFrame.HandleCommand(cmd, args)
}

func (iv *ImageView) Close() {
	// The whole screen mode is a state of the manager, not of the frame, so
	// leaving the viewer has to hand the bars back.
	iv.full = false
	iv.stopSlideShow()
	vtui.FrameManager.HideBars = false
	iv.BaseFrame.Close()
	if iv.OnClose != nil {
		iv.OnClose()
	}
}

func (iv *ImageView) GetKeyLabels() *vtui.KeySet {
	return &vtui.KeySet{
		Normal: vtui.KeyBarLabels{
			"", "", "", "", "", "", "", "", "", "Quit",
		},
		NormalIcons: vtui.KeyBarIconNames{"", "", "", "", "", "", "", "", "", "x", "", ""},
	}
}

func (iv *ImageView) GetType() vtui.FrameType { return vtui.TypeUser + 7 }

func (iv *ImageView) GetTitle() string {
	if iv.path != "" {
		return "Image: " + filepath.Base(iv.path)
	}
	return "Image"
}

func (iv *ImageView) GetWorkspaceTabTitle() string {
	if iv.path != "" {
		return filepath.Base(iv.path)
	}
	return "Image"
}

func (iv *ImageView) GetWorkspaceTabMarker() string { return "I" }
