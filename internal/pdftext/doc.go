package pdftext

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
)

// Errors a caller can tell apart.
var (
	ErrNotPDF    = errors.New("not a PDF file")
	ErrEncrypted = errors.New("the PDF is encrypted")
)

// Limits on what is read from an untrusted file.
const (
	MaxFileSize   = 128 << 20
	maxStreamSize = 64 << 20
	maxPages      = 5000
	maxObjects    = 1 << 21
)

var objHeader = regexp.MustCompile(`(\d{1,10})\s+(\d{1,5})\s+obj\b`)

type doc struct {
	data []byte
	objs map[int]any
	root any
}

func loadDoc(data []byte) (*doc, error) {
	head := data
	if len(head) > 1024 {
		head = head[:1024]
	}
	if !bytes.Contains(head, []byte("%PDF-")) {
		return nil, ErrNotPDF
	}
	d := &doc{data: data, objs: make(map[int]any)}
	for _, m := range objHeader.FindAllSubmatchIndex(data, -1) {
		if len(d.objs) >= maxObjects {
			break
		}
		num, _ := strconv.Atoi(string(data[m[2]:m[3]]))
		ps := &parser{b: data, p: m[1], refs: true}
		v, err := ps.value()
		if err != nil {
			continue
		}
		if dict, ok := v.(Dict); ok {
			ps.skipWS()
			if bytes.HasPrefix(data[ps.p:], []byte("stream")) {
				v = d.readStream(dict, ps.p+len("stream"))
			}
		}
		d.objs[num] = v
	}
	d.unpackObjectStreams()
	if err := d.findRoot(data); err != nil {
		return nil, err
	}
	return d, nil
}

// readStream takes the bytes that follow the "stream" keyword at start.
func (d *doc) readStream(dict Dict, start int) Stream {
	data := d.data
	if start < len(data) && data[start] == '\r' {
		start++
	}
	if start < len(data) && data[start] == '\n' {
		start++
	}
	if n, ok := dict["Length"].(float64); ok && n >= 0 && start+int(n) <= len(data) {
		end := start + int(n)
		rest := bytes.TrimLeft(data[end:min(end+8, len(data))], "\r\n ")
		if bytes.HasPrefix(rest, []byte("endstream")) {
			return Stream{Dict: dict, Raw: data[start:end]}
		}
	}
	end := bytes.Index(data[start:], []byte("endstream"))
	if end < 0 {
		return Stream{Dict: dict, Raw: data[start:]}
	}
	raw := data[start : start+end]
	raw = bytes.TrimSuffix(raw, []byte("\n"))
	raw = bytes.TrimSuffix(raw, []byte("\r"))
	return Stream{Dict: dict, Raw: raw}
}

// resolve follows references to the object they name.
func (d *doc) resolve(v any) any {
	for i := 0; i < 16; i++ {
		ref, ok := v.(Ref)
		if !ok {
			return v
		}
		v = d.objs[ref.Num]
	}
	return nil
}

func (d *doc) dict(v any) Dict {
	switch t := d.resolve(v).(type) {
	case Dict:
		return t
	case Stream:
		return t.Dict
	}
	return nil
}

func (d *doc) name(v any) Name {
	n, _ := d.resolve(v).(Name)
	return n
}

func (d *doc) array(v any) Array {
	a, _ := d.resolve(v).(Array)
	return a
}

func (d *doc) number(v any) (float64, bool) {
	f, ok := d.resolve(v).(float64)
	return f, ok
}

func (d *doc) unpackObjectStreams() {
	var found []Stream
	for _, v := range d.objs {
		if s, ok := v.(Stream); ok && s.Dict["Type"] == Name("ObjStm") {
			found = append(found, s)
		}
	}
	for _, s := range found {
		data, err := d.decode(s)
		if err != nil {
			continue
		}
		n, _ := d.number(s.Dict["N"])
		first, _ := d.number(s.Dict["First"])
		if n <= 0 || n > 1<<20 || first < 0 || int(first) > len(data) {
			continue
		}
		ps := &parser{b: data}
		type entry struct{ num, off int }
		var entries []entry
		for i := 0; i < int(n); i++ {
			a, err1 := ps.value()
			b, err2 := ps.value()
			num, ok1 := a.(float64)
			off, ok2 := b.(float64)
			if err1 != nil || err2 != nil || !ok1 || !ok2 {
				break
			}
			entries = append(entries, entry{int(num), int(off)})
		}
		for _, e := range entries {
			at := int(first) + e.off
			if at < 0 || at >= len(data) {
				continue
			}
			if _, exists := d.objs[e.num]; exists {
				continue
			}
			op := &parser{b: data, p: at, refs: true}
			if v, err := op.value(); err == nil {
				d.objs[e.num] = v
			}
		}
	}
}

