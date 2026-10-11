package dotnet

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"unicode/utf16"
)

// blobHeap builds a #Blob (or #US) heap and hands out indexes into it.
type blobHeap struct{ data []byte }

func compressed(n int) []byte {
	switch {
	case n < 0x80:
		return []byte{byte(n)} //nolint:gosec // test data
	case n < 0x4000:
		return []byte{byte(0x80 | n>>8), byte(n)} //nolint:gosec // test data
	}
	return []byte{byte(0xC0 | n>>24), byte(n >> 16), byte(n >> 8), byte(n)} //nolint:gosec // test data
}

func (h *blobHeap) add(blob []byte) uint32 {
	if len(h.data) == 0 {
		h.data = []byte{0}
	}
	at := len(h.data)
	h.data = append(h.data, compressed(len(blob))...)
	h.data = append(h.data, blob...)
	return uint32(at) //nolint:gosec // test data
}

// addString stores a user string: UTF-16 characters and the closing flag byte.
func (h *blobHeap) addString(s string) uint32 {
	var raw []byte
	for _, u := range utf16.Encode([]rune(s)) {
		raw = append(raw, byte(u), byte(u>>8)) //nolint:gosec // test data
	}
	return h.add(append(raw, 0))
}

// buildTables lays out the given rows (cells as raw values) with the widths the
// production code computes, so tokens and signatures resolve as in a real file.
func buildTables(strs, blob, us []byte, rows map[int][][]uint32) *tables {
	t := &tables{strs: strs, blob: blob, us: us, colWidths: map[int][]int{}}
	for table, list := range rows {
		t.rowCount[table] = uint32(len(list)) //nolint:gosec // test data
	}
	for table, list := range rows {
		schema := tableSchema[table]
		widths := make([]int, len(schema))
		size := 0
		for i, col := range schema {
			widths[i] = t.width(col)
			size += widths[i]
		}
		t.colWidths[table], t.rowSize[table], t.offset[table] = widths, size, len(t.data)
		for _, row := range list {
			for i, cell := range row {
				if widths[i] == 2 {
					t.data = binary.LittleEndian.AppendUint16(t.data, uint16(cell)) //nolint:gosec // test data
				} else {
					t.data = binary.LittleEndian.AppendUint32(t.data, cell)
				}
			}
		}
	}
	return t
}

// ilFixture has types System.Object (TypeRef 1), My.Ns.Foo (TypeDef 1) and
// Bar (TypeDef 2), one member reference, one field, two methods, a type spec
// for List<int>, a method spec, a local signature and user strings.
type ilFixture struct {
	t          *tables
	hello      uint32 // #US token index of "hello"
	long       uint32 // #US index of a 300-character string
	listSpec   uint32 // TypeSpec row of List<int>
	localsSig  uint32 // StandAloneSig token
	methodSpec uint32 // MethodSpec token
}

