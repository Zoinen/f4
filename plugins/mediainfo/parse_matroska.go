package mediainfo

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

type ebmlState struct {
	docType       string
	timecodeScale uint64
	duration      float64
}
type ebmlContext struct {
	track     *Stream
	chapter   *Chapter
	tagName   string
	tagTarget *string
}

func parseMatroska(p *probe, _ []byte) error {
	p.report.General.Format = "Matroska"
	p.report.General.MIME = "video/x-matroska"
	st := &ebmlState{timecodeScale: 1000000}
	if err := walkEBML(p, st, 0, p.src.Size, 0, ebmlContext{}); err != nil {
		return err
	}
	if st.docType != "matroska" && st.docType != "webm" {
		return ErrUnsupported
	}
	if st.docType == "webm" {
		p.report.General.Format = "WebM"
		p.report.General.MIME = "video/webm"
	}
	if st.duration > 0 {
		p.report.General.Duration = time.Duration(st.duration * float64(st.timecodeScale))
	}
	for i := range p.report.Streams {
		if p.report.Streams[i].Duration == 0 {
			p.report.Streams[i].Duration = p.report.General.Duration
		}
	}
	return nil
}

func readEBMLVInt(p *probe, off int64, keep bool) (uint64, int, bool, error) {
	b, e := p.readAt(off, 1)
	if e != nil {
		return 0, 0, false, e
	}
	first := b[0]
	if first == 0 {
		return 0, 0, false, errors.New("invalid EBML variable integer")
	}
	n := 1
	mask := byte(0x80)
	for n <= 8 && first&mask == 0 {
		n++
		mask >>= 1
	}
	if n > 8 {
		return 0, 0, false, errors.New("invalid EBML variable integer length")
	}
	all, e := p.readAt(off, n)
	if e != nil {
		return 0, 0, false, e
	}
	var v uint64
	if keep {
		for _, x := range all {
			v = v<<8 | uint64(x)
		}
	} else {
		v = uint64(all[0] &^ mask)
		for _, x := range all[1:] {
			v = v<<8 | uint64(x)
		}
	}
	unknown := !keep && v == (uint64(1)<<(7*n))-1
	return v, n, unknown, nil
}

func walkEBML(p *probe, st *ebmlState, start, end int64, depth int, ctx ebmlContext) error {
	if depth > 24 {
		return &ParseError{Format: "Matroska", Offset: start, Err: errors.New("EBML nesting too deep")}
	}
	for pos := start; pos < end; {
		if err := p.step(); err != nil {
			return err
		}
		id, idn, _, e := readEBMLVInt(p, pos, true)
		if e != nil {
			if errors.Is(e, io.EOF) && pos == end {
				return nil
			}
			return &ParseError{Format: "Matroska", Offset: pos, Err: e}
		}
		sz, szn, unknown, e := readEBMLVInt(p, pos+int64(idn), false)
		if e != nil {
			return &ParseError{Format: "Matroska", Offset: pos, Err: e}
		}
		data := pos + int64(idn+szn)
		if sz > math.MaxInt64 {
			return &ParseError{Format: "Matroska", Offset: pos, Err: errors.New("element size exceeds the supported range")}
		}
		// #nosec G115 -- the explicit MaxInt64 check above makes this conversion lossless.
		size := int64(sz)
		if unknown {
			size = end - data
		}
		if size < 0 || data > end || size > end-data {
			return &ParseError{Format: "Matroska", Offset: pos, Err: errors.New("element exceeds parent")}
		}
		if id == 0x1f43b675 || id == 0x1941a469 {
			pos = data + size
			continue
		} // Cluster, Attachments
		child := ctx
		if id == 0xae {
			if len(p.report.Streams) >= p.opts.MaxStreams {
				p.report.Truncated = true
				pos = data + size
				continue
			}
			p.report.Streams = append(p.report.Streams, Stream{Index: len(p.report.Streams), ID: fmt.Sprint(len(p.report.Streams) + 1)})
			child.track = &p.report.Streams[len(p.report.Streams)-1]
		}
		if id == 0xb6 && inEBMLMaster(depth, ctx, "chapter") {
			if len(p.report.Chapters) >= p.opts.MaxChapters {
				p.report.Truncated = true
				pos = data + size
				continue
			}
			p.report.Chapters = append(p.report.Chapters, Chapter{ID: fmt.Sprint(len(p.report.Chapters) + 1)})
			child.chapter = &p.report.Chapters[len(p.report.Chapters)-1]
		}
		if id == 0x67c8 {
			child.tagName = ""
		}
		if id == 0x7373 {
			target := ""
			child.tagTarget = &target
		}
		if isEBMLMaster(id) {
			if e := walkEBML(p, st, data, data+size, depth+1, child); e != nil {
				return e
			}
		} else if e := parseEBMLLeaf(p, st, id, data, size, &ctx); e != nil {
			return e
		}
		if data+size <= pos {
			return &ParseError{Format: "Matroska", Offset: pos, Err: errors.New("non-advancing element")}
		}
		pos = data + size
	}
	return nil
}

