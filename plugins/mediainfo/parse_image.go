package mediainfo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"
	"time"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

func isImageMagic(b []byte) bool {
	return len(b) >= 2 && b[0] == 0xff && b[1] == 0xd8 || len(b) >= 8 && bytes.Equal(b[:8], []byte("\x89PNG\r\n\x1a\n")) || len(b) >= 6 && (string(b[:6]) == "GIF87a" || string(b[:6]) == "GIF89a") || len(b) >= 2 && string(b[:2]) == "BM" || isTIFFMagic(b) || len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP"
}

func parseImage(p *probe, _ []byte) error {
	sr, e := p.section(0, p.src.Size)
	if e != nil {
		return e
	}
	cfg, format, e := image.DecodeConfig(sr)
	if e != nil {
		return &ParseError{Format: "Image", Offset: 0, Err: e}
	}
	p.report.General.Format = strings.ToUpper(format)
	p.report.General.MIME = "image/" + strings.ToLower(format)
	im := &Image{Width: cfg.Width, Height: cfg.Height, FrameCount: 1, ColorModel: fmt.Sprint(cfg.ColorModel)}
	s := Stream{Index: 0, ID: "1", Kind: StreamImage, Format: p.report.General.Format, Image: im}
	switch format {
	case "jpeg":
		parseJPEGMeta(p, im)
	case "png":
		parsePNGMeta(p, im)
	case "gif":
		parseGIFMeta(p, im)
	case "webp":
		parseWebPMeta(p, im)
	case "tiff":
		if n := min64(p.src.Size, p.opts.MaxSingleMetadataBytes); n > 0 {
			b, _ := p.readAt(0, int(n))
			parseTIFFMeta(p, b, im)
		}
	}
	p.report.Streams = append(p.report.Streams, s)
	return nil
}

func parseJPEGMeta(p *probe, im *Image) {
	for pos := int64(2); pos+4 <= p.src.Size; {
		h, e := p.readAt(pos, 4)
		if e != nil || h[0] != 0xff {
			return
		}
		marker := h[1]
		if marker == 0xd9 || marker == 0xda {
			return
		}
		sz := int64(binary.BigEndian.Uint16(h[2:4]))
		if sz < 2 || pos+2+sz > p.src.Size {
			return
		}
		if marker == 0xe1 && sz-2 <= p.opts.MaxSingleMetadataBytes {
			b, e := p.readAt(pos+4, int(sz-2))
			if e == nil && len(b) >= 6 && string(b[:6]) == "Exif\x00\x00" {
				parseTIFFMeta(p, b[6:], im)
			} else if e == nil && bytes.Contains(b, []byte("http://ns.adobe.com/xap/1.0/")) {
				p.addTag("", "XMP", "Present")
			}
		} else if marker == 0xed {
			p.addTag("", "IPTC", "Present")
		}
		pos += 2 + sz
	}
}

func parsePNGMeta(p *probe, im *Image) {
	for pos := int64(8); pos+12 <= p.src.Size; {
		if err := p.step(); err != nil {
			p.report.Truncated = true
			return
		}
		h, e := p.readAt(pos, 8)
		if e != nil {
			return
		}
		n := int64(binary.BigEndian.Uint32(h[:4]))
		typ := string(h[4:8])
		data := pos + 8
		if n < 0 || data+n+4 > p.src.Size {
			return
		}
		switch typ {
		case "IHDR":
			if n >= 13 {
				b, _ := p.readAt(data, 13)
				if len(b) >= 13 {
					im.BitDepth = int(b[8])
				}
			}
		case "acTL":
			if n >= 8 {
				b, _ := p.readAt(data, 8)
				if len(b) >= 8 {
					im.FrameCount = int(binary.BigEndian.Uint32(b[:4]))
					im.Animated = im.FrameCount > 1
				}
			}
		case "fcTL":
			if n >= 26 {
				b, _ := p.readAt(data, 26)
				if len(b) >= 26 {
					numerator := binary.BigEndian.Uint16(b[20:22])
					denominator := binary.BigEndian.Uint16(b[22:24])
					if denominator == 0 {
						denominator = 100
					}
					im.AnimationDuration += time.Duration(numerator) * time.Second / time.Duration(denominator)
				}
			}
		case "pHYs":
			if n >= 9 {
				b, _ := p.readAt(data, 9)
				if len(b) >= 9 && b[8] == 1 {
					im.DPIX = float64(binary.BigEndian.Uint32(b[:4])) * 0.0254
					im.DPIY = float64(binary.BigEndian.Uint32(b[4:8])) * 0.0254
				}
			}
		case "eXIf":
			if n <= p.opts.MaxSingleMetadataBytes {
				b, _ := p.readAt(data, int(n))
				parseTIFFMeta(p, b, im)
			}
		case "tEXt":
			if n <= p.opts.MaxSingleMetadataBytes {
				b, _ := p.readAt(data, int(n))
				if i := bytes.IndexByte(b, 0); i > 0 {
					p.addTag("", string(b[:i]), string(b[i+1:]))
				}
			}
		case "IEND":
			return
		}
		pos = data + n + 4
	}
}