func newILFixture() *ilFixture {
	var strs heap
	strs.add("")
	sidx := func(name string) uint32 { return uint32(strs.add(name)) } //nolint:gosec // test data
	object := sidx("Object")
	system := sidx("System")
	foo := sidx("Foo")
	ns := sidx("My.Ns")
	bar := sidx("Bar")
	ctor := sidx(".ctor")
	run := sidx("Run")
	count := sidx("Count")
	list := sidx("List`1")
	generic := sidx("System.Collections.Generic")
	var blob, us blobHeap
	voidSig := blob.add([]byte{0x20, 0x00, 0x01})            // instance void ()
	runSig := blob.add([]byte{0x20, 0x02, 0x08, 0x0e, 0x08}) // instance int (string, int)
	fieldSig := blob.add([]byte{0x06, 0x0e})                 // string
	listTypeSpec := blob.add([]byte{0x15, 0x12, byte(2<<2 | 1), 0x01, 0x08})
	locals := blob.add([]byte{0x07, 0x02, 0x08, 0x0e}) // int, string
	inst := blob.add([]byte{0x0a, 0x01, 0x0e})         // <string>
	notLocals := blob.add([]byte{0x08, 0x01})
	shortLocals := blob.add([]byte{0x07, 0x03, 0x08})
	notInst := blob.add([]byte{0x0b, 0x01, 0x0e})
	shortInst := blob.add([]byte{0x0a, 0x05, 0x0e})
	hugeInst := blob.add([]byte{0x0a, 0x7F, 0x0e})
	f := &ilFixture{hello: us.addString("hello"), long: us.addString(strings.Repeat("x", 300))}
	rows := map[int][][]uint32{
		// TypeRef: ResolutionScope, Name, Namespace.
		0x01: {{0, object, system}, {0, list, generic}},
		// TypeDef: Flags, Name, Namespace, Extends, FieldList, MethodList.
		0x02: {{0, foo, ns, 0, 1, 1}, {0, bar, 0, 0, 2, 3}},
		// Field: Flags, Name, Signature.
		0x04: {{0, count, fieldSig}},
		// MethodDef: RVA, ImplFlags, Flags, Name, Signature, ParamList.
		0x06: {{0, 0, 0, ctor, voidSig, 1}, {0, 0, 0, run, runSig, 1}},
		// MemberRef: Parent (TypeRef 1), Name, Signature.
		0x0A: {{1<<3 | 1, ctor, voidSig}, {1 << 3, run, runSig}, {1<<3 | 4, run, runSig}, {1<<3 | 3, run, runSig}},
		0x11: {{locals}, {notLocals}, {shortLocals}},
		0x1B: {{listTypeSpec}},
		// MethodSpec: Method (MethodDef 2), Instantiation.
		0x2B: {{2 << 1, inst}, {1<<1 | 1, inst}, {9 << 1, inst}, {2 << 1, notInst}, {2 << 1, shortInst}, {2 << 1, hugeInst}},
	}
	f.t = buildTables(strs.data, blob.data, us.data, rows)
	f.listSpec = 1
	f.localsSig = 0x11000001
	f.methodSpec = 0x2B000001
	return f
}

func TestReadCompressed(t *testing.T) {
	for _, c := range []struct {
		in   []byte
		want uint32
		used int
		ok   bool
	}{
		{[]byte{0x03}, 3, 1, true},
		{[]byte{0x7F}, 0x7F, 1, true},
		{[]byte{0x80, 0x80}, 0x80, 2, true},
		{[]byte{0xBF, 0xFF}, 0x3FFF, 2, true},
		{[]byte{0xC0, 0x00, 0x40, 0x00}, 0x4000, 4, true},
		{nil, 0, 0, false},
		{[]byte{0x80}, 0, 0, false},
		{[]byte{0xC0, 0, 0}, 0, 0, false},
		{[]byte{0xFF}, 0, 0, false},
	} {
		got, used, ok := readCompressed(c.in)
		if got != c.want || used != c.used || ok != c.ok {
			t.Errorf("readCompressed(%x) = %d, %d, %v; want %d, %d, %v", c.in, got, used, ok, c.want, c.used, c.ok)
		}
	}
}

func TestBlobAt(t *testing.T) {
	f := newILFixture()
	if got := f.t.blobAt(1); len(got) != 3 || got[0] != 0x20 {
		t.Errorf("blobAt(1) = %x", got)
	}
	if f.t.blobAt(uint32(len(f.t.blob))) != nil || f.t.blobAt(1<<30) != nil { //nolint:gosec // test data
		t.Error("blobAt past the heap must be nil")
	}
	// A length that runs past the end of the heap.
	bad := &tables{blob: []byte{0, 0x7F, 1}}
	if bad.blobAt(1) != nil {
		t.Error("blobAt with an overlong length must be nil")
	}
}

func TestTypeNames(t *testing.T) {
	f := newILFixture()
	tab := f.t
	for _, c := range []struct {
		coded uint32
		want  string
	}{
		{1 << 2, "My.Ns.Foo"},
		{2 << 2, "Bar"},
		{1<<2 | 1, "System.Object"},
		{9 << 2, "?"},
		{9<<2 | 1, "?"},
		{1<<2 | 2, "<type spec>"},
		{0, "?"},
	} {
		if got := tab.typeName(c.coded); got != c.want {
			t.Errorf("typeName(%#x) = %q, want %q", c.coded, got, c.want)
		}
	}
}

