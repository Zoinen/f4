package vtui

import (
	"os"
	"sort"
	"strings"
	"sync"
)

// GraphicsProtocol identifies the transport used to get pixel data onto the
// physical screen. Text backends encode pixels into escape sequences, while
// GUI backends blit them straight into their own framebuffer.
type GraphicsProtocol int

const (
	GraphicsNone GraphicsProtocol = iota
	GraphicsKitty
	GraphicsITerm2
	GraphicsSixel
	GraphicsNative
)

func (p GraphicsProtocol) String() string {
	switch p {
	case GraphicsKitty:
		return "kitty"
	case GraphicsITerm2:
		return "iterm2"
	case GraphicsSixel:
		return "sixel"
	case GraphicsNative:
		return "native"
	}
	return "none"
}

// ParseGraphicsProtocol converts a user supplied name into a protocol value.
func ParseGraphicsProtocol(s string) (GraphicsProtocol, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "kitty":
		return GraphicsKitty, true
	case "iterm", "iterm2":
		return GraphicsITerm2, true
	case "sixel":
		return GraphicsSixel, true
	case "native":
		return GraphicsNative, true
	case "none", "off", "no", "0":
		return GraphicsNone, true
	}
	return GraphicsNone, false
}

// DetectGraphicsProtocol guesses the best available protocol from the
// environment. Applications that can query the terminal directly should
// override the result with GraphicsLayer.SetProtocol.
func DetectGraphicsProtocol() GraphicsProtocol {
	return detectGraphicsProtocol(os.Getenv)
}

func detectGraphicsProtocol(env func(string) string) GraphicsProtocol {
	if forced := env("VTUI_GRAPHICS"); forced != "" {
		if p, ok := ParseGraphicsProtocol(forced); ok {
			return p
		}
	}

	// Multiplexers drop escape sequences they do not understand, and a
	// half transmitted image corrupts the whole session. Stay silent
	// unless the user explicitly opted in above.
	if env("TMUX") != "" || strings.HasPrefix(env("TERM"), "screen") {
		return GraphicsNone
	}

	term := strings.ToLower(env("TERM"))
	prog := strings.ToLower(env("TERM_PROGRAM"))

	switch {
	case env("KITTY_WINDOW_ID") != "" || strings.Contains(term, "kitty"):
		return GraphicsKitty
	case prog == "ghostty" || env("GHOSTTY_RESOURCES_DIR") != "":
		return GraphicsKitty
	case prog == "wezterm" || env("WEZTERM_PANE") != "":
		return GraphicsKitty
	case prog == "iterm.app" || env("ITERM_SESSION_ID") != "":
		return GraphicsITerm2
	case env("KONSOLE_VERSION") != "":
		return GraphicsSixel
	case strings.Contains(term, "foot") || strings.Contains(term, "mlterm") ||
		strings.Contains(term, "yaft") || strings.Contains(term, "contour") ||
		strings.Contains(term, "wayst"):
		return GraphicsSixel
	}

	return GraphicsNone
}

// ImageSurface is a plain top-down RGBA8 pixel buffer. It deliberately does
// not depend on image.Image so that the rendering layer stays free of any
// decoding concerns: decoders live in the application, vtui only ships bytes.
type ImageSurface struct {
	Width  int
	Height int
	Stride int
	Pix    []byte

	hash      uint64
	hashValid bool
}

// NewImageSurface allocates a zeroed (fully transparent) surface.
func NewImageSurface(w, h int) *ImageSurface {
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return &ImageSurface{
		Width:  w,
		Height: h,
		Stride: w * 4,
		Pix:    make([]byte, w*h*4),
	}
}

// NewImageSurfaceFromPix wraps an existing RGBA buffer without copying it.
// It returns nil when the buffer is too small for the declared geometry.
func NewImageSurfaceFromPix(w, h, stride int, pix []byte) *ImageSurface {
	if w <= 0 || h <= 0 || stride < w*4 || len(pix) < stride*(h-1)+w*4 {
		return nil
	}
	return &ImageSurface{Width: w, Height: h, Stride: stride, Pix: pix}
}

