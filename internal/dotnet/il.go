package dotnet

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// operand kinds of a CIL instruction (ECMA-335 III, opcode tables).
type operand int

const (
	opNone     operand = iota
	opInt8             // ldc.i4.s
	opInt32            // ldc.i4
	opInt64            // ldc.i8
	opFloat32          // ldc.r4
	opFloat64          // ldc.r8
	opVar8             // short argument/local index
	opVar16            // argument/local index
	opTarget8          // short branch offset
	opTarget32         // branch offset
	opSwitch           // count, then that many offsets
	opMethod           // MethodDef, MemberRef or MethodSpec token
	opField            // Field or MemberRef token
	opType             // TypeDef, TypeRef or TypeSpec token
	opString           // #US token
	opSig              // StandAloneSig token
	opToken            // any member token (ldtoken)
	opUint8            // unaligned. alignment
)

type opcode struct {
	name string
	arg  operand
}

// opcodes maps a one-byte opcode; prefixed maps the second byte of a 0xFE one.
var opcodes = map[byte]opcode{
	0x00: {"nop", opNone}, 0x01: {"break", opNone},
	0x02: {"ldarg.0", opNone}, 0x03: {"ldarg.1", opNone}, 0x04: {"ldarg.2", opNone}, 0x05: {"ldarg.3", opNone},
	0x06: {"ldloc.0", opNone}, 0x07: {"ldloc.1", opNone}, 0x08: {"ldloc.2", opNone}, 0x09: {"ldloc.3", opNone},
	0x0a: {"stloc.0", opNone}, 0x0b: {"stloc.1", opNone}, 0x0c: {"stloc.2", opNone}, 0x0d: {"stloc.3", opNone},
	0x0e: {"ldarg.s", opVar8}, 0x0f: {"ldarga.s", opVar8}, 0x10: {"starg.s", opVar8},
	0x11: {"ldloc.s", opVar8}, 0x12: {"ldloca.s", opVar8}, 0x13: {"stloc.s", opVar8},
	0x14: {"ldnull", opNone}, 0x15: {"ldc.i4.m1", opNone},
	0x16: {"ldc.i4.0", opNone}, 0x17: {"ldc.i4.1", opNone}, 0x18: {"ldc.i4.2", opNone}, 0x19: {"ldc.i4.3", opNone},
	0x1a: {"ldc.i4.4", opNone}, 0x1b: {"ldc.i4.5", opNone}, 0x1c: {"ldc.i4.6", opNone}, 0x1d: {"ldc.i4.7", opNone},
	0x1e: {"ldc.i4.8", opNone}, 0x1f: {"ldc.i4.s", opInt8}, 0x20: {"ldc.i4", opInt32},
	0x21: {"ldc.i8", opInt64}, 0x22: {"ldc.r4", opFloat32}, 0x23: {"ldc.r8", opFloat64},
	0x25: {"dup", opNone}, 0x26: {"pop", opNone}, 0x27: {"jmp", opMethod}, 0x28: {"call", opMethod},
	0x29: {"calli", opSig}, 0x2a: {"ret", opNone},
	0x2b: {"br.s", opTarget8}, 0x2c: {"brfalse.s", opTarget8}, 0x2d: {"brtrue.s", opTarget8},
	0x2e: {"beq.s", opTarget8}, 0x2f: {"bge.s", opTarget8}, 0x30: {"bgt.s", opTarget8}, 0x31: {"ble.s", opTarget8},
	0x32: {"blt.s", opTarget8}, 0x33: {"bne.un.s", opTarget8}, 0x34: {"bge.un.s", opTarget8},
	0x35: {"bgt.un.s", opTarget8}, 0x36: {"ble.un.s", opTarget8}, 0x37: {"blt.un.s", opTarget8},
	0x38: {"br", opTarget32}, 0x39: {"brfalse", opTarget32}, 0x3a: {"brtrue", opTarget32},
	0x3b: {"beq", opTarget32}, 0x3c: {"bge", opTarget32}, 0x3d: {"bgt", opTarget32}, 0x3e: {"ble", opTarget32},
	0x3f: {"blt", opTarget32}, 0x40: {"bne.un", opTarget32}, 0x41: {"bge.un", opTarget32},
	0x42: {"bgt.un", opTarget32}, 0x43: {"ble.un", opTarget32}, 0x44: {"blt.un", opTarget32},
	0x45: {"switch", opSwitch},
	0x46: {"ldind.i1", opNone}, 0x47: {"ldind.u1", opNone}, 0x48: {"ldind.i2", opNone}, 0x49: {"ldind.u2", opNone},
	0x4a: {"ldind.i4", opNone}, 0x4b: {"ldind.u4", opNone}, 0x4c: {"ldind.i8", opNone}, 0x4d: {"ldind.i", opNone},
	0x4e: {"ldind.r4", opNone}, 0x4f: {"ldind.r8", opNone}, 0x50: {"ldind.ref", opNone},
	0x51: {"stind.ref", opNone}, 0x52: {"stind.i1", opNone}, 0x53: {"stind.i2", opNone}, 0x54: {"stind.i4", opNone},
	0x55: {"stind.i8", opNone}, 0x56: {"stind.r4", opNone}, 0x57: {"stind.r8", opNone},
	0x58: {"add", opNone}, 0x59: {"sub", opNone}, 0x5a: {"mul", opNone}, 0x5b: {"div", opNone}, 0x5c: {"div.un", opNone},
	0x5d: {"rem", opNone}, 0x5e: {"rem.un", opNone}, 0x5f: {"and", opNone}, 0x60: {"or", opNone}, 0x61: {"xor", opNone},
	0x62: {"shl", opNone}, 0x63: {"shr", opNone}, 0x64: {"shr.un", opNone}, 0x65: {"neg", opNone}, 0x66: {"not", opNone},
	0x67: {"conv.i1", opNone}, 0x68: {"conv.i2", opNone}, 0x69: {"conv.i4", opNone}, 0x6a: {"conv.i8", opNone},
	0x6b: {"conv.r4", opNone}, 0x6c: {"conv.r8", opNone}, 0x6d: {"conv.u4", opNone}, 0x6e: {"conv.u8", opNone},
	0x6f: {"callvirt", opMethod}, 0x70: {"cpobj", opType}, 0x71: {"ldobj", opType}, 0x72: {"ldstr", opString},
	0x73: {"newobj", opMethod}, 0x74: {"castclass", opType}, 0x75: {"isinst", opType}, 0x76: {"conv.r.un", opNone},
	0x79: {"unbox", opType}, 0x7a: {"throw", opNone},
	0x7b: {"ldfld", opField}, 0x7c: {"ldflda", opField}, 0x7d: {"stfld", opField},
	0x7e: {"ldsfld", opField}, 0x7f: {"ldsflda", opField}, 0x80: {"stsfld", opField}, 0x81: {"stobj", opType},
	0x82: {"conv.ovf.i1.un", opNone}, 0x83: {"conv.ovf.i2.un", opNone}, 0x84: {"conv.ovf.i4.un", opNone},
	0x85: {"conv.ovf.i8.un", opNone}, 0x86: {"conv.ovf.u1.un", opNone}, 0x87: {"conv.ovf.u2.un", opNone},
	0x88: {"conv.ovf.u4.un", opNone}, 0x89: {"conv.ovf.u8.un", opNone}, 0x8a: {"conv.ovf.i.un", opNone},
	0x8b: {"conv.ovf.u.un", opNone}, 0x8c: {"box", opType}, 0x8d: {"newarr", opType}, 0x8e: {"ldlen", opNone},
	0x8f: {"ldelema", opType},
	0x90: {"ldelem.i1", opNone}, 0x91: {"ldelem.u1", opNone}, 0x92: {"ldelem.i2", opNone}, 0x93: {"ldelem.u2", opNone},
	0x94: {"ldelem.i4", opNone}, 0x95: {"ldelem.u4", opNone}, 0x96: {"ldelem.i8", opNone}, 0x97: {"ldelem.i", opNone},
	0x98: {"ldelem.r4", opNone}, 0x99: {"ldelem.r8", opNone}, 0x9a: {"ldelem.ref", opNone},
	0x9b: {"stelem.i", opNone}, 0x9c: {"stelem.i1", opNone}, 0x9d: {"stelem.i2", opNone}, 0x9e: {"stelem.i4", opNone},
	0x9f: {"stelem.i8", opNone}, 0xa0: {"stelem.r4", opNone}, 0xa1: {"stelem.r8", opNone}, 0xa2: {"stelem.ref", opNone},
	0xa3: {"ldelem", opType}, 0xa4: {"stelem", opType}, 0xa5: {"unbox.any", opType},
	0xb3: {"conv.ovf.i1", opNone}, 0xb4: {"conv.ovf.u1", opNone}, 0xb5: {"conv.ovf.i2", opNone},
	0xb6: {"conv.ovf.u2", opNone}, 0xb7: {"conv.ovf.i4", opNone}, 0xb8: {"conv.ovf.u4", opNone},
	0xb9: {"conv.ovf.i8", opNone}, 0xba: {"conv.ovf.u8", opNone},
	0xc2: {"refanyval", opType}, 0xc3: {"ckfinite", opNone}, 0xc6: {"mkrefany", opType},
	0xd0: {"ldtoken", opToken}, 0xd1: {"conv.u2", opNone}, 0xd2: {"conv.u1", opNone}, 0xd3: {"conv.i", opNone},
	0xd4: {"conv.ovf.i", opNone}, 0xd5: {"conv.ovf.u", opNone}, 0xd6: {"add.ovf", opNone}, 0xd7: {"add.ovf.un", opNone},
	0xd8: {"mul.ovf", opNone}, 0xd9: {"mul.ovf.un", opNone}, 0xda: {"sub.ovf", opNone}, 0xdb: {"sub.ovf.un", opNone},
	0xdc: {"endfinally", opNone}, 0xdd: {"leave", opTarget32}, 0xde: {"leave.s", opTarget8},
	0xdf: {"stind.i", opNone}, 0xe0: {"conv.u", opNone},
}

