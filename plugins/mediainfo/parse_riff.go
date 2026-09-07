package mediainfo

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
)

type riffChunk struct {
	id, listType string
	offset, size int64
	path         []string
}

func parseRIFF(p *probe, _ []byte) error {
	h, err := p.readAt(0, 12)
	if err != nil {
		return &ParseError{Format: "RIFF", Offset: 0, Err: err}
	}
	var order binary.ByteOrder = binary.LittleEndian
	if string(h[:4]) == "RIFX" {
		order = binary.BigEndian
	}
	switch string(h[8:12]) {
	case "WAVE":
		return parseWave(p, order, string(h[:4]))
	case "AVI ":
		return parseAVI(p, order, string(h[:4]))
	default:
		return ErrUnsupported
	}
}

func walkRIFF(p *probe, order binary.ByteOrder, start, end int64, path []string, fn func(riffChunk) error) error {
	if end > p.src.Size {
		end = p.src.Size
	}
	for pos := start; pos+8 <= end; {
		if err := p.step(); err != nil {
			return err
		}
		h, err := p.readAt(pos, 8)
		if err != nil {
			return err
		}
		id := string(h[:4])
		sz := int64(u32(h[4:8], order))
		data := pos + 8
		// RF64/BW64 use 0xffffffff as a placeholder for the data chunk;
		// its exact 64-bit length lives in the earlier ds64 chunk. Treat the
		// payload as extending to this RIFF segment's bound so it can be skipped
		// without trying to read audio data.
		if id == "data" && sz == int64(^uint32(0)) {
			sz = end - data
		}
		if sz < 0 || data > end || sz > end-data {
			return &ParseError{Format: "RIFF", Offset: pos, Err: fmt.Errorf("chunk %q exceeds file", id)}
		}
		c := riffChunk{id: id, offset: data, size: sz, path: append([]string(nil), path...)}
		if (id == "LIST" || id == "RIFF") && sz >= 4 {
			t, err := p.readAt(data, 4)
			if err != nil {
				return err
			}
			c.listType = string(t)
			if err := fn(c); err != nil {
				return err
			}
			if c.listType != "movi" && len(path) < 6 {
				if err := walkRIFF(p, order, data+4, data+sz, append(path, c.listType), fn); err != nil {
					return err
				}
			}
		} else if err := fn(c); err != nil {
			return err
		}
		next := data + sz
		if next&1 != 0 {
			next++
		}
		if next <= pos {
			return &ParseError{Format: "RIFF", Offset: pos, Err: errors.New("non-advancing chunk")}
		}
		pos = next
	}
	return nil
}