func TestSignatures(t *testing.T) {
	f := newILFixture()
	tab := f.t
	ref := func(row int) byte { return byte(row<<2 | 1) } //nolint:gosec // test data
	def := func(row int) byte { return byte(row << 2) }   //nolint:gosec // test data
	cases := []struct {
		name string
		sig  []byte
		want string
	}{
		{"primitives", []byte{0x00, 0x02, 0x02, 0x03, 0x04}, "bool(char, sbyte)"},
		{"more primitives", []byte{0x00, 0x03, 0x05, 0x06, 0x07, 0x09}, "byte(short, ushort, uint)"},
		{"wide", []byte{0x00, 0x03, 0x0a, 0x0b, 0x0c, 0x0d}, "long(ulong, float, double)"},
		{"native", []byte{0x00, 0x03, 0x18, 0x19, 0x1c, 0x16}, "nint(nuint, object, typedref)"},
		{"void", []byte{0x00, 0x00, 0x01}, "void()"},
		{"instance", []byte{0x20, 0x01, 0x01, 0x0e}, "instance void(string)"},
		{"generic method", []byte{0x30, 0x02, 0x01, 0x01, 0x1e, 0x00}, "instance void<2>(!!0)"},
		{"class and valuetype", []byte{0x00, 0x02, 0x01, 0x12, ref(1), 0x11, def(1)}, "void(System.Object, My.Ns.Foo)"},
		{"pointer and byref", []byte{0x00, 0x02, 0x01, 0x0f, 0x08, 0x10, 0x08}, "void(int*, ref int)"},
		{"szarray", []byte{0x00, 0x01, 0x01, 0x1d, 0x08}, "void(int[])"},
		{"array rank 2", []byte{0x00, 0x01, 0x01, 0x14, 0x08, 0x02, 0x00, 0x00}, "void(int[,])"},
		{"array with bounds", []byte{0x00, 0x01, 0x01, 0x14, 0x08, 0x01, 0x01, 0x05, 0x01, 0x00}, "void(int[])"},
		{"bad rank", []byte{0x00, 0x01, 0x01, 0x14, 0x08, 0x00, 0x00, 0x00}, "void(int[?])"},
		{"generic instance", []byte{0x00, 0x01, 0x01, 0x15, 0x12, ref(2), 0x02, 0x08, 0x0e}, "void(System.Collections.Generic.List`1<int, string>)"},
		{"type variables", []byte{0x00, 0x02, 0x01, 0x13, 0x01, 0x1e, 0x00}, "void(!1, !!0)"},
		{"modifiers", []byte{0x00, 0x01, 0x01, 0x1f, ref(1), 0x20, ref(1), 0x08}, "void(int)"},
		{"pinned", []byte{0x00, 0x01, 0x01, 0x45, 0x0f, 0x08}, "void(int*)"},
		{"sentinel", []byte{0x05, 0x02, 0x01, 0x08, 0x41, 0x0e}, "void(int, ..., string)"},
		{"function pointer", []byte{0x00, 0x01, 0x01, 0x1b, 0x00, 0x01, 0x01, 0x08}, "void(method*(void(int)))"},
	}
	for _, c := range cases {
		idx := blobIndexOf(tab, c.sig)
		if got := tab.methodSignature(idx, ""); got != c.want {
			t.Errorf("%s: methodSignature = %q, want %q", c.name, got, c.want)
		}
	}
	if got := tab.methodSignature(0, "x"); got != "" {
		t.Errorf("empty blob = %q", got)
	}
}

// blobIndexOf appends a blob to the tables' heap and returns its index.
func blobIndexOf(t *tables, blob []byte) uint32 {
	h := blobHeap{data: t.blob}
	idx := h.add(blob)
	t.blob = h.data
	return idx
}

func TestSignatureLimits(t *testing.T) {
	f := newILFixture()
	tab := f.t
	// Truncated: a parameter is announced and missing.
	if got := tab.methodSignature(blobIndexOf(tab, []byte{0x00, 0x02, 0x01, 0x08}), ""); got != "" {
		t.Errorf("truncated signature = %q", got)
	}
	// Unknown element type.
	if got := tab.methodSignature(blobIndexOf(tab, []byte{0x00, 0x01, 0x01, 0x77}), ""); got != "" {
		t.Errorf("unknown element type = %q", got)
	}
	// Absurd parameter count.
	if got := tab.methodSignature(blobIndexOf(tab, []byte{0x00, 0xC0, 0x00, 0x10, 0x00, 0x01}), ""); got != "" {
		t.Errorf("huge parameter count = %q", got)
	}
	// Nesting deeper than the limit.
	deep := []byte{0x00, 0x01, 0x01}
	for i := 0; i < maxSigDepth+2; i++ {
		deep = append(deep, 0x1d)
	}
	deep = append(deep, 0x08)
	if got := tab.methodSignature(blobIndexOf(tab, deep), ""); got != "" {
		t.Errorf("too deep = %q", got)
	}
	// Absurd generic argument count.
	if got := tab.methodSignature(blobIndexOf(tab, []byte{0x00, 0x01, 0x01, 0x15, 0x12, 0x05, 0x7F}), ""); got != "" {
		t.Errorf("huge generic count = %q", got)
	}
}

