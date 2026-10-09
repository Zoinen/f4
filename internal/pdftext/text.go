package pdftext

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Output caps.
const (
	maxPageText  = 1 << 20
	maxTotalText = 16 << 20
	maxFormDepth = 4
)

// font decodes the character codes of one PDF font into text.
type font struct {
	toUnicode map[uint32]string
	codeLen   int // bytes per code when toUnicode is used
	simple    [256]rune
	twoByte   bool
	lost      *int // counts characters that could not be decoded
}

const cp1252High = "€\u0081‚ƒ„…†‡ˆ‰Š‹Œ\u008dŽ\u008f\u0090‘’“”•–—˜™š›œ\u009džŸ"

var glyphNames = map[string]rune{
	"space": ' ', "exclam": '!', "quotedbl": '"', "numbersign": '#', "dollar": '$', "percent": '%',
	"ampersand": '&', "quotesingle": '\'', "parenleft": '(', "parenright": ')', "asterisk": '*',
	"plus": '+', "comma": ',', "hyphen": '-', "minus": '−', "period": '.', "slash": '/',
	"zero": '0', "one": '1', "two": '2', "three": '3', "four": '4', "five": '5', "six": '6',
	"seven": '7', "eight": '8', "nine": '9', "colon": ':', "semicolon": ';', "less": '<',
	"equal": '=', "greater": '>', "question": '?', "at": '@', "bracketleft": '[',
	"backslash": '\\', "bracketright": ']', "underscore": '_', "grave": '`', "braceleft": '{',
	"bar": '|', "braceright": '}', "asciitilde": '~', "bullet": '•', "endash": '–',
	"emdash": '—', "quoteleft": '‘', "quoteright": '’', "quotedblleft": '“',
	"quotedblright": '”', "ellipsis": '…', "fi": 'ﬁ', "fl": 'ﬂ',
}

func glyphRune(name Name) (rune, bool) {
	s := string(name)
	if r, ok := glyphNames[s]; ok {
		return r, true
	}
	if len(s) == 1 {
		return rune(s[0]), true
	}
	if strings.HasPrefix(s, "uni") && len(s) == 7 {
		if v, err := strconv.ParseUint(s[3:], 16, 32); err == nil {
			return rune(v), true //nolint:gosec // bounded by the syntax being parsed
		}
	}
	return 0, false
}

func (d *doc) loadFont(dict Dict, lost *int) *font {
	f := &font{lost: lost}
	for i := 0; i < 256; i++ {
		f.simple[i] = rune(i)
	}
	if hi := []rune(cp1252High); len(hi) == 32 {
		for i, r := range hi {
			f.simple[0x80+i] = r
		}
	}
	f.twoByte = d.name(dict["Subtype"]) == "Type0"
	f.codeLen = 1
	if f.twoByte {
		f.codeLen = 2
	}
	if enc, ok := d.resolve(dict["Encoding"]).(Dict); ok {
		code := 0
		for _, e := range d.array(enc["Differences"]) {
			switch t := d.resolve(e).(type) {
			case float64:
				code = int(t)
			case Name:
				if r, ok := glyphRune(t); ok && code >= 0 && code < 256 {
					f.simple[code] = r
				}
				code++
			}
		}
	}
	if s, ok := d.resolve(dict["ToUnicode"]).(Stream); ok {
		if data, err := d.decode(s); err == nil {
			m, n := parseCMap(data)
			if len(m) > 0 {
				f.toUnicode = m
				if n > 0 {
					f.codeLen = n
				}
			}
		}
	}
	return f
}

// decode turns the bytes of a shown string into text.
func (f *font) decode(s []byte) string {
	var b strings.Builder
	if f.toUnicode != nil {
		n := f.codeLen
		for i := 0; i < len(s); {
			size := n
			if i+size > len(s) {
				size = len(s) - i
			}
			var code uint32
			for _, c := range s[i : i+size] {
				code = code<<8 | uint32(c)
			}
			i += size
			if t, ok := f.toUnicode[code]; ok {
				b.WriteString(t)
			} else if !f.twoByte && size == 1 {
				b.WriteRune(f.simple[byte(code)])
			} else {
				b.WriteRune(utf8.RuneError)
				*f.lost++
			}
		}
		return b.String()
	}
	if f.twoByte {
		// Identity-H without a ToUnicode map: the glyph ids say nothing about
		// the characters.
		*f.lost += len(s) / 2
		return strings.Repeat(string(utf8.RuneError), len(s)/2)
	}
	for _, c := range s {
		b.WriteRune(f.simple[c])
	}
	return b.String()
}

