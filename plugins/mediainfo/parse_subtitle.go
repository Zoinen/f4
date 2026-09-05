package mediainfo

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var subtitleExts = map[string]bool{".srt": true, ".vtt": true, ".ass": true, ".ssa": true, ".ttml": true, ".dfxp": true, ".sub": true, ".stl": true}

func isSubtitleExtension(ext string) bool { return subtitleExts[strings.ToLower(ext)] }
func looksLikeText(b []byte) bool {
	if len(b) >= 2 && ((b[0] == 0xff && b[1] == 0xfe) || (b[0] == 0xfe && b[1] == 0xff)) {
		return true
	}
	if !utf8.Valid(b) {
		return false
	}
	zeros := bytes.Count(b, []byte{0})
	return zeros == 0 || zeros*100 < len(b)
}

func parseSubtitle(p *probe, _ []byte) error {
	n := p.src.Size
	if n > p.opts.MaxTextBytes {
		n = p.opts.MaxTextBytes
		p.report.Truncated = true
	}
	b, e := p.readBounded(0, n)
	if e != nil && !errors.Is(e, io.EOF) {
		return e
	}
	text, enc := decodeSubtitleBytes(b)
	ext := strings.ToLower(filepath.Ext(p.src.Name))
	t := &Text{Encoding: enc}
	s := Stream{Index: 0, ID: "1", Kind: StreamText, Text: t}
	switch ext {
	case ".srt":
		s.Format = "SubRip"
		if e := parseSRT(text, t, p); e != nil {
			return &ParseError{Format: s.Format, Offset: -1, Err: e}
		}
	case ".vtt":
		s.Format = "WebVTT"
		if e := parseVTT(text, t, p); e != nil {
			if errors.Is(e, ErrUnsupported) {
				return ErrUnsupported
			}
			return &ParseError{Format: s.Format, Offset: -1, Err: e}
		}
	case ".ass", ".ssa":
		s.Format = strings.ToUpper(strings.TrimPrefix(ext, "."))
		if e := parseASS(text, t, p); e != nil {
			return &ParseError{Format: s.Format, Offset: -1, Err: e}
		}
	case ".ttml", ".dfxp":
		s.Format = "TTML"
		if e := parseTTML(text, t, p); e != nil {
			return &ParseError{Format: "TTML", Offset: 0, Err: e}
		}
	case ".sub":
		s.Format = "MicroDVD"
		if e := parseMicroDVD(text, t, p); e != nil {
			return &ParseError{Format: s.Format, Offset: -1, Err: e}
		}
	default:
		return ErrUnsupported
	}
	if t.CueCount == 0 && s.Format != "TTML" {
		return ErrUnsupported
	}
	if t.LastCue > 0 {
		s.Duration = t.LastCue
		p.report.General.Duration = t.LastCue
	}
	p.report.General.Format = s.Format
	p.report.General.MIME = "text/plain"
	p.report.Streams = append(p.report.Streams, s)
	return nil
}

func decodeSubtitleBytes(b []byte) (string, string) {
	if len(b) >= 2 && b[0] == 0xff && b[1] == 0xfe {
		return decodeUTF16(b[2:], true), "UTF-16LE"
	}
	if len(b) >= 2 && b[0] == 0xfe && b[1] == 0xff {
		return decodeUTF16(b[2:], false), "UTF-16BE"
	}
	b = bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf})
	return string(b), "UTF-8"
}

var srtTimeRE = regexp.MustCompile(`(?m)(?:(\d{1,3}):)?(\d{2}):(\d{2})[,.](\d{3})\s*-->\s*(?:(\d{1,3}):)?(\d{2}):(\d{2})[,.](\d{3})`)

func subtitleTime(parts []string, off int) time.Duration {
	vals := make([]int, 4)
	for i := range vals {
		vals[i], _ = strconv.Atoi(parts[off+i])
	}
	return time.Duration(vals[0])*time.Hour + time.Duration(vals[1])*time.Minute + time.Duration(vals[2])*time.Second + time.Duration(vals[3])*time.Millisecond
}
func newSubtitleScanner(s string, p *probe) *bufio.Scanner {
	scan := bufio.NewScanner(strings.NewReader(s))
	initial := p.opts.MaxValueBytes
	if initial > 4096 {
		initial = 4096
	}
	if initial < 1 {
		initial = 1
	}
	scan.Buffer(make([]byte, initial), p.opts.MaxValueBytes)
	return scan
}

func scanSubtitleLines(s string, p *probe, visit func(string) error) error {
	scan := newSubtitleScanner(s, p)
	for scan.Scan() {
		if err := p.step(); err != nil {
			return err
		}
		if err := visit(scan.Text()); err != nil {
			return err
		}
	}
	return scan.Err()
}