// Valid reports whether the surface can be sampled at all.
func (s *ImageSurface) Valid() bool {
	return s != nil && s.Width > 0 && s.Height > 0 && s.Stride >= s.Width*4 &&
		len(s.Pix) >= s.Stride*(s.Height-1)+s.Width*4
}

// SetPixel writes one RGBA pixel. Out of range coordinates are ignored.
func (s *ImageSurface) SetPixel(x, y int, r, g, b, a byte) {
	if !s.Valid() || x < 0 || y < 0 || x >= s.Width || y >= s.Height {
		return
	}
	o := y*s.Stride + x*4
	s.Pix[o] = r
	s.Pix[o+1] = g
	s.Pix[o+2] = b
	s.Pix[o+3] = a
	s.hashValid = false
}

// PixelAt reads one RGBA pixel; out of range coordinates read as transparent.
func (s *ImageSurface) PixelAt(x, y int) (r, g, b, a byte) {
	if !s.Valid() || x < 0 || y < 0 || x >= s.Width || y >= s.Height {
		return 0, 0, 0, 0
	}
	o := y*s.Stride + x*4
	return s.Pix[o], s.Pix[o+1], s.Pix[o+2], s.Pix[o+3]
}

// Invalidate marks the cached content hash as stale. Call it after writing
// into Pix directly instead of through SetPixel.
func (s *ImageSurface) Invalidate() {
	if s != nil {
		s.hashValid = false
	}
}

const (
	fnvOffset64 = 14695981039346656037
	fnvPrime64  = 1099511628211
)

// Hash returns a content hash used by the backends to recognise a surface
// they have already uploaded to the terminal.
func (s *ImageSurface) Hash() uint64 {
	if s == nil {
		return 0
	}
	if s.hashValid {
		return s.hash
	}
	h := uint64(fnvOffset64)
	mix := func(v uint64) {
		for i := 0; i < 8; i++ {
			h ^= (v >> (uint(i) * 8)) & 0xFF
			h *= fnvPrime64
		}
	}
	mix(uint64(s.Width))
	mix(uint64(s.Height))
	for _, b := range s.Pix {
		h ^= uint64(b)
		h *= fnvPrime64
	}
	s.hash = h
	s.hashValid = true
	return h
}

// Crop copies a rectangular region into a fresh tightly packed surface.
// The rectangle is clamped to the source bounds.
func (s *ImageSurface) Crop(x, y, w, h int) *ImageSurface {
	if !s.Valid() {
		return nil
	}
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > s.Width {
		w = s.Width - x
	}
	if y+h > s.Height {
		h = s.Height - y
	}
	if w <= 0 || h <= 0 {
		return nil
	}
	out := NewImageSurface(w, h)
	for row := 0; row < h; row++ {
		src := (y+row)*s.Stride + x*4
		dst := row * out.Stride
		copy(out.Pix[dst:dst+w*4], s.Pix[src:src+w*4])
	}
	return out
}

// ImagePlacement describes one image drawn over a rectangular block of cells.
// Geometry is expressed in cells so that the same placement works for a
// terminal protocol and for a GUI backend that knows its own cell metrics.
type ImagePlacement struct {
	ID      uint32
	Surface *ImageSurface

	Col  int
	Row  int
	Cols int
	Rows int

	// Source rectangle in pixels. A zero SrcW or SrcH means "whole surface".
	SrcX int
	SrcY int
	SrcW int
	SrcH int

	ZIndex int
	Opaque bool
}

// Source resolves the effective source rectangle, clamped to the surface.
func (p *ImagePlacement) Source() (x, y, w, h int) {
	if p == nil || !p.Surface.Valid() {
		return 0, 0, 0, 0
	}
	x, y, w, h = p.SrcX, p.SrcY, p.SrcW, p.SrcH
	if w <= 0 || h <= 0 {
		x, y = 0, 0
		w, h = p.Surface.Width, p.Surface.Height
	}
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > p.Surface.Width {
		w = p.Surface.Width - x
	}
	if y+h > p.Surface.Height {
		h = p.Surface.Height - y
	}
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return x, y, w, h
}

