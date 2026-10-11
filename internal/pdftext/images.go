package pdftext

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"sort"
)

// Limits on the pictures taken out of one file.
const (
	maxImages      = 500
	maxImagePixels = 100 << 20
	maxImageSide   = 30000
)

// Image is a picture embedded in a PDF, ready to be saved or shown as a file
// of the given extension.
type Image struct {
	Page   int // 1-based page it was found on
	Name   string
	Ext    string // "jpg", "jp2" or "png"
	Data   []byte
	Width  int
	Height int
}

// ExtractImages returns the embedded pictures the reader can hand over as
// ordinary files: JPEG and JPEG 2000 streams as they are, and 8-bit gray or RGB
// Flate/ASCII-filtered pictures re-encoded as PNG. Other kinds (indexed,
// CMYK, masks, fax) are skipped. Vector drawing on a page is not rendered.
func ExtractImages(r io.ReaderAt, size int64) ([]Image, error) {
	d, err := load(r, size)
	if err != nil {
		return nil, err
	}
	var out []Image
	seen := make(map[int]bool)
	for i, page := range d.pages() {
		d.collectImages(d.dict(page["Resources"]), i+1, 0, seen, &out)
		if len(out) >= maxImages {
			break
		}
	}
	return out, nil
}

func (d *doc) collectImages(res Dict, page, depth int, seen map[int]bool, out *[]Image) {
	xo := d.dict(res["XObject"])
	names := make([]string, 0, len(xo))
	for name := range xo {
		names = append(names, string(name))
	}
	sort.Strings(names)
	for _, name := range names {
		if len(*out) >= maxImages {
			return
		}
		entry := xo[Name(name)]
		if ref, ok := entry.(Ref); ok {
			if seen[ref.Num] {
				continue
			}
			seen[ref.Num] = true
		}
		s, ok := d.resolve(entry).(Stream)
		if !ok {
			continue
		}
		switch d.name(s.Dict["Subtype"]) {
		case "Image":
			if img, ok := d.imageOf(s); ok {
				img.Page, img.Name = page, name
				*out = append(*out, img)
			}
		case "Form":
			if depth < maxFormDepth {
				if inner := d.dict(s.Dict["Resources"]); inner != nil {
					d.collectImages(inner, page, depth+1, seen, out)
				}
			}
		}
	}
}

func (d *doc) filterNames(s Stream) []Name {
	switch f := d.resolve(s.Dict["Filter"]).(type) {
	case Name:
		return []Name{f}
	case Array:
		var names []Name
		for _, e := range f {
			names = append(names, d.name(e))
		}
		return names
	}
	return nil
}

func (d *doc) imageOf(s Stream) (Image, bool) {
	w, okW := d.number(s.Dict["Width"])
	h, okH := d.number(s.Dict["Height"])
	if !okW || !okH || w < 1 || h < 1 || w > maxImageSide || h > maxImageSide || w*h > maxImagePixels {
		return Image{}, false
	}
	img := Image{Width: int(w), Height: int(h)}
	filters := d.filterNames(s)
	if len(filters) == 1 {
		switch filters[0] {
		case "DCTDecode", "DCT":
			img.Ext, img.Data = "jpg", s.Raw
			return img, true
		case "JPXDecode":
			img.Ext, img.Data = "jp2", s.Raw
			return img, true
		}
	}
	bpc, _ := d.number(s.Dict["BitsPerComponent"])
	channels := 0
	switch d.name(s.Dict["ColorSpace"]) {
	case "DeviceGray", "CalGray", "G":
		channels = 1
	case "DeviceRGB", "CalRGB", "RGB":
		channels = 3
	}
	if channels == 0 || bpc != 8 {
		return Image{}, false
	}
	raw, err := d.decode(s)
	if err != nil {
		return Image{}, false
	}
	if parms := d.dict(s.Dict["DecodeParms"]); parms != nil {
		if p, _ := d.number(parms["Predictor"]); p >= 10 {
			cols, ok := d.number(parms["Columns"])
			if !ok {
				cols = 1
			}
			raw, err = unpredictPNG(raw, int(cols)*channels, channels)
			if err != nil {
				return Image{}, false
			}
		} else if p >= 2 {
			return Image{}, false
		}
	}
	if len(raw) < img.Width*img.Height*channels {
		return Image{}, false
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, pixels(raw, img.Width, img.Height, channels)); err != nil {
		return Image{}, false
	}
	img.Ext, img.Data = "png", buf.Bytes()
	return img, true
}

func pixels(raw []byte, w, h, channels int) image.Image {
	if channels == 1 {
		g := image.NewGray(image.Rect(0, 0, w, h))
		copy(g.Pix, raw[:w*h])
		return g
	}
	n := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		n.SetNRGBA(i%w, i/w, color.NRGBA{R: raw[i*3], G: raw[i*3+1], B: raw[i*3+2], A: 255})
	}
	return n
}

// unpredictPNG undoes the PNG row filters of a Predictor >= 10 stream whose
// rows hold rowBytes bytes of pixels each, bpp bytes per pixel.
func unpredictPNG(data []byte, rowBytes, bpp int) ([]byte, error) {
	if rowBytes < 1 || bpp < 1 {
		return nil, errors.New("bad predictor parameters")
	}
	stride := rowBytes + 1
	rows := len(data) / stride
	out := make([]byte, rows*rowBytes)
	for y := 0; y < rows; y++ {
		kind := data[y*stride]
		src := data[y*stride+1 : (y+1)*stride]
		cur := out[y*rowBytes : (y+1)*rowBytes]
		var up []byte
		if y > 0 {
			up = out[(y-1)*rowBytes : y*rowBytes]
		}
		for x := range cur {
			var left, above, upLeft int
			if x >= bpp {
				left = int(cur[x-bpp])
			}
			if up != nil {
				above = int(up[x])
				if x >= bpp {
					upLeft = int(up[x-bpp])
				}
			}
			var add int
			switch kind {
			case 0:
			case 1:
				add = left
			case 2:
				add = above
			case 3:
				add = (left + above) / 2
			case 4:
				add = paeth(left, above, upLeft)
			default:
				return nil, errors.New("unknown PNG row filter")
			}
			cur[x] = src[x] + byte(add&0xff)
		}
	}
	return out, nil
}

func paeth(a, b, c int) int {
	p := a + b - c
	pa, pb, pc := abs(p-a), abs(p-b), abs(p-c)
	switch {
	case pa <= pb && pa <= pc:
		return a
	case pb <= pc:
		return b
	}
	return c
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
