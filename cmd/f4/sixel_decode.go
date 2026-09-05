package main

// Receiving half of the DEC sixel graphics protocol: a program running inside
// the built-in terminal can print a sixel image and see it, the same way it
// would in xterm, foot or Windows Terminal.
//
// This file owns the decoding only — turning the body of a DCS ... q sequence
// into pixels. Where the picture lands on the grid, and where the cursor ends
// up afterwards, is the business of sixel_terminal.go.
//
// The colour registers are resolved at the moment they are used rather than at
// the end of the sequence, which is what Windows Terminal does and what makes
// full colour possible at all: an encoder that has only 256 registers can
// still paint an unlimited number of colours by redefining a register between
// bands, and every band then keeps the colour it was drawn with.

import (
	"fmt"

	"github.com/unxed/vtui"
)

const (
	// sixelMaxSide and sixelMaxPixels bound the raster a client may ask us
	// to allocate. They match the caps the kitty receiver uses, so neither
	// protocol is a cheaper way of exhausting memory than the other.
	sixelMaxSide   = 65535
	sixelMaxPixels = 16 << 20

	// sixelMaxRegister is the highest colour register number accepted. The
	// palette is grown on demand, so an image that uses sixteen registers
	// never pays for the rest.
	sixelMaxRegister = 65535

	// sixelMaxRepeat caps one ! run. A run longer than the widest raster we
	// would accept cannot draw anything more.
	sixelMaxRepeat = sixelMaxSide

	// sixelMaxAspect caps the vertical pixel multiplier. Without a cap a
	// two byte aspect ratio would turn a handful of bands into a raster
	// taller than the pixel budget.
	sixelMaxAspect = 10
)

// sixelDefaultPalette is the VT340 colour map, in the percentages the
// hardware used. Registers above the sixteenth repeat it, which is what
// xterm does; the protocol leaves an undefined register undefined, and
// repeating at least keeps such an image visible instead of black on black.
var sixelDefaultPalette = [16][3]int{
	{0, 0, 0}, {20, 20, 80}, {80, 13, 13}, {20, 80, 20},
	{80, 20, 80}, {20, 80, 80}, {80, 80, 20}, {53, 53, 53},
	{26, 26, 26}, {33, 33, 60}, {60, 26, 26}, {33, 60, 33},
	{60, 33, 60}, {33, 60, 60}, {60, 60, 33}, {80, 80, 80},
}

// sixelImage is the outcome of one sixel sequence.
type sixelImage struct {
	Surface *vtui.ImageSurface

	// EndY is where the sixel active position finished, in pixels from the
	// top of the image. It can be past the bottom of the picture, because
	// the graphics new line at the end of a dump moves it there, and the
	// cursor rule needs to know: that trailing '-' is exactly what decides
	// whether the next line of text lands on the image or below it.
	EndY int
}

// sixelDecoder walks the body of one sequence. The same walk runs twice: once
// with no canvas, to learn how large the picture is, and once to paint it.
// Doing it twice costs a second pass over the data and buys a canvas that is
// allocated once, at the right size, with no restriding as the image grows.
type sixelDecoder struct {
	pal    [][3]byte
	palSet []bool
	colour [3]byte

	transparent bool
	bg          [3]byte

	// vres is how many pixel rows one sixel row covers, from the pixel
	// aspect ratio.
	vres int

	// w, h and pix are the canvas. pix is nil on the measuring pass.
	w, h int
	pix  []byte

	// x is the active position in pixels; y is the top of the current band.
	x, y int

	maxX, maxY int

	rasterW, rasterH int
	rasterSeen       bool
}

func newSixelDecoder(params []int, bg [3]byte) *sixelDecoder {
	d := &sixelDecoder{
		bg:   bg,
		vres: sixelAspect(sixelParam(params, 0)),
		pal:  make([][3]byte, 16),
	}
	d.palSet = make([]bool, 16)
	for i := range d.pal {
		d.pal[i] = sixelPercentRGB(sixelDefaultPalette[i][0], sixelDefaultPalette[i][1], sixelDefaultPalette[i][2])
		d.palSet[i] = true
	}
	d.colour = d.pal[0]

	// P2 selects what happens to the pixels the image does not paint: 1
	// leaves them alone, everything else fills them with the background.
	d.transparent = sixelParam(params, 1) == 1
	return d
}

func sixelParam(params []int, i int) int {
	if i < len(params) {
		return params[i]
	}
	return 0
}

