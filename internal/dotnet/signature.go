package dotnet

import (
	"strconv"
	"strings"
)

// A blob of the #Blob heap starts with its length, compressed as in ECMA-335
// II.23.2: one byte below 0x80, two bytes below 0xC0, four bytes otherwise.
func (t *tables) blobAt(index uint32) []byte {
	if int(index) >= len(t.blob) {
		return nil
	}
	data := t.blob[index:]
	n, used, ok := readCompressed(data)
	if !ok || int64(used)+int64(n) > int64(len(data)) {
		return nil
	}
	return data[used : used+int(n)]
}

// readCompressed decodes an unsigned integer compressed as in II.23.2 and
// reports how many bytes it took.
func readCompressed(data []byte) (uint32, int, bool) {
	if len(data) == 0 {
		return 0, 0, false
	}
	b := data[0]
	switch {
	case b&0x80 == 0:
		return uint32(b), 1, true
	case b&0xC0 == 0x80:
		if len(data) < 2 {
			return 0, 0, false
		}
		return uint32(b&0x3F)<<8 | uint32(data[1]), 2, true
	case b&0xE0 == 0xC0:
		if len(data) < 4 {
			return 0, 0, false
		}
		return uint32(b&0x1F)<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3]), 4, true
	}
	return 0, 0, false
}

// sigReader walks a signature blob; every read past its end sets bad.
type sigReader struct {
	t     *tables
	data  []byte
	pos   int
	bad   bool
	depth int
}

func (r *sigReader) byte() byte {
	if r.pos >= len(r.data) {
		r.bad = true
		return 0
	}
	b := r.data[r.pos]
	r.pos++
	return b
}

func (r *sigReader) number() uint32 {
	n, used, ok := readCompressed(r.data[min(r.pos, len(r.data)):])
	if !ok {
		r.bad = true
		return 0
	}
	r.pos += used
	return n
}

// typeName resolves a TypeDefOrRef coded token (II.23.2.8) to a readable name.
func (t *tables) typeName(coded uint32) string {
	row := coded >> 2
	switch coded & 3 {
	case 0:
		return t.typeDefName(row)
	case 1:
		return t.typeRefName(row)
	}
	return "<type spec>"
}

func qualified(ns, name string) string {
	if ns == "" {
		return name
	}
	return ns + "." + name
}

func (t *tables) typeDefName(row uint32) string {
	if row == 0 || row > t.rowCount[0x02] {
		return "?"
	}
	return qualified(t.str(t.cell(0x02, row, 2)), t.str(t.cell(0x02, row, 1)))
}

func (t *tables) typeRefName(row uint32) string {
	if row == 0 || row > t.rowCount[0x01] {
		return "?"
	}
	return qualified(t.str(t.cell(0x01, row, 2)), t.str(t.cell(0x01, row, 1)))
}

var primitiveNames = map[byte]string{
	0x01: "void", 0x02: "bool", 0x03: "char", 0x04: "sbyte", 0x05: "byte",
	0x06: "short", 0x07: "ushort", 0x08: "int", 0x09: "uint", 0x0a: "long",
	0x0b: "ulong", 0x0c: "float", 0x0d: "double", 0x0e: "string",
	0x16: "typedref", 0x18: "nint", 0x19: "nuint", 0x1c: "object",
}

// maxSigDepth bounds the nesting of a type in a signature.
const maxSigDepth = 32

// typeString reads one Type (II.23.2.12) and returns it in C# spelling.
func (r *sigReader) typeString() string {
	if r.depth > maxSigDepth {
		r.bad = true
		return "?"
	}
	r.depth++
	defer func() { r.depth-- }()
	for {
		e := r.byte()
		if r.bad {
			return "?"
		}
		if name, ok := primitiveNames[e]; ok {
			return name
		}
		switch e {
		case 0x0f: // PTR
			return r.typeString() + "*"
		case 0x10: // BYREF
			return "ref " + r.typeString()
		case 0x11, 0x12: // VALUETYPE, CLASS
			return r.t.typeName(r.number())
		case 0x13: // VAR
			return "!" + strconv.Itoa(int(r.number()))
		case 0x1e: // MVAR
			return "!!" + strconv.Itoa(int(r.number()))
		case 0x1d: // SZARRAY
			return r.typeString() + "[]"
		case 0x14: // ARRAY
			elem := r.typeString()
			rank := int(r.number())
			for skip := 0; skip < 2 && !r.bad; skip++ { // sizes, then lower bounds
				count := int(r.number())
				for i := 0; i < count && !r.bad; i++ {
					r.number()
				}
			}
			if rank < 1 || rank > 32 {
				return elem + "[?]"
			}
			return elem + "[" + strings.Repeat(",", rank-1) + "]"
		case 0x15: // GENERICINST
			r.byte() // CLASS or VALUETYPE
			name := r.t.typeName(r.number())
			count := int(r.number())
			if count > 64 {
				r.bad = true
				return "?"
			}
			args := make([]string, 0, count)
			for i := 0; i < count && !r.bad; i++ {
				args = append(args, r.typeString())
			}
			return name + "<" + strings.Join(args, ", ") + ">"
		case 0x1b: // FNPTR
			return "method*(" + r.methodString("") + ")"
		case 0x1f, 0x20: // CMOD_REQD, CMOD_OPT: a modifier, then the type
			r.number()
			continue
		case 0x45: // PINNED
			continue
		}
		r.bad = true
		return "?"
	}
}

// methodString reads a method signature (II.23.2.1) as "ret name(params)".
func (r *sigReader) methodString(name string) string {
	conv := r.byte()
	generics := 0
	if conv&0x10 != 0 {
		generics = int(r.number())
	}
	count := int(r.number())
	if count > 1024 {
		r.bad = true
		return "?"
	}
	ret := r.typeString()
	params := make([]string, 0, count)
	for i := 0; i < count && !r.bad; i++ {
		if r.pos < len(r.data) && r.data[r.pos] == 0x41 { // SENTINEL
			r.pos++
			params = append(params, "...")
		}
		params = append(params, r.typeString())
	}
	head := ret
	if name != "" {
		head += " " + name
	}
	if generics > 0 {
		head += "<" + strconv.Itoa(generics) + ">"
	}
	prefix := ""
	if conv&0x20 != 0 {
		prefix = "instance "
	}
	return prefix + head + "(" + strings.Join(params, ", ") + ")"
}

// methodSignature renders a MethodDef signature blob, or "" when it cannot be
// read.
func (t *tables) methodSignature(blobIndex uint32, name string) string {
	data := t.blobAt(blobIndex)
	if len(data) == 0 {
		return ""
	}
	r := &sigReader{t: t, data: data}
	out := r.methodString(name)
	if r.bad {
		return ""
	}
	return out
}

// fieldSignature renders a Field signature blob (II.23.2.4) as its type.
func (t *tables) fieldSignature(blobIndex uint32) string {
	data := t.blobAt(blobIndex)
	if len(data) < 2 || data[0] != 0x06 {
		return ""
	}
	r := &sigReader{t: t, data: data, pos: 1}
	out := r.typeString()
	if r.bad {
		return ""
	}
	return out
}
