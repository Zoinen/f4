package vtui

import (
	"net"
	"sync"
)

type QtRenderer struct {
	mu   sync.Mutex
	conn net.Conn
	send *qtMessageSender

	cursorX, cursorY int
	cursorVis        bool
	cursorShape      CursorShape
	cursorDirty      bool

	palette      [256]uint32
	paletteValid bool

	pendingPalette []uint32
	pendingFrame   map[string]any
	pendingScene   map[string]any
	closed         bool
}

func NewQtRenderer(conn net.Conn) *QtRenderer {
	return NewQtRendererWithSender(conn, &qtMessageSender{w: conn})
}

func NewQtRendererWithSender(conn net.Conn, sender *qtMessageSender) *QtRenderer {
	return &QtRenderer{conn: conn, send: sender, cursorDirty: true}
}

func (r *QtRenderer) SetPalette(pal *[256]uint32) {
	if pal == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	changed := !r.paletteValid
	if !changed {
		for i, c := range pal {
			if r.palette[i] != c {
				changed = true
				break
			}
		}
	}
	if !changed {
		return
	}

	r.palette = *pal
	r.paletteValid = true
	colors := make([]uint32, len(pal))
	copy(colors, pal[:])
	r.pendingPalette = colors
}

func (r *QtRenderer) SetCursor(x, y int, visible bool, shape CursorShape) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cursorX == x && r.cursorY == y && r.cursorVis == visible && r.cursorShape == shape {
		return
	}
	r.cursorX, r.cursorY = x, y
	r.cursorVis = visible
	r.cursorShape = shape
	r.cursorDirty = true
}

func (r *QtRenderer) Render(buf, shadow []CharInfo, width, height int, forceRedraw bool) {
	if width <= 0 || height <= 0 || len(buf) == 0 {
		return
	}

	needsRedraw := forceRedraw
	if !needsRedraw {
		for i := 0; i < width*height && i < len(buf) && i < len(shadow); i++ {
			if buf[i] != shadow[i] {
				needsRedraw = true
				break
			}
		}
	}
	if !needsRedraw {
		return
	}

	limit := width * height
	if limit > len(buf) {
		limit = len(buf)
	}
	cells := make([][3]uint64, 0, limit)
	if forceRedraw || len(shadow) < limit {
		for i := 0; i < limit; i++ {
			c := buf[i]
			cells = append(cells, [3]uint64{uint64(i), c.Char, c.Attributes})
		}
	} else {
		for i := 0; i < limit; i++ {
			c := buf[i]
			if c != shadow[i] {
				cells = append(cells, [3]uint64{uint64(i), c.Char, c.Attributes})
			}
		}
	}

	r.mu.Lock()
	r.pendingFrame = map[string]any{
		"type":   "frame",
		"width":  width,
		"height": height,
		"full":   forceRedraw || len(shadow) < limit,
		"cells":  cells,
	}
	r.mu.Unlock()
}

func (r *QtRenderer) SetSemanticScene(scene map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pendingScene = scene
}

func (r *QtRenderer) Flush() {
	var messages []map[string]any

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	if r.pendingPalette != nil {
		messages = append(messages, map[string]any{
			"type":   "palette",
			"colors": r.pendingPalette,
		})
		r.pendingPalette = nil
	}
	if r.pendingFrame != nil {
		messages = append(messages, r.pendingFrame)
		r.pendingFrame = nil
	}
	if r.pendingScene != nil {
		messages = append(messages, r.pendingScene)
		r.pendingScene = nil
	}
	if r.cursorDirty {
		messages = append(messages, map[string]any{
			"type":    "cursor",
			"x":       r.cursorX,
			"y":       r.cursorY,
			"visible": r.cursorVis,
			"shape":   int(r.cursorShape),
		})
		r.cursorDirty = false
	}
	r.mu.Unlock()

	for _, msg := range messages {
		if err := r.send.Send(msg); err != nil {
			DebugLog("QT_RENDERER: send failed: %v", err)
			r.mu.Lock()
			r.closed = true
			r.mu.Unlock()
			return
		}
	}
}