func TestFieldSignature(t *testing.T) {
	f := newILFixture()
	if got := f.t.fieldSignature(f.t.cell(0x04, 1, 2)); got != "string" {
		t.Errorf("fieldSignature = %q", got)
	}
	for _, sig := range [][]byte{{0x07, 0x08}, {0x06}, {0x06, 0x77}} {
		if got := f.t.fieldSignature(blobIndexOf(f.t, sig)); got != "" {
			t.Errorf("fieldSignature(%x) = %q, want empty", sig, got)
		}
	}
}

func TestTokenText(t *testing.T) {
	f := newILFixture()
	tab := f.t
	for _, c := range []struct {
		kind  operand
		token uint32
		want  string
	}{
		{opString, 0x70000000 | f.hello, `"hello"`},
		{opType, 0x02000001, "My.Ns.Foo"},
		{opType, 0x01000001, "System.Object"},
		{opType, 0x1B000001, "System.Collections.Generic.List`1<int>"},
		{opType, 0x1B000009, "<type spec>"},
		{opMethod, 0x06000002, "My.Ns.Foo::instance int Run(string, int)"},
		{opMethod, 0x06000009, "<bad method token 0x06000009>"},
		{opField, 0x04000001, "My.Ns.Foo::Count"},
		{opField, 0x04000009, "<bad field token 0x04000009>"},
		{opMethod, 0x0A000001, "System.Object::.ctor"},
		{opMethod, 0x0A000002, "My.Ns.Foo::Run"},
		{opMethod, 0x0A000003, "System.Collections.Generic.List`1<int>::Run"},
		{opMethod, 0x0A000004, "My.Ns.Foo::Run"},
		{opMethod, 0x0A000009, "<bad member token 0x0a000009>"},
		{opMethod, f.methodSpec, "My.Ns.Foo::Run<string>"},
		{opMethod, 0x2B000002, "System.Object::.ctor<string>"},
		{opMethod, 0x2B000009, "<method spec>"},
		{opMethod, 0x2B000003, "<method><string>"},
		{opMethod, 0x2B000004, "My.Ns.Foo::Run"},
		{opMethod, 0x2B000005, "My.Ns.Foo::Run"},
		{opMethod, 0x2B000006, "My.Ns.Foo::Run"},
		{opSig, 0x11000001, "<signature 0x11000001>"},
		{opToken, 0x7F000001, "<token 0x7f000001>"},
	} {
		if got := tab.tokenText(c.kind, c.token); got != c.want {
			t.Errorf("tokenText(%#x) = %q, want %q", c.token, got, c.want)
		}
	}
	if got := quoteUserString(tab.userString(f.long)); !strings.HasSuffix(got, `..."`) || len(got) > 210 {
		t.Errorf("long string quoted as %q", got)
	}
	if tab.userString(1<<20) != "" || tab.userString(0) != "" {
		t.Error("userString out of range must be empty")
	}
}

func TestOwners(t *testing.T) {
	f := newILFixture()
	tab := f.t
	// Foo lists methods 1-2 and field 1; Bar starts at method 3 and field 2.
	if tab.methodOwner(1) != 1 || tab.methodOwner(2) != 1 || tab.methodOwner(3) != 2 || tab.fieldOwner(1) != 1 || tab.fieldOwner(2) != 2 {
		t.Errorf("owners = %d %d %d / %d %d", tab.methodOwner(1), tab.methodOwner(2), tab.methodOwner(3), tab.fieldOwner(1), tab.fieldOwner(2))
	}
}

func TestLocalsText(t *testing.T) {
	f := newILFixture()
	if got := f.t.localsText(f.localsSig); got != "int V_0, string V_1" {
		t.Errorf("localsText = %q", got)
	}
	// Not a signature token, a row past the table, a blob that is not a local
	// signature, and one that announces more locals than it holds.
	for _, token := range []uint32{0, 0x06000001, 0x11000009, 0x11000002, 0x11000003} {
		if got := f.t.localsText(token); got != "" {
			t.Errorf("localsText(%#x) = %q, want empty", token, got)
		}
	}
}

