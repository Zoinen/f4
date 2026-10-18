package vtui

import "image"

// Images follow the painter's order of the frame stack. A placement declared
// with DrawImage belongs to the frame painting it at that moment, and
// whatever is written into its cells afterwards in the same painting pass —
// a menu, a dialog and its shadow, a toast — lies on top of it. Those cells
// are recorded in a CoverMask, and the backends leave the picture out of
// them: the native ones blit around the covered cells, the terminal
// protocols get the uncovered part as a few rectangular crops of the picture
// (see splitCovered).

// CoverMask records what lies over the cells of a placement. A cell is open,
// shaded — a frame's shadow fell on it, so the picture shows there darkened —
// or covered, and then the picture does not show at all. Coordinates are
// screen cells, like the placement's own.
type CoverMask struct {
	Col, Row   int
	Cols, Rows int
	cells      []cellCover
	covered    int
	shaded     int
}

type cellCover uint8

const (
	cellOpen cellCover = iota
	cellShaded
	cellCovered
)

// ShadeKeep is how much of the picture's colour a shaded cell keeps, as a
// fraction: the 3/8 ApplyShadow applies to the text grid.
const (
	ShadeKeepNum = 3
	ShadeKeepDen = 8
)

func newCoverMask(p *ImagePlacement) *CoverMask {
	if p.Cols <= 0 || p.Rows <= 0 {
		return nil
	}
	return &CoverMask{Col: p.Col, Row: p.Row, Cols: p.Cols, Rows: p.Rows, cells: make([]cellCover, p.Cols*p.Rows)}
}

func (m *CoverMask) at(col, row int) cellCover {
	if m == nil {
		return cellOpen
	}
	x, y := col-m.Col, row-m.Row
	if x < 0 || y < 0 || x >= m.Cols || y >= m.Rows {
		return cellOpen
	}
	return m.cells[y*m.Cols+x]
}

// Covered reports whether the cell at screen position (col, row) is covered.
// A nil mask covers nothing.
func (m *CoverMask) Covered(col, row int) bool { return m.at(col, row) == cellCovered }

// Shaded reports whether a shadow fell on the cell at (col, row) and nothing
// covered it after that.
func (m *CoverMask) Shaded(col, row int) bool { return m.at(col, row) == cellShaded }

// Any reports whether at least one cell is covered or shaded.
func (m *CoverMask) Any() bool { return m != nil && m.covered+m.shaded > 0 }

// AnyCovered reports whether at least one cell is covered.
func (m *CoverMask) AnyCovered() bool { return m != nil && m.covered > 0 }

// Full reports whether every cell is covered, leaving nothing to draw.
func (m *CoverMask) Full() bool { return m != nil && m.covered == len(m.cells) }

// mark sets the cells of the screen rectangle (x1,y1)-(x2,y2) that fall
// inside the mask to state. A shadow does not lift a cover: a shadow falling
// on a menu over the picture darkens the menu, not the picture.
func (m *CoverMask) mark(x1, y1, x2, y2 int, state cellCover) {
	x1, y1 = max(x1, m.Col), max(y1, m.Row)
	x2, y2 = min(x2, m.Col+m.Cols-1), min(y2, m.Row+m.Rows-1)
	for y := y1; y <= y2; y++ {
		row := (y - m.Row) * m.Cols
		for x := x1; x <= x2; x++ {
			i := row + x - m.Col
			old := m.cells[i]
			if old == state || (old == cellCovered && state == cellShaded) {
				continue
			}
			switch old {
			case cellCovered:
				m.covered--
			case cellShaded:
				m.shaded--
			}
			switch state {
			case cellCovered:
				m.covered++
			case cellShaded:
				m.shaded++
			}
			m.cells[i] = state
		}
	}
}

func (m *CoverMask) cover(x1, y1, x2, y2 int) { m.mark(x1, y1, x2, y2, cellCovered) }

func (m *CoverMask) shade(x1, y1, x2, y2 int) { m.mark(x1, y1, x2, y2, cellShaded) }

// uncoveredRects splits the uncovered cells into rectangles: a run of
// uncovered cells per row, and runs with the same columns in consecutive rows
// merged. A menu or a dialog over a picture leaves two or three of them.
func (m *CoverMask) uncoveredRects() []image.Rectangle {
	var done, open []image.Rectangle
	for row := m.Row; row < m.Row+m.Rows; row++ {
		var runs []image.Rectangle
		for col := m.Col; col < m.Col+m.Cols; {
			if m.Covered(col, row) {
				col++
				continue
			}
			start := col
			for col < m.Col+m.Cols && !m.Covered(col, row) {
				col++
			}
			runs = append(runs, image.Rect(start, row, col, row+1))
		}
		var next []image.Rectangle
		for _, run := range runs {
			merged := false
			for i, r := range open {
				if r.Min.X == run.Min.X && r.Max.X == run.Max.X {
					r.Max.Y = run.Max.Y
					next = append(next, r)
					open = append(open[:i], open[i+1:]...)
					merged = true
					break
				}
			}
			if !merged {
				next = append(next, run)
			}
		}
		done = append(done, open...)
		open = next
	}
	return append(done, open...)
}