func inEBMLMaster(_ int, _ ebmlContext, _ string) bool { return true }
func isEBMLMaster(id uint64) bool {
	switch id {
	case 0x1a45dfa3, 0x18538067, 0x114d9b74, 0x1549a966, 0x1654ae6b, 0xae, 0xe0, 0xe1, 0x55b0, 0x1254c367, 0x7373, 0x63c0, 0x67c8, 0x1043a770, 0x45b9, 0xb6, 0x80, 0x6d80, 0x6240, 0x5035:
		return true
	}
	return false
}

func parseEBMLLeaf(p *probe, st *ebmlState, id uint64, off, size int64, ctx *ebmlContext) error {
	kind := classifyEBMLLeaf(id)
	if kind == ebmlLeafUnknown || !needEBMLLeaf(id, ctx) {
		// Most EBML leaves are codec private data, checksums, seek indexes, or
		// other values this normalized report does not expose. Do not spend the
		// metadata budget reading them merely to discard them.
		return nil
	}

	var (
		uintValue  uint64
		floatValue float64
		textValue  string
		ok         bool
		e          error
	)
	switch kind {
	case ebmlLeafUint:
		uintValue, ok, e = readEBMLUint(p, off, size)
	case ebmlLeafFloat:
		floatValue, ok, e = readEBMLFloat(p, off, size)
	case ebmlLeafText:
		textValue, ok, e = readEBMLText(p, off, size)
	case ebmlLeafDocType:
		textValue, ok, e = readEBMLDocType(p, off, size)
	}
	if e != nil || !ok {
		return e
	}
	u := func() uint64 { return uintValue }
	str := func() string { return textValue }
	flt := func() float64 { return floatValue }
	integer := func(field string) (int, error) {
		value := u()
		converted, ok := metadataInt(value)
		if !ok {
			return 0, &ParseError{Format: "Matroska", Offset: off, Err: fmt.Errorf("%s value %d exceeds the supported integer range", field, value)}
		}
		return converted, nil
	}
	duration := func(field string) (time.Duration, error) {
		value := u()
		converted, ok := metadataDuration(value)
		if !ok {
			return 0, &ParseError{Format: "Matroska", Offset: off, Err: fmt.Errorf("%s value %d exceeds the supported duration range", field, value)}
		}
		return converted, nil
	}
	switch id {
	case 0x4282:
		st.docType = str()
	case 0x2ad7b1:
		st.timecodeScale = u()
	case 0x4489:
		st.duration = flt()
	case 0x7ba9:
		p.addTag("", "Title", str())
	case 0x4d80:
		p.report.General.MuxingApp = str()
	case 0x5741:
		p.report.General.WritingApp = str()
	case 0xd7:
		if ctx.track != nil {
			ctx.track.ID = fmt.Sprint(u())
		}
	case 0x73c5:
		if ctx.track != nil && ctx.track.ID == "" {
			ctx.track.ID = fmt.Sprint(u())
		}
	case 0x83:
		if ctx.track != nil {
			setMatroskaTrackType(ctx.track, u())
		}
	case 0x88:
		if ctx.track != nil {
			ctx.track.Default = boolPtr(u() != 0)
		}
	case 0x55aa:
		if ctx.track != nil {
			ctx.track.Forced = boolPtr(u() != 0)
		}
	case 0x536e:
		if ctx.track != nil {
			ctx.track.Title = str()
		}
	case 0x22b59d, 0x22b59c:
		if ctx.track != nil {
			ctx.track.Language = str()
		}
	case 0x86:
		if ctx.track != nil {
			ctx.track.CodecID = str()
			ctx.track.Format = matroskaCodec(ctx.track.CodecID)
		}
	case 0x258688:
		if ctx.track != nil {
			ctx.track.CodecName = str()
		}
	case 0x23e383:
		if ctx.track != nil && u() > 0 {
			ctx.track.FrameRate = 1e9 / float64(u())
		}
	case 0xb0:
		if ctx.track != nil {
			value, err := integer("PixelWidth")
			if err != nil {
				return err
			}
			ensureVideo(ctx.track).Width = value
		}
	case 0xba:
		if ctx.track != nil {
			value, err := integer("PixelHeight")
			if err != nil {
				return err
			}
			ensureVideo(ctx.track).Height = value
		}
	case 0x54b0:
		if ctx.track != nil {
			value, err := integer("DisplayWidth")
			if err != nil {
				return err
			}
			ensureVideo(ctx.track).DisplayWidth = value
		}
	case 0x54ba:
		if ctx.track != nil {
			value, err := integer("DisplayHeight")
			if err != nil {
				return err
			}
			ensureVideo(ctx.track).DisplayHeight = value
		}
	case 0x9a:
		if ctx.track != nil && u() != 0 {
			ensureVideo(ctx.track).ScanType = "Interlaced"
		}
	case 0x55b1:
		if ctx.track != nil {
			ensureVideo(ctx.track).MatrixCoefficients = fmt.Sprint(u())
		}
	case 0x55b9:
		if ctx.track != nil {
			ensureVideo(ctx.track).ColorRange = map[uint64]string{1: "Limited", 2: "Full"}[u()]
		}
	case 0x55ba:
		if ctx.track != nil {
			ensureVideo(ctx.track).TransferCharacteristics = fmt.Sprint(u())
		}
	case 0x55bb:
		if ctx.track != nil {
			ensureVideo(ctx.track).ColorPrimaries = fmt.Sprint(u())
		}
	case 0x55b2:
		if ctx.track != nil {
			value, err := integer("BitsPerChannel")
			if err != nil {
				return err
			}
			ensureVideo(ctx.track).BitDepth = value
		}
	case 0xb5:
		if ctx.track != nil {
			ensureAudio(ctx.track).SampleRate = int(flt())
		}
	case 0x9f:
		if ctx.track != nil {
			value, err := integer("Channels")
			if err != nil {
				return err
			}
			ensureAudio(ctx.track).Channels = value
		}
	case 0x6264:
		if ctx.track != nil {
			value, err := integer("BitDepth")
			if err != nil {
				return err
			}
			ensureAudio(ctx.track).BitDepth = value
		}
	case 0x45a3:
		ctx.tagName = str()
	case 0x4487:
		if ctx.tagName != "" {
			target := ""
			if ctx.tagTarget != nil {
				target = *ctx.tagTarget
			}
			p.addTag(target, canonicalTag(ctx.tagName), str())
		}
	case 0x63c5:
		if ctx.tagTarget != nil {
			*ctx.tagTarget = fmt.Sprint(u())
		}
	case 0x91:
		if ctx.chapter != nil {
			value, err := duration("ChapterTimeStart")
			if err != nil {
				return err
			}
			ctx.chapter.Start = value
		}
	case 0x92:
		if ctx.chapter != nil {
			value, err := duration("ChapterTimeEnd")
			if err != nil {
				return err
			}
			ctx.chapter.End = value
		}
	case 0x85:
		if ctx.chapter != nil {
			ctx.chapter.Title = str()
		}
	case 0x437c:
		if ctx.chapter != nil {
			ctx.chapter.Language = str()
		}
	}
	return nil
}

