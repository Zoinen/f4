package mediainfo

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

type isoBox struct {
	typ                        string
	start, data, size, payload int64
}
type isoState struct {
	seenMdat    bool
	sampleBytes map[int]int64
}

func parseISOBaseMedia(p *probe, _ []byte) error {
	p.report.General.Format = "MPEG-4"
	p.report.General.MIME = "video/mp4"
	st := &isoState{sampleBytes: make(map[int]int64)}
	if err := walkISO(p, st, 0, p.src.Size, 0, nil, ""); err != nil {
		return err
	}
	if p.report.General.CodecID == "qt  " {
		p.report.General.Format = "QuickTime"
		p.report.General.MIME = "video/quicktime"
	}
	if len(p.report.Streams) == 0 && p.report.General.Format == "MPEG-4" {
		p.warn("no_tracks", "no movie tracks were found", -1)
	}
	for i := range p.report.Streams {
		s := &p.report.Streams[i]
		if s.Duration > 0 && s.BitRate == 0 && st.sampleBytes[i] > 0 {
			s.BitRate = int64(float64(st.sampleBytes[i]*8) / s.Duration.Seconds())
		}
	}
	return nil
}

func readISOBox(p *probe, pos, end int64) (isoBox, error) {
	if pos+8 > end {
		return isoBox{}, io.EOF
	}
	h, e := p.readAt(pos, 8)
	if e != nil {
		return isoBox{}, e
	}
	sz := int64(binary.BigEndian.Uint32(h[:4]))
	header := int64(8)
	typ := string(h[4:8])
	switch sz {
	case 1:
		b, e := p.readAt(pos+8, 8)
		if e != nil {
			return isoBox{}, e
		}
		u := binary.BigEndian.Uint64(b)
		if u > math.MaxInt64 {
			return isoBox{}, errors.New("box is too large")
		}
		sz = int64(u)
		header = 16
	case 0:
		sz = end - pos
	}
	if sz < header || sz > end-pos {
		return isoBox{}, &ParseError{Format: "ISO BMFF", Offset: pos, Err: fmt.Errorf("invalid %q box size %d", typ, sz)}
	}
	return isoBox{typ: typ, start: pos, data: pos + header, size: sz, payload: sz - header}, nil
}

func walkISO(p *probe, st *isoState, start, end int64, depth int, track *Stream, parent string) error {
	if depth > 16 {
		return &ParseError{Format: "ISO BMFF", Offset: start, Err: errors.New("box nesting too deep")}
	}
	for pos := start; pos+8 <= end; {
		if e := p.step(); e != nil {
			return e
		}
		box, e := readISOBox(p, pos, end)
		if e != nil {
			return e
		}
		var childTrack = track
		switch box.typ {
		case "ftyp", "styp":
			parseFTYP(p, box)
		case "mdat":
			st.seenMdat = true
		case "moov":
			v := !st.seenMdat
			p.report.General.Streamable = &v
		case "trak":
			if len(p.report.Streams) < p.opts.MaxStreams {
				p.report.Streams = append(p.report.Streams, Stream{Index: len(p.report.Streams), ID: fmt.Sprint(len(p.report.Streams) + 1)})
				childTrack = &p.report.Streams[len(p.report.Streams)-1]
			} else {
				p.report.Truncated = true
			}
		case "mvhd":
			parseMVHD(p, box)
		case "tkhd":
			if track != nil {
				parseTKHD(p, box, track)
			}
		case "mdhd":
			if track != nil {
				parseMDHD(p, box, track)
			}
		case "hdlr":
			if track != nil {
				parseHDLR(p, box, track)
			}
		case "stsd":
			if track != nil {
				if e := parseSTSD(p, st, box, track); e != nil {
					return e
				}
			}
		case "stts":
			if track != nil && p.opts.Mode == ModeDetailed {
				parseSTTS(p, box, track)
			}
		case "stsz":
			if track != nil && p.opts.Mode == ModeDetailed {
				st.sampleBytes[track.Index] = parseSTSZ(p, box, track)
			}
		case "btrt":
			if track != nil {
				parseBTRT(p, box, track)
			}
		case "pasp":
			if track != nil && track.Video != nil {
				if b, e := p.readAt(box.data, 8); e == nil {
					h, v := binary.BigEndian.Uint32(b[:4]), binary.BigEndian.Uint32(b[4:])
					if v > 0 {
						track.Video.PixelAspectRatio = float64(h) / float64(v)
					}
				}
			}
		case "colr":
			if track != nil && track.Video != nil {
				parseCOLR(p, box, track.Video)
			}
		case "data":
			if strings.HasPrefix(parent, "tag:") {
				parseISODataTag(p, box, strings.TrimPrefix(parent, "tag:"))
			}
		case "sidx":
			if p.opts.Mode == ModeDetailed {
				parseSIDX(p, box)
			}
		}

		childStart := box.data
		container := isoContainer(box.typ)
		childParent := box.typ
		if box.typ == "meta" && box.payload >= 4 {
			childStart += 4
		}
		if isISOTagBox(box.typ) && parent == "ilst" {
			container = true
			childParent = "tag:" + box.typ
		}
		if container && childStart < box.start+box.size && box.typ != "stsd" {
			if e := walkISO(p, st, childStart, box.start+box.size, depth+1, childTrack, childParent); e != nil {
				return e
			}
		}
		pos = box.start + box.size
	}
	return nil
}