// CoversCell reports whether the placement paints over the given cell.
func (p *ImagePlacement) CoversCell(col, row int) bool {
	if p == nil {
		return false
	}
	return col >= p.Col && col < p.Col+p.Cols && row >= p.Row && row < p.Row+p.Rows
}

// GraphicsLayer holds every image currently shown on top of the text grid.
// It is owned by a ScreenBuf and is safe for concurrent use: decoders often
// finish on a worker goroutine while the UI thread is flushing a frame.
type GraphicsLayer struct {
	mu         sync.Mutex
	placements []ImagePlacement
	gen        uint64
	nextID     uint32

	protocol      GraphicsProtocol
	protocolValid bool

	cellW int
	cellH int

	repaint bool

	frameMode bool
	seen      map[string]bool
	byKey     map[string]uint32
}

// Protocol returns the active protocol, detecting it on first use.
func (g *GraphicsLayer) Protocol() GraphicsProtocol {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.protocolValid {
		g.protocol = DetectGraphicsProtocol()
		g.protocolValid = true
	}
	return g.protocol
}

// SetProtocol overrides the detected protocol.
func (g *GraphicsLayer) SetProtocol(p GraphicsProtocol) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.protocolValid && g.protocol == p {
		return
	}
	g.protocol = p
	g.protocolValid = true
	g.gen++
	g.repaint = true
}

// Supported reports whether images can be displayed at all.
func (g *GraphicsLayer) Supported() bool {
	return g.Protocol() != GraphicsNone
}

// SetCellSize records the pixel size of one character cell. Backends need it
// to convert a desired pixel size into a cell rectangle.
func (g *GraphicsLayer) SetCellSize(w, h int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	if g.cellW == w && g.cellH == h {
		return
	}
	g.cellW, g.cellH = w, h
	g.gen++
}

// CellSize returns the pixel size of one character cell, or zeroes if unknown.
func (g *GraphicsLayer) CellSize() (int, int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.cellW, g.cellH
}

// Add registers a new placement and returns its identifier.
func (g *GraphicsLayer) Add(p ImagePlacement) uint32 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nextID++
	p.ID = g.nextID
	g.placements = append(g.placements, p)
	g.gen++
	return p.ID
}

// Update mutates an existing placement in place.
func (g *GraphicsLayer) Update(id uint32, mutate func(*ImagePlacement)) bool {
	if mutate == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for i := range g.placements {
		if g.placements[i].ID == id {
			mutate(&g.placements[i])
			g.placements[i].ID = id
			g.repaint = true
			g.gen++
			return true
		}
	}
	return false
}

// Remove drops a single placement.
func (g *GraphicsLayer) Remove(id uint32) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for i := range g.placements {
		if g.placements[i].ID == id {
			g.placements = append(g.placements[:i], g.placements[i+1:]...)
			g.gen++
			g.repaint = true
			return true
		}
	}
	return false
}

// Clear removes every placement.
func (g *GraphicsLayer) Clear() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.placements) == 0 {
		return
	}
	g.placements = g.placements[:0]
	g.gen++
	g.repaint = true
}

// Len returns the number of placements.
func (g *GraphicsLayer) Len() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.placements)
}

// Generation is bumped on every observable change and lets renderers skip
// work when nothing moved since the previous frame.
func (g *GraphicsLayer) Generation() uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.gen
}

// TakeRepaintRequest reports, and clears, a pending request to repaint the
// text under the images. A placement that moved or disappeared leaves the
// pixels of the previous frame behind, and only a full redraw clears them.
func (g *GraphicsLayer) TakeRepaintRequest() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.repaint {
		return false
	}
	g.repaint = false
	return true
}

// Invalidate forces the next frame to re-emit everything, for example after
// the terminal has been reset or re-attached.
func (g *GraphicsLayer) Invalidate() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gen++
}

