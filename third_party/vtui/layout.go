package vtui

// Alignment defines how an element is positioned within its layout container.
type Alignment int

const (
	AlignLeft Alignment = iota
	AlignCenter
	AlignRight
	AlignFill // Stretches the element to fill the available space
	AlignTop
	AlignBottom
)

// Margins defines the spacing around a layout element.
type Margins struct {
	Left, Top, Right, Bottom int
}

// LayoutItem binds a UIElement to its layout constraints.
type LayoutItem struct {
	Element UIElement
	Margins Margins
	Align   Alignment
}

// VBoxLayout stacks elements vertically.
type VBoxLayout struct {
	ScreenObject
	X, Y, W, H int
	Items      []LayoutItem
}

// NewVBoxLayout creates a new vertical layout manager.
func NewVBoxLayout(x, y, w, h int) *VBoxLayout {
	v := &VBoxLayout{X: x, Y: y, W: w, H: h}
	v.SetPosition(x, y, x+w-1, y+h-1)
	return v
}

func (v *VBoxLayout) SetPosition(x1, y1, x2, y2 int) {
	v.ScreenObject.SetPosition(x1, y1, x2, y2)
	v.X, v.Y = x1, y1
	v.W, v.H = x2-x1+1, y2-y1+1
}

func (v *VBoxLayout) MoveRelative(dx, dy int) {
	v.ScreenObject.MoveRelative(dx, dy)
	v.X += dx
	v.Y += dy
}

func (v *VBoxLayout) Show(scr *ScreenBuf) {} // Invisible container

// Add appends a UIElement to the vertical layout.
func (v *VBoxLayout) Add(el UIElement, m Margins, align Alignment) {
	v.Items = append(v.Items, LayoutItem{Element: el, Margins: m, Align: align})
}

// Apply calculates and sets the coordinates for all added elements.
func (v *VBoxLayout) Apply() {
	currY := v.Y
	for _, itm := range v.Items {
		currY += itm.Margins.Top
		ix1, iy1, ix2, iy2 := itm.Element.GetPosition()
		iw := ix2 - ix1 + 1
		ih := iy2 - iy1 + 1

		var finalX, finalW int
		switch itm.Align {
		case AlignFill:
			finalX = v.X + itm.Margins.Left
			finalW = v.W - itm.Margins.Left - itm.Margins.Right
		case AlignCenter:
			finalW = iw
			finalX = v.X + (v.W-finalW)/2
		case AlignRight:
			finalW = iw
			finalX = v.X + v.W - itm.Margins.Right - finalW
		case AlignLeft:
			fallthrough
		default:
			finalW = iw
			finalX = v.X + itm.Margins.Left
		}

		itm.Element.SetPosition(finalX, currY, finalX+finalW-1, currY+ih-1)

		// If child is also a layout, trigger its recursive apply
		if sub, ok := itm.Element.(interface{ Apply() }); ok {
			sub.Apply()
		}

		currY += ih + itm.Margins.Bottom
	}
}

// HBoxLayout stacks elements horizontally.
type HBoxLayout struct {
	ScreenObject
	X, Y, W, H      int
	Items           []LayoutItem
	HorizontalAlign Alignment
	Spacing         int
}

// NewHBoxLayout creates a new horizontal layout manager.
func NewHBoxLayout(x, y, w, h int) *HBoxLayout {
	v := &HBoxLayout{X: x, Y: y, W: w, H: h, HorizontalAlign: AlignLeft, Spacing: 1}
	v.SetPosition(x, y, x+w-1, y+h-1)
	return v
}

func (h *HBoxLayout) SetPosition(x1, y1, x2, y2 int) {
	h.ScreenObject.SetPosition(x1, y1, x2, y2)
	h.X, h.Y = x1, y1
	h.W, h.H = x2-x1+1, y2-y1+1
}

func (h *HBoxLayout) MoveRelative(dx, dy int) {
	h.ScreenObject.MoveRelative(dx, dy)
	h.X += dx
	h.Y += dy
}

func (h *HBoxLayout) Show(scr *ScreenBuf) {} // Invisible container

// Add appends a UIElement to the horizontal layout.
func (h *HBoxLayout) Add(el UIElement, m Margins, align Alignment) {
	h.Items = append(h.Items, LayoutItem{Element: el, Margins: m, Align: align})
}

// Apply calculates and sets the coordinates for all added elements.
func (h *HBoxLayout) Apply() {
	totalW := 0
	for _, itm := range h.Items {
		ix1, _, ix2, _ := itm.Element.GetPosition()
		totalW += itm.Margins.Left + (ix2 - ix1 + 1) + itm.Margins.Right
	}
	if len(h.Items) > 1 {
		totalW += (len(h.Items) - 1) * h.Spacing
	}

	currX := h.X
	switch h.HorizontalAlign {
	case AlignCenter:
		currX = h.X + (h.W-totalW)/2
	case AlignRight:
		currX = h.X + h.W - totalW
	}

	for _, itm := range h.Items {
		currX += itm.Margins.Left
		ix1, iy1, ix2, iy2 := itm.Element.GetPosition()
		iw := ix2 - ix1 + 1
		ih := iy2 - iy1 + 1

		var finalY, finalH int
		switch itm.Align {
		case AlignFill:
			finalY = h.Y + itm.Margins.Top
			finalH = h.H - itm.Margins.Top - itm.Margins.Bottom
		case AlignCenter:
			finalH = ih
			finalY = h.Y + (h.H-finalH)/2
		case AlignBottom:
			finalH = ih
			finalY = h.Y + h.H - itm.Margins.Bottom - finalH
		case AlignTop:
			fallthrough
		default:
			finalH = ih
			finalY = h.Y + itm.Margins.Top
		}

		itm.Element.SetPosition(currX, finalY, currX+iw-1, finalY+finalH-1)

		// If child is also a layout, trigger its recursive apply
		if sub, ok := itm.Element.(interface{ Apply() }); ok {
			sub.Apply()
		}

		currX += iw + itm.Margins.Right + h.Spacing
	}
}