func isoContainer(t string) bool {
	switch t {
	case "moov", "trak", "mdia", "minf", "stbl", "edts", "dinf", "udta", "ilst", "meta", "mvex", "moof", "traf", "mfra", "tref", "wave":
		return true
	}
	return false
}
func isISOTagBox(t string) bool {
	if len(t) != 4 {
		return false
	}
	switch t {
	case "covr":
		return false
	case "\xa9nam", "\xa9ART", "\xa9alb", "\xa9day", "\xa9gen", "\xa9cmt", "\xa9wrt", "\xa9too", "aART", "trkn", "disk", "gnre", "cprt", "desc", "tmpo":
		return true
	}
	return t == "----"
}

func parseFTYP(p *probe, b isoBox) {
	if b.payload < 8 {
		return
	}
	n := b.payload
	if n > 256 {
		n = 256
	}
	d, e := p.readAt(b.data, int(n))
	if e != nil || len(d) < 8 {
		return
	}
	major := string(d[:4])
	p.report.General.CodecID = major
	p.report.General.CompatibleBrands = nil
	for i := 8; i+4 <= len(d); i += 4 {
		p.report.General.CompatibleBrands = append(p.report.General.CompatibleBrands, string(d[i:i+4]))
	}
	switch strings.TrimSpace(major) {
	case "M4A", "M4B":
		p.report.General.MIME = "audio/mp4"
	case "3gp4", "3gp5", "3gp6", "3gp7", "3gp8", "3gp9":
		p.report.General.Format = "3GPP"
		p.report.General.MIME = "video/3gpp"
	case "3g2a", "3g2b", "3g2c":
		p.report.General.Format = "3GPP2"
		p.report.General.MIME = "video/3gpp2"
	}
}

func parseMVHD(p *probe, b isoBox) {
	n := b.payload
	if n > 40 {
		n = 40
	}
	d, e := p.readAt(b.data, int(n))
	if e != nil || len(d) < 20 {
		return
	}
	ver := d[0]
	var scale uint32
	var dur uint64
	if ver == 1 && len(d) >= 32 {
		scale = binary.BigEndian.Uint32(d[20:24])
		dur = binary.BigEndian.Uint64(d[24:32])
	} else {
		scale = binary.BigEndian.Uint32(d[12:16])
		dur = uint64(binary.BigEndian.Uint32(d[16:20]))
	}
	p.report.General.Duration = durationFromUnits(dur, uint64(scale))
}

func parseTKHD(p *probe, b isoBox, s *Stream) {
	n := b.payload
	if n > 96 {
		n = 96
	}
	d, e := p.readAt(b.data, int(n))
	if e != nil || len(d) < 40 {
		return
	}
	ver := d[0]
	var id uint32
	var matrixOff, widthOff int
	if ver == 1 {
		id = binary.BigEndian.Uint32(d[20:24])
		matrixOff = 52
		widthOff = 88
	} else {
		id = binary.BigEndian.Uint32(d[12:16])
		matrixOff = 40
		widthOff = 76
	}
	if id > 0 {
		s.ID = fmt.Sprint(id)
	}
	if len(d) >= widthOff+8 {
		w := binary.BigEndian.Uint32(d[widthOff : widthOff+4])
		h := binary.BigEndian.Uint32(d[widthOff+4 : widthOff+8])
		if w != 0 || h != 0 {
			video := ensureVideo(s)
			video.DisplayWidth = int(w >> 16)
			video.DisplayHeight = int(h >> 16)
		}
	}
	if len(d) >= matrixOff+20 {
		a := signedInt32Bits(binary.BigEndian.Uint32(d[matrixOff : matrixOff+4]))
		bb := signedInt32Bits(binary.BigEndian.Uint32(d[matrixOff+4 : matrixOff+8]))
		rotation := 0.0
		switch {
		case a == 0 && bb == 0x10000:
			rotation = 90
		case a == -0x10000 && bb == 0:
			rotation = 180
		case a == 0 && bb == -0x10000:
			rotation = 270
		}
		if rotation != 0 {
			ensureVideo(s).Rotation = rotation
		}
	}
}