type ebmlLeafKind uint8

const (
	ebmlLeafUnknown ebmlLeafKind = iota
	ebmlLeafUint
	ebmlLeafFloat
	ebmlLeafText
	ebmlLeafDocType
)

// classifyEBMLLeaf is intentionally exhaustive for the fields handled by
// parseEBMLLeaf. Classification happens before any payload read so an unknown
// multi-gigabyte leaf costs only its EBML header.
func classifyEBMLLeaf(id uint64) ebmlLeafKind {
	switch id {
	case 0x4282:
		return ebmlLeafDocType
	case 0x2ad7b1, 0xd7, 0x73c5, 0x83, 0x88, 0x55aa, 0x23e383,
		0xb0, 0xba, 0x54b0, 0x54ba, 0x9a, 0x55b1, 0x55b9, 0x55ba,
		0x55bb, 0x55b2, 0x9f, 0x6264, 0x63c5, 0x91, 0x92:
		return ebmlLeafUint
	case 0x4489, 0xb5:
		return ebmlLeafFloat
	case 0x7ba9, 0x4d80, 0x5741, 0x536e, 0x22b59d, 0x22b59c,
		0x86, 0x258688, 0x45a3, 0x4487, 0x85, 0x437c:
		return ebmlLeafText
	default:
		return ebmlLeafUnknown
	}
}

// needEBMLLeaf avoids bounded-but-still-unnecessary reads for fields outside
// a track/chapter/tag context.
func needEBMLLeaf(id uint64, ctx *ebmlContext) bool {
	switch id {
	case 0xd7, 0x83, 0x88, 0x55aa, 0x536e, 0x22b59d, 0x22b59c,
		0x86, 0x258688, 0x23e383, 0xb0, 0xba, 0x54b0, 0x54ba, 0x9a,
		0x55b1, 0x55b9, 0x55ba, 0x55bb, 0x55b2, 0xb5, 0x9f, 0x6264:
		return ctx.track != nil
	case 0x73c5:
		return ctx.track != nil && ctx.track.ID == ""
	case 0x4487:
		return ctx.tagName != ""
	case 0x63c5:
		return ctx.tagTarget != nil
	case 0x91, 0x92, 0x85, 0x437c:
		return ctx.chapter != nil
	default:
		return true
	}
}