func parseWave(p *probe, order binary.ByteOrder, riffID string) error {
	p.report.General.Format = "Wave"
	p.report.General.CodecID = riffID
	p.report.General.MIME = "audio/wav"
	stream := Stream{Index: 0, ID: "1", Kind: StreamAudio, Format: "PCM", Audio: &Audio{}}
	var dataSize uint64
	var sampleCount uint64
	err := walkRIFF(p, order, 12, p.src.Size, nil, func(c riffChunk) error {
		switch c.id {
		case "fmt ":
			if c.size < 16 {
				return &ParseError{Format: "Wave", Offset: c.offset, Err: errors.New("short fmt chunk")}
			}
			n := c.size
			if n > 80 {
				n = 80
			}
			b, err := p.readAt(c.offset, int(n))
			if err != nil {
				return err
			}
			tag := u16(b[0:2], order)
			stream.CodecID = fmt.Sprintf("0x%04X", tag)
			stream.Format = waveCodec(tag, b)
			stream.Audio.Channels = int(u16(b[2:4], order))
			stream.Audio.SampleRate = int(u32(b[4:8], order))
			stream.BitRate = int64(u32(b[8:12], order)) * 8
			stream.Audio.BitDepth = int(u16(b[14:16], order))
			if tag == 1 || tag == 3 {
				stream.Audio.CompressionMode = "Lossless"
			}
		case "fact":
			if c.size >= 4 {
				b, e := p.readAt(c.offset, 4)
				if e == nil {
					sampleCount = uint64(u32(b, order))
				}
			}
		case "data":
			if dataSize == 0 || (riffID != "RF64" && riffID != "BW64") {
				// #nosec G115 -- walkRIFF rejects negative chunk sizes before invoking this callback.
				dataSize = uint64(c.size)
			}
		case "ds64":
			if c.size >= 24 {
				b, e := p.readAt(c.offset, 24)
				if e == nil {
					dataSize = u64(b[8:16], binary.LittleEndian)
					sampleCount = u64(b[16:24], binary.LittleEndian)
				}
			}
		case "bext":
			n := c.size
			if n > 602 {
				n = 602
			}
			b, e := p.readAt(c.offset, int(n))
			if e == nil && len(b) >= 256 {
				p.addTag("", "Description", cleanText(b[:256]))
			}
		default:
			if len(c.path) > 0 && c.path[len(c.path)-1] == "INFO" && c.size <= p.opts.MaxSingleMetadataBytes {
				b, e := p.readAt(c.offset, int(c.size))
				if e == nil {
					p.addTag("", canonicalTag(c.id), cleanText(b))
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if stream.Audio.SampleRate > 0 && sampleCount > 0 {
		stream.Duration = durationFromUnits(sampleCount, uint64(stream.Audio.SampleRate))
	}
	if stream.Duration == 0 && stream.BitRate > 0 && dataSize > 0 {
		stream.Duration = time.Duration(float64(dataSize*8) / float64(stream.BitRate) * float64(time.Second))
	}
	p.report.General.Duration = stream.Duration
	p.report.Streams = append(p.report.Streams, stream)
	return nil
}

func waveCodec(tag uint16, b []byte) string {
	switch tag {
	case 0x0001:
		return "PCM"
	case 0x0003:
		return "IEEE Float"
	case 0x0006:
		return "A-law"
	case 0x0007:
		return "Mu-law"
	case 0x0011:
		return "IMA ADPCM"
	case 0x0050:
		return "MPEG Audio"
	case 0x0055:
		return "MPEG Audio Layer 3"
	case 0x00ff:
		return "AAC"
	case 0x0161, 0x0162:
		return "Windows Media Audio"
	case 0x2000:
		return "AC-3"
	case 0xfffe:
		if len(b) >= 40 {
			return "WaveFormatExtensible"
		}
		return "Extensible"
	default:
		return "Unknown"
	}
}

func parseAVI(p *probe, order binary.ByteOrder, riffID string) error {
	p.report.General.Format = "AVI"
	p.report.General.CodecID = riffID
	p.report.General.MIME = "video/x-msvideo"
	var current *Stream
	var microseconds, totalFrames uint32
	err := walkRIFF(p, order, 12, p.src.Size, nil, func(c riffChunk) error {
		switch c.id {
		case "avih":
			if c.size < 40 {
				return nil
			}
			b, err := p.readAt(c.offset, 40)
			if err != nil {
				return err
			}
			microseconds, totalFrames = u32(b[0:4], order), u32(b[16:20], order)
			if microseconds > 0 {
				p.report.General.Duration = durationFromUnits(uint64(microseconds)*uint64(totalFrames), uint64(time.Second/time.Microsecond))
				p.report.General.FrameRate = 1e6 / float64(microseconds)
				p.report.General.FrameCount = int64(totalFrames)
			}
		case "dmlh":
			if c.size >= 4 {
				b, err := p.readAt(c.offset, 4)
				if err != nil {
					return err
				}
				if frames := u32(b, order); frames > 0 {
					totalFrames = frames
				}
			}
		case "strh":
			// A following strf always belongs to this strh. Clear the previous
			// association even if the new stream is rejected or malformed.
			current = nil
			if len(p.report.Streams) >= p.opts.MaxStreams {
				p.report.Truncated = true
				return nil
			}
			if c.size < 48 {
				return nil
			}
			b, err := p.readAt(c.offset, 48)
			if err != nil {
				return err
			}
			typ, handler := string(b[:4]), fourCC(b[4:8])
			s := Stream{Index: len(p.report.Streams), ID: fmt.Sprint(len(p.report.Streams) + 1), CodecID: handler}
			scale, rate, length := u32(b[20:24], order), u32(b[24:28], order), u32(b[32:36], order)
			if scale > 0 && rate > 0 {
				s.Duration = durationFromUnits(uint64(length)*uint64(scale), uint64(rate))
				s.FrameCount = int64(length)
				s.FrameRate = float64(rate) / float64(scale)
			}
			switch typ {
			case "vids":
				s.Kind, s.Format, s.Video = StreamVideo, videoCodec(handler), &Video{}
			case "auds":
				s.Kind, s.Format, s.Audio = StreamAudio, "Audio", &Audio{}
			case "txts":
				s.Kind, s.Format, s.Text = StreamText, "Text", &Text{}
			default:
				s.Kind, s.Format = StreamText, strings.TrimSpace(typ)
			}
			p.report.Streams = append(p.report.Streams, s)
			current = &p.report.Streams[len(p.report.Streams)-1]
		case "strf":
			if current == nil {
				return nil
			}
			n := c.size
			if n > 128 {
				n = 128
			}
			b, err := p.readAt(c.offset, int(n))
			if err != nil {
				return err
			}
			if current.Kind == StreamVideo && len(b) >= 20 {
				current.Video.Width = int(signedInt32Bits(u32(b[4:8], order)))
				current.Video.Height = int(signedInt32Bits(u32(b[8:12], order)))
				current.Video.BitDepth = int(u16(b[14:16], order))
				current.CodecID = fourCC(b[16:20])
				current.Format = videoCodec(current.CodecID)
			} else if current.Kind == StreamAudio && len(b) >= 16 {
				tag := u16(b[:2], order)
				current.CodecID = fmt.Sprintf("0x%04X", tag)
				current.Format = waveCodec(tag, b)
				current.Audio.Channels = int(u16(b[2:4], order))
				current.Audio.SampleRate = int(u32(b[4:8], order))
				current.BitRate = int64(u32(b[8:12], order)) * 8
				current.Audio.BitDepth = int(u16(b[14:16], order))
			}
		default:
			if len(c.path) > 0 && c.path[len(c.path)-1] == "INFO" && c.size <= p.opts.MaxSingleMetadataBytes {
				b, e := p.readAt(c.offset, int(c.size))
				if e == nil {
					p.addTag("", canonicalTag(c.id), cleanText(b))
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if microseconds > 0 && totalFrames > 0 {
		p.report.General.Duration = durationFromUnits(uint64(microseconds)*uint64(totalFrames), uint64(time.Second/time.Microsecond))
		p.report.General.FrameRate = 1e6 / float64(microseconds)
		p.report.General.FrameCount = int64(totalFrames)
	}
	if p.report.General.Duration == 0 {
		for _, s := range p.report.Streams {
			if s.Duration > p.report.General.Duration {
				p.report.General.Duration = s.Duration
			}
		}
	}
	return nil
}

func videoCodec(id string) string {
	switch strings.ToUpper(strings.TrimSpace(id)) {
	case "H264", "X264", "AVC1":
		return "AVC"
	case "H265", "HEVC", "HVC1", "HEV1":
		return "HEVC"
	case "AV01":
		return "AV1"
	case "VP80":
		return "VP8"
	case "VP90":
		return "VP9"
	case "DIVX", "DX50", "XVID", "MP4V":
		return "MPEG-4 Visual"
	case "MJPG", "JPEG":
		return "Motion JPEG"
	case "THEO":
		return "Theora"
	default:
		if id == "" {
			return "Video"
		}
		return id
	}
}