var prefixedOpcodes = map[byte]opcode{
	0x00: {"arglist", opNone}, 0x01: {"ceq", opNone}, 0x02: {"cgt", opNone}, 0x03: {"cgt.un", opNone},
	0x04: {"clt", opNone}, 0x05: {"clt.un", opNone}, 0x06: {"ldftn", opMethod}, 0x07: {"ldvirtftn", opMethod},
	0x09: {"ldarg", opVar16}, 0x0a: {"ldarga", opVar16}, 0x0b: {"starg", opVar16},
	0x0c: {"ldloc", opVar16}, 0x0d: {"ldloca", opVar16}, 0x0e: {"stloc", opVar16},
	0x0f: {"localloc", opNone}, 0x11: {"endfilter", opNone}, 0x12: {"unaligned.", opUint8},
	0x13: {"volatile.", opNone}, 0x14: {"tail.", opNone}, 0x15: {"initobj", opType},
	0x16: {"constrained.", opType}, 0x17: {"cpblk", opNone}, 0x18: {"initblk", opNone},
	0x1a: {"rethrow", opNone}, 0x1c: {"sizeof", opType}, 0x1d: {"refanytype", opNone}, 0x1e: {"readonly.", opNone},
}

// maxInstructions bounds how much of one method body is decoded.
const maxInstructions = 200000

