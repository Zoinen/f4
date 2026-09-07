package mediainfo

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

func isTIFFMagic(b []byte) bool {
	return len(b) >= 4 && (string(b[:4]) == "II*\x00" || string(b[:4]) == "MM\x00*")
}

type tiffImageScanner struct {
	p           *probe
	b           []byte
	order       binary.ByteOrder
	seen        map[uint32]bool
	pending     map[uint32]bool
	width       uint32
	height      uint32
	bitDepth    int
	compression uint16
	isDNG       bool
}

const (
	maxTIFFIFDDepth             = 8
	maxTIFFIFDNodes             = 4096
	maxTIFFBitsPerSampleValues  = 64
	maxTIFFSubIFDReferenceCount = 256
)

func parseTIFFImage(p *probe, _ []byte) error {
	n := min64(p.src.Size, p.opts.MaxSingleMetadataBytes)
	if n < 8 {
		return &ParseError{Format: "TIFF", Offset: 0, Err: errors.New("short TIFF header")}
	}
	b, err := p.readAt(0, int(n))
	if err != nil {
		return err
	}
	var order binary.ByteOrder
	switch string(b[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return ErrUnsupported
	}
	if order.Uint16(b[2:4]) != 42 {
		return ErrUnsupported
	}
	scanner := &tiffImageScanner{
		p: p, b: b, order: order,
		seen: make(map[uint32]bool), pending: make(map[uint32]bool),
	}
	if err := scanner.walkIFD(order.Uint32(b[4:8]), 0); err != nil {
		return err
	}
	if scanner.width == 0 || scanner.height == 0 {
		return &ParseError{Format: "TIFF", Offset: 0, Err: errors.New("image dimensions not found")}
	}
	im := &Image{
		Width: int(scanner.width), Height: int(scanner.height), BitDepth: scanner.bitDepth,
		Compression: tiffCompression(scanner.compression), FrameCount: 1,
	}
	parseTIFFMeta(p, b, im)
	format, mime := tiffFormat(p.src.Name, b, scanner.isDNG)
	p.report.General.Format = format
	p.report.General.MIME = mime
	p.report.Streams = append(p.report.Streams, Stream{
		Index: 0, ID: "1", Kind: StreamImage, Format: format, Image: im,
	})
	if n < p.src.Size {
		p.warn("metadata_window", "TIFF metadata outside the bounded header window was not inspected", n)
	}
	return nil
}

func (s *tiffImageScanner) walkIFD(off uint32, depth int) error {
	if err := s.p.ctx.Err(); err != nil {
		return err
	}
	delete(s.pending, off)
	if depth > maxTIFFIFDDepth {
		return nil
	}
	// The next-IFD link keeps the same logical depth. Follow that chain in a
	// loop so a valid but very long multipage file cannot grow the Go stack.
	for off != 0 {
		if err := s.p.ctx.Err(); err != nil {
			return err
		}
		delete(s.pending, off)
		if s.seen[off] || uint64(off)+2 > uint64(len(s.b)) {
			return nil
		}
		if len(s.seen) >= maxTIFFIFDNodes {
			return ErrLimit
		}
		s.seen[off] = true

		count := int(s.order.Uint16(s.b[off : off+2]))
		if count > 4096 {
			return &ParseError{Format: "TIFF", Offset: int64(off), Err: errors.New("too many IFD entries")}
		}
		end := uint64(off) + 2 + uint64(count)*12
		if end > uint64(len(s.b)) {
			return nil
		}
		var width, height uint32
		nested := make([]uint32, 0, 4)
		for i := 0; i < count; i++ {
			if err := s.p.step(); err != nil {
				return err
			}
			pos := int(off) + 2 + i*12
			entry := s.b[pos : pos+12]
			tag := s.order.Uint16(entry[:2])

			// Classify first. In particular, do not materialize the declared
			// values of an unknown tag: hostile files commonly make thousands
			// of entries point at one large shared payload.
			switch tag {
			case 0x0100: // ImageWidth
				if value, ok, err := s.firstValue(entry); err != nil {
					return err
				} else if ok {
					width = value
				}
			case 0x0101: // ImageLength
				if value, ok, err := s.firstValue(entry); err != nil {
					return err
				} else if ok {
					height = value
				}
			case 0x0102: // BitsPerSample
				values, ok, err := s.valueSet(entry, maxTIFFBitsPerSampleValues)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				for i := uint32(0); i < values.count; i++ {
					if err := s.p.step(); err != nil {
						return err
					}
					if value := values.at(i); int(value) > s.bitDepth && value <= 64 {
						s.bitDepth = int(value)
					}
				}
			case 0x0103: // Compression
				if value, ok, err := s.firstValue(entry); err != nil {
					return err
				} else if ok {
					if value > math.MaxUint16 {
						return &ParseError{Format: "TIFF", Offset: int64(pos), Err: fmt.Errorf("compression value %d exceeds uint16", value)}
					}
					// #nosec G115 -- the explicit MaxUint16 check above makes this conversion lossless.
					s.compression = uint16(value)
				}
			case 0x014a: // SubIFDs
				values, ok, err := s.valueSet(entry, maxTIFFSubIFDReferenceCount)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				for i := uint32(0); i < values.count; i++ {
					if err := s.p.step(); err != nil {
						return err
					}
					if err := s.queueIFD(&nested, values.at(i)); err != nil {
						return err
					}
				}
			case 0x8769: // Exif IFD
				if value, ok, err := s.firstValue(entry); err != nil {
					return err
				} else if ok {
					if err := s.queueIFD(&nested, value); err != nil {
						return err
					}
				}
			case 0xc612: // DNGVersion
				if _, ok, err := s.firstValue(entry); err != nil {
					return err
				} else if ok {
					s.isDNG = true
				}
			}
		}
		if width > 0 && height > 0 && uint64(width)*uint64(height) > uint64(s.width)*uint64(s.height) {
			s.width, s.height = width, height
		}
		for _, child := range nested {
			if err := s.p.ctx.Err(); err != nil {
				return err
			}
			if err := s.walkIFD(child, depth+1); err != nil {
				return err
			}
		}

		var next uint32
		if end+4 <= uint64(len(s.b)) {
			next = s.order.Uint32(s.b[end : end+4])
			if next != 0 {
				// The link itself is another decoded reference. Charge it even
				// when it points at an IFD that has already been visited.
				if err := s.p.step(); err != nil {
					return err
				}
			}
		}
		off = next
	}
	return nil
}

type tiffValueSet struct {
	raw   []byte
	typ   uint16
	count uint32
	order binary.ByteOrder
}

func (s *tiffImageScanner) valueSet(entry []byte, maxValues uint32) (tiffValueSet, bool, error) {
	if len(entry) < 12 {
		return tiffValueSet{}, false, nil
	}
	typ := s.order.Uint16(entry[2:4])
	count := s.order.Uint32(entry[4:8])
	var unit uint64
	switch typ {
	case 1:
		unit = 1
	case 3:
		unit = 2
	case 4, 9:
		unit = 4
	}
	if unit == 0 || count == 0 {
		return tiffValueSet{}, false, nil
	}
	if maxValues > 0 && count > maxValues {
		return tiffValueSet{}, false, ErrLimit
	}
	if count > 4096 {
		return tiffValueSet{}, false, nil
	}
	sz := uint64(count) * unit
	var raw []byte
	if sz <= 4 {
		raw = entry[8 : 8+int(sz)]
	} else {
		off := uint64(s.order.Uint32(entry[8:12]))
		if off > uint64(len(s.b)) || sz > uint64(len(s.b))-off {
			return tiffValueSet{}, false, nil
		}
		raw = s.b[off : off+sz]
	}
	return tiffValueSet{raw: raw, typ: typ, count: count, order: s.order}, true, nil
}

func (s *tiffImageScanner) firstValue(entry []byte) (uint32, bool, error) {
	values, ok, err := s.valueSet(entry, 0)
	if err != nil || !ok {
		return 0, ok, err
	}
	if err := s.p.step(); err != nil {
		return 0, false, err
	}
	return values.at(0), true, nil
}

func (s *tiffImageScanner) queueIFD(nested *[]uint32, off uint32) error {
	if off == 0 || s.seen[off] || s.pending[off] || uint64(off)+2 > uint64(len(s.b)) {
		return nil
	}
	if len(s.seen)+len(s.pending) >= maxTIFFIFDNodes {
		return ErrLimit
	}
	s.pending[off] = true
	*nested = append(*nested, off)
	return nil
}

func (v tiffValueSet) at(i uint32) uint32 {
	switch v.typ {
	case 1:
		return uint32(v.raw[i])
	case 3:
		i *= 2
		return uint32(v.order.Uint16(v.raw[i : i+2]))
	case 4, 9:
		i *= 4
		return v.order.Uint32(v.raw[i : i+4])
	default:
		return 0
	}
}

func tiffFormat(name string, b []byte, dng bool) (string, string) {
	if dng || strings.EqualFold(filepath.Ext(name), ".dng") {
		return "DNG", "image/x-adobe-dng"
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".cr2":
		if len(b) >= 12 && string(b[8:12]) == "CR\x02\x00" {
			return "Canon CR2", "image/x-canon-cr2"
		}
		return "Canon RAW", "image/x-canon-cr2"
	case ".nef":
		return "Nikon NEF", "image/x-nikon-nef"
	case ".pef":
		return "Pentax PEF", "image/x-pentax-pef"
	case ".arw":
		return "Sony ARW", "image/x-sony-arw"
	default:
		return "TIFF", "image/tiff"
	}
}

func tiffCompression(v uint16) string {
	switch v {
	case 1:
		return "Uncompressed"
	case 5:
		return "LZW"
	case 6, 7:
		return "JPEG"
	case 8, 32946:
		return "Deflate"
	case 32773:
		return "PackBits"
	case 34712:
		return "JPEG 2000"
	case 34892, 34893:
		return "Lossy JPEG"
	case 65000:
		return "Kodak DCR"
	case 0:
		return ""
	default:
		return fmt.Sprintf("TIFF compression %d", v)
	}
}