// Snapshot copies the placements into dst (which may be reused between
// frames) sorted back to front, and returns the current generation.
func (g *GraphicsLayer) Snapshot(dst []ImagePlacement) ([]ImagePlacement, uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	dst = append(dst[:0], g.placements...)
	sort.SliceStable(dst, func(i, j int) bool {
		if dst[i].ZIndex != dst[j].ZIndex {
			return dst[i].ZIndex < dst[j].ZIndex
		}
		return dst[i].ID < dst[j].ID
	})
	return dst, g.gen
}

// BeginFrame starts an immediate mode painting pass: every keyed placement is
// marked stale, and only the ones re-declared through DrawImage survive
// EndFrame. This is what ties an image to the frame that paints it, so a
// window that is not drawn cannot leave its picture on the screen.
func (g *GraphicsLayer) BeginFrame() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.frameMode = true
	if g.seen == nil {
		g.seen = make(map[string]bool)
	}
	for k := range g.seen {
		delete(g.seen, k)
	}
}

// DrawImage declares an image for the current painting pass. The key
// identifies the owner, so the same caller keeps the same placement across
// frames and only its geometry changes. An unchanged declaration costs
// nothing: the generation is bumped only when something really moved.
func (g *GraphicsLayer) DrawImage(key string, p ImagePlacement) uint32 {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.seen == nil {
		g.seen = make(map[string]bool)
	}
	if g.byKey == nil {
		g.byKey = make(map[string]uint32)
	}
	g.seen[key] = true

	if id, ok := g.byKey[key]; ok {
		for i := range g.placements {
			if g.placements[i].ID == id {
				p.ID = id
				if g.placements[i] != p {
					g.placements[i] = p
					g.gen++
					g.repaint = true
				}
				return id
			}
		}
	}

	g.nextID++
	p.ID = g.nextID
	g.placements = append(g.placements, p)
	g.byKey[key] = p.ID
	g.gen++
	return p.ID
}

// EndFrame drops every keyed placement that was not re-declared during the
// pass. Placements added through Add are untouched.
func (g *GraphicsLayer) EndFrame() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.frameMode {
		return
	}
	g.frameMode = false

	for key, id := range g.byKey {
		if g.seen[key] {
			continue
		}
		delete(g.byKey, key)
		for i := range g.placements {
			if g.placements[i].ID == id {
				g.placements = append(g.placements[:i], g.placements[i+1:]...)
				g.gen++
				g.repaint = true
				break
			}
		}
	}
}

// DirtyUnder reports whether the text under any placement was repainted in
// this frame. Terminal protocols draw images above the cell grid, so the
// image has to be sent again whenever its background changed.
func (g *GraphicsLayer) DirtyUnder(buf, shadow []CharInfo, width, height int) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.placements) == 0 || width <= 0 || height <= 0 {
		return false
	}
	if len(buf) < width*height || len(shadow) < width*height {
		return false
	}
	for i := range g.placements {
		p := &g.placements[i]
		for row := p.Row; row < p.Row+p.Rows; row++ {
			if row < 0 || row >= height {
				continue
			}
			for col := p.Col; col < p.Col+p.Cols; col++ {
				if col < 0 || col >= width {
					continue
				}
				idx := row*width + col
				if buf[idx] != shadow[idx] {
					return true
				}
			}
		}
	}
	return false
}

// GraphicsRenderer is the optional half of SurfaceRenderer that knows how to
// put pixels on screen. Renderers without image support simply do not
// implement it and the layer stays invisible.
type GraphicsRenderer interface {
	RenderGraphics(layer *GraphicsLayer, buf, shadow []CharInfo, width, height int, forceRedraw bool)
}

// Graphics returns the image layer attached to this screen buffer.
func (s *ScreenBuf) Graphics() *GraphicsLayer {
	return &s.graphics
}

// SupportsGraphics reports whether the active renderer can display images.
func (s *ScreenBuf) SupportsGraphics() bool {
	s.mu.Lock()
	r := s.Renderer
	s.mu.Unlock()
	if _, ok := r.(GraphicsRenderer); !ok {
		return false
	}
	return s.graphics.Supported()
}