func parseSRT(s string, t *Text, p *probe) error {
	return scanSubtitleLines(s, p, func(line string) error {
		parseSubtitleTimingLine(line, t)
		return nil
	})
}
func parseVTT(s string, t *Text, p *probe) error {
	t.FormatVersion = "WebVTT"
	first := true
	err := scanSubtitleLines(s, p, func(line string) error {
		if first {
			first = false
			if !strings.HasPrefix(strings.TrimSpace(line), "WEBVTT") {
				return ErrUnsupported
			}
			return nil
		}
		parseSubtitleTimingLine(line, t)
		return nil
	})
	if err == nil && first {
		return ErrUnsupported
	}
	return err
}

func parseSubtitleTimingLine(line string, t *Text) {
	x := srtTimeRE.FindStringSubmatch(line)
	if len(x) == 0 {
		return
	}
	t.CueCount++
	start, end := subtitleTime(x, 1), subtitleTime(x, 5)
	if t.CueCount == 1 || start < t.FirstCue {
		t.FirstCue = start
	}
	if end > t.LastCue {
		t.LastCue = end
	}
}

var assTimeRE = regexp.MustCompile(`(?i)^Dialogue\s*:[^,]*,([0-9]+):([0-9]{2}):([0-9]{2})[.]([0-9]{2}),([0-9]+):([0-9]{2}):([0-9]{2})[.]([0-9]{2}),`)

func parseASS(s string, t *Text, p *probe) error {
	return scanSubtitleLines(s, p, func(rawLine string) error {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(strings.ToLower(line), "style:") {
			t.StyleCount++
		}
		m := assTimeRE.FindStringSubmatch(line)
		if len(m) == 9 {
			t.CueCount++
			vals := make([]int, 8)
			for i := range vals {
				vals[i], _ = strconv.Atoi(m[i+1])
			}
			start := time.Duration(vals[0])*time.Hour + time.Duration(vals[1])*time.Minute + time.Duration(vals[2])*time.Second + time.Duration(vals[3])*10*time.Millisecond
			end := time.Duration(vals[4])*time.Hour + time.Duration(vals[5])*time.Minute + time.Duration(vals[6])*time.Second + time.Duration(vals[7])*10*time.Millisecond
			if t.CueCount == 1 || start < t.FirstCue {
				t.FirstCue = start
			}
			if end > t.LastCue {
				t.LastCue = end
			}
		}
		return nil
	})
}

func parseTTML(s string, t *Text, p *probe) error {
	// encoding/xml materializes every attribute of a start element before it
	// returns the token.  Preflight markup boundaries and attribute counts so a
	// single hostile tag cannot turn the bounded subtitle input into an
	// unbounded []xml.Attr allocation.
	if err := validateTTMLMarkup(s, p); err != nil {
		return err
	}
	dec := xml.NewDecoder(strings.NewReader(s))
	depth := 0
	for {
		if e := p.step(); e != nil {
			return e
		}
		tok, e := dec.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		switch x := tok.(type) {
		case xml.StartElement:
			depth++
			if depth > 64 {
				return errors.New("XML nesting too deep")
			}
			if x.Name.Local == "tt" {
				for _, a := range x.Attr {
					if a.Name.Local == "lang" {
						p.addTag("", "Language", a.Value)
					}
				}
			}
			if x.Name.Local == "p" {
				var begin, end, dur time.Duration
				for _, a := range x.Attr {
					switch a.Name.Local {
					case "begin":
						begin = parseClock(a.Value)
					case "end":
						end = parseClock(a.Value)
					case "dur":
						dur = parseClock(a.Value)
					}
				}
				if end == 0 {
					end = begin + dur
				}
				t.CueCount++
				if t.CueCount == 1 || begin < t.FirstCue {
					t.FirstCue = begin
				}
				if end > t.LastCue {
					t.LastCue = end
				}
			}
		case xml.EndElement:
			depth--
		}
	}
	if t.CueCount == 0 {
		return errors.New("no TTML cues")
	}
	return nil
}

const (
	maxTTMLMarkupBytes = 64 << 10
	maxTTMLAttributes  = 256
)

// validateTTMLMarkup bounds the pieces that encoding/xml buffers as one
// token. Attribute work is charged to the same element budget as decoded XML
// tokens. This is intentionally a lexical preflight; encoding/xml remains the
// authority for XML well-formedness.
func validateTTMLMarkup(s string, p *probe) error {
	for start := 0; start < len(s); {
		if start&0xfff == 0 {
			if err := p.ctx.Err(); err != nil {
				return err
			}
		}
		markupStart, err := nextTTMLMarkup(s, start, p)
		if err != nil {
			return err
		}
		if markupStart < 0 {
			return p.ctx.Err()
		}
		start = markupStart

		end, attributes, err := scanTTMLMarkup(s, start, p)
		if err != nil {
			if errors.Is(err, ErrLimit) {
				p.report.Truncated = true
			}
			return err
		}
		if attributes > maxTTMLAttributes {
			p.report.Truncated = true
			return ErrLimit
		}
		for range attributes {
			if err := p.step(); err != nil {
				if errors.Is(err, ErrLimit) {
					p.report.Truncated = true
				}
				return err
			}
		}
		start = end
	}
	return p.ctx.Err()
}