// disassemble decodes the IL of one method body (the bytes at its RVA: header
// and code) into text. Anything it cannot decode is written as a comment and
// ends the listing, so a hostile or broken body never fails the caller.
func (t *tables) disassemble(body []byte) string {
	code, maxStack, localSig, ok := splitBody(body)
	if !ok {
		return "// method body is not readable\n"
	}
	var b strings.Builder
	if maxStack > 0 {
		fmt.Fprintf(&b, "    .maxstack %d\n", maxStack)
	}
	if locals := t.localsText(localSig); locals != "" {
		fmt.Fprintf(&b, "    .locals (%s)\n", locals)
	}
	pos := 0
	for count := 0; pos < len(code) && count < maxInstructions; count++ {
		start := pos
		op, ok := opcodes[code[pos]]
		pos++
		if code[start] == 0xFE {
			if pos >= len(code) {
				fmt.Fprintf(&b, "    // IL_%04x: truncated prefix\n", start)
				return b.String()
			}
			op, ok = prefixedOpcodes[code[pos]]
			pos++
		}
		if !ok {
			fmt.Fprintf(&b, "    // IL_%04x: unknown opcode 0x%02x\n", start, code[start])
			return b.String()
		}
		text, used, ok := t.operandText(op.arg, code[pos:], pos)
		if !ok {
			fmt.Fprintf(&b, "    // IL_%04x: %s has a truncated operand\n", start, op.name)
			return b.String()
		}
		pos += used
		if text == "" {
			fmt.Fprintf(&b, "    IL_%04x: %s\n", start, op.name)
		} else {
			fmt.Fprintf(&b, "    IL_%04x: %s %s\n", start, op.name, text)
		}
	}
	if pos < len(code) {
		fmt.Fprintf(&b, "    // listing stops after %d instructions\n", maxInstructions)
	}
	return b.String()
}