func parseMDHD(p *probe, b isoBox, s *Stream) {
	n := b.payload
	if n > 40 {
		n = 40
	}
	d, e := p.readAt(b.data, int(n))
	if e != nil || len(d) < 20 {
		return
	}
	ver := d[0]
	var scale uint32
	var dur uint64
	var langOff int
	if ver == 1 && len(d) >= 34 {
		scale = binary.BigEndian.Uint32(d[20:24])
		dur = binary.BigEndian.Uint64(d[24:32])
		langOff = 32
	} else {
		scale = binary.BigEndian.Uint32(d[12:16])
		dur = uint64(binary.BigEndian.Uint32(d[16:20]))
		langOff = 20
	}
	s.Duration = durationFromUnits(dur, uint64(scale))
	if len(d) >= langOff+2 {
		s.Language = parseISO639(binary.BigEndian.Uint16(d[langOff : langOff+2]))
	}
}

func parseHDLR(p *probe, b isoBox, s *Stream) {
	n := b.payload
	if n > 256 {
		n = 256
	}
	d, e := p.readAt(b.data, int(n))
	if e != nil || len(d) < 12 {
		return
	}
	typ := string(d[8:12])
	switch typ {
	case "vide":
		s.Kind = StreamVideo
		ensureVideo(s)
		s.Format = "Video"
	case "soun":
		s.Kind = StreamAudio
		s.Audio = &Audio{}
		s.Format = "Audio"
	case "text", "sbtl", "subt", "clcp":
		s.Kind = StreamText
		s.Text = &Text{}
		s.Format = "Text"
	case "pict":
		s.Kind = StreamImage
		s.Image = &Image{}
		s.Format = "Image"
	case "meta":
		s.Kind = StreamMenu
		s.Format = "Metadata"
	}
	if len(d) > 24 {
		s.Title = cleanText(d[24:])
	}
}

func parseSTSD(p *probe, st *isoState, b isoBox, s *Stream) error {
	if b.payload < 8 {
		return nil
	}
	h, e := p.readAt(b.data, 8)
	if e != nil {
		return e
	}
	count := int(binary.BigEndian.Uint32(h[4:8]))
	pos := b.data + 8
	end := b.start + b.size
	for i := 0; i < count && pos+8 <= end; i++ {
		if e := p.step(); e != nil {
			return e
		}
		entry, e := readISOBox(p, pos, end)
		if e != nil {
			return e
		}
		s.CodecID = entry.typ
		s.Format = isoCodec(entry.typ, s.Kind)
		base := entry.data
		if s.Kind == StreamVideo && entry.payload >= 78 {
			d, e := p.readAt(base, 78)
			if e == nil {
				s.Video.Width = int(binary.BigEndian.Uint16(d[24:26]))
				s.Video.Height = int(binary.BigEndian.Uint16(d[26:28]))
				s.Video.BitDepth = int(binary.BigEndian.Uint16(d[74:76]))
				if s.Video.DisplayWidth == 0 {
					s.Video.DisplayWidth = s.Video.Width
					s.Video.DisplayHeight = s.Video.Height
				}
			}
			if entry.payload > 78 {
				_ = walkISO(p, st, base+78, entry.start+entry.size, 6, s, "sample")
			}
		} else if s.Kind == StreamAudio && entry.payload >= 28 {
			d, e := p.readAt(base, 28)
			if e == nil {
				s.Audio.Channels = int(binary.BigEndian.Uint16(d[16:18]))
				s.Audio.BitDepth = int(binary.BigEndian.Uint16(d[18:20]))
				s.Audio.SampleRate = int(binary.BigEndian.Uint32(d[24:28]) >> 16)
			}
			if entry.payload > 28 {
				_ = walkISO(p, st, base+28, entry.start+entry.size, 6, s, "sample")
			}
		}
		pos = entry.start + entry.size
	}
	return nil
}