// body helpers ----------------------------------------------------------------

func tinyBody(code ...byte) []byte {
	return append([]byte{byte(len(code)<<2 | 2)}, code...) //nolint:gosec // test data
}

func fatBody(maxStack int, localSig uint32, code ...byte) []byte {
	head := make([]byte, 12)
	binary.LittleEndian.PutUint16(head, 3|3<<12)
	binary.LittleEndian.PutUint16(head[2:], uint16(maxStack))  //nolint:gosec // test data
	binary.LittleEndian.PutUint32(head[4:], uint32(len(code))) //nolint:gosec // test data
	binary.LittleEndian.PutUint32(head[8:], localSig)
	return append(head, code...)
}

func TestDisassemble(t *testing.T) {
	f := newILFixture()
	tab := f.t
	hello := byte(f.hello) //nolint:gosec // test data
	code := []byte{
		0x00,       // IL_0000 nop
		0x1f, 0xF6, // ldc.i4.s -10
		0x20, 0x10, 0x27, 0, 0, // ldc.i4 10000
		0x21, 1, 0, 0, 0, 0, 0, 0, 0, // ldc.i8 1
		0x22, 0, 0, 0x80, 0x3f, // ldc.r4 1
		0x23, 0, 0, 0, 0, 0, 0, 0xf8, 0x3f, // ldc.r8 1.5
		0x72, hello, 0, 0, 0x70, // ldstr "hello"
		0x28, 1, 0, 0, 0x0A, // call System.Object::.ctor
		0x2b, 0x02, // br.s +2
		0x2c, 0xFE, // brfalse.s -2
		0x38, 0x00, 0x00, 0x00, 0x00, // br +0
		0x45, 0x02, 0, 0, 0, 0x00, 0, 0, 0, 0x05, 0, 0, 0, // switch (2 targets)
		0x0e, 0x03, // ldarg.s 3
		0xFE, 0x09, 0x0a, 0x00, // ldarg 10
		0xFE, 0x01, // ceq
		0xFE, 0x12, 0x04, // unaligned. 4
		0xFE, 0x16, 0x01, 0x00, 0x00, 0x01, // constrained. System.Object
		0xFE, 0x06, 0x02, 0x00, 0x00, 0x06, // ldftn Run
		0x7b, 0x01, 0x00, 0x00, 0x04, // ldfld Count
		0x8c, 0x01, 0x00, 0x00, 0x02, // box My.Ns.Foo
		0xd0, 0x01, 0x00, 0x00, 0x1B, // ldtoken List<int>
		0x6f, 0x01, 0x00, 0x00, 0x2B, // callvirt Run<string>
		0x29, 0x01, 0x00, 0x00, 0x11, // calli
		0x2a, // ret
	}
	got := tab.disassemble(fatBody(4, f.localsSig, code...))
	for _, want := range []string{
		".maxstack 4", ".locals (int V_0, string V_1)",
		"IL_0000: nop", "ldc.i4.s -10", "ldc.i4 10000", "ldc.i8 1", "ldc.r4 1", "ldc.r8 1.5",
		`ldstr "hello"`, "call System.Object::.ctor",
		": br.s IL_", ": brfalse.s IL_", ": switch (IL_", "ldarg.s 3", "ldarg 10", ": ceq",
		"unaligned. 4", "constrained. System.Object", "ldftn My.Ns.Foo::instance int Run(string, int)",
		"ldfld My.Ns.Foo::Count", "box My.Ns.Foo", "ldtoken System.Collections.Generic.List`1<int>",
		"callvirt My.Ns.Foo::Run<string>", "calli <signature 0x11000001>", ": ret",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("listing lacks %q:\n%s", want, got)
		}
	}
	// The br.s at IL_0029 ends at 0x2b, so +2 targets 0x2d.
	if !strings.Contains(got, "br.s IL_002d") {
		t.Errorf("short branch target is wrong:\n%s", got)
	}
}

