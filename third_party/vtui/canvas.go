package vtui

// Canvas places an RGBA image surface over the terminal cell grid. It is a
// small adapter around ScreenBuf.Graphics; GUI applications use goWidgets'
// interactive Canvas API instead.
type Canvas struct {
	ScreenObject
	Surface        *ImageSurface
	placementID    uint32
	placementLayer *GraphicsLayer
}

// NewCanvas returns a terminal image canvas that expands with its container.
func NewCanvas() *Canvas {
	c := &Canvas{}
	c.SetGrowMode(GrowAll)
	return c
}

// SetSurface replaces the pixels displayed by this canvas.
func (c *Canvas) SetSurface(surface *ImageSurface) {
	if c.Surface == surface {
		return
	}
	c.Surface = surface
	c.NotifyChange()
	c.Invalidate()
}

// Invalidate marks caller-written pixels as changed and schedules the current
// graphics placement for a fresh terminal render.
func (c *Canvas) Invalidate() {
	if c.Surface != nil {
		c.Surface.Invalidate()
	}
	if c.placementLayer != nil && c.placementID != 0 {
		c.updatePlacement()
	}
}

func (c *Canvas) Show(scr *ScreenBuf) {
	c.ScreenObject.Show(scr)
	if scr == nil || c.IsLocked() {
		return
	}
	c.placementLayer = scr.Graphics()
	if c.Surface == nil || !c.Surface.Valid() {
		c.removePlacement()
		return
	}
	c.updatePlacement()
}

func (c *Canvas) Hide(scr *ScreenBuf) {
	c.ScreenObject.Hide(scr)
	if scr != nil && c.placementLayer == nil {
		c.placementLayer = scr.Graphics()
	}
	c.removePlacement()
}

func (c *Canvas) updatePlacement() {
	if c.placementLayer == nil {
		return
	}
	if c.Surface == nil || !c.Surface.Valid() {
		c.removePlacement()
		return
	}
	x1, y1, x2, y2 := c.GetPosition()
	p := ImagePlacement{
		Surface: c.Surface,
		Col:     x1, Row: y1,
		Cols: x2 - x1 + 1, Rows: y2 - y1 + 1,
		Opaque: c.Surface.Opaque,
	}
	if p.Cols <= 0 || p.Rows <= 0 {
		c.removePlacement()
		return
	}
	if c.placementID == 0 {
		c.placementID = c.placementLayer.Add(p)
		return
	}
	c.placementLayer.Update(c.placementID, func(dst *ImagePlacement) { *dst = p })
}

func (c *Canvas) removePlacement() {
	if c.placementLayer != nil && c.placementID != 0 {
		c.placementLayer.Remove(c.placementID)
	}
	c.placementID = 0
}