// splitBody separates the code from the tiny or fat method header (II.25.4);
// localSig is the StandAloneSig token of the local variables (0 for none).
func splitBody(body []byte) (code []byte, maxStack int, localSig uint32, ok bool) {
	if len(body) == 0 {
		return nil, 0, 0, false
	}
	switch body[0] & 3 {
	case 2: // tiny: the size is in the upper six bits
		size := int(body[0] >> 2)
		if 1+size > len(body) {
			return nil, 0, 0, false
		}
		return body[1 : 1+size], 8, 0, true
	case 3: // fat
		if len(body) < 12 {
			return nil, 0, 0, false
		}
		headerWords := int(binary.LittleEndian.Uint16(body) >> 12)
		headerLen := headerWords * 4
		size := int(binary.LittleEndian.Uint32(body[4:]))
		if headerLen < 12 || headerLen > len(body) || size < 0 || size > len(body)-headerLen {
			return nil, 0, 0, false
		}
		return body[headerLen : headerLen+size], int(binary.LittleEndian.Uint16(body[2:])), binary.LittleEndian.Uint32(body[8:]), true
	}
	return nil, 0, 0, false
}

// operandText renders the operand at the start of data; next is the offset just
// after the operand's own bytes, needed to turn a branch offset into a target.
func (t *tables) operandText(kind operand, data []byte, at int) (text string, used int, ok bool) {
	need := func(n int) bool { return len(data) >= n }
	u32 := func() uint32 { return binary.LittleEndian.Uint32(data) }
	switch kind {
	case opNone:
		return "", 0, true
	case opInt8:
		if !need(1) {
			return "", 0, false
		}
		return fmt.Sprint(int8(data[0])), 1, true //nolint:gosec // the operand is the same bits read as signed
	case opUint8, opVar8:
		if !need(1) {
			return "", 0, false
		}
		return fmt.Sprint(data[0]), 1, true
	case opVar16:
		if !need(2) {
			return "", 0, false
		}
		return fmt.Sprint(binary.LittleEndian.Uint16(data)), 2, true
	case opInt32:
		if !need(4) {
			return "", 0, false
		}
		return fmt.Sprint(int32(u32())), 4, true //nolint:gosec // the operand is the same bits read as signed
	case opInt64:
		if !need(8) {
			return "", 0, false
		}
		return fmt.Sprint(int64(binary.LittleEndian.Uint64(data))), 8, true //nolint:gosec // the operand is the same bits read as signed
	case opFloat32:
		if !need(4) {
			return "", 0, false
		}
		return fmt.Sprint(math.Float32frombits(u32())), 4, true
	case opFloat64:
		if !need(8) {
			return "", 0, false
		}
		return fmt.Sprint(math.Float64frombits(binary.LittleEndian.Uint64(data))), 8, true
	case opTarget8:
		if !need(1) {
			return "", 0, false
		}
		return fmt.Sprintf("IL_%04x", at+1+int(int8(data[0]))), 1, true //nolint:gosec // the operand is the same bits read as signed
	case opTarget32:
		if !need(4) {
			return "", 0, false
		}
		return fmt.Sprintf("IL_%04x", at+4+int(int32(u32()))), 4, true //nolint:gosec // the operand is the same bits read as signed
	case opSwitch:
		if !need(4) {
			return "", 0, false
		}
		n := int(u32())
		if n < 0 || n > (len(data)-4)/4 {
			return "", 0, false
		}
		base := at + 4 + 4*n
		targets := make([]string, n)
		for i := range targets {
			targets[i] = fmt.Sprintf("IL_%04x", base+int(int32(binary.LittleEndian.Uint32(data[4+4*i:])))) //nolint:gosec // the operand is the same bits read as signed
		}
		return "(" + strings.Join(targets, ", ") + ")", 4 + 4*n, true
	default: // a metadata token
		if !need(4) {
			return "", 0, false
		}
		return t.tokenText(kind, u32()), 4, true
	}
}

