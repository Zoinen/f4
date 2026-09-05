package mediainfo

import (
	"encoding/binary"
	"errors"
	"strconv"
	"strings"
	"time"
)

// parseEBUSTL reads the fixed-size GSI/TTI records defined by EBU Tech 3264.
// It deliberately does not decode the subtitle payload: timing and catalogue
// metadata are sufficient for an information view and keep the probe bounded.
func parseEBUSTL(p *probe, _ []byte) error {
	if p.src.Size < 1024 {
		return ErrUnsupported
	}
	gsi, err := p.readAt(0, 1024)
	if err != nil {
		return err
	}
	dfc := cleanSTLField(gsi[3:11])
	if !strings.HasPrefix(dfc, "STL") {
		return ErrUnsupported
	}
	fps := 25
	if strings.Contains(dfc, "30") {
		fps = 30
	} else if strings.Contains(dfc, "24") {
		fps = 24
	}
	encoding := stlCharacterTable(cleanSTLField(gsi[12:14]))
	text := &Text{FormatVersion: dfc, Encoding: encoding}
	stream := Stream{Index: 0, ID: "1", Kind: StreamText, Format: "EBU STL", Text: text}
	p.report.General.Format = "EBU STL"
	p.report.General.FormatProfile = dfc
	p.report.General.MIME = "application/x-ebu-stl"
	p.addTag("", "Original programme title", cleanSTLField(gsi[16:48]))
	p.addTag("", "Original episode title", cleanSTLField(gsi[48:80]))
	p.addTag("", "Language", cleanSTLField(gsi[14:16]))
	p.addTag("", "Publisher", cleanSTLField(gsi[277:309]))

	blocks := int((p.src.Size - 1024) / 128)
	if declared := parseSTLDecimal(gsi[238:243]); declared > 0 && declared < blocks {
		blocks = declared
	}
	seen := make(map[uint32]struct{})
	for i := 0; i < blocks; i++ {
		if err := p.step(); err != nil {
			p.report.Streams = append(p.report.Streams, stream)
			return err
		}
		off := int64(1024 + i*128)
		tti, err := p.readAt(off, 128)
		if err != nil {
			return &ParseError{Format: "EBU STL", Offset: off, Err: err}
		}
		if tti[15] != 0 { // CF: comment rather than subtitle data.
			continue
		}
		key := uint32(tti[0])<<16 | uint32(binary.BigEndian.Uint16(tti[1:3]))
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			text.CueCount++
		}
		start, okStart := stlTimecode(tti[5:9], fps)
		end, okEnd := stlTimecode(tti[9:13], fps)
		if okStart && (text.CueCount == 1 || start < text.FirstCue) {
			text.FirstCue = start
		}
		if okEnd && end > text.LastCue {
			text.LastCue = end
		}
	}
	if text.CueCount == 0 {
		return &ParseError{Format: "EBU STL", Offset: 1024, Err: errors.New("no subtitle records")}
	}
	stream.Duration = text.LastCue
	p.report.General.Duration = text.LastCue
	p.report.Streams = append(p.report.Streams, stream)
	return nil
}

func cleanSTLField(b []byte) string {
	end := len(b)
	for end > 0 && (b[end-1] == 0 || b[end-1] == 0x8f) {
		end--
	}
	return strings.TrimSpace(string(b[:end]))
}

func parseSTLDecimal(b []byte) int {
	v, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return v
}

func stlCharacterTable(code string) string {
	switch code {
	case "00":
		return "ISO 6937-2 Latin"
	case "01":
		return "ISO 8859-5 Cyrillic"
	case "02":
		return "ISO 8859-6 Arabic"
	case "03":
		return "ISO 8859-7 Greek"
	case "04":
		return "ISO 8859-8 Hebrew"
	default:
		return code
	}
}

func stlTimecode(b []byte, fps int) (time.Duration, bool) {
	if len(b) < 4 || fps <= 0 || int(b[1]) >= 60 || int(b[2]) >= 60 || int(b[3]) >= fps {
		return 0, false
	}
	return time.Duration(b[0])*time.Hour + time.Duration(b[1])*time.Minute +
		time.Duration(b[2])*time.Second + time.Duration(b[3])*time.Second/time.Duration(fps), true
}
