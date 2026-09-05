package mediainfo

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

type mpegHeader struct {
	version, layer                                    string
	bitRate, sampleRate, channels, frameSize, samples int
	channelMode                                       string
}

func looksLikeMPEGAudio(b []byte) bool {
	for i := 0; i+4 <= len(b) && i < 4096; i++ {
		if _, ok := decodeMPEGHeader(b[i : i+4]); ok {
			return true
		}
	}
	return false
}

func decodeMPEGHeader(b []byte) (mpegHeader, bool) {
	if len(b) < 4 || b[0] != 0xff || b[1]&0xe0 != 0xe0 {
		return mpegHeader{}, false
	}
	vbits, lbits := (b[1]>>3)&3, (b[1]>>1)&3
	if vbits == 1 || lbits == 0 {
		return mpegHeader{}, false
	}
	version := map[byte]string{0: "2.5", 2: "2", 3: "1"}[vbits]
	layerNum := map[byte]int{1: 3, 2: 2, 3: 1}[lbits]
	brIndex, srIndex := (b[2]>>4)&15, (b[2]>>2)&3
	if brIndex == 0 || brIndex == 15 || srIndex == 3 {
		return mpegHeader{}, false
	}
	var brs []int
	if version == "1" {
		switch layerNum {
		case 1:
			brs = []int{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448}
		case 2:
			brs = []int{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384}
		default:
			brs = []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
		}
	} else {
		if layerNum == 1 {
			brs = []int{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256}
		} else {
			brs = []int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
		}
	}
	base := []int{44100, 48000, 32000}[srIndex]
	switch version {
	case "2":
		base /= 2
	case "2.5":
		base /= 4
	}
	bitRate := brs[brIndex] * 1000
	padding := int((b[2] >> 1) & 1)
	samples := 1152
	var size int
	if layerNum == 1 {
		samples = 384
		size = (12*bitRate/base + padding) * 4
	} else if layerNum == 3 && version != "1" {
		samples = 576
		size = 72*bitRate/base + padding
	} else {
		size = 144*bitRate/base + padding
	}
	mode := (b[3] >> 6) & 3
	modeName := []string{"Stereo", "Joint stereo", "Dual channel", "Mono"}[mode]
	return mpegHeader{version: version, layer: fmt.Sprint(layerNum), bitRate: bitRate, sampleRate: base, channels: map[bool]int{true: 1, false: 2}[mode == 3], frameSize: size, samples: samples, channelMode: modeName}, size >= 4
}

