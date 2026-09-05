package mediainfo

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strings"
)

var heifBrands = map[string]bool{
	"mif1": true, "msf1": true, "heic": true, "heix": true,
	"hevc": true, "hevx": true, "heim": true, "heis": true,
	"avic": true, "avif": true, "avis": true,
}

func isHEIFSource(name string, head []byte) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".heif" || ext == ".heic" || ext == ".avif" {
		return true
	}
	if len(head) < 16 || string(head[4:8]) != "ftyp" {
		return false
	}
	end := len(head)
	if size := int(binary.BigEndian.Uint32(head[:4])); size >= 16 && size < end {
		end = size
	}
	for pos := 8; pos+4 <= end; pos += 4 {
		if heifBrands[string(head[pos:pos+4])] {
			return true
		}
	}
	return false
}

type heifState struct {
	format string
	brands []string
	width  int
	height int
	depth  int
	codec  string
}

func parseHEIF(p *probe, _ []byte) error {
	st := &heifState{format: "HEIF"}
	if err := walkHEIFBoxes(p, st, 0, p.src.Size, 0, false); err != nil {
		return err
	}
	if st.width <= 0 || st.height <= 0 {
		return &ParseError{Format: st.format, Offset: 0, Err: errors.New("image spatial property not found")}
	}
	p.report.General.Format = st.format
	p.report.General.CompatibleBrands = append([]string(nil), st.brands...)
	switch st.format {
	case "AVIF":
		p.report.General.MIME = "image/avif"
	case "HEIC":
		p.report.General.MIME = "image/heic"
	default:
		p.report.General.MIME = "image/heif"
	}
	streamFormat := st.codec
	if streamFormat == "" {
		streamFormat = st.format
	}
	p.report.Streams = append(p.report.Streams, Stream{
		Index: 0, ID: "1", Kind: StreamImage, Format: streamFormat,
		Image: &Image{Width: st.width, Height: st.height, BitDepth: st.depth, FrameCount: 1},
	})
	return nil
}

func walkHEIFBoxes(p *probe, st *heifState, start, end int64, depth int, metaChildren bool) error {
	if depth > 16 {
		return &ParseError{Format: "HEIF", Offset: start, Err: errors.New("box nesting too deep")}
	}
	if metaChildren {
		start += 4 // FullBox version and flags.
	}
	for pos := start; pos+8 <= end; {
		if err := p.step(); err != nil {
			return err
		}
		h, err := p.readAt(pos, 8)
		if err != nil {
			return err
		}
		size := int64(binary.BigEndian.Uint32(h[:4]))
		header := int64(8)
		switch size {
		case 1:
			x, err := p.readAt(pos+8, 8)
			if err != nil {
				return err
			}
			extendedSize := binary.BigEndian.Uint64(x)
			if extendedSize > math.MaxInt64 {
				return &ParseError{Format: "HEIF", Offset: pos, Err: fmt.Errorf("invalid %q box size", string(h[4:8]))}
			}
			// #nosec G115 -- the explicit MaxInt64 check above makes this conversion lossless.
			size, header = int64(extendedSize), 16
		case 0:
			size = end - pos
		}
		if size < header || pos > end-size {
			return &ParseError{Format: "HEIF", Offset: pos, Err: fmt.Errorf("invalid %q box size", string(h[4:8]))}
		}
		typ, data, boxEnd := string(h[4:8]), pos+header, pos+size
		switch typ {
		case "ftyp":
			if err := parseHEIFFTYP(p, st, data, boxEnd); err != nil {
				return err
			}
		case "meta":
			if err := walkHEIFBoxes(p, st, data, boxEnd, depth+1, true); err != nil {
				return err
			}
		case "iprp", "ipco":
			if err := walkHEIFBoxes(p, st, data, boxEnd, depth+1, false); err != nil {
				return err
			}
		case "ispe":
			if boxEnd-data >= 12 {
				b, err := p.readAt(data, 12)
				if err != nil {
					return err
				}
				w, h := int(binary.BigEndian.Uint32(b[4:8])), int(binary.BigEndian.Uint32(b[8:12]))
				if w > 0 && h > 0 && int64(w)*int64(h) > int64(st.width)*int64(st.height) {
					st.width, st.height = w, h
				}
			}
		case "pixi":
			if boxEnd-data >= 6 {
				b, err := p.readAt(data, int(min64(boxEnd-data, 32)))
				if err != nil {
					return err
				}
				channels := int(b[4])
				for i := 0; i < channels && 5+i < len(b); i++ {
					if int(b[5+i]) > st.depth {
						st.depth = int(b[5+i])
					}
				}
			}
		case "hvcC":
			st.codec = "HEVC"
		case "av1C":
			st.codec = "AV1"
		}
		pos = boxEnd
	}
	return nil
}

func parseHEIFFTYP(p *probe, st *heifState, start, end int64) error {
	if end-start < 8 {
		return io.ErrUnexpectedEOF
	}
	n := end - start
	if n > 256 {
		n = 256
	}
	b, err := p.readAt(start, int(n))
	if err != nil {
		return err
	}
	for pos := 0; pos+4 <= len(b); pos += 4 {
		if len(st.brands) >= 64 {
			break
		}
		if err := p.step(); err != nil {
			return err
		}
		brand := string(b[pos : pos+4])
		if pos == 4 { // minor version, not a brand
			continue
		}
		if brand != "" && brand != "\x00\x00\x00\x00" {
			st.brands = append(st.brands, brand)
		}
		switch brand {
		case "avif", "avis", "avic":
			st.format = "AVIF"
			st.codec = "AV1"
		case "heic", "heix", "hevc", "hevx", "heim", "heis":
			if st.format != "AVIF" {
				st.format = "HEIC"
				st.codec = "HEVC"
			}
		}
	}
	return nil
}