// sixelAspect turns P1 into the vertical pixel multiplier. Every modern
// encoder overrides this with raster attributes of 1;1, but the ones that do
// not are relying on the table, and so are the hardware test files.
func sixelAspect(p1 int) int {
	switch p1 {
	case 2:
		return 5
	case 3, 4:
		return 3
	case 7, 8, 9:
		return 1
	default:
		return 2
	}
}

func sixelPercentRGB(r, g, b int) [3]byte {
	return [3]byte{sixelPercent(r), sixelPercent(g), sixelPercent(b)}
}

func sixelPercent(v int) byte {
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return byte((v*255 + 50) / 100)
}

// sixelHLS converts a colour given in DEC's HLS space. Its wheel is the
// familiar one turned by 240 degrees: hue 0 is blue, 120 red and 240 green.
func sixelHLS(h, l, s int) [3]byte {
	h = ((h % 360) + 360) % 360
	if l < 0 {
		l = 0
	}
	if l > 100 {
		l = 100
	}
	if s < 0 {
		s = 0
	}
	if s > 100 {
		s = 100
	}

	hue := float64((h+240)%360) / 360
	lum := float64(l) / 100
	sat := float64(s) / 100
	if sat == 0 {
		v := byte(lum*255 + 0.5)
		return [3]byte{v, v, v}
	}

	q := lum + sat - lum*sat
	if lum < 0.5 {
		q = lum * (1 + sat)
	}
	p := 2*lum - q
	return [3]byte{
		sixelHue(p, q, hue+1.0/3.0),
		sixelHue(p, q, hue),
		sixelHue(p, q, hue-1.0/3.0),
	}
}

func sixelHue(p, q, t float64) byte {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	var v float64
	switch {
	case t < 1.0/6.0:
		v = p + (q-p)*6*t
	case t < 1.0/2.0:
		v = q
	case t < 2.0/3.0:
		v = p + (q-p)*(2.0/3.0-t)*6
	default:
		v = p
	}
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return byte(v*255 + 0.5)
}

// decodeSixel turns the parameters and body of one DCS ... q sequence into a
// picture. bg is the colour the unpainted pixels take when the client did not
// ask for transparency.
func decodeSixel(params []int, body string, bg [3]byte) (*sixelImage, error) {
	measure := newSixelDecoder(params, bg)
	measure.scan(body)

	w, h := measure.extent()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("the image is empty")
	}
	if w > sixelMaxSide || h > sixelMaxSide || w*h > sixelMaxPixels {
		return nil, fmt.Errorf("the image is too large")
	}

	paint := newSixelDecoder(params, bg)
	paint.alloc(w, h)
	paint.scan(body)

	surf := vtui.NewImageSurfaceFromPix(w, h, w*4, paint.pix)
	if surf == nil {
		return nil, fmt.Errorf("the image is too large")
	}
	surf.Opaque = !paint.transparent
	return &sixelImage{Surface: surf, EndY: paint.y}, nil
}

// extent decides how large the canvas is. Raster attributes win when the
// client gave them: they are a promise about the size of the picture, and
// honouring them means a client that pads its last band with empty sixels
// does not end up with a taller image than it declared.
func (d *sixelDecoder) extent() (int, int) {
	w, h := d.maxX, d.maxY
	if d.rasterSeen {
		if d.rasterW > 0 {
			w = d.rasterW
		}
		if d.rasterH > 0 {
			h = d.rasterH
		}
	}
	return w, h
}

func (d *sixelDecoder) alloc(w, h int) {
	d.w, d.h = w, h
	d.pix = make([]byte, w*h*4)
	if d.transparent {
		return
	}
	for i := 0; i < len(d.pix); i += 4 {
		d.pix[i] = d.bg[0]
		d.pix[i+1] = d.bg[1]
		d.pix[i+2] = d.bg[2]
		d.pix[i+3] = 0xFF
	}
}

func (d *sixelDecoder) scan(s string) {
	repeat := 1
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '#':
			i = d.readColour(s, i+1)
		case c == '"':
			i = d.readRaster(s, i+1)
		case c == '!':
			var n int
			n, i = sixelInt(s, i+1)
			if n < 1 {
				n = 1
			}
			if n > sixelMaxRepeat {
				n = sixelMaxRepeat
			}
			repeat = n
		case c == '$':
			d.x = 0
			repeat = 1
			i++
		case c == '-':
			d.x = 0
			d.y += 6 * d.vres
			repeat = 1
			i++
		case c >= 0x3F && c <= 0x7E:
			d.put(c-0x3F, repeat)
			repeat = 1
			i++
		default:
			// Whitespace and control characters are ignored: encoders
			// wrap long lines and the wrapping is not data.
			i++
		}
	}
}