func parseMPEGAudio(p *probe, head []byte) error {
	p.report.General.Format = "MPEG Audio"
	p.report.General.MIME = "audio/mpeg"
	start := int64(0)
	if len(head) >= 10 && string(head[:3]) == "ID3" {
		sz := int64(syncSafe(head[6:10]))
		start = 10 + sz
		if head[5]&0x10 != 0 {
			start += 10
		}
		if sz <= p.opts.MaxSingleMetadataBytes {
			if err := parseID3v2(p, 0, int(10+sz)); err != nil {
				p.warn("id3", err.Error(), 0)
			}
		} else {
			p.warn("tag_limit", "ID3 tag is too large and was skipped", 0)
		}
	}
	search := int64(256 << 10)
	if p.src.Size-start < search {
		search = p.src.Size - start
	}
	if search <= 0 {
		return &ParseError{Format: "MPEG Audio", Offset: start, Err: errors.New("audio frame not found")}
	}
	b, err := p.readAt(start, int(search))
	if err != nil && !errors.Is(err, ErrLimit) {
		return err
	}
	frameOff := -1
	var mh mpegHeader
	for i := 0; i+4 <= len(b); i++ {
		h, ok := decodeMPEGHeader(b[i : i+4])
		if ok {
			if i+h.frameSize+4 <= len(b) {
				if h2, ok2 := decodeMPEGHeader(b[i+h.frameSize : i+h.frameSize+4]); !ok2 || h2.sampleRate != h.sampleRate {
					continue
				}
			}
			frameOff = i
			mh = h
			break
		}
	}
	if frameOff < 0 {
		return &ParseError{Format: "MPEG Audio", Offset: start, Err: errors.New("audio frame not found")}
	}
	start += int64(frameOff)
	p.report.General.FormatProfile = "Layer " + mh.layer
	stream := Stream{Index: 0, ID: "1", Kind: StreamAudio, Format: "MPEG Audio", Profile: "Version " + mh.version + " Layer " + mh.layer, CodecID: "MP" + mh.layer, BitRate: int64(mh.bitRate), BitRateMode: "CBR", Audio: &Audio{Channels: mh.channels, ChannelLayout: mh.channelMode, SampleRate: mh.sampleRate, CompressionMode: "Lossy"}}
	firstLen := mh.frameSize
	if firstLen > int(p.opts.MaxSingleMetadataBytes) {
		firstLen = int(p.opts.MaxSingleMetadataBytes)
	}
	first, _ := p.readAt(start, firstLen)
	frames, audioBytes := parseXingVBRI(first, mh)
	if frames > 0 {
		stream.FrameCount = int64(frames)
		// #nosec G115 -- frames comes from uint32 metadata, while samples and sampleRate are bounded MPEG-header table values.
		stream.Duration = durationFromUnits(uint64(frames)*uint64(mh.samples), uint64(mh.sampleRate))
		stream.BitRateMode = "VBR"
		if audioBytes > 0 && stream.Duration > 0 {
			stream.BitRate = int64(float64(audioBytes*8) / stream.Duration.Seconds())
		}
	}
	if stream.Duration == 0 {
		payload := p.src.Size - start
		if payload > 128 {
			tail, _ := p.readAt(p.src.Size-128, 128)
			if len(tail) >= 3 && string(tail[:3]) == "TAG" {
				payload -= 128
				parseID3v1(p, tail)
			}
		}
		stream.Duration = time.Duration(float64(payload*8) / float64(mh.bitRate) * float64(time.Second))
		stream.DurationEstimated = true
	}
	p.report.General.Duration = stream.Duration
	p.report.Streams = append(p.report.Streams, stream)
	return nil
}

func parseXingVBRI(frame []byte, h mpegHeader) (frames int, bytesCount int64) {
	for _, sig := range []string{"Xing", "Info"} {
		if i := bytes.Index(frame, []byte(sig)); i >= 0 && i+12 <= len(frame) {
			flags := binary.BigEndian.Uint32(frame[i+4 : i+8])
			pos := i + 8
			if flags&1 != 0 && pos+4 <= len(frame) {
				frames = int(binary.BigEndian.Uint32(frame[pos : pos+4]))
				pos += 4
			}
			if flags&2 != 0 && pos+4 <= len(frame) {
				bytesCount = int64(binary.BigEndian.Uint32(frame[pos : pos+4]))
			}
			return
		}
	}
	if i := bytes.Index(frame, []byte("VBRI")); i >= 0 && i+18 <= len(frame) {
		bytesCount = int64(binary.BigEndian.Uint32(frame[i+10 : i+14]))
		frames = int(binary.BigEndian.Uint32(frame[i+14 : i+18]))
	}
	return
}

func syncSafe(b []byte) int {
	if len(b) < 4 {
		return 0
	}
	return int(b[0]&0x7f)<<21 | int(b[1]&0x7f)<<14 | int(b[2]&0x7f)<<7 | int(b[3]&0x7f)
}

func parseID3v2(p *probe, off int64, total int) error {
	b, err := p.readAt(off, total)
	if err != nil {
		return err
	}
	if len(b) < 10 {
		return io.ErrUnexpectedEOF
	}
	ver := b[3]
	data := b[10:]
	if b[5]&0x80 != 0 {
		data = deunsync(data)
	}
	for len(data) >= 6 && data[0] != 0 {
		var id string
		var size, header int
		if ver == 2 {
			id = string(data[:3])
			size = int(data[3])<<16 | int(data[4])<<8 | int(data[5])
			header = 6
		} else {
			if len(data) < 10 {
				break
			}
			id = string(data[:4])
			if ver == 4 {
				size = syncSafe(data[4:8])
			} else {
				size = int(binary.BigEndian.Uint32(data[4:8]))
			}
			header = 10
		}
		if size < 0 || header+size > len(data) {
			break
		}
		payload := data[header : header+size]
		name := canonicalTag(id)
		if isID3Text(id) && len(payload) > 1 {
			p.addTag("", name, decodeID3Text(payload))
		} else if id == "COMM" && len(payload) > 4 {
			p.addTag("", "Comment", decodeID3Text(append([]byte{payload[0]}, payload[4:]...)))
		}
		data = data[header+size:]
	}
	return nil
}