func nextTTMLMarkup(s string, start int, p *probe) (int, error) {
	for start < len(s) {
		if err := p.ctx.Err(); err != nil {
			return -1, err
		}
		end := start + 4096
		if end > len(s) {
			end = len(s)
		}
		if relative := strings.IndexByte(s[start:end], '<'); relative >= 0 {
			return start + relative, nil
		}
		start = end
	}
	return -1, nil
}

func scanTTMLMarkup(s string, start int, p *probe) (end, attributes int, err error) {
	if start < 0 || start >= len(s) || s[start] != '<' {
		return 0, 0, io.ErrUnexpectedEOF
	}

	// Comments, CDATA and processing instructions have terminators that may
	// contain a plain '>'. Bound the entire token rather than stopping early.
	type delimiter struct {
		prefix string
		close  string
	}
	for _, candidate := range []delimiter{
		{prefix: "<!--", close: "-->"},
		{prefix: "<![CDATA[", close: "]]>"},
		{prefix: "<?", close: "?>"},
	} {
		if !strings.HasPrefix(s[start:], candidate.prefix) {
			continue
		}
		searchEnd := start + maxTTMLMarkupBytes + len(candidate.close)
		if searchEnd > len(s) {
			searchEnd = len(s)
		}
		closeAt := strings.Index(s[start+len(candidate.prefix):searchEnd], candidate.close)
		if closeAt < 0 {
			if searchEnd < len(s) {
				return 0, 0, ErrLimit
			}
			return 0, 0, io.ErrUnexpectedEOF
		}
		end = start + len(candidate.prefix) + closeAt + len(candidate.close)
		if end-start > maxTTMLMarkupBytes {
			return 0, 0, ErrLimit
		}
		return end, 0, nil
	}

	isStartElement := start+1 < len(s) && s[start+1] != '/' && s[start+1] != '!'
	quote := byte(0)
	bracketDepth := 0
	for index := start + 1; index < len(s); index++ {
		if index-start > maxTTMLMarkupBytes {
			return 0, 0, ErrLimit
		}
		if index&0xfff == 0 {
			if err := p.ctx.Err(); err != nil {
				return 0, attributes, err
			}
		}
		ch := s[index]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '=':
			if isStartElement {
				attributes++
				if attributes > maxTTMLAttributes {
					return 0, attributes, ErrLimit
				}
			}
		case '>':
			if bracketDepth == 0 {
				return index + 1, attributes, nil
			}
		}
	}
	return 0, attributes, io.ErrUnexpectedEOF
}

func parseClock(v string) time.Duration {
	v = strings.TrimSpace(v)
	if strings.HasSuffix(v, "ms") {
		f, _ := strconv.ParseFloat(strings.TrimSuffix(v, "ms"), 64)
		return time.Duration(f * float64(time.Millisecond))
	}
	if strings.HasSuffix(v, "s") {
		f, _ := strconv.ParseFloat(strings.TrimSuffix(v, "s"), 64)
		return time.Duration(f * float64(time.Second))
	}
	parts := strings.Split(v, ":")
	if len(parts) == 3 {
		h, _ := strconv.ParseFloat(parts[0], 64)
		m, _ := strconv.ParseFloat(parts[1], 64)
		sec, _ := strconv.ParseFloat(parts[2], 64)
		return time.Duration((h*3600 + m*60 + sec) * float64(time.Second))
	}
	return 0
}

var microDVDRE = regexp.MustCompile(`(?m)^\{(\d+)\}\{(\d+)\}([^\r\n]*)`)

func parseMicroDVD(s string, t *Text, p *probe) error {
	fps := 25.0
	matched := 0
	return scanSubtitleLines(s, p, func(line string) error {
		x := microDVDRE.FindStringSubmatch(line)
		if len(x) == 0 {
			return nil
		}
		a, _ := strconv.Atoi(x[1])
		b, _ := strconv.Atoi(x[2])
		if matched == 0 && ((a == 1 && b == 1) || (a == 0 && b == 0)) {
			if declared := parseDecimal(strings.Replace(strings.TrimSpace(x[3]), ",", ".", 1)); declared > 0 {
				fps = declared
			}
			matched++
			return nil
		}
		matched++
		start := time.Duration(float64(a) / fps * float64(time.Second))
		end := time.Duration(float64(b) / fps * float64(time.Second))
		t.CueCount++
		if t.CueCount == 1 || start < t.FirstCue {
			t.FirstCue = start
		}
		if end > t.LastCue {
			t.LastCue = end
		}
		return nil
	})
}