var (
	cmapCodespace = regexp.MustCompile(`(?s)begincodespacerange\s*<([0-9A-Fa-f]+)>`)
	cmapBlock     = regexp.MustCompile(`(?s)begin(bfchar|bfrange)(.*?)end(?:bfchar|bfrange)`)
)

// parseCMap reads the bfchar and bfrange sections of a ToUnicode CMap and
// reports the code length in bytes its codespace declares (0 if none).
func parseCMap(data []byte) (map[uint32]string, int) {
	out := make(map[uint32]string)
	codeLen := 0
	if m := cmapCodespace.FindSubmatch(data); m != nil {
		codeLen = len(m[1]) / 2
	}
	for _, block := range cmapBlock.FindAllSubmatch(data, -1) {
		ps := &parser{b: block[2]}
		var items []any
		for len(items) < 1<<20 {
			v, err := ps.value()
			if err != nil {
				break
			}
			items = append(items, v)
		}
		if string(block[1]) == "bfchar" {
			for i := 0; i+1 < len(items); i += 2 {
				src, ok1 := items[i].(Str)
				dst, ok2 := items[i+1].(Str)
				if ok1 && ok2 {
					out[codeOf(src)] = utf16String(dst)
				}
			}
			continue
		}
		for i := 0; i+2 < len(items); i += 3 {
			lo, ok1 := items[i].(Str)
			hi, ok2 := items[i+1].(Str)
			if !ok1 || !ok2 {
				continue
			}
			from, to := codeOf(lo), codeOf(hi)
			if to < from || to-from > 0xFFFF || to == ^uint32(0) {
				continue
			}
			switch dst := items[i+2].(type) {
			case Str:
				base := []rune(utf16String(dst))
				if len(base) == 0 {
					continue
				}
				for c := from; c <= to; c++ {
					r := append([]rune(nil), base...)
					r[len(r)-1] += rune(c - from) //nolint:gosec // bounded by the syntax being parsed
					out[c] = string(r)
				}
			case Array:
				for j, e := range dst {
					if s, ok := e.(Str); ok && from+uint32(j) <= to {
						out[from+uint32(j)] = utf16String(s)
					}
				}
			}
		}
	}
	return out, codeLen
}

func codeOf(s Str) uint32 {
	var v uint32
	for _, c := range s {
		v = v<<8 | uint32(c)
	}
	return v
}

// utf16String decodes big-endian UTF-16 bytes.
func utf16String(s Str) string {
	var units []uint16
	for i := 0; i+1 < len(s); i += 2 {
		units = append(units, uint16(s[i])<<8|uint16(s[i+1]))
	}
	if len(s) == 1 {
		units = append(units, uint16(s[0]))
	}
	var b strings.Builder
	for i := 0; i < len(units); i++ {
		u := units[i]
		if u >= 0xD800 && u < 0xDC00 && i+1 < len(units) && units[i+1] >= 0xDC00 && units[i+1] < 0xE000 {
			b.WriteRune(0x10000 + (rune(u)-0xD800)<<10 + rune(units[i+1]) - 0xDC00)
			i++
			continue
		}
		b.WriteRune(rune(u))
	}
	return b.String()
}

// textState is what the content interpreter carries between operators.
type textState struct {
	out   strings.Builder
	font  *font
	fonts map[int]*font
	lost  *int
	lastY float64
	haveY bool
}

func (t *textState) space() {
	s := t.out.String()
	if s != "" && !strings.HasSuffix(s, " ") && !strings.HasSuffix(s, "\n") {
		t.out.WriteByte(' ')
	}
}

func (t *textState) newline() {
	s := t.out.String()
	if s != "" && !strings.HasSuffix(s, "\n") {
		t.out.WriteByte('\n')
	}
}

func (t *textState) show(s []byte) {
	if t.font == nil || t.out.Len() > maxPageText {
		return
	}
	t.out.WriteString(t.font.decode(s))
}

var inlineImageEnd = regexp.MustCompile(`\sEI(\s|$)`)

