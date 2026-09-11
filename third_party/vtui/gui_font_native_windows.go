//go:build windows

package vtui

import (
	"image"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const (
	ggoGray8Bitmap     = 6
	defaultCharSet     = 1
	antialiasedQuality = 4
)

var (
	procCreateFontWNative = gdi32DLL.NewProc("CreateFontW")
	procGetGlyphOutlineW  = gdi32DLL.NewProc("GetGlyphOutlineW")
)

type win32Fixed struct {
	fract int16
	value int16
}

type win32Mat2 struct {
	eM11 win32Fixed
	eM12 win32Fixed
	eM21 win32Fixed
	eM22 win32Fixed
}

type win32GlyphPoint struct {
	x int32
	y int32
}

type win32GlyphMetrics struct {
	blackBoxX uint32
	blackBoxY uint32
	origin    win32GlyphPoint
	cellIncX  int16
	cellIncY  int16
}

// win32NativeFace keeps the x/image face for ordinary glyphs and asks GDI for
// glyphs that x/image can identify but cannot rasterize, such as Syriac glyphs
// in Segoe UI Symbol. This is deliberately a per-font fallback: the regular
// code path remains unchanged on every other platform and for every glyph that
// x/image already renders.
type win32NativeFace struct {
	base   font.Face
	dc     uintptr
	font   uintptr
	oldObj uintptr
	mu     sync.Mutex
}

func wrapGUIFace(path string, face font.Face, size, dpi float64) font.Face {
	family := guiFontFamilyForPath(path)
	if family == "" {
		return face
	}
	familyPtr, err := syscall.UTF16PtrFromString(family)
	if err != nil {
		return face
	}
	height := int(size * dpi / 72.0)
	if height < 1 {
		height = 1
	}
	dc, _, _ := procCreateCompatDC.Call(0)
	if dc == 0 {
		return face
	}
	hfont, _, _ := procCreateFontWNative.Call(
		uintptr(-height), 0, 0, 0, 400, 0, 0, 0,
		defaultCharSet, 0, 0, antialiasedQuality, 0,
		uintptr(unsafe.Pointer(familyPtr)),
	)
	if hfont == 0 {
		procDeleteDC.Call(dc)
		return face
	}
	oldObj, _, _ := procSelectObject.Call(dc, hfont)
	if oldObj == 0 {
		procDeleteObject.Call(hfont)
		procDeleteDC.Call(dc)
		return face
	}
	return &win32NativeFace{base: face, dc: dc, font: hfont, oldObj: oldObj}
}

func guiFontFamilyForPath(path string) string {
	switch strings.ToLower(filepath.Base(path)) {
	case "seguisym.ttf":
		return "Segoe UI Symbol"
	case "estre.ttf":
		return "Estrangelo Edessa"
	case "ebrima.ttf":
		return "Ebrima"
	case "nirmala.ttf":
		return "Nirmala UI"
	case "mangal.ttf":
		return "Mangal"
	case "segoeui.ttf":
		return "Segoe UI"
	default:
		return ""
	}
}

func (f *win32NativeFace) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.oldObj != 0 {
		procSelectObject.Call(f.dc, f.oldObj)
		f.oldObj = 0
	}
	if f.font != 0 {
		procDeleteObject.Call(f.font)
		f.font = 0
	}
	if f.dc != 0 {
		procDeleteDC.Call(f.dc)
		f.dc = 0
	}
	return f.base.Close()
}

func (f *win32NativeFace) Metrics() font.Metrics { return f.base.Metrics() }

func (f *win32NativeFace) Kern(r0, r1 rune) fixed.Int26_6 { return f.base.Kern(r0, r1) }

func (f *win32NativeFace) GlyphAdvance(r rune) (fixed.Int26_6, bool) {
	advance, ok := f.base.GlyphAdvance(r)
	if ok {
		return advance, true
	}
	_, _, advance, ok = f.nativeGlyph(r)
	return advance, ok
}

func (f *win32NativeFace) GlyphBounds(r rune) (fixed.Rectangle26_6, fixed.Int26_6, bool) {
	bounds, advance, ok := f.base.GlyphBounds(r)
	if ok {
		return bounds, advance, true
	}
	dr, _, advance, ok := f.nativeGlyph(r)
	if !ok {
		return fixed.Rectangle26_6{}, 0, false
	}
	return fixed.Rectangle26_6{
		Min: fixed.P(dr.Min.X, dr.Min.Y),
		Max: fixed.P(dr.Max.X, dr.Max.Y),
	}, advance, true
}

func (f *win32NativeFace) Glyph(dot fixed.Point26_6, r rune) (image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	dr, mask, maskp, advance, ok := f.base.Glyph(dot, r)
	if ok {
		return dr, mask, maskp, advance, true
	}
	nativeDr, nativeMask, advance, ok := f.nativeGlyph(r)
	if !ok {
		return image.Rectangle{}, nil, image.Point{}, 0, false
	}
	nativeDr = nativeDr.Add(image.Pt(dot.X.Floor(), dot.Y.Floor()))
	return nativeDr, nativeMask, image.Point{}, advance, true
}

func (f *win32NativeFace) nativeGlyph(r rune) (image.Rectangle, image.Image, fixed.Int26_6, bool) {
	if r < 0 || r > 0xffff {
		return image.Rectangle{}, nil, 0, false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dc == 0 || f.font == 0 {
		return image.Rectangle{}, nil, 0, false
	}

	var metrics win32GlyphMetrics
	mat := win32Mat2{
		eM11: win32Fixed{value: 1},
		eM22: win32Fixed{value: 1},
	}
	bufferSize, _, _ := procGetGlyphOutlineW.Call(
		f.dc, uintptr(uint32(r)), ggoGray8Bitmap,
		uintptr(unsafe.Pointer(&metrics)), 0, 0,
		uintptr(unsafe.Pointer(&mat)),
	)
	if bufferSize == 0 || bufferSize == ^uintptr(0) || metrics.blackBoxX == 0 || metrics.blackBoxY == 0 {
		return image.Rectangle{}, nil, 0, false
	}
	data := make([]byte, bufferSize)
	got, _, _ := procGetGlyphOutlineW.Call(
		f.dc, uintptr(uint32(r)), ggoGray8Bitmap,
		uintptr(unsafe.Pointer(&metrics)), bufferSize,
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(&mat)),
	)
	if got == ^uintptr(0) {
		return image.Rectangle{}, nil, 0, false
	}

	width, height := int(metrics.blackBoxX), int(metrics.blackBoxY)
	mask := image.NewAlpha(image.Rect(0, 0, width, height))
	stride := (width + 3) &^ 3
	for y := 0; y < height; y++ {
		srcY := height - 1 - y
		for x := 0; x < width; x++ {
			value := data[srcY*stride+x]
			mask.Pix[y*mask.Stride+x] = value * 255 / 64
		}
	}
	dr := image.Rect(int(metrics.origin.x), -int(metrics.origin.y),
		int(metrics.origin.x)+width, -int(metrics.origin.y)+height)
	return dr, mask, fixed.I(int(metrics.cellIncX)), true
}