// tokenText names the metadata item a token points at.
func (t *tables) tokenText(kind operand, token uint32) string {
	table, row := int(token>>24), token&0x00FFFFFF
	switch table {
	case 0x70: // #US string
		return quoteUserString(t.userString(row))
	case 0x02:
		return t.typeDefName(row)
	case 0x01:
		return t.typeRefName(row)
	case 0x1B:
		return t.typeSpecName(row)
	case 0x06:
		if row == 0 || row > t.rowCount[0x06] {
			return fmt.Sprintf("<bad method token 0x%08x>", token)
		}
		name := t.str(t.cell(0x06, row, 3))
		sig := t.methodSignature(t.cell(0x06, row, 4), name)
		return t.typeDefName(t.methodOwner(row)) + "::" + orDefault(sig, name)
	case 0x04:
		if row == 0 || row > t.rowCount[0x04] {
			return fmt.Sprintf("<bad field token 0x%08x>", token)
		}
		return t.typeDefName(t.fieldOwner(row)) + "::" + t.str(t.cell(0x04, row, 1))
	case 0x0A:
		if row == 0 || row > t.rowCount[0x0A] {
			return fmt.Sprintf("<bad member token 0x%08x>", token)
		}
		parent := t.cell(0x0A, row, 0)
		name := t.str(t.cell(0x0A, row, 1))
		return t.memberParentName(parent) + "::" + name
	case 0x2B:
		return t.methodSpecText(row)
	case 0x11:
		return fmt.Sprintf("<signature 0x%08x>", token)
	}
	return fmt.Sprintf("<token 0x%08x>", token)
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// memberParentName resolves a MemberRefParent coded token (II.24.2.6).
func (t *tables) memberParentName(coded uint32) string {
	row := coded >> 3
	switch coded & 7 {
	case 0:
		return t.typeDefName(row)
	case 1:
		return t.typeRefName(row)
	case 3:
		if row >= 1 && row <= t.rowCount[0x06] {
			return t.typeDefName(t.methodOwner(row))
		}
	case 4:
		return t.typeSpecName(row)
	}
	return "<parent>"
}

// typeSpecName renders the type of a TypeSpec row (a constructed type, such as
// a generic instance or an array), or a placeholder when its blob is unreadable.
func (t *tables) typeSpecName(row uint32) string {
	if row == 0 || row > t.rowCount[0x1B] {
		return "<type spec>"
	}
	data := t.blobAt(t.cell(0x1B, row, 0))
	if len(data) == 0 {
		return "<type spec>"
	}
	r := &sigReader{t: t, data: data}
	out := r.typeString()
	if r.bad {
		return "<type spec>"
	}
	return out
}

// methodOwner and fieldOwner find the TypeDef row that lists a method or field
// row: the last type whose list starts at or before it.
func (t *tables) methodOwner(row uint32) uint32 { return t.owner(row, 5) }
func (t *tables) fieldOwner(row uint32) uint32  { return t.owner(row, 4) }

func (t *tables) owner(row uint32, listCol int) uint32 {
	// The list columns never decrease down the TypeDef table, so the owner is
	// the last type whose list starts at or before row.
	i := sort.Search(int(t.rowCount[0x02]), func(i int) bool {
		return t.cell(0x02, uint32(i)+1, listCol) > row //nolint:gosec // bounded by the row count of the TypeDef table
	})
	return uint32(i) //nolint:gosec // bounded by the row count of the TypeDef table
}

// userString reads the #US entry at index: a compressed byte length, the
// UTF-16 characters, and one closing flag byte.
func (t *tables) userString(index uint32) string {
	if int(index) >= len(t.us) {
		return ""
	}
	data := t.us[index:]
	n, used, ok := readCompressed(data)
	if !ok || n == 0 || int64(used)+int64(n) > int64(len(data)) {
		return ""
	}
	raw := data[used : used+int(n)-1]
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(raw[2*i:])
	}
	return string(utf16.Decode(units))
}

