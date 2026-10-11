package dotnet

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"
)

// The arguments of a custom attribute (ECMA-335 II.23.3): the blob starts with
// the prolog 0x0001, then one value for each parameter of the constructor, its
// type known from the constructor's signature, then a count and the named
// field and property arguments, each with its own type tag. Only what can be
// read without the metadata of another assembly is decoded: the numbers,
// booleans, characters, strings, System.Type, one-dimensional arrays, boxed
// values of those, and enums declared in the same file. An attribute with
// anything else among its arguments is shown without arguments at all, since
// the values after an unreadable one cannot be located.

// attrType is the type of a constructor parameter or of a named argument.
type attrType struct {
	code byte // an ELEMENT_TYPE_* code, or attrEnum, attrTypeObj
	elem *attrType
	enum string // the enum's name when code is attrEnum
	wide byte   // the enum's underlying ELEMENT_TYPE_* code
}

const (
	attrEnum    = 0xF0 // an enum declared in this file; wide holds its underlying type
	attrTypeObj = 0xF1 // System.Type, stored as a string
	maxAttrArgs = 16
	maxAttrText = 240
	maxAttrElem = 64
)

// argType reads one type of a constructor signature.
func (r *sigReader) argType() (attrType, bool) {
	if r.depth > 4 {
		return attrType{}, false
	}
	code := r.byte()
	if r.bad {
		return attrType{}, false
	}
	switch code {
	case 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x1c:
		return attrType{code: code}, true
	case 0x1d: // SZARRAY
		r.depth++
		elem, ok := r.argType()
		r.depth--
		if !ok {
			return attrType{}, false
		}
		return attrType{code: code, elem: &elem}, true
	case 0x11, 0x12: // VALUETYPE, CLASS followed by a TypeDefOrRef token
		tok := r.number()
		if r.bad || r.t == nil {
			return attrType{}, false
		}
		if code == 0x12 {
			if r.t.typeName(tok) == "System.Type" {
				return attrType{code: attrTypeObj}, true
			}
			return attrType{}, false
		}
		if tok&3 != 0 { // an enum of another assembly: its width is unknown
			return attrType{}, false
		}
		wide, ok := r.t.enumUnderlying(tok >> 2)
		if !ok {
			return attrType{}, false
		}
		return attrType{code: attrEnum, enum: r.t.typeDefName(tok >> 2), wide: wide}, true
	}
	return attrType{}, false
}

// enumUnderlying finds the type an enum's values are stored as: the type of its
// "value__" field.
func (t *tables) enumUnderlying(typeRow uint32) (byte, bool) {
	if typeRow == 0 || typeRow > t.rowCount[0x02] {
		return 0, false
	}
	start := t.cell(0x02, typeRow, 4)
	end := t.rowCount[0x04] + 1
	if typeRow < t.rowCount[0x02] {
		end = t.cell(0x02, typeRow+1, 4)
	}
	if end > t.rowCount[0x04]+1 {
		end = t.rowCount[0x04] + 1
	}
	for row := start; row >= 1 && row < end; row++ {
		if t.str(t.cell(0x04, row, 1)) != "value__" {
			continue
		}
		sig := t.blobAt(t.cell(0x04, row, 2))
		if len(sig) >= 2 && sig[0] == 0x06 {
			switch sig[1] {
			case 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b:
				return sig[1], true
			}
		}
		return 0, false
	}
	return 0, false
}

// ctorParams reads the parameter types of a constructor signature.
func (t *tables) ctorParams(sig []byte) ([]attrType, bool) {
	r := &sigReader{t: t, data: sig}
	r.byte() // calling convention
	n := r.number()
	if r.bad || n > maxAttrArgs {
		return nil, false
	}
	r.typeString() // the return type, void
	var params []attrType
	for i := uint32(0); i < n; i++ {
		p, ok := r.argType()
		if !ok {
			return nil, false
		}
		params = append(params, p)
	}
	return params, !r.bad
}

// attrBlob reads the values of an attribute blob.
type attrBlob struct {
	data []byte
	pos  int
	bad  bool
}

func (b *attrBlob) take(n int) []byte {
	if b.bad || n < 0 || b.pos+n > len(b.data) {
		b.bad = true
		return nil
	}
	out := b.data[b.pos : b.pos+n]
	b.pos += n
	return out
}

func (b *attrBlob) u8() byte {
	if v := b.take(1); v != nil {
		return v[0]
	}
	return 0
}

// serString reads a SerString: 0xFF for null, else a compressed length and
// UTF-8 bytes.
func (b *attrBlob) serString() (string, bool) {
	if b.pos < len(b.data) && b.data[b.pos] == 0xFF {
		b.pos++
		return "", false
	}
	n, used, ok := readCompressed(b.data[min(b.pos, len(b.data)):])
	if !ok {
		b.bad = true
		return "", false
	}
	b.pos += used
	if int64(n) > int64(len(b.data)) {
		b.bad = true
		return "", false
	}
	return string(b.take(int(n))), !b.bad
}

func quoteAttr(s string) string {
	q := strconv.Quote(s)
	if len(q) > maxAttrText {
		q = q[:maxAttrText] + "..."
	}
	return q
}