func readEBMLUint(p *probe, off, size int64) (uint64, bool, error) {
	if size < 1 || size > 8 {
		if size > 8 {
			p.report.Truncated = true
		}
		return 0, false, nil
	}
	b, err := p.readAt(off, int(size))
	if err != nil {
		return 0, false, err
	}
	var value uint64
	for _, x := range b {
		value = value<<8 | uint64(x)
	}
	return value, true, nil
}

func readEBMLFloat(p *probe, off, size int64) (float64, bool, error) {
	if size != 4 && size != 8 {
		if size > 8 {
			p.report.Truncated = true
		}
		return 0, false, nil
	}
	b, err := p.readAt(off, int(size))
	if err != nil {
		return 0, false, err
	}
	if size == 4 {
		return float64(math.Float32frombits(binary.BigEndian.Uint32(b))), true, nil
	}
	return math.Float64frombits(binary.BigEndian.Uint64(b)), true, nil
}

func readEBMLText(p *probe, off, size int64) (string, bool, error) {
	if size < 0 {
		return "", false, nil
	}
	if size == 0 {
		return "", true, nil
	}
	limit := int64(p.opts.MaxValueBytes)
	if p.opts.MaxSingleMetadataBytes < limit {
		limit = p.opts.MaxSingleMetadataBytes
	}
	readSize := size
	if readSize > limit {
		readSize = limit
		p.report.Truncated = true
	}
	b, err := p.readAt(off, int(readSize))
	if err != nil {
		return "", false, err
	}
	// Clone after trimming so the report owns exactly the retained bytes and
	// never keeps a larger metadata allocation alive.
	return strings.Clone(cleanText(b)), true, nil
}

func readEBMLDocType(p *probe, off, size int64) (string, bool, error) {
	// The Matroska/WebM DocType values are tiny. Refuse implausibly large
	// declarations rather than reading or accepting an unverified prefix.
	const maxDocTypeBytes = 32
	if size < 1 || size > maxDocTypeBytes || size > p.opts.MaxSingleMetadataBytes {
		if size > maxDocTypeBytes || size > p.opts.MaxSingleMetadataBytes {
			p.report.Truncated = true
		}
		return "", false, nil
	}
	b, err := p.readAt(off, int(size))
	if err != nil {
		return "", false, err
	}
	return strings.Clone(cleanText(b)), true, nil
}

func setMatroskaTrackType(s *Stream, v uint64) {
	switch v {
	case 1:
		s.Kind = StreamVideo
		s.Video = &Video{}
	case 2:
		s.Kind = StreamAudio
		s.Audio = &Audio{}
	case 17:
		s.Kind = StreamText
		s.Text = &Text{}
	default:
		s.Kind = StreamText
	}
}
func ensureVideo(s *Stream) *Video {
	if s.Video == nil {
		s.Video = &Video{}
	}
	return s.Video
}
func ensureAudio(s *Stream) *Audio {
	if s.Audio == nil {
		s.Audio = &Audio{}
	}
	return s.Audio
}
func matroskaCodec(id string) string {
	switch id {
	case "V_MPEG4/ISO/AVC":
		return "AVC"
	case "V_MPEGH/ISO/HEVC":
		return "HEVC"
	case "V_AV1":
		return "AV1"
	case "V_VP8":
		return "VP8"
	case "V_VP9":
		return "VP9"
	case "V_THEORA":
		return "Theora"
	case "A_AAC":
		return "AAC"
	case "A_OPUS":
		return "Opus"
	case "A_VORBIS":
		return "Vorbis"
	case "A_FLAC":
		return "FLAC"
	case "A_MPEG/L3":
		return "MPEG Audio Layer 3"
	case "A_AC3":
		return "AC-3"
	case "A_EAC3":
		return "E-AC-3"
	case "A_DTS":
		return "DTS"
	case "S_TEXT/UTF8":
		return "SubRip"
	case "S_TEXT/ASS":
		return "ASS"
	case "S_TEXT/SSA":
		return "SSA"
	case "S_TEXT/WEBVTT":
		return "WebVTT"
	case "S_HDMV/PGS":
		return "PGS"
	case "S_VOBSUB":
		return "VobSub"
	}
	if i := strings.LastIndexByte(id, '/'); i >= 0 {
		return strings.Clone(id[i+1:])
	}
	return strings.Clone(id)
}