func quoteUserString(s string) string {
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return fmt.Sprintf("%q", s)
}

// localsText renders the local variable types of a method from its
// StandAloneSig token (II.23.2.6), e.g. "int V_0, string V_1".
func (t *tables) localsText(token uint32) string {
	if token>>24 != 0x11 {
		return ""
	}
	row := token & 0x00FFFFFF
	if row == 0 || row > t.rowCount[0x11] {
		return ""
	}
	data := t.blobAt(t.cell(0x11, row, 0))
	if len(data) < 2 || data[0] != 0x07 {
		return ""
	}
	r := &sigReader{t: t, data: data, pos: 1}
	count := int(r.number())
	if count > 65535 {
		return ""
	}
	locals := make([]string, 0, count)
	for i := 0; i < count && !r.bad; i++ {
		locals = append(locals, r.typeString()+" V_"+strconv.Itoa(i))
	}
	if r.bad {
		return ""
	}
	return strings.Join(locals, ", ")
}

// methodSpecText renders a MethodSpec row (a generic method instantiation) as
// "Type::Name<args>".
func (t *tables) methodSpecText(row uint32) string {
	if row == 0 || row > t.rowCount[0x2B] {
		return "<method spec>"
	}
	coded := t.cell(0x2B, row, 0)
	name := "<method>"
	switch mrow := coded >> 1; coded & 1 {
	case 0:
		if mrow >= 1 && mrow <= t.rowCount[0x06] {
			name = t.typeDefName(t.methodOwner(mrow)) + "::" + t.str(t.cell(0x06, mrow, 3))
		}
	case 1:
		if mrow >= 1 && mrow <= t.rowCount[0x0A] {
			name = t.memberParentName(t.cell(0x0A, mrow, 0)) + "::" + t.str(t.cell(0x0A, mrow, 1))
		}
	}
	data := t.blobAt(t.cell(0x2B, row, 1))
	if len(data) < 2 || data[0] != 0x0A {
		return name
	}
	r := &sigReader{t: t, data: data, pos: 1}
	count := int(r.number())
	if count > 64 {
		return name
	}
	args := make([]string, 0, count)
	for i := 0; i < count && !r.bad; i++ {
		args = append(args, r.typeString())
	}
	if r.bad {
		return name
	}
	return name + "<" + strings.Join(args, ", ") + ">"
}