func isID3Text(id string) bool {
	return strings.HasPrefix(id, "T") && id != "TXXX" || id == "TT2" || id == "TP1" || id == "TAL" || id == "TRK" || id == "TYE"
}
func decodeID3Text(b []byte) string {
	if len(b) < 2 {
		return ""
	}
	enc := b[0]
	d := b[1:]
	switch enc {
	case 0, 3:
		return cleanText(d)
	case 1:
		if len(d) >= 2 && d[0] == 0xff && d[1] == 0xfe {
			return decodeUTF16(d[2:], true)
		}
		if len(d) >= 2 && d[0] == 0xfe && d[1] == 0xff {
			return decodeUTF16(d[2:], false)
		}
		return decodeUTF16(d, true)
	case 2:
		return decodeUTF16(d, false)
	}
	return ""
}
func deunsync(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		out = append(out, b[i])
		if b[i] == 0xff && i+1 < len(b) && b[i+1] == 0 {
			i++
		}
	}
	return out
}
func parseID3v1(p *probe, b []byte) {
	if len(b) < 128 {
		return
	}
	p.addTag("", "Title", cleanText(b[3:33]))
	p.addTag("", "Artist", cleanText(b[33:63]))
	p.addTag("", "Album", cleanText(b[63:93]))
	p.addTag("", "Date", cleanText(b[93:97]))
}

func parseFLAC(p *probe, _ []byte) error {
	p.report.General.Format = "FLAC"
	p.report.General.MIME = "audio/flac"
	stream := Stream{Index: 0, ID: "1", Kind: StreamAudio, Format: "FLAC", Audio: &Audio{CompressionMode: "Lossless"}}
	pos := int64(4)
	last := false
	for !last {
		if err := p.step(); err != nil {
			return err
		}
		h, err := p.readAt(pos, 4)
		if err != nil {
			return &ParseError{Format: "FLAC", Offset: pos, Err: err}
		}
		last = h[0]&0x80 != 0
		typ := h[0] & 0x7f
		sz := int64(h[1])<<16 | int64(h[2])<<8 | int64(h[3])
		data := pos + 4
		if sz < 0 || data+sz > p.src.Size {
			return &ParseError{Format: "FLAC", Offset: pos, Err: errors.New("metadata block exceeds file")}
		}
		switch typ {
		case 0:
			if sz >= 34 {
				b, e := p.readAt(data, 34)
				if e != nil {
					return e
				}
				v := binary.BigEndian.Uint64(b[10:18])
				rate := int(v >> 44)
				channels := int((v>>41)&7) + 1
				bits := int((v>>36)&31) + 1
				samples := v & 0xfffffffff
				stream.Audio.SampleRate = rate
				stream.Audio.Channels = channels
				stream.Audio.BitDepth = bits
				stream.Duration = durationFromUnits(samples, uint64(rate))
			}
		case 4:
			if sz <= p.opts.MaxSingleMetadataBytes {
				b, e := p.readAt(data, int(sz))
				if e == nil {
					parseVorbisComments(p, b)
				}
			}
		case 6:
			if sz >= 32 && sz <= p.opts.MaxSingleMetadataBytes {
				b, e := p.readAt(data, int(sz))
				if e == nil {
					parseFLACPictureInfo(p, b)
				}
			}
		}
		pos = data + sz
	}
	p.report.General.Duration = stream.Duration
	p.report.Streams = append(p.report.Streams, stream)
	return nil
}