// run interprets one content stream against the resources it draws with.
func (d *doc) run(data []byte, res Dict, t *textState, depth int) {
	ps := &parser{b: data}
	var args []any
	for {
		v, err := ps.value()
		if err != nil {
			if err == errSyntax {
				continue
			}
			return
		}
		kw, isOp := v.(Keyword)
		if !isOp {
			if len(args) < 64 {
				args = append(args, v)
			}
			continue
		}
		op := string(kw)
		num := func(i int) float64 {
			if i < len(args) {
				if f, ok := args[i].(float64); ok {
					return f
				}
			}
			return 0
		}
		switch op {
		case "BI":
			if loc := inlineImageEnd.FindIndex(data[ps.p:]); loc != nil {
				ps.p += loc[1]
			} else {
				return
			}
		case "Tf":
			if len(args) >= 1 {
				if name, ok := args[0].(Name); ok {
					t.font = d.fontFor(res, name, t)
				}
			}
		case "Td", "TD":
			if num(1) != 0 {
				t.newline()
			} else if num(0) != 0 {
				t.space()
			}
		case "Tm":
			y := num(5)
			if t.haveY && (y-t.lastY > 0.5 || t.lastY-y > 0.5) {
				t.newline()
			} else if t.haveY {
				t.space()
			}
			t.lastY, t.haveY = y, true
		case "T*":
			t.newline()
		case "BT":
			t.haveY = false
		case "ET":
			t.newline()
		case "Tj":
			if len(args) >= 1 {
				if s, ok := args[0].(Str); ok {
					t.show(s)
				}
			}
		case "'", "\"":
			t.newline()
			if len(args) >= 1 {
				if s, ok := args[len(args)-1].(Str); ok {
					t.show(s)
				}
			}
		case "TJ":
			if len(args) >= 1 {
				if arr, ok := args[0].(Array); ok {
					for _, e := range arr {
						switch x := e.(type) {
						case Str:
							t.show(x)
						case float64:
							if x < -180 {
								t.space()
							}
						}
					}
				}
			}
		case "Do":
			if depth < maxFormDepth && len(args) >= 1 {
				d.doForm(res, args[0], t, depth)
			}
		}
		args = args[:0]
		if t.out.Len() > maxPageText {
			return
		}
	}
}

func (d *doc) fontFor(res Dict, name Name, t *textState) *font {
	fonts := d.dict(res["Font"])
	entry, ok := fonts[name]
	if !ok {
		return nil
	}
	if ref, ok := entry.(Ref); ok {
		if f, ok := t.fonts[ref.Num]; ok {
			return f
		}
		f := d.loadFont(d.dict(entry), t.lost)
		t.fonts[ref.Num] = f
		return f
	}
	return d.loadFont(d.dict(entry), t.lost)
}

func (d *doc) doForm(res Dict, arg any, t *textState, depth int) {
	name, ok := arg.(Name)
	if !ok {
		return
	}
	xo := d.dict(res["XObject"])
	s, ok := d.resolve(xo[name]).(Stream)
	if !ok || d.name(s.Dict["Subtype"]) != "Form" {
		return
	}
	data, err := d.decode(s)
	if err != nil {
		return
	}
	inner := d.dict(s.Dict["Resources"])
	if inner == nil {
		inner = res
	}
	t.newline()
	d.run(data, inner, t, depth+1)
	t.newline()
}

// pageText extracts the text of one page.
func (d *doc) pageText(page Dict, lost *int) string {
	res := d.dict(page["Resources"])
	var streams []Stream
	switch c := d.resolve(page["Contents"]).(type) {
	case Stream:
		streams = append(streams, c)
	case Array:
		for _, e := range c {
			if s, ok := d.resolve(e).(Stream); ok {
				streams = append(streams, s)
			}
		}
	}
	var joined bytes.Buffer
	for _, s := range streams {
		data, err := d.decode(s)
		if err != nil {
			continue
		}
		joined.Write(data)
		joined.WriteByte('\n')
		if joined.Len() > maxStreamSize {
			break
		}
	}
	t := &textState{fonts: make(map[int]*font), lost: lost}
	d.run(joined.Bytes(), res, t, 0)
	return tidy(t.out.String())
}

// tidy trims each line and collapses runs of blank lines.
func tidy(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			blank++
			if blank > 1 || len(out) == 0 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
