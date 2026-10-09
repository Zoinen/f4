// Package pdftext extracts the text of a PDF file page by page in pure Go
// (f4#1665): no CGO, no external program, and nothing in the file is executed.
// It is a reader for the common case, not a PDF engine: it finds the objects
// by scanning the file (so a damaged cross-reference table does not matter),
// unpacks object streams and Flate/ASCII filters, follows the page tree, and
// reads the text operators of each page's content, decoding characters through
// the fonts' ToUnicode maps or simple encodings. Encrypted files, other stream
// filters and text drawn as pictures are reported, not guessed at. The file is
// untrusted: every size, depth and count is capped.
package pdftext

import (
	"errors"
	"io"
	"strconv"
)

// Name is a PDF name object (/Type).
type Name string

// Str is a PDF string object, literal or hexadecimal.
type Str []byte

// Ref is an indirect reference (12 0 R).
type Ref struct{ Num, Gen int }

// Keyword is a bare word: an operator in a content stream, or a marker such
// as "endobj" in the file body.
type Keyword string

// Dict is a PDF dictionary.
type Dict map[Name]any

// Array is a PDF array.
type Array []any

// Stream is a dictionary with the raw, still filtered bytes that follow it.
type Stream struct {
	Dict Dict
	Raw  []byte
}

var errSyntax = errors.New("pdf syntax error")

const maxDepth = 48

type parser struct {
	b    []byte
	p    int
	refs bool // recognise "12 0 R" (file body) rather than plain numbers (content)
}

func isSpace(c byte) bool {
	return c == 0 || c == 9 || c == 10 || c == 12 || c == 13 || c == 32
}