func TestDisassembleStopsOnBrokenCode(t *testing.T) {
	tab := newILFixture().t
	for name, tc := range map[string]struct {
		body []byte
		want string
	}{
		"unknown opcode":    {tinyBody(0x24), "unknown opcode 0x24"},
		"unknown prefixed":  {tinyBody(0xFE, 0x08), "unknown opcode 0xfe"},
		"lone prefix":       {tinyBody(0xFE), "truncated prefix"},
		"truncated operand": {tinyBody(0x20, 1, 2), "truncated operand"},
		"switch overrun":    {tinyBody(0x45, 0xFF, 0xFF, 0xFF, 0x7F), "truncated operand"},
		"empty":             {nil, "not readable"},
		"bad header":        {[]byte{0x00}, "not readable"},
		"short tiny":        {[]byte{0x0E, 0x00}, "not readable"},
		"short fat":         {[]byte{0x03, 0x30, 0, 0}, "not readable"},
	} {
		if got := tab.disassemble(tc.body); !strings.Contains(got, tc.want) {
			t.Errorf("%s: %q lacks %q", name, got, tc.want)
		}
	}
	// A run of nops longer than the limit stops with a note.
	long := fatBody(1, 0, bytes.Repeat([]byte{0}, maxInstructions+5)...)
	if got := tab.disassemble(long); !strings.Contains(got, "listing stops after") {
		t.Error("an over-long body must stop with a note")
	}
}

func TestOperandTextEdges(t *testing.T) {
	tab := newILFixture().t
	for _, kind := range []operand{opInt8, opUint8, opVar8, opVar16, opInt32, opInt64, opFloat32, opFloat64, opTarget8, opTarget32, opSwitch, opMethod} {
		if _, _, ok := tab.operandText(kind, nil, 0); ok {
			t.Errorf("operand kind %d accepted no data", kind)
		}
	}
	if text, used, ok := tab.operandText(opNone, nil, 0); text != "" || used != 0 || !ok {
		t.Error("opNone must accept anything")
	}
}

func TestSplitBody(t *testing.T) {
	code, stack, local, ok := splitBody(fatBody(7, 0x11000001, 1, 2, 3))
	if !ok || len(code) != 3 || stack != 7 || local != 0x11000001 {
		t.Errorf("fat = %v %d %#x %v", code, stack, local, ok)
	}
	if code, stack, local, ok := splitBody(tinyBody(1, 2)); !ok || len(code) != 2 || stack != 8 || local != 0 {
		t.Errorf("tiny = %v %d %#x %v", code, stack, local, ok)
	}
	// A fat header claiming more code than there is.
	bad := fatBody(1, 0, 1, 2)
	binary.LittleEndian.PutUint32(bad[4:], 99)
	if _, _, _, ok := splitBody(bad); ok {
		t.Error("oversized code length accepted")
	}
	// A fat header shorter than 12 bytes.
	short := fatBody(1, 0)
	binary.LittleEndian.PutUint16(short, 3|2<<12)
	if _, _, _, ok := splitBody(short); ok {
		t.Error("short fat header accepted")
	}
}

func TestReadBodiesAndMethodIL(t *testing.T) {
	f := newILFixture()
	tinyRVA, fatRVA, badRVA, bigRVA := uint32(0x1000), uint32(0x2000), uint32(0x3000), uint32(0x4000)
	bodies := map[uint32][]byte{
		tinyRVA: tinyBody(0x2a),
		fatRVA:  fatBody(2, 0, 0x00, 0x2a),
		badRVA:  {0x01}, // neither tiny nor fat
		bigRVA:  fatBody(8, 0, bytes.Repeat([]byte{0}, 4)...),
	}
	binary.LittleEndian.PutUint32(bodies[bigRVA][4:], maxBodySize+1) // claims too much
	rvaRead := func(rva, n uint32) ([]byte, error) {
		body, ok := bodies[rva]
		if !ok {
			return nil, errors.New("no such rva")
		}
		if int(n) > len(body) {
			if n == 12 {
				return nil, errors.New("short read") // the tiny-header fallback
			}
			return nil, errors.New("past the end")
		}
		return body[:n], nil
	}
	info := &Info{tab: f.t, Members: map[string][]Member{"T": {
		{Kind: "field", Name: "f"},
		{Kind: "method", Name: "tiny", rva: tinyRVA},
		{Kind: "method", Name: "fat", rva: fatRVA},
		{Kind: "method", Name: "bad", rva: badRVA},
		{Kind: "method", Name: "big", rva: bigRVA},
		{Kind: "method", Name: "missing", rva: 0x9000},
		{Kind: "method", Name: "abstract"},
	}}}
	info.readBodies(rvaRead)
	list := info.Members["T"]
	if list[1].body == nil || list[2].body == nil {
		t.Fatal("tiny and fat bodies must be kept")
	}
	if list[3].body != nil || list[4].body != nil || list[5].body != nil || list[6].body != nil || list[0].body != nil {
		t.Error("unreadable, oversized, missing and bodiless members must have no body")
	}
	if il := info.MethodIL("T", 1); !strings.Contains(il, "ret") {
		t.Errorf("MethodIL(tiny) = %q", il)
	}
	if il := info.MethodIL("T", 2); !strings.Contains(il, ".maxstack 2") {
		t.Errorf("MethodIL(fat) = %q", il)
	}
	for _, index := range []int{-1, 0, 3, 99} {
		if il := info.MethodIL("T", index); il != "" {
			t.Errorf("MethodIL(%d) = %q, want empty", index, il)
		}
	}
	if (&Info{}).MethodIL("T", 0) != "" {
		t.Error("MethodIL without tables must be empty")
	}
}