func parseGIFMeta(p *probe, im *Image) {
	if p.opts.Mode == ModeFast {
		return
	}
	limit := p.src.Size
	if limit > p.opts.MaxTextBytes {
		limit = p.opts.MaxTextBytes
		p.report.Truncated = true
	}
	pos := int64(13)
	if descriptor, e := p.readAt(10, 1); e == nil && descriptor[0]&0x80 != 0 {
		pos += int64(3 * (1 << ((descriptor[0] & 7) + 1)))
	}
	frames := 0
	var duration time.Duration
	for pos < limit {
		b, e := p.readAt(pos, 1)
		if e != nil {
			return
		}
		switch b[0] {
		case 0x2c:
			if pos+10 > limit {
				return
			}
			d, _ := p.readAt(pos+1, 9)
			packed := d[8]
			pos += 10
			if packed&0x80 != 0 {
				pos += int64(3 * (1 << ((packed & 7) + 1)))
			}
			pos++
			pos = skipGIFBlocks(p, pos, limit)
			frames++
		case 0x21:
			lab, _ := p.readAt(pos+1, 1)
			if lab[0] == 0xf9 && pos+8 <= limit {
				d, _ := p.readAt(pos+2, 6)
				if len(d) >= 6 {
					duration += time.Duration(binary.LittleEndian.Uint16(d[2:4])) * 10 * time.Millisecond
				}
				pos += 8
			} else {
				pos = skipGIFBlocks(p, pos+2, limit)
			}
		case 0x3b:
			pos = limit
		default:
			return
		}
	}
	if frames > 0 {
		im.FrameCount = frames
		im.Animated = frames > 1
		im.AnimationDuration = duration
	}
}
func skipGIFBlocks(p *probe, pos, limit int64) int64 {
	for pos < limit {
		b, e := p.readAt(pos, 1)
		if e != nil {
			return limit
		}
		pos++
		if b[0] == 0 {
			return pos
		}
		pos += int64(b[0])
	}
	return limit
}

func parseWebPMeta(p *probe, im *Image) {
	frames := 0
	var duration time.Duration
	for pos := int64(12); pos+8 <= p.src.Size; {
		if err := p.step(); err != nil {
			p.report.Truncated = true
			return
		}
		h, e := p.readAt(pos, 8)
		if e != nil {
			return
		}
		typ := string(h[:4])
		n := int64(binary.LittleEndian.Uint32(h[4:8]))
		data := pos + 8
		if data+n > p.src.Size {
			return
		}
		switch typ {
		case "VP8X":
			if n >= 10 {
				b, _ := p.readAt(data, 10)
				if len(b) >= 10 {
					im.Animated = b[0]&2 != 0
					im.Width = 1 + (int(b[4]) | int(b[5])<<8 | int(b[6])<<16)
					im.Height = 1 + (int(b[7]) | int(b[8])<<8 | int(b[9])<<16)
				}
			}
		case "ANMF":
			frames++
			im.Animated = true
			if n >= 16 {
				b, _ := p.readAt(data, 16)
				if len(b) >= 16 {
					milliseconds := int(b[12]) | int(b[13])<<8 | int(b[14])<<16
					duration += time.Duration(milliseconds) * time.Millisecond
				}
			}
		case "EXIF":
			if n <= p.opts.MaxSingleMetadataBytes {
				b, _ := p.readAt(data, int(n))
				if bytes.HasPrefix(b, []byte("Exif\x00\x00")) {
					b = b[6:]
				}
				parseTIFFMeta(p, b, im)
			}
		case "XMP ":
			p.addTag("", "XMP", "Present")
		}
		pos = data + n
		if pos&1 != 0 {
			pos++
		}
	}
	if frames > 0 {
		im.FrameCount = frames
		im.AnimationDuration = duration
	} else if im.FrameCount == 0 {
		im.FrameCount = 1
	}
}