// put draws one sixel character repeat times and advances the active position.
func (d *sixelDecoder) put(bits byte, repeat int) {
	if repeat < 1 {
		repeat = 1
	}
	x0 := d.x
	d.x += repeat
	if d.x > sixelMaxSide {
		d.x = sixelMaxSide
	}
	if d.x > d.maxX {
		d.maxX = d.x
	}
	if end := d.y + 6*d.vres; end > d.maxY {
		d.maxY = end
	}
	if d.pix == nil || bits == 0 {
		return
	}

	xEnd := d.x
	if xEnd > d.w {
		xEnd = d.w
	}
	if x0 < 0 {
		x0 = 0
	}
	for bit := 0; bit < 6; bit++ {
		if bits&(1<<uint(bit)) == 0 {
			continue
		}
		for v := 0; v < d.vres; v++ {
			y := d.y + bit*d.vres + v
			if y < 0 || y >= d.h {
				continue
			}
			off := (y*d.w + x0) * 4
			for x := x0; x < xEnd; x++ {
				d.pix[off] = d.colour[0]
				d.pix[off+1] = d.colour[1]
				d.pix[off+2] = d.colour[2]
				d.pix[off+3] = 0xFF
				off += 4
			}
		}
	}
}

// readColour handles the # introducer, which both defines and selects.
func (d *sixelDecoder) readColour(s string, i int) int {
	ps, next := sixelParams(s, i)
	if len(ps) == 0 {
		return next
	}
	reg := ps[0]
	if reg < 0 || reg > sixelMaxRegister {
		reg = 0
	}
	if len(ps) >= 5 {
		switch ps[1] {
		case 1:
			d.define(reg, sixelHLS(ps[2], ps[3], ps[4]))
		case 2:
			d.define(reg, sixelPercentRGB(ps[2], ps[3], ps[4]))
		}
	}
	d.colour = d.register(reg)
	return next
}

// readRaster handles the " introducer: the pixel aspect ratio and the size of
// the picture. Only the first one sizes the canvas, because a later one
// cannot make the raster we already allocated any bigger.
func (d *sixelDecoder) readRaster(s string, i int) int {
	ps, next := sixelParams(s, i)
	if d.rasterSeen {
		return next
	}
	d.rasterSeen = true

	pan, pad := sixelParam(ps, 0), sixelParam(ps, 1)
	if pad <= 0 {
		pad = 1
	}
	if pan <= 0 {
		pan = 1
	}
	vres := (pan + pad/2) / pad
	if vres < 1 {
		vres = 1
	}
	if vres > sixelMaxAspect {
		vres = sixelMaxAspect
	}
	d.vres = vres

	d.rasterW, d.rasterH = sixelParam(ps, 2), sixelParam(ps, 3)
	if d.rasterW < 0 || d.rasterW > sixelMaxSide {
		d.rasterW = 0
	}
	if d.rasterH < 0 || d.rasterH > sixelMaxSide {
		d.rasterH = 0
	}
	return next
}

func (d *sixelDecoder) define(reg int, c [3]byte) {
	d.grow(reg)
	d.pal[reg] = c
	d.palSet[reg] = true
}

func (d *sixelDecoder) register(reg int) [3]byte {
	d.grow(reg)
	if !d.palSet[reg] {
		return sixelPercentRGB(
			sixelDefaultPalette[reg%16][0],
			sixelDefaultPalette[reg%16][1],
			sixelDefaultPalette[reg%16][2])
	}
	return d.pal[reg]
}

func (d *sixelDecoder) grow(reg int) {
	if reg < len(d.pal) {
		return
	}
	pal := make([][3]byte, reg+1)
	set := make([]bool, reg+1)
	copy(pal, d.pal)
	copy(set, d.palSet)
	d.pal, d.palSet = pal, set
}

// sixelInt reads one decimal number, returning zero when there is none.
func sixelInt(s string, i int) (int, int) {
	n := 0
	seen := false
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		if n < sixelMaxSide {
			n = n*10 + int(s[i]-'0')
		}
		seen = true
		i++
	}
	if !seen {
		return 0, i
	}
	return n, i
}

// sixelParams reads a semicolon separated list of numbers. An empty field
// counts as zero, which is what the protocol says a defaulted parameter is.
func sixelParams(s string, i int) ([]int, int) {
	var ps []int
	for {
		var n int
		n, i = sixelInt(s, i)
		ps = append(ps, n)
		if i < len(s) && s[i] == ';' {
			i++
			continue
		}
		return ps, i
	}
}