func TestReadBodiesTotalLimit(t *testing.T) {
	f := newILFixture()
	body := tinyBody(0x2a)
	info := &Info{tab: f.t, Members: map[string][]Member{"T": {{Kind: "method", Name: "m", rva: 1}}}}
	// The read hands out a body but the running total is already spent.
	info.readBodiesFrom(func(rva, n uint32) ([]byte, error) { return body[:n], nil }, maxBodyTotal)
	if info.Members["T"][0].body != nil {
		t.Error("no body may be kept once the total limit is spent")
	}
}

func TestReadyToRunImages(t *testing.T) {
	plain := wrapPE(sampleMetadata(), true)
	const machineAt = 0x40 + 4
	for _, key := range readyToRunOSKeys {
		image := append([]byte(nil), plain...)
		binary.LittleEndian.PutUint16(image[machineAt:], 0x14c^key)
		info, err := Read(bytes.NewReader(image), int64(len(image)))
		if err != nil || info.Name != "Sample" {
			t.Errorf("key %#x: %v %+v", key, err, info)
		}
	}
	// A machine type that is unknown even after every key stays rejected.
	image := append([]byte(nil), plain...)
	binary.LittleEndian.PutUint16(image[machineAt:], 0x1234)
	if _, err := Read(bytes.NewReader(image), int64(len(image))); !errors.Is(err, ErrNotAssembly) {
		t.Errorf("unknown machine: err = %v", err)
	}
	// Broken headers: a pointer past the end, a huge pointer, too short a file.
	for name, mutate := range map[string]func([]byte){
		"far pointer":  func(b []byte) { binary.LittleEndian.PutUint32(b[0x3c:], 1<<28) },
		"past the end": func(b []byte) { binary.LittleEndian.PutUint32(b[0x3c:], uint32(len(b))) }, //nolint:gosec // test data
		"no pointer":   func(b []byte) { binary.LittleEndian.PutUint32(b[0x3c:], 0) },
	} {
		image := append([]byte(nil), plain...)
		mutate(image)
		if _, err := Read(bytes.NewReader(image), int64(len(image))); !errors.Is(err, ErrNotAssembly) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := Read(bytes.NewReader([]byte("MZ")), 2); !errors.Is(err, ErrNotAssembly) {
		t.Errorf("tiny file: err = %v", err)
	}
}

func TestMachinePatchReadsAcrossTheField(t *testing.T) {
	src := []byte{0, 1, 2, 3, 4, 5, 6, 7}
	patch := &machinePatch{r: bytes.NewReader(src), at: 3, plain: 0xAABB}
	for _, tc := range []struct {
		off, n int
		want   []byte
	}{
		{0, 8, []byte{0, 1, 2, 0xBB, 0xAA, 5, 6, 7}},
		{4, 2, []byte{0xAA, 5}}, // starts inside the field
		{0, 3, []byte{0, 1, 2}}, // ends before it
		{6, 2, []byte{6, 7}},    // starts after it
	} {
		buf := make([]byte, tc.n)
		n, _ := patch.ReadAt(buf, int64(tc.off))
		if n != tc.n || !bytes.Equal(buf, tc.want) {
			t.Errorf("ReadAt(%d, %d) = %v, want %v", tc.off, tc.n, buf, tc.want)
		}
	}
}