func parseTIFFMeta(p *probe, b []byte, im *Image) {
	if len(b) < 8 {
		return
	}
	var order binary.ByteOrder
	switch string(b[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return
	}
	if order.Uint16(b[2:4]) != 42 {
		return
	}
	type ifdKind uint8
	const (
		ifdMain ifdKind = iota
		ifdEXIF
		ifdGPS
	)
	type gpsState struct {
		latRef, lonRef string
		lat, lon       []float64
		altRef         int
		alt            *float64
	}
	gps := gpsState{}
	var xResolution, yResolution float64
	resolutionUnit := uint64(2) // TIFF defaults to inches when the tag is absent.
	seen := map[uint32]bool{}
	var walk func(uint32, int, ifdKind)
	walk = func(off uint32, depth int, kind ifdKind) {
		if depth > 4 || seen[off] || int(off)+2 > len(b) {
			return
		}
		seen[off] = true
		count := int(order.Uint16(b[off : off+2]))
		if count > 4096 {
			return
		}
		for i := 0; i < count; i++ {
			if err := p.step(); err != nil {
				p.report.Truncated = true
				return
			}
			pos := int(off) + 2 + i*12
			if pos+12 > len(b) {
				break
			}
			tag := order.Uint16(b[pos : pos+2])
			typ := order.Uint16(b[pos+2 : pos+4])
			units := order.Uint32(b[pos+4 : pos+8])
			if !usefulTIFFTag(kind, tag) {
				continue
			}
			unitSize := map[uint16]int{1: 1, 2: 1, 3: 2, 4: 4, 5: 8, 7: 1, 9: 4, 10: 8}[typ]
			if unitSize == 0 || uint64(units)*uint64(unitSize) > uint64(len(b)) { // #nosec G115 -- unitSize and slice lengths are non-negative.
				continue
			}
			// #nosec G115 -- the product was checked against len(b), so units fits int here.
			n := int(units) * unitSize
			var raw []byte
			if n <= 4 {
				raw = b[pos+8 : pos+8+n]
			} else {
				o := int(order.Uint32(b[pos+8 : pos+12]))
				if o < 0 || o+n > len(b) {
					continue
				}
				raw = b[o : o+n]
			}
			if kind == ifdGPS {
				switch tag {
				case 0x0001:
					gps.latRef = boundedTIFFText(p, raw)
				case 0x0002:
					gps.lat = tiffRationals(order, raw, false, 3)
				case 0x0003:
					gps.lonRef = boundedTIFFText(p, raw)
				case 0x0004:
					gps.lon = tiffRationals(order, raw, false, 3)
				case 0x0005:
					if len(raw) > 0 {
						gps.altRef = int(raw[0])
					}
				case 0x0006:
					if values := tiffRationals(order, raw, false, 1); len(values) == 1 {
						gps.alt = &values[0]
					}
				}
				continue
			}
			switch tag {
			case 0x010f:
				im.CameraMake = boundedTIFFText(p, raw)
			case 0x0110:
				im.CameraModel = boundedTIFFText(p, raw)
			case 0xa434:
				im.LensModel = boundedTIFFText(p, raw)
			case 0x0131:
				addImageEXIF(p, im, "Software", boundedTIFFText(p, raw))
			case 0x013b:
				addImageEXIF(p, im, "Artist", boundedTIFFText(p, raw))
			case 0x8298:
				addImageEXIF(p, im, "Copyright", boundedTIFFText(p, raw))
			case 0xa431:
				addImageEXIF(p, im, "Camera serial number", boundedTIFFText(p, raw))
			case 0xa433:
				addImageEXIF(p, im, "Lens make", boundedTIFFText(p, raw))
			case 0xa435:
				addImageEXIF(p, im, "Lens serial number", boundedTIFFText(p, raw))
			case 0x0112:
				if len(raw) >= 2 {
					im.Orientation = int(order.Uint16(raw[:2]))
				}
			case 0x011a:
				if values := tiffRationals(order, raw, false, 1); len(values) == 1 {
					xResolution = values[0]
				}
			case 0x011b:
				if values := tiffRationals(order, raw, false, 1); len(values) == 1 {
					yResolution = values[0]
				}
			case 0x0128:
				if value, ok := tiffFirstInteger(order, raw, typ); ok {
					resolutionUnit = value
				}
			case 0x9003, 0x9004, 0x0132:
				if im.TakenAt == nil {
					text := boundedTIFFText(p, raw)
					if t, e := time.Parse("2006:01:02 15:04:05", text); e == nil {
						im.TakenAt = &t
					}
				}
			case 0x8769:
				if len(raw) >= 4 {
					walk(order.Uint32(raw[:4]), depth+1, ifdEXIF)
				}
			case 0x8825:
				if len(raw) >= 4 {
					walk(order.Uint32(raw[:4]), depth+1, ifdGPS)
				}
			case 0x829a:
				addImageEXIF(p, im, "Exposure time", formatEXIFExposure(tiffRationals(order, raw, false, 1)))
			case 0x829d:
				addImageEXIF(p, im, "F-number", formatEXIFAperture(tiffRationals(order, raw, false, 1)))
			case 0x8827:
				if value, ok := tiffFirstInteger(order, raw, typ); ok {
					addImageEXIF(p, im, "ISO speed", fmt.Sprint(value))
				}
			case 0x8822:
				if value, ok := tiffFirstInteger(order, raw, typ); ok {
					addImageEXIF(p, im, "Exposure program", exifExposureProgram(value))
				}
			case 0x9000:
				addImageEXIF(p, im, "EXIF version", formatEXIFVersion(raw))
			case 0x9204:
				if values := tiffRationals(order, raw, true, 1); len(values) == 1 {
					addImageEXIF(p, im, "Exposure bias", fmt.Sprintf("%+.3g EV", values[0]))
				}
			case 0x9203:
				if values := tiffRationals(order, raw, true, 1); len(values) == 1 {
					addImageEXIF(p, im, "Brightness", fmt.Sprintf("%.3g EV", values[0]))
				}
			case 0x9206:
				if values := tiffRationals(order, raw, false, 1); len(values) == 1 {
					addImageEXIF(p, im, "Subject distance", fmt.Sprintf("%.4g m", values[0]))
				}
			case 0x9207:
				if value, ok := tiffFirstInteger(order, raw, typ); ok {
					addImageEXIF(p, im, "Metering mode", exifMeteringMode(value))
				}
			case 0x9209:
				if value, ok := tiffFirstInteger(order, raw, typ); ok {
					addImageEXIF(p, im, "Flash", exifFlash(value))
				}
			case 0x9208:
				if value, ok := tiffFirstInteger(order, raw, typ); ok {
					addImageEXIF(p, im, "Light source", exifLightSource(value))
				}
			case 0x920a:
				if values := tiffRationals(order, raw, false, 1); len(values) == 1 {
					addImageEXIF(p, im, "Focal length", fmt.Sprintf("%.4g mm", values[0]))
				}
			case 0x9286:
				addImageEXIF(p, im, "User comment", boundedEXIFComment(p, raw))
			case 0xa403:
				if value, ok := tiffFirstInteger(order, raw, typ); ok {
					switch value {
					case 0:
						addImageEXIF(p, im, "White balance", "Auto")
					case 1:
						addImageEXIF(p, im, "White balance", "Manual")
					}
				}
			case 0xa402:
				if value, ok := tiffFirstInteger(order, raw, typ); ok {
					addImageEXIF(p, im, "Exposure mode", map[uint64]string{0: "Auto", 1: "Manual", 2: "Auto bracket"}[value])
				}
			case 0xa404:
				if values := tiffRationals(order, raw, false, 1); len(values) == 1 && values[0] > 0 {
					addImageEXIF(p, im, "Digital zoom", fmt.Sprintf("%.3g×", values[0]))
				}
			case 0xa405:
				if value, ok := tiffFirstInteger(order, raw, typ); ok {
					addImageEXIF(p, im, "Focal length in 35mm", fmt.Sprintf("%d mm", value))
				}
			case 0xa406:
				if value, ok := tiffFirstInteger(order, raw, typ); ok {
					addImageEXIF(p, im, "Scene capture type", map[uint64]string{0: "Standard", 1: "Landscape", 2: "Portrait", 3: "Night scene"}[value])
				}
			case 0xa408, 0xa409, 0xa40a:
				if value, ok := tiffFirstInteger(order, raw, typ); ok {
					name := map[uint16]string{0xa408: "Contrast", 0xa409: "Saturation", 0xa40a: "Sharpness"}[tag]
					labels := map[uint64]string{0: "Normal", 1: "Soft", 2: "Hard"}
					if tag == 0xa409 {
						labels = map[uint64]string{0: "Normal", 1: "Low", 2: "High"}
					}
					addImageEXIF(p, im, name, labels[value])
				}
			case 0xa420:
				addImageEXIF(p, im, "Image unique ID", boundedTIFFText(p, raw))
			}
		}
	}
	walk(order.Uint32(b[4:8]), 0, ifdMain)
	if len(gps.lat) == 3 {
		value := gps.lat[0] + gps.lat[1]/60 + gps.lat[2]/3600
		if strings.EqualFold(gps.latRef, "S") {
			value = -value
		}
		im.Latitude = &value
	}
	if len(gps.lon) == 3 {
		value := gps.lon[0] + gps.lon[1]/60 + gps.lon[2]/3600
		if strings.EqualFold(gps.lonRef, "W") {
			value = -value
		}
		im.Longitude = &value
	}
	if gps.alt != nil {
		value := *gps.alt
		if gps.altRef == 1 {
			value = -value
		}
		im.GPSAltitude = &value
	}
	if resolutionUnit == 3 {
		xResolution *= 2.54
		yResolution *= 2.54
	}
	if xResolution > 0 {
		im.DPIX = xResolution
	}
	if yResolution > 0 {
		im.DPIY = yResolution
	}
}

func usefulTIFFTag[T ~uint8](kind T, tag uint16) bool {
	if uint8(kind) == 2 { // GPS IFD
		return tag >= 0x0001 && tag <= 0x0006
	}
	switch tag {
	case 0x010f, 0x0110, 0x0112, 0x011a, 0x011b, 0x0128, 0x0131, 0x0132, 0x013b, 0x8298,
		0x829a, 0x829d, 0x8769, 0x8822, 0x8825, 0x8827, 0x9000, 0x9003, 0x9004,
		0x9203, 0x9204, 0x9206, 0x9207, 0x9208, 0x9209, 0x920a, 0x9286,
		0xa402, 0xa403, 0xa404, 0xa405, 0xa406, 0xa408, 0xa409, 0xa40a,
		0xa420, 0xa431, 0xa433, 0xa434, 0xa435:
		return true
	}
	return false
}

func addImageEXIF(p *probe, im *Image, name, value string) {
	if name == "" || value == "" {
		return
	}
	if len(im.EXIF) >= p.opts.MaxTags {
		p.report.Truncated = true
		return
	}
	if len(value) > p.opts.MaxValueBytes {
		value = value[:p.opts.MaxValueBytes]
		p.report.Truncated = true
	}
	im.EXIF = append(im.EXIF, Field{Name: strings.Clone(name), Value: strings.Clone(value)})
}

func tiffRationals(order binary.ByteOrder, raw []byte, signed bool, limit int) []float64 {
	count := len(raw) / 8
	if count > limit {
		count = limit
	}
	values := make([]float64, 0, count)
	for index := 0; index < count; index++ {
		part := raw[index*8 : index*8+8]
		var numerator, denominator float64
		if signed {
			numerator = float64(signedInt32Bits(order.Uint32(part[:4])))
			denominator = float64(signedInt32Bits(order.Uint32(part[4:])))
		} else {
			numerator = float64(order.Uint32(part[:4]))
			denominator = float64(order.Uint32(part[4:]))
		}
		if denominator == 0 {
			return nil
		}
		values = append(values, numerator/denominator)
	}
	return values
}

func tiffFirstInteger(order binary.ByteOrder, raw []byte, typ uint16) (uint64, bool) {
	switch typ {
	case 1, 7:
		if len(raw) >= 1 {
			return uint64(raw[0]), true
		}
	case 3:
		if len(raw) >= 2 {
			return uint64(order.Uint16(raw[:2])), true
		}
	case 4:
		if len(raw) >= 4 {
			return uint64(order.Uint32(raw[:4])), true
		}
	}
	return 0, false
}

func formatEXIFExposure(values []float64) string {
	if len(values) != 1 || values[0] <= 0 {
		return ""
	}
	if values[0] < 1 {
		return fmt.Sprintf("1/%.0f s", 1/values[0])
	}
	return fmt.Sprintf("%.4g s", values[0])
}

func formatEXIFAperture(values []float64) string {
	if len(values) != 1 || values[0] <= 0 {
		return ""
	}
	return fmt.Sprintf("f/%.3g", values[0])
}

func formatEXIFVersion(raw []byte) string {
	value := strings.TrimSpace(string(raw))
	if len(value) == 4 && value[0] >= '0' && value[0] <= '9' {
		return value[:2] + "." + value[2:]
	}
	return cleanText(raw)
}

func boundedEXIFComment(p *probe, raw []byte) string {
	if len(raw) >= 8 {
		prefix := string(raw[:8])
		if strings.HasPrefix(prefix, "ASCII") || strings.HasPrefix(prefix, "UNICODE") || strings.HasPrefix(prefix, "JIS") {
			raw = raw[8:]
		}
	}
	return boundedTIFFText(p, raw)
}

func exifMeteringMode(value uint64) string {
	return map[uint64]string{0: "Unknown", 1: "Average", 2: "Center-weighted average", 3: "Spot", 4: "Multi-spot", 5: "Pattern", 6: "Partial", 255: "Other"}[value]
}

func exifExposureProgram(value uint64) string {
	return map[uint64]string{0: "Undefined", 1: "Manual", 2: "Normal program", 3: "Aperture priority", 4: "Shutter priority", 5: "Creative", 6: "Action", 7: "Portrait", 8: "Landscape", 9: "Bulb"}[value]
}

func exifLightSource(value uint64) string {
	return map[uint64]string{0: "Unknown", 1: "Daylight", 2: "Fluorescent", 3: "Tungsten", 4: "Flash", 9: "Fine weather", 10: "Cloudy", 11: "Shade", 12: "Daylight fluorescent", 13: "Day white fluorescent", 14: "Cool white fluorescent", 15: "White fluorescent", 17: "Standard light A", 18: "Standard light B", 19: "Standard light C", 20: "D55", 21: "D65", 22: "D75", 23: "D50", 24: "ISO studio tungsten", 255: "Other"}[value]
}

func exifFlash(value uint64) string {
	if value&1 != 0 {
		return "Fired"
	}
	return "Did not fire"
}

func boundedTIFFText(p *probe, raw []byte) string {
	if len(raw) > p.opts.MaxValueBytes {
		raw = raw[:p.opts.MaxValueBytes]
		p.report.Truncated = true
	}
	return cleanText(raw)
}