func isDelim(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func (ps *parser) skipWS() {
	for ps.p < len(ps.b) {
		c := ps.b[ps.p]
		switch {
		case isSpace(c):
			ps.p++
		case c == '%':
			for ps.p < len(ps.b) && ps.b[ps.p] != '\n' && ps.b[ps.p] != '\r' {
				ps.p++
			}
		default:
			return
		}
	}
}

func (ps *parser) token() string {
	start := ps.p
	for ps.p < len(ps.b) && !isSpace(ps.b[ps.p]) && !isDelim(ps.b[ps.p]) {
		ps.p++
	}
	return string(ps.b[start:ps.p])
}

func isInt(s string) bool {
	if s == "" || len(s) > 10 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// value reads the next object. It returns io.EOF at the end of the data.
func (ps *parser) value() (any, error) { return ps.valueAt(0) }

func (ps *parser) valueAt(depth int) (any, error) {
	ps.skipWS()
	if ps.p >= len(ps.b) {
		return nil, io.EOF
	}
	if depth > maxDepth {
		ps.p = len(ps.b) // give up on the rest: the nesting is hostile
		return nil, errSyntax
	}
	switch c := ps.b[ps.p]; {
	case c == '/':
		ps.p++
		return ps.name(), nil
	case c == '(':
		return ps.literalString(), nil
	case c == '<' && ps.p+1 < len(ps.b) && ps.b[ps.p+1] == '<':
		ps.p += 2
		return ps.dict(depth)
	case c == '<':
		return ps.hexString(), nil
	case c == '[':
		ps.p++
		return ps.array(depth)
	case c == ']' || c == '>' || c == ')' || c == '{' || c == '}':
		ps.p++
		return Keyword(string(c)), nil
	}
	tok := ps.token()
	if tok == "" {
		ps.p++
		return nil, errSyntax
	}
	switch tok {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}
	if f, err := strconv.ParseFloat(tok, 64); err == nil && (tok[0] == '-' || tok[0] == '+' || tok[0] == '.' || (tok[0] >= '0' && tok[0] <= '9')) {
		if ps.refs && isInt(tok) {
			if ref, ok := ps.tryRef(int(f)); ok {
				return ref, nil
			}
		}
		return f, nil
	}
	return Keyword(tok), nil
}

// tryRef reads "gen R" after an integer that may be an object number.
func (ps *parser) tryRef(num int) (Ref, bool) {
	save := ps.p
	ps.skipWS()
	gen := ps.token()
	if !isInt(gen) {
		ps.p = save
		return Ref{}, false
	}
	ps.skipWS()
	if ps.p < len(ps.b) && ps.b[ps.p] == 'R' && (ps.p+1 == len(ps.b) || isSpace(ps.b[ps.p+1]) || isDelim(ps.b[ps.p+1])) {
		ps.p++
		g, _ := strconv.Atoi(gen)
		return Ref{Num: num, Gen: g}, true
	}
	ps.p = save
	return Ref{}, false
}

func unhex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

func (ps *parser) name() Name {
	var out []byte
	for ps.p < len(ps.b) && !isSpace(ps.b[ps.p]) && !isDelim(ps.b[ps.p]) {
		c := ps.b[ps.p]
		if c == '#' && ps.p+2 < len(ps.b) && unhex(ps.b[ps.p+1]) >= 0 && unhex(ps.b[ps.p+2]) >= 0 {
			out = append(out, byte(unhex(ps.b[ps.p+1])<<4|unhex(ps.b[ps.p+2]))) //nolint:gosec // bounded by the syntax being parsed
			ps.p += 3
			continue
		}
		out = append(out, c)
		ps.p++
	}
	return Name(out)
}

func (ps *parser) literalString() Str {
	ps.p++ // (
	depth := 1
	out := []byte{}
	for ps.p < len(ps.b) {
		c := ps.b[ps.p]
		ps.p++
		switch c {
		case '(':
			depth++
			out = append(out, c)
		case ')':
			depth--
			if depth == 0 {
				return Str(out)
			}
			out = append(out, c)
		case '\\':
			if ps.p >= len(ps.b) {
				return Str(out)
			}
			e := ps.b[ps.p]
			ps.p++
			switch e {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '\r':
				if ps.p < len(ps.b) && ps.b[ps.p] == '\n' {
					ps.p++
				}
			case '\n':
			default:
				if e >= '0' && e <= '7' {
					v := int(e - '0')
					for i := 0; i < 2 && ps.p < len(ps.b) && ps.b[ps.p] >= '0' && ps.b[ps.p] <= '7'; i++ {
						v = v*8 + int(ps.b[ps.p]-'0')
						ps.p++
					}
					out = append(out, byte(v)) //nolint:gosec // bounded by the syntax being parsed
				} else {
					out = append(out, e)
				}
			}
		default:
			out = append(out, c)
		}
	}
	return Str(out)
}

func (ps *parser) hexString() Str {
	ps.p++ // <
	var out []byte
	hi := -1
	for ps.p < len(ps.b) {
		c := ps.b[ps.p]
		ps.p++
		if c == '>' {
			break
		}
		v := unhex(c)
		if v < 0 {
			continue
		}
		if hi < 0 {
			hi = v
		} else {
			out = append(out, byte(hi<<4|v)) //nolint:gosec // bounded by the syntax being parsed
			hi = -1
		}
	}
	if hi >= 0 {
		out = append(out, byte(hi<<4)) //nolint:gosec // bounded by the syntax being parsed
	}
	return Str(out)
}

func (ps *parser) array(depth int) (any, error) {
	arr := Array{}
	for {
		v, err := ps.valueAt(depth + 1)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return arr, nil
			}
			continue
		}
		if k, ok := v.(Keyword); ok && k == "]" {
			return arr, nil
		}
		arr = append(arr, v)
		if len(arr) > 1<<20 {
			return nil, errSyntax
		}
	}
}

func (ps *parser) dict(depth int) (any, error) {
	d := Dict{}
	for {
		k, err := ps.valueAt(depth + 1)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return d, nil
			}
			continue
		}
		if kw, ok := k.(Keyword); ok {
			if kw == ">" {
				// The second > of ">>".
				if ps.p < len(ps.b) && ps.b[ps.p] == '>' {
					ps.p++
				}
				return d, nil
			}
			continue
		}
		key, ok := k.(Name)
		if !ok {
			continue
		}
		v, err := ps.valueAt(depth + 1)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return d, nil
			}
			continue
		}
		if kw, ok := v.(Keyword); ok && kw == ">" {
			if ps.p < len(ps.b) && ps.b[ps.p] == '>' {
				ps.p++
			}
			return d, nil
		}
		d[key] = v
		if len(d) > 1<<16 {
			return nil, errSyntax
		}
	}
}