func parseVorbisComments(p *probe, b []byte) {
	if len(b) < 8 {
		return
	}
	vendorSize := uint64(binary.LittleEndian.Uint32(b[:4]))
	if vendorSize > uint64(len(b)-8) { // #nosec G115 -- len(b)-8 is non-negative because the header length was checked above.
		return
	}
	// #nosec G115 -- vendorSize was checked against the in-memory slice length above.
	vendorLen := int(vendorSize)
	vendor := b[4 : 4+vendorLen]
	if len(vendor) > p.opts.MaxValueBytes {
		vendor = vendor[:p.opts.MaxValueBytes]
		p.report.Truncated = true
	}
	p.report.General.WritingApp = strings.Clone(cleanText(vendor))
	pos := 4 + vendorLen
	count := int(binary.LittleEndian.Uint32(b[pos : pos+4]))
	pos += 4
	for i := 0; i < count && pos+4 <= len(b); i++ {
		n := int(binary.LittleEndian.Uint32(b[pos : pos+4]))
		pos += 4
		if n < 0 || pos+n > len(b) {
			break
		}
		s := string(b[pos : pos+n])
		pos += n
		if eq := strings.IndexByte(s, '='); eq > 0 {
			p.addTag("", canonicalTag(s[:eq]), s[eq+1:])
		}
	}
}
func parseFLACPictureInfo(p *probe, b []byte) {
	if len(b) < 32 {
		return
	}
	pos := 4
	mimeLen := int(binary.BigEndian.Uint32(b[pos : pos+4]))
	pos += 4 + mimeLen
	if pos+4 > len(b) {
		return
	}
	descLen := int(binary.BigEndian.Uint32(b[pos : pos+4]))
	pos += 4 + descLen
	if pos+16 > len(b) {
		return
	}
	p.addTag("", "Cover dimensions", fmt.Sprintf("%dx%d", binary.BigEndian.Uint32(b[pos:pos+4]), binary.BigEndian.Uint32(b[pos+4:pos+8])))
}

func parseOgg(p *probe, _ []byte) error {
	p.report.General.Format = "Ogg"
	p.report.General.MIME = "application/ogg"
	packets, serial, err := readOggPackets(p, 3)
	if err != nil {
		return err
	}
	if len(packets) == 0 {
		return &ParseError{Format: "Ogg", Offset: 0, Err: errors.New("no packets")}
	}
	s := Stream{Index: 0, ID: fmt.Sprint(serial), Kind: StreamAudio, Audio: &Audio{}}
	first := packets[0]
	switch {
	case len(first) >= 30 && first[0] == 1 && string(first[1:7]) == "vorbis":
		s.Format = "Vorbis"
		s.Audio.Channels = int(first[11])
		s.Audio.SampleRate = int(binary.LittleEndian.Uint32(first[12:16]))
		s.Audio.CompressionMode = "Lossy"
		if len(packets) > 1 && len(packets[1]) > 7 && packets[1][0] == 3 {
			parseVorbisComments(p, packets[1][7:])
		}
	case len(first) >= 19 && string(first[:8]) == "OpusHead":
		s.Format = "Opus"
		s.Audio.Channels = int(first[9])
		s.Audio.SampleRate = 48000
		s.Audio.Delay = durationFromUnits(uint64(binary.LittleEndian.Uint16(first[10:12])), 48000)
		s.Audio.CompressionMode = "Lossy"
		if len(packets) > 1 && len(packets[1]) > 8 && string(packets[1][:8]) == "OpusTags" {
			parseVorbisComments(p, packets[1][8:])
		}
	case len(first) >= 80 && string(first[:8]) == "Speex   ":
		s.Format = "Speex"
		s.Audio.SampleRate = int(binary.LittleEndian.Uint32(first[36:40]))
		s.Audio.Channels = int(binary.LittleEndian.Uint32(first[48:52]))
		s.Audio.CompressionMode = "Lossy"
	case len(first) >= 7 && first[0] == 0x80 && string(first[1:7]) == "theora":
		s.Kind = StreamVideo
		s.Format = "Theora"
		s.Audio = nil
		s.Video = &Video{}
		parseTheoraIdentification(first, &s)
	case isOggFLACHeader(first):
		s.Format = "FLAC"
		s.CodecID = "FLAC"
		s.Audio.CompressionMode = "Lossless"
		if !parseOggFLACStreamInfo(first, &s) && len(packets) > 1 {
			parseOggFLACStreamInfo(packets[1], &s)
		}
	default:
		s.Format = "Unknown"
		s.CodecID = fmt.Sprintf("%x", first[:minInt(len(first), 8)])
	}
	granule := findLastOggGranule(p, serial)
	if granule > 0 && s.Audio != nil && s.Audio.SampleRate > 0 {
		s.Duration = durationFromUnits(uint64(granule), uint64(s.Audio.SampleRate))
		p.report.General.Duration = s.Duration
	}
	p.report.Streams = append(p.report.Streams, s)
	return nil
}