func (d *doc) findRoot(data []byte) error {
	var root any
	encrypted := false
	note := func(dict Dict) {
		if dict == nil {
			return
		}
		if r, ok := dict["Root"]; ok {
			root = r
		}
		if _, ok := dict["Encrypt"]; ok {
			encrypted = true
		}
	}
	for at := 0; ; {
		i := bytes.Index(data[at:], []byte("trailer"))
		if i < 0 {
			break
		}
		at += i + len("trailer")
		ps := &parser{b: data, p: at, refs: true}
		if v, err := ps.value(); err == nil {
			if dict, ok := v.(Dict); ok {
				note(dict)
			}
		}
	}
	for _, v := range d.objs {
		if s, ok := v.(Stream); ok && s.Dict["Type"] == Name("XRef") {
			note(s.Dict)
		}
	}
	if encrypted {
		return ErrEncrypted
	}
	if d.dict(root) == nil {
		for _, v := range d.objs {
			if dict, ok := v.(Dict); ok && dict["Type"] == Name("Catalog") {
				root = dict
				break
			}
		}
	}
	d.root = root
	if d.dict(root) == nil {
		return errors.New("the PDF has no document catalog")
	}
	return nil
}

// decode applies the stream's filters.
func (d *doc) decode(s Stream) ([]byte, error) {
	var filters []Name
	switch f := d.resolve(s.Dict["Filter"]).(type) {
	case Name:
		filters = []Name{f}
	case Array:
		for _, e := range f {
			filters = append(filters, d.name(e))
		}
	}
	data := s.Raw
	for _, f := range filters {
		var err error
		switch f {
		case "FlateDecode", "Fl":
			data, err = inflate(data)
		case "ASCIIHexDecode", "AHx":
			data = asciiHex(data)
		case "ASCII85Decode", "A85":
			data, err = ascii85(data)
		default:
			return nil, fmt.Errorf("unsupported stream filter %s", f)
		}
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

func inflate(raw []byte) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()
	out, err := io.ReadAll(io.LimitReader(zr, maxStreamSize+1))
	if len(out) > maxStreamSize {
		return nil, errors.New("a stream inflates to an implausible size")
	}
	if err != nil && len(out) == 0 {
		return nil, err
	}
	// A truncated or damaged tail still leaves the text before it.
	return out, nil
}

func asciiHex(raw []byte) []byte {
	var out []byte
	hi := -1
	for _, c := range raw {
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
	return out
}

func ascii85(raw []byte) ([]byte, error) {
	var out []byte
	var group [5]byte
	n := 0
	for _, c := range raw {
		switch {
		case isSpace(c):
			continue
		case c == '~':
			return flush85(out, group[:], n), nil
		case c == 'z' && n == 0:
			out = append(out, 0, 0, 0, 0)
			continue
		case c < '!' || c > 'u':
			return nil, errors.New("bad ASCII85 data")
		}
		group[n] = c - '!'
		n++
		if n == 5 {
			out = flush85(out, group[:], 5)
			n = 0
		}
		if len(out) > maxStreamSize {
			return nil, errors.New("a stream decodes to an implausible size")
		}
	}
	return flush85(out, group[:], n), nil
}

func flush85(out, group []byte, n int) []byte {
	if n < 2 {
		return out
	}
	for i := n; i < 5; i++ {
		group[i] = 'u' - '!'
	}
	var v uint32
	for i := 0; i < 5; i++ {
		v = v*85 + uint32(group[i])
	}
	b := []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
	return append(out, b[:n-1]...)
}

// pages returns the page dictionaries in document order, each with the
// resources it inherits filled in.
func (d *doc) pages() []Dict {
	var out []Dict
	visited := make(map[int]bool)
	var walk func(node any, inherited any, depth int)
	walk = func(node any, inherited any, depth int) {
		if depth > 32 || len(out) >= maxPages {
			return
		}
		if ref, ok := node.(Ref); ok {
			if visited[ref.Num] {
				return
			}
			visited[ref.Num] = true
		}
		dict := d.dict(node)
		if dict == nil {
			return
		}
		res := inherited
		if r, ok := dict["Resources"]; ok {
			res = r
		}
		if kids, ok := d.resolve(dict["Kids"]).(Array); ok {
			for _, kid := range kids {
				walk(kid, res, depth+1)
			}
			return
		}
		if d.name(dict["Type"]) == "Pages" {
			return
		}
		page := make(Dict, len(dict)+1)
		for k, v := range dict {
			page[k] = v
		}
		if res != nil {
			page["Resources"] = res
		}
		out = append(out, page)
	}
	catalog := d.dict(d.root)
	walk(catalog["Pages"], nil, 0)
	return out
}