// maxCoverCrops bounds how many crops a covered picture is sent as; a cover
// more ragged than that holds the picture back until it clears. It also
// keeps the crop's id (k << 24 | id) clear of the placement ids.
const maxCoverCrops = 32

// splitCovered turns a partly covered placement into crops of the picture
// over its uncovered rectangles, for transports that cannot leave cells out
// of a picture. Each crop maps its cells onto the same part of the source
// the whole placement would have shown there, under an id of its own.
func splitCovered(p ImagePlacement) []ImagePlacement {
	rects := p.Covered.uncoveredRects()
	if len(rects) == 0 || len(rects) > maxCoverCrops || p.ID >= 1<<24 {
		return nil
	}
	sx, sy, sw, sh := p.Source()
	out := make([]ImagePlacement, 0, len(rects))
	for k, r := range rects {
		c := p
		c.ID = uint32(k+1)<<24 | p.ID //nolint:gosec // G115: k < maxCoverCrops
		c.Col, c.Row = r.Min.X, r.Min.Y
		c.Cols, c.Rows = r.Dx(), r.Dy()
		x0 := sx + (r.Min.X-p.Col)*sw/p.Cols
		x1 := sx + (r.Max.X-p.Col)*sw/p.Cols
		y0 := sy + (r.Min.Y-p.Row)*sh/p.Rows
		y1 := sy + (r.Max.Y-p.Row)*sh/p.Rows
		if x1 <= x0 || y1 <= y0 {
			continue
		}
		c.SrcX, c.SrcY, c.SrcW, c.SrcH = x0, y0, x1-x0, y1-y0
		c.Covered = nil
		out = append(out, c)
	}
	return out
}

func (m *CoverMask) equal(o *CoverMask) bool {
	if !m.Any() || !o.Any() {
		return m.Any() == o.Any()
	}
	if m.Col != o.Col || m.Row != o.Row || m.Cols != o.Cols || m.Rows != o.Rows || m.covered != o.covered || m.shaded != o.shaded {
		return false
	}
	for i := range m.cells {
		if m.cells[i] != o.cells[i] {
			return false
		}
	}
	return true
}

// coverCells is ScreenBuf's report that the rectangle (x1,y1)-(x2,y2) was
// just painted. Every placement declared earlier in the current pass loses
// those cells. Outside a painting pass, or before the first DrawImage of
// one, it costs a single atomic load.
func (g *GraphicsLayer) coverCells(x1, y1, x2, y2 int) { g.markCells(x1, y1, x2, y2, cellCovered) }

// shadeCells is ScreenBuf's report that a frame's shadow fell on the
// rectangle: the pictures under it show there darkened, not cut out.
func (g *GraphicsLayer) shadeCells(x1, y1, x2, y2 int) { g.markCells(x1, y1, x2, y2, cellShaded) }

func (g *GraphicsLayer) markCells(x1, y1, x2, y2 int, state cellCover) {
	if g.liveCount.Load() == 0 || x1 > x2 || y1 > y2 {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, id := range g.live {
		p := g.placementByID(id)
		if p == nil || x2 < p.Col || x1 >= p.Col+p.Cols || y2 < p.Row || y1 >= p.Row+p.Rows {
			continue
		}
		m := g.pendingCovers[id]
		if m == nil {
			if m = newCoverMask(p); m == nil {
				continue
			}
			g.pendingCovers[id] = m
		}
		m.mark(x1, y1, x2, y2, state)
	}
}

// openImageCells sets dst[x] for the cells of screen row y in which a
// placement shows its picture, that is every cell of it nothing was painted
// over, and reports whether there is one. dst is the row, one entry per
// column; the caller clears it. A text renderer leaves such a cell alone
// while its text is unchanged: the terminal shows the picture there, and
// writing the cell again would put the text background over the picture
// until the picture is sent again (f4#1885).
func (g *GraphicsLayer) openImageCells(y int, dst []bool) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	found := false
	for i := range g.placements {
		p := &g.placements[i]
		if y < p.Row || y >= p.Row+p.Rows {
			continue
		}
		m := g.covers[p.ID]
		for x := max(p.Col, 0); x < p.Col+p.Cols && x < len(dst); x++ {
			if !m.Covered(x, y) {
				dst[x] = true
				found = true
			}
		}
	}
	return found
}

func (g *GraphicsLayer) placementByID(id uint32) *ImagePlacement {
	for i := range g.placements {
		if g.placements[i].ID == id {
			return &g.placements[i]
		}
	}
	return nil
}

// commitCovers makes the masks gathered during the pass the visible ones,
// and bumps the generation when any of them changed so the backends redraw
// the pictures. Called by EndFrame with g.mu held.
func (g *GraphicsLayer) commitCovers() {
	changed := len(g.covers) != len(g.pendingCovers)
	for id, m := range g.pendingCovers {
		if !m.equal(g.covers[id]) {
			changed = true
		}
	}
	g.covers, g.pendingCovers = g.pendingCovers, g.covers
	clear(g.pendingCovers)
	if changed {
		g.gen++
	}
}