func isOggFLACHeader(b []byte) bool {
	return len(b) >= 4 && string(b[:4]) == "fLaC" || len(b) >= 5 && b[0] == 0x7f && string(b[1:5]) == "FLAC"
}

func parseOggFLACStreamInfo(packet []byte, stream *Stream) bool {
	marker := bytes.Index(packet, []byte("fLaC"))
	start := 0
	if marker >= 0 {
		start = marker + 4
	}
	if start+4+34 > len(packet) {
		return false
	}
	h := packet[start : start+4]
	if h[0]&0x7f != 0 || int(h[1])<<16|int(h[2])<<8|int(h[3]) < 34 {
		return false
	}
	b := packet[start+4 : start+4+34]
	v := binary.BigEndian.Uint64(b[10:18])
	rate := int(v >> 44)
	stream.Audio.SampleRate = rate
	stream.Audio.Channels = int((v>>41)&7) + 1
	stream.Audio.BitDepth = int((v>>36)&31) + 1
	stream.Duration = durationFromUnits(v&0xfffffffff, uint64(rate))
	return rate > 0
}

func parseTheoraIdentification(packet []byte, stream *Stream) {
	if len(packet) < 42 || stream.Video == nil {
		return
	}
	width := int(packet[14])<<16 | int(packet[15])<<8 | int(packet[16])
	height := int(packet[17])<<16 | int(packet[18])<<8 | int(packet[19])
	if width == 0 {
		width = int(binary.BigEndian.Uint16(packet[10:12])) * 16
	}
	if height == 0 {
		height = int(binary.BigEndian.Uint16(packet[12:14])) * 16
	}
	stream.Video.Width, stream.Video.Height = width, height
	numerator := binary.BigEndian.Uint32(packet[22:26])
	denominator := binary.BigEndian.Uint32(packet[26:30])
	if numerator > 0 && denominator > 0 {
		stream.FrameRate = float64(numerator) / float64(denominator)
	}
	stream.Profile = fmt.Sprintf("Theora %d.%d.%d", packet[7], packet[8], packet[9])
}

func readOggPackets(p *probe, maxPackets int) ([][]byte, uint32, error) {
	var packets [][]byte
	var cur []byte
	pos := int64(0)
	var serial uint32
	for pos+27 <= p.src.Size && len(packets) < maxPackets {
		if err := p.step(); err != nil {
			return packets, serial, err
		}
		h, e := p.readAt(pos, 27)
		if e != nil {
			return packets, serial, e
		}
		if string(h[:4]) != "OggS" {
			return packets, serial, &ParseError{Format: "Ogg", Offset: pos, Err: errors.New("bad page capture")}
		}
		nseg := int(h[26])
		lace, e := p.readAt(pos+27, nseg)
		if e != nil {
			return packets, serial, e
		}
		payloadLen := 0
		for _, v := range lace {
			payloadLen += int(v)
		}
		if int64(payloadLen) > p.opts.MaxSingleMetadataBytes {
			return packets, serial, ErrLimit
		}
		// A packet may span arbitrarily many pages even though each individual
		// page payload is small. Validate the accumulated packet size before
		// reading this page's payload or growing cur.
		packetLen := int64(len(cur))
		packetCount := len(packets)
		for _, v := range lace {
			segmentLen := int64(v)
			if packetLen > p.opts.MaxSingleMetadataBytes-segmentLen {
				return packets, serial, ErrLimit
			}
			packetLen += segmentLen
			if v < 255 {
				packetCount++
				packetLen = 0
				if packetCount >= maxPackets {
					break
				}
			}
		}
		payload, e := p.readAt(pos+27+int64(nseg), payloadLen)
		if e != nil {
			return packets, serial, e
		}
		if serial == 0 {
			serial = binary.LittleEndian.Uint32(h[14:18])
		}
		at := 0
		for _, v := range lace {
			cur = append(cur, payload[at:at+int(v)]...)
			at += int(v)
			if v < 255 {
				packets = append(packets, cur)
				cur = nil
				if len(packets) >= maxPackets {
					break
				}
			}
		}
		pos += 27 + int64(nseg+payloadLen)
	}
	return packets, serial, nil
}
func findLastOggGranule(p *probe, serial uint32) int64 {
	n := int64(256 << 10)
	if p.src.Size < n {
		n = p.src.Size
	}
	b, e := p.readAt(p.src.Size-n, int(n))
	if e != nil {
		return 0
	}
	for i := len(b) - 27; i >= 0; i-- {
		if string(b[i:i+4]) == "OggS" && binary.LittleEndian.Uint32(b[i+14:i+18]) == serial {
			granule := binary.LittleEndian.Uint64(b[i+6 : i+14])
			if granule > math.MaxInt64 {
				return 0
			}
			// #nosec G115 -- the explicit MaxInt64 check above makes this conversion lossless.
			return int64(granule)
		}
	}
	return 0
}

