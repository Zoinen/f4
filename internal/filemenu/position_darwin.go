package filemenu

import "github.com/ebitengine/purego/objc"

type cocoaSize struct{ Width, Height float64 }
type cocoaRect struct {
	Origin cocoaPoint
	Size   cocoaSize
}

// PanelPoint uses an existing Cocoa host window. Terminal-mode F4 has no such
// window and falls back to its cell-positioned menu without loading AppKit.
func PanelPoint(x, y, cols, rows int) Point {
	class := objc.GetClass("NSApplication")
	if class == 0 || cols <= 0 || rows <= 0 {
		return Point{}
	}
	if objc.ID(objc.GetClass("NSThread")).Send(objc.RegisterName("isMainThread")) == 0 {
		return Point{}
	}
	app := objc.ID(class).Send(objc.RegisterName("sharedApplication"))
	window := app.Send(objc.RegisterName("keyWindow"))
	if window == 0 {
		return Point{}
	}
	view := window.Send(objc.RegisterName("contentView"))
	if view == 0 {
		return Point{}
	}
	bounds := objc.Send[cocoaRect](view, objc.RegisterName("bounds"))
	if bounds.Size.Width <= 0 || bounds.Size.Height <= 0 {
		return Point{}
	}
	point := cocoaPoint{X: bounds.Origin.X + float64(x)*bounds.Size.Width/float64(cols), Y: float64(y) * bounds.Size.Height / float64(rows)}
	if view.Send(objc.RegisterName("isFlipped")) == 0 {
		point.Y = bounds.Size.Height - point.Y
	}
	point.Y += bounds.Origin.Y
	point = objc.Send[cocoaPoint](view, objc.RegisterName("convertPoint:toView:"), point, objc.ID(0))
	point = objc.Send[cocoaPoint](window, objc.RegisterName("convertPointToScreen:"), point)
	return Point{X: int(point.X), Y: int(point.Y), Valid: true}
}