// value reads one value of type typ and writes it as C# would.
func (b *attrBlob) value(typ attrType, depth int) string {
	if depth > 4 {
		b.bad = true
		return ""
	}
	switch typ.code {
	case 0x02:
		if b.u8() != 0 {
			return "true"
		}
		return "false"
	case 0x03:
		v := b.take(2)
		if v == nil {
			return ""
		}
		return strconv.QuoteRune(rune(binary.LittleEndian.Uint16(v)))
	case 0x04:
		return strconv.FormatInt(int64(int8(b.u8())), 10) // #nosec G115 -- reinterpreting a stored byte
	case 0x05:
		return strconv.Itoa(int(b.u8()))
	case 0x06:
		if v := b.take(2); v != nil {
			return strconv.FormatInt(int64(int16(binary.LittleEndian.Uint16(v))), 10) // #nosec G115 -- reinterpreting stored bits
		}
	case 0x07:
		if v := b.take(2); v != nil {
			return strconv.Itoa(int(binary.LittleEndian.Uint16(v)))
		}
	case 0x08:
		if v := b.take(4); v != nil {
			return strconv.FormatInt(int64(int32(binary.LittleEndian.Uint32(v))), 10) // #nosec G115 -- reinterpreting stored bits
		}
	case 0x09:
		if v := b.take(4); v != nil {
			return strconv.FormatUint(uint64(binary.LittleEndian.Uint32(v)), 10)
		}
	case 0x0a:
		if v := b.take(8); v != nil {
			return strconv.FormatInt(int64(binary.LittleEndian.Uint64(v)), 10) // #nosec G115 -- reinterpreting stored bits
		}
	case 0x0b:
		if v := b.take(8); v != nil {
			return strconv.FormatUint(binary.LittleEndian.Uint64(v), 10)
		}
	case 0x0c:
		if v := b.take(4); v != nil {
			return strconv.FormatFloat(float64(math.Float32frombits(binary.LittleEndian.Uint32(v))), 'g', -1, 32)
		}
	case 0x0d:
		if v := b.take(8); v != nil {
			return strconv.FormatFloat(math.Float64frombits(binary.LittleEndian.Uint64(v)), 'g', -1, 64)
		}
	case 0x0e:
		s, ok := b.serString()
		if !ok {
			if b.bad {
				return ""
			}
			return "null"
		}
		return quoteAttr(s)
	case attrTypeObj:
		s, ok := b.serString()
		if !ok {
			if b.bad {
				return ""
			}
			return "null"
		}
		return "typeof(" + s + ")"
	case attrEnum:
		return typ.enum + "(" + b.value(attrType{code: typ.wide}, depth+1) + ")"
	case 0x1c: // a boxed value: its own type tag first
		tag := b.u8()
		switch tag {
		case 0x50:
			return b.value(attrType{code: attrTypeObj}, depth+1)
		case 0x1d:
			elem := b.u8()
			return b.value(attrType{code: 0x1d, elem: &attrType{code: elem}}, depth+1)
		case 0x55, 0x51:
			b.bad = true // an enum of another assembly, or a nested boxed value
			return ""
		}
		return b.value(attrType{code: tag}, depth+1)
	case 0x1d:
		if typ.elem == nil {
			b.bad = true
			return ""
		}
		n := b.take(4)
		if n == nil {
			return ""
		}
		count := binary.LittleEndian.Uint32(n)
		if count == math.MaxUint32 {
			return "null"
		}
		if count > maxAttrElem {
			b.bad = true
			return ""
		}
		parts := make([]string, 0, count)
		for i := uint32(0); i < count && !b.bad; i++ {
			parts = append(parts, b.value(*typ.elem, depth+1))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		b.bad = true
		return ""
	}
	b.bad = true
	return ""
}

// attributeText writes the arguments of one attribute: the positional ones,
// then Name = value for the named ones. It returns "" when the blob cannot be
// read completely.
func attributeText(params []attrType, blob []byte) string {
	if len(blob) < 2 || blob[0] != 0x01 || blob[1] != 0x00 {
		return ""
	}
	b := &attrBlob{data: blob, pos: 2}
	var args []string
	for _, p := range params {
		args = append(args, b.value(p, 0))
		if b.bad {
			return ""
		}
	}
	if b.pos < len(blob) {
		count := b.take(2)
		if count == nil {
			return ""
		}
		named := int(binary.LittleEndian.Uint16(count))
		if named > maxAttrArgs {
			return ""
		}
		for i := 0; i < named; i++ {
			kind := b.u8()
			if kind != 0x53 && kind != 0x54 {
				return ""
			}
			var typ attrType
			switch tag := b.u8(); tag {
			case 0x50:
				typ = attrType{code: attrTypeObj}
			case 0x1d:
				elem := b.u8()
				typ = attrType{code: 0x1d, elem: &attrType{code: elem}}
			case 0x55: // an enum named by a string: its width is not known here
				return ""
			default:
				typ = attrType{code: tag}
			}
			name, _ := b.serString()
			if b.bad {
				return ""
			}
			args = append(args, name+" = "+b.value(typ, 0))
			if b.bad {
				return ""
			}
		}
	}
	if len(args) > maxAttrArgs {
		return ""
	}
	return strings.Join(args, ", ")
}

// attributeArgs finds the constructor's signature and the value blob of one
// CustomAttribute row and decodes the arguments.
func (t *tables) attributeArgs(ctorCoded, valueBlob uint32) string {
	row := ctorCoded >> 3
	var sig []byte
	switch ctorCoded & 7 {
	case 2:
		if row >= 1 && row <= t.rowCount[0x06] {
			sig = t.blobAt(t.cell(0x06, row, 4))
		}
	case 3:
		if row >= 1 && row <= t.rowCount[0x0A] {
			sig = t.blobAt(t.cell(0x0A, row, 2))
		}
	}
	if len(sig) == 0 {
		return ""
	}
	params, ok := t.ctorParams(sig)
	if !ok {
		return ""
	}
	return attributeText(params, t.blobAt(valueBlob))
}