func isoCodec(id string, k StreamKind) string {
	switch id {
	case "avc1", "avc3":
		return "AVC"
	case "hvc1", "hev1":
		return "HEVC"
	case "av01":
		return "AV1"
	case "vp08":
		return "VP8"
	case "vp09":
		return "VP9"
	case "mp4v":
		return "MPEG-4 Visual"
	case "jpeg", "mjpg", "mjpa", "mjpb":
		return "Motion JPEG"
	case "mp4a":
		return "AAC"
	case "alac":
		return "ALAC"
	case "ac-3":
		return "AC-3"
	case "ec-3":
		return "E-AC-3"
	case "Opus":
		return "Opus"
	case "fLaC":
		return "FLAC"
	case "lpcm", "sowt", "twos", "ipcm", "fpcm":
		return "PCM"
	case "tx3g":
		return "Timed Text"
	case "wvtt":
		return "WebVTT"
	case "stpp":
		return "TTML"
	}
	if k == StreamVideo {
		return videoCodec(id)
	}
	return id
}

func parseSTTS(p *probe, b isoBox, s *Stream) {
	if b.payload < 8 || b.payload > p.opts.MaxSingleMetadataBytes {
		return
	}
	d, e := p.readAt(b.data, int(b.payload))
	if e != nil {
		return
	}
	count := int(binary.BigEndian.Uint32(d[4:8]))
	pos := 8
	var samples uint64
	var ticks uint64
	for i := 0; i < count && pos+8 <= len(d); i++ {
		n := uint64(binary.BigEndian.Uint32(d[pos : pos+4]))
		delta := uint64(binary.BigEndian.Uint32(d[pos+4 : pos+8]))
		samples += n
		ticks += n * delta
		pos += 8
	}
	s.FrameCount = int64(samples)
	if s.Duration > 0 {
		s.FrameRate = float64(samples) / s.Duration.Seconds()
	}
	_ = ticks
}
func parseSTSZ(p *probe, b isoBox, s *Stream) int64 {
	if b.payload < 12 || b.payload > p.opts.MaxSingleMetadataBytes {
		return 0
	}
	d, e := p.readAt(b.data, int(b.payload))
	if e != nil {
		return 0
	}
	size := binary.BigEndian.Uint32(d[4:8])
	count := binary.BigEndian.Uint32(d[8:12])
	if s.FrameCount == 0 {
		s.FrameCount = int64(count)
	}
	if size > 0 {
		return int64(size) * int64(count)
	}
	var sum int64
	for pos := 12; pos+4 <= len(d); pos += 4 {
		sum += int64(binary.BigEndian.Uint32(d[pos : pos+4]))
	}
	return sum
}
func parseBTRT(p *probe, b isoBox, s *Stream) {
	if b.payload < 12 {
		return
	}
	d, e := p.readAt(b.data, 12)
	if e == nil {
		s.BitRate = int64(binary.BigEndian.Uint32(d[8:12]))
	}
}
func parseCOLR(p *probe, b isoBox, v *Video) {
	if b.payload < 10 {
		return
	}
	d, e := p.readAt(b.data, int(min64(b.payload, 32)))
	if e != nil {
		return
	}
	if string(d[:4]) == "nclx" || string(d[:4]) == "nclc" {
		v.ColorPrimaries = fmt.Sprint(binary.BigEndian.Uint16(d[4:6]))
		v.TransferCharacteristics = fmt.Sprint(binary.BigEndian.Uint16(d[6:8]))
		v.MatrixCoefficients = fmt.Sprint(binary.BigEndian.Uint16(d[8:10]))
		if len(d) > 10 && d[10]&0x80 != 0 {
			v.ColorRange = "Full"
		} else {
			v.ColorRange = "Limited"
		}
	}
}
func parseISODataTag(p *probe, b isoBox, key string) {
	if key == "covr" || b.payload <= 8 || b.payload-8 > p.opts.MaxSingleMetadataBytes {
		return
	}
	d, e := p.readAt(b.data+8, int(b.payload-8))
	if e != nil {
		return
	}
	value := cleanText(d)
	if key == "trkn" && len(d) >= 6 {
		value = fmt.Sprint(binary.BigEndian.Uint16(d[2:4]))
	}
	if key == "disk" && len(d) >= 6 {
		value = fmt.Sprint(binary.BigEndian.Uint16(d[2:4]))
	}
	p.addTag("", canonicalTag(key), value)
}
func parseSIDX(p *probe, b isoBox) {
	if b.payload < 20 || p.report.General.Duration > 0 {
		return
	}
	d, e := p.readAt(b.data, int(min64(b.payload, 40)))
	if e != nil {
		return
	}
	ver := d[0]
	scale := binary.BigEndian.Uint32(d[8:12])
	pos := 12
	if ver == 0 {
		pos += 8
	} else {
		pos += 16
	}
	if scale == 0 || len(d) < pos+4 {
		return
	}
}
func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