func parseAIFF(p *probe, _ []byte) error {
	h, e := p.readAt(0, 12)
	if e != nil {
		return e
	}
	form := string(h[8:12])
	p.report.General.Format = form
	p.report.General.MIME = "audio/aiff"
	s := Stream{Index: 0, ID: "1", Kind: StreamAudio, Format: "PCM", Audio: &Audio{CompressionMode: "Lossless"}}
	for pos := int64(12); pos+8 <= p.src.Size; {
		if e := p.step(); e != nil {
			return e
		}
		ch, e := p.readAt(pos, 8)
		if e != nil {
			return e
		}
		id := string(ch[:4])
		sz := int64(binary.BigEndian.Uint32(ch[4:8]))
		data := pos + 8
		if data+sz > p.src.Size {
			return &ParseError{Format: form, Offset: pos, Err: io.ErrUnexpectedEOF}
		}
		switch id {
		case "COMM":
			n := sz
			if n > 64 {
				n = 64
			}
			b, _ := p.readAt(data, int(n))
			if len(b) >= 18 {
				s.Audio.Channels = int(binary.BigEndian.Uint16(b[:2]))
				frames := binary.BigEndian.Uint32(b[2:6])
				s.Audio.BitDepth = int(binary.BigEndian.Uint16(b[6:8]))
				sampleRate := extended80(b[8:18])
				if math.IsNaN(sampleRate) || math.IsInf(sampleRate, 0) || sampleRate <= 0 || sampleRate > float64(^uint(0)>>1) {
					return &ParseError{Format: form, Offset: data + 8, Err: errors.New("invalid AIFF sample rate")}
				}
				s.Audio.SampleRate = int(sampleRate)
				s.Duration = durationFromUnits(uint64(frames), uint64(sampleRate))
				if form == "AIFC" && len(b) >= 22 {
					s.CodecID = fourCC(b[18:22])
					s.Format = aiffCodec(s.CodecID)
				}
			}
		case "NAME":
			if sz <= p.opts.MaxSingleMetadataBytes {
				b, _ := p.readAt(data, int(sz))
				p.addTag("", "Title", cleanText(b))
			}
		case "AUTH":
			if sz <= p.opts.MaxSingleMetadataBytes {
				b, _ := p.readAt(data, int(sz))
				p.addTag("", "Artist", cleanText(b))
			}
		case "ANNO":
			if sz <= p.opts.MaxSingleMetadataBytes {
				b, _ := p.readAt(data, int(sz))
				p.addTag("", "Comment", cleanText(b))
			}
		}
		pos = data + sz
		if pos&1 != 0 {
			pos++
		}
	}
	p.report.General.Duration = s.Duration
	p.report.Streams = append(p.report.Streams, s)
	return nil
}
func extended80(b []byte) float64 {
	if len(b) < 10 {
		return 0
	}
	exp := int(binary.BigEndian.Uint16(b[:2])&0x7fff) - 16383
	mant := binary.BigEndian.Uint64(b[2:])
	if mant == 0 {
		return 0
	}
	return math.Ldexp(float64(mant), exp-63)
}
func aiffCodec(id string) string {
	switch strings.TrimSpace(id) {
	case "NONE", "twos", "sowt":
		return "PCM"
	case "fl32", "FL32", "fl64", "FL64":
		return "IEEE Float"
	case "ulaw":
		return "Mu-law"
	case "alaw":
		return "A-law"
	case "ima4":
		return "IMA ADPCM"
	}
	return id
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
