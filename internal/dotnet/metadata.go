// Package dotnet reads the metadata of a .NET assembly (ECMA-335, partition
// II) without running or loading any of its code: the assembly's identity, the
// assemblies it references, its types by namespace and its embedded resource
// names (f4#1666). It is pure Go on top of debug/pe, needs no external tool
// and treats the file as untrusted: every offset and count is checked against
// the size of the data it points into, and the amount read is capped.
package dotnet

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// ErrNotAssembly reports a file that is a valid container but holds no .NET
// metadata (a native PE file, or not a PE file at all).
var ErrNotAssembly = errors.New("not a .NET assembly")

// Limits on what is read from an untrusted file.
const (
	maxMetadataSize = 64 << 20
	maxRows         = 1 << 22
)

// Version is a four-part assembly version.
type Version struct{ Major, Minor, Build, Revision int }

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d.%d", v.Major, v.Minor, v.Build, v.Revision)
}

// Ref is one referenced assembly.
type Ref struct {
	Name    string
	Version Version
}

// Info is what Read learns about an assembly.
type Info struct {
	// RuntimeVersion is the metadata version string, such as "v4.0.30319".
	RuntimeVersion string
	// Name, Version and Culture identify the assembly; Name is empty for a
	// netmodule, which has no manifest.
	Name    string
	Version Version
	Culture string
	// References are the assemblies this one depends on, in file order.
	References []Ref
	// Types maps a namespace ("" for the global one) to the sorted names of
	// its top-level types; nested types and the <Module> pseudo-type are left
	// out.
	Types map[string][]string
	// Nested maps a namespace to the sorted names of the nested types of the
	// types in it, written as the runtime writes them: "Outer+Inner" (and
	// "Outer+Inner+Innermost"). A nested type is in the namespace of the
	// outermost type. Their members are in Members under "namespace.Outer+Inner".
	Nested map[string][]string
	// TypeCount is the number of top-level types listed in Types.
	TypeCount int
	// Resources are the names of the managed resources.
	Resources []string
	// Members maps "namespace.Type" (just "Type" in the global namespace) to
	// the fields and methods declared by that top-level type, in file order.
	Members map[string][]Member
	// Attributes maps the same keys as Members to the custom attributes of the
	// type itself.
	Attributes map[string][]string
	// Generics maps the same keys to the generic parameters of the type, in
	// declaration order, with their constraints.
	Generics map[string][]GenericParam
	// Blobs are the bytes of the resources embedded in the file, by name; a
	// resource kept in another file or assembly has no entry, and neither has
	// one that is implausibly large or lies outside the resource area.
	Blobs []Blob

	resRefs []resourceRef
	tab     *tables
}

// Member is a field or a method of a type.
type Member struct {
	Kind string // "field" or "method"
	Name string
	// Signature is the C#-style signature: the type of a field, or
	// "ret name(params)" of a method; empty when the blob cannot be read.
	Signature string
	// Attributes are the custom attributes applied to it, by the name of the
	// attribute type without the "Attribute" suffix ("System.Obsolete").
	Attributes []string
	// Generics are the generic parameters of a method, with their constraints.
	Generics []GenericParam

	rva  uint32 // method body RVA, 0 for a field or a method with no body
	body []byte // header and code of the method, when it could be read
}

// GenericParam is a generic parameter of a type: "out T : class, IFoo, new()".
type GenericParam struct {
	Name string
	// Variance is "in", "out" or "".
	Variance string
	// Class, Struct and New are the special constraints (class, struct, new()).
	Class, Struct, New bool
	// Constraints are the types the parameter must derive from or implement.
	Constraints []string
}

// Blob is one embedded resource.
type Blob struct {
	Name string
	Data []byte
}

type resourceRef struct {
	name     string
	offset   uint32
	embedded bool
}

// Limits on the resource bytes taken out of one file.
const (
	maxMembers   = 20000
	maxBlobSize  = 64 << 20
	maxBlobTotal = 128 << 20
)

// Namespaces returns the namespaces of Types in sorted order.
func (in *Info) Namespaces() []string {
	names := make([]string, 0, len(in.Types))
	for ns := range in.Types {
		names = append(names, ns)
	}
	sort.Strings(names)
	return names
}

type colKind int

const (
	kU2 colKind = iota
	kU4
	kStr
	kGUID
	kBlob
	kIdx   // simple index into a table, arg is the table number
	kCoded // coded index, arg is a codedKind
)

type column struct {
	kind colKind
	arg  int
}

func c(kind colKind, arg int) column { return column{kind, arg} }

type codedKind int

const (
	cTypeDefOrRef codedKind = iota
	cHasConstant
	cHasCustomAttribute
	cHasFieldMarshal
	cHasDeclSecurity
	cMemberRefParent
	cHasSemantics
	cMethodDefOrRef
	cMemberForwarded
	cImplementation
	cCustomAttributeType
	cResolutionScope
	cTypeOrMethodDef
)

// codedTables lists, per coded index, the tables its tag selects; -1 marks an
// unused tag value. The tag width is the smallest number of bits that holds
// len(tables).
var codedTables = map[codedKind][]int{
	cTypeDefOrRef:        {0x02, 0x01, 0x1B},
	cHasConstant:         {0x04, 0x08, 0x17},
	cHasCustomAttribute:  {0x06, 0x04, 0x01, 0x02, 0x08, 0x09, 0x0A, 0x00, 0x0E, 0x17, 0x14, 0x11, 0x1A, 0x1B, 0x20, 0x23, 0x26, 0x27, 0x28, 0x2A, 0x2C, 0x2B},
	cHasFieldMarshal:     {0x04, 0x08},
	cHasDeclSecurity:     {0x02, 0x06, 0x20},
	cMemberRefParent:     {0x02, 0x01, 0x1A, 0x06, 0x1B},
	cHasSemantics:        {0x14, 0x17},
	cMethodDefOrRef:      {0x06, 0x0A},
	cMemberForwarded:     {0x04, 0x06},
	cImplementation:      {0x26, 0x23, 0x27},
	cCustomAttributeType: {-1, -1, 0x06, 0x0A, -1},
	cResolutionScope:     {0x00, 0x1A, 0x23, 0x01},
	cTypeOrMethodDef:     {0x02, 0x06},
}

func tagBits(n int) int {
	bits := 0
	for (1 << bits) < n {
		bits++
	}
	return bits
}

// tableSchema is the column layout of every metadata table, by table number.
var tableSchema = map[int][]column{
	0x00: {c(kU2, 0), c(kStr, 0), c(kGUID, 0), c(kGUID, 0), c(kGUID, 0)},
	0x01: {c(kCoded, int(cResolutionScope)), c(kStr, 0), c(kStr, 0)},
	0x02: {c(kU4, 0), c(kStr, 0), c(kStr, 0), c(kCoded, int(cTypeDefOrRef)), c(kIdx, 0x04), c(kIdx, 0x06)},
	0x03: {c(kIdx, 0x04)},
	0x04: {c(kU2, 0), c(kStr, 0), c(kBlob, 0)},
	0x05: {c(kIdx, 0x06)},
	0x06: {c(kU4, 0), c(kU2, 0), c(kU2, 0), c(kStr, 0), c(kBlob, 0), c(kIdx, 0x08)},
	0x07: {c(kIdx, 0x08)},
	0x08: {c(kU2, 0), c(kU2, 0), c(kStr, 0)},
	0x09: {c(kIdx, 0x02), c(kCoded, int(cTypeDefOrRef))},
	0x0A: {c(kCoded, int(cMemberRefParent)), c(kStr, 0), c(kBlob, 0)},
	0x0B: {c(kU2, 0), c(kCoded, int(cHasConstant)), c(kBlob, 0)},
	0x0C: {c(kCoded, int(cHasCustomAttribute)), c(kCoded, int(cCustomAttributeType)), c(kBlob, 0)},
	0x0D: {c(kCoded, int(cHasFieldMarshal)), c(kBlob, 0)},
	0x0E: {c(kU2, 0), c(kCoded, int(cHasDeclSecurity)), c(kBlob, 0)},
	0x0F: {c(kU2, 0), c(kU4, 0), c(kIdx, 0x02)},
	0x10: {c(kU4, 0), c(kIdx, 0x04)},
	0x11: {c(kBlob, 0)},
	0x12: {c(kIdx, 0x02), c(kIdx, 0x14)},
	0x13: {c(kIdx, 0x14)},
	0x14: {c(kU2, 0), c(kStr, 0), c(kCoded, int(cTypeDefOrRef))},
	0x15: {c(kIdx, 0x02), c(kIdx, 0x17)},
	0x16: {c(kIdx, 0x17)},
	0x17: {c(kU2, 0), c(kStr, 0), c(kBlob, 0)},
	0x18: {c(kU2, 0), c(kIdx, 0x06), c(kCoded, int(cHasSemantics))},
	0x19: {c(kIdx, 0x02), c(kCoded, int(cMethodDefOrRef)), c(kCoded, int(cMethodDefOrRef))},
	0x1A: {c(kStr, 0)},
	0x1B: {c(kBlob, 0)},
	0x1C: {c(kU2, 0), c(kCoded, int(cMemberForwarded)), c(kStr, 0), c(kIdx, 0x1A)},
	0x1D: {c(kU4, 0), c(kIdx, 0x04)},
	0x1E: {c(kU4, 0), c(kU4, 0)},
	0x1F: {c(kU4, 0)},
	0x20: {c(kU4, 0), c(kU2, 0), c(kU2, 0), c(kU2, 0), c(kU2, 0), c(kU4, 0), c(kBlob, 0), c(kStr, 0), c(kStr, 0)},
	0x21: {c(kU4, 0)},
	0x22: {c(kU4, 0), c(kU4, 0), c(kU4, 0)},
	0x23: {c(kU2, 0), c(kU2, 0), c(kU2, 0), c(kU2, 0), c(kU4, 0), c(kBlob, 0), c(kStr, 0), c(kStr, 0), c(kBlob, 0)},
	0x24: {c(kU4, 0), c(kIdx, 0x23)},
	0x25: {c(kU4, 0), c(kU4, 0), c(kU4, 0), c(kIdx, 0x23)},
	0x26: {c(kU4, 0), c(kStr, 0), c(kBlob, 0)},
	0x27: {c(kU4, 0), c(kU4, 0), c(kStr, 0), c(kStr, 0), c(kCoded, int(cImplementation))},
	0x28: {c(kU4, 0), c(kU4, 0), c(kStr, 0), c(kCoded, int(cImplementation))},
	0x29: {c(kIdx, 0x02), c(kIdx, 0x02)},
	0x2A: {c(kU2, 0), c(kU2, 0), c(kCoded, int(cTypeOrMethodDef)), c(kStr, 0)},
	0x2B: {c(kCoded, int(cMethodDefOrRef)), c(kBlob, 0)},
	0x2C: {c(kIdx, 0x2A), c(kCoded, int(cTypeDefOrRef))},
}

// tables holds the located rows of one metadata image.
type tables struct {
	strs      []byte
	blob      []byte // the #Blob heap
	us        []byte // the #US heap
	data      []byte // the whole #~ stream
	rowCount  [64]uint32
	offset    [64]int // start of each table's rows inside data
	rowSize   [64]int
	strWide   bool
	guidWide  bool
	blobWide  bool
	colWidths map[int][]int
	// attrs holds the custom attributes by owner, see attrKey.
	attrs map[uint64][]string
	// generics holds the generic parameters of each TypeDef row, and
	// methodGenerics those of each MethodDef row.
	generics       map[uint32][]GenericParam
	methodGenerics map[uint32][]GenericParam
}

func (t *tables) codedWidth(kind codedKind) int {
	list := codedTables[kind]
	limit := uint32(1) << (16 - tagBits(len(list)))
	for _, table := range list {
		if table >= 0 && t.rowCount[table] >= limit {
			return 4
		}
	}
	return 2
}

func (t *tables) width(col column) int {
	switch col.kind {
	case kU2:
		return 2
	case kU4:
		return 4
	case kStr:
		if t.strWide {
			return 4
		}
		return 2
	case kGUID:
		if t.guidWide {
			return 4
		}
		return 2
	case kBlob:
		if t.blobWide {
			return 4
		}
		return 2
	case kIdx:
		if t.rowCount[col.arg] >= 1<<16 {
			return 4
		}
		return 2
	default:
		return t.codedWidth(codedKind(col.arg))
	}
}

// cell reads column col of row (1-based) in table.
func (t *tables) cell(table int, row uint32, col int) uint32 {
	widths := t.colWidths[table]
	off := t.offset[table] + int(row-1)*t.rowSize[table]
	for i := 0; i < col; i++ {
		off += widths[i]
	}
	if widths[col] == 2 {
		return uint32(binary.LittleEndian.Uint16(t.data[off:]))
	}
	return binary.LittleEndian.Uint32(t.data[off:])
}

// str resolves a #Strings index; a bad index yields the empty string.
func (t *tables) str(index uint32) string {
	if int(index) >= len(t.strs) {
		return ""
	}
	end := bytes.IndexByte(t.strs[index:], 0)
	if end < 0 {
		return ""
	}
	return string(t.strs[index : int(index)+end])
}

func parseTables(stream, strs []byte) (*tables, error) {
	if len(stream) < 24 {
		return nil, errors.New("metadata table stream is truncated")
	}
	heapSizes := stream[6]
	valid := binary.LittleEndian.Uint64(stream[8:])
	t := &tables{
		strs:      strs,
		data:      stream,
		strWide:   heapSizes&0x01 != 0,
		guidWide:  heapSizes&0x02 != 0,
		blobWide:  heapSizes&0x04 != 0,
		colWidths: make(map[int][]int),
	}
	pos := 24
	for table := 0; table < 64; table++ {
		if valid&(1<<uint(table)) == 0 {
			continue
		}
		if pos+4 > len(stream) {
			return nil, errors.New("metadata row counts are truncated")
		}
		n := binary.LittleEndian.Uint32(stream[pos:])
		if n > maxRows {
			return nil, errors.New("metadata table is implausibly large")
		}
		t.rowCount[table] = n
		pos += 4
	}
	for table := 0; table < 64; table++ {
		if t.rowCount[table] == 0 {
			continue
		}
		schema, ok := tableSchema[table]
		if !ok {
			return nil, fmt.Errorf("unknown metadata table 0x%02x", table)
		}
		widths := make([]int, len(schema))
		size := 0
		for i, col := range schema {
			widths[i] = t.width(col)
			size += widths[i]
		}
		t.colWidths[table] = widths
		t.rowSize[table] = size
		t.offset[table] = pos
		total := int64(size) * int64(t.rowCount[table])
		if int64(pos)+total > int64(len(stream)) {
			return nil, errors.New("metadata table runs past the end of its stream")
		}
		pos += int(total)
	}
	return t, nil
}

// Read parses the .NET metadata of a PE file. size is the length of r.
func Read(r io.ReaderAt, size int64) (*Info, error) {
	file, err := openPE(r)
	if err != nil {
		return nil, ErrNotAssembly
	}
	var dir pe.DataDirectory
	switch oh := file.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		if oh.NumberOfRvaAndSizes > 14 {
			dir = oh.DataDirectory[14]
		}
	case *pe.OptionalHeader64:
		if oh.NumberOfRvaAndSizes > 14 {
			dir = oh.DataDirectory[14]
		}
	}
	if dir.VirtualAddress == 0 || dir.Size < 16 {
		return nil, ErrNotAssembly
	}
	rvaRead := func(rva, n uint32) ([]byte, error) {
		if n > maxMetadataSize {
			return nil, errors.New("metadata is implausibly large")
		}
		for _, s := range file.Sections {
			if rva >= s.VirtualAddress && rva-s.VirtualAddress < s.Size {
				fileOff := int64(s.Offset) + int64(rva-s.VirtualAddress)
				if fileOff+int64(n) > size {
					return nil, errors.New("metadata runs past the end of the file")
				}
				buf := make([]byte, n)
				if _, err := r.ReadAt(buf, fileOff); err != nil {
					return nil, err
				}
				return buf, nil
			}
		}
		return nil, errors.New("metadata is outside every section")
	}

	corLen := uint32(16)
	if dir.Size >= 32 {
		corLen = 32
	}
	cor, err := rvaRead(dir.VirtualAddress, corLen)
	if err != nil {
		return nil, err
	}
	mdRVA := binary.LittleEndian.Uint32(cor[8:])
	mdSize := binary.LittleEndian.Uint32(cor[12:])
	if mdRVA == 0 || mdSize == 0 {
		return nil, ErrNotAssembly
	}
	root, err := rvaRead(mdRVA, mdSize)
	if err != nil {
		return nil, err
	}
	info, err := parseMetadata(root)
	if err != nil {
		return nil, err
	}
	info.readBodies(rvaRead)
	if corLen == 32 && len(info.resRefs) > 0 {
		resRVA := binary.LittleEndian.Uint32(cor[24:])
		resSize := binary.LittleEndian.Uint32(cor[28:])
		if resRVA != 0 && resSize != 0 && resSize <= maxBlobTotal {
			// The bytes are a bonus: a file whose resource area cannot be read
			// still yields its identity, references and types.
			if area, err := rvaRead(resRVA, resSize); err == nil {
				info.Blobs = blobsOf(info.resRefs, area)
			}
		}
	}
	return info, nil
}

// blobsOf cuts the embedded resources out of the resource area, where each
// one is a 4-byte length followed by its bytes.
func blobsOf(refs []resourceRef, area []byte) []Blob {
	var out []Blob
	total := 0
	for _, ref := range refs {
		if !ref.embedded || int64(ref.offset)+4 > int64(len(area)) {
			continue
		}
		n := binary.LittleEndian.Uint32(area[ref.offset:])
		if n > maxBlobSize || int64(ref.offset)+4+int64(n) > int64(len(area)) {
			continue
		}
		total += int(n)
		if total > maxBlobTotal {
			break
		}
		out = append(out, Blob{Name: ref.name, Data: area[ref.offset+4 : ref.offset+4+n]})
	}
	return out
}

// parseMetadata reads a metadata root: the version string and the stream
// directory, then the tables.
func parseMetadata(root []byte) (*Info, error) {
	if len(root) < 20 || binary.LittleEndian.Uint32(root) != 0x424A5342 {
		return nil, ErrNotAssembly
	}
	verLen := int(binary.LittleEndian.Uint32(root[12:]))
	if verLen < 0 || 16+verLen+4 > len(root) {
		return nil, errors.New("metadata root is truncated")
	}
	info := &Info{RuntimeVersion: strings.TrimRight(string(root[16:16+verLen]), "\x00")}
	pos := 16 + verLen + 2 // flags
	streamCount := int(binary.LittleEndian.Uint16(root[pos:]))
	pos += 2
	streams := make(map[string][]byte)
	for i := 0; i < streamCount; i++ {
		if pos+8 > len(root) {
			return nil, errors.New("metadata stream directory is truncated")
		}
		off := int(binary.LittleEndian.Uint32(root[pos:]))
		length := int(binary.LittleEndian.Uint32(root[pos+4:]))
		pos += 8
		end := bytes.IndexByte(root[pos:], 0)
		if end < 0 {
			return nil, errors.New("metadata stream name is unterminated")
		}
		name := string(root[pos : pos+end])
		pos += (end + 1 + 3) &^ 3
		if off < 0 || length < 0 || off > len(root) || length > len(root)-off {
			return nil, errors.New("metadata stream runs past the end of the metadata")
		}
		streams[name] = root[off : off+length]
	}
	stream, ok := streams["#~"]
	if !ok {
		stream, ok = streams["#-"]
	}
	if !ok {
		return nil, errors.New("metadata has no table stream")
	}
	t, err := parseTables(stream, streams["#Strings"])
	if err != nil {
		return nil, err
	}
	t.blob, t.us = streams["#Blob"], streams["#US"]
	info.tab = t
	fill(info, t)
	return info, nil
}

// members lists the rows of a member table (fields or methods) that the
// TypeDef row owns: from its list column up to where the next TypeDef's list
// starts, or the end of the member table for the last one.
func (t *tables) members(typeRow uint32, table, nameCol, listCol int, kind string) []Member {
	start := t.cell(0x02, typeRow, listCol)
	end := t.rowCount[table] + 1
	if typeRow < t.rowCount[0x02] {
		end = t.cell(0x02, typeRow+1, listCol)
	}
	if end > t.rowCount[table]+1 {
		end = t.rowCount[table] + 1
	}
	var out []Member
	for r := start; r >= 1 && r < end && len(out) < maxMembers; r++ {
		m := Member{Kind: kind, Name: t.str(t.cell(table, r, nameCol)), Attributes: t.attrs[attrKey(table, r)]}
		if kind == "method" {
			m.Generics = t.methodGenerics[r]
		}
		if kind == "method" {
			m.rva = t.cell(table, r, 0)
			m.Signature = t.methodSignature(t.cell(table, r, 4), m.Name)
		} else {
			m.Signature = t.fieldSignature(t.cell(table, r, 2))
		}
		out = append(out, m)
	}
	return out
}

// maxNesting bounds the chain of enclosing types followed for one nested type,
// so that a file whose NestedClass rows loop cannot make the walk endless.
const maxNesting = 16

// nestedName gives the namespace and the "Outer+Inner" name of a nested
// TypeDef row, following the NestedClass rows out to the outermost type. It
// reports false for a row that has no name, whose chain loops or is too deep,
// or that points outside the TypeDef table.
func (t *tables) nestedName(row uint32, enclosing map[uint32]uint32) (ns, name string, ok bool) {
	parts := []string{t.str(t.cell(0x02, row, 1))}
	if parts[0] == "" {
		return "", "", false
	}
	cur := row
	for depth := 0; ; depth++ {
		outer := enclosing[cur]
		if outer == 0 {
			break // cur is the outermost type
		}
		if depth >= maxNesting || outer > t.rowCount[0x02] {
			return "", "", false
		}
		parts = append([]string{t.str(t.cell(0x02, outer, 1))}, parts...)
		cur = outer
	}
	return t.str(t.cell(0x02, cur, 2)), strings.Join(parts, "+"), true
}

// maxAttrsPerOwner bounds the attributes kept for one type, field or method.
const maxAttrsPerOwner = 16

func attrKey(table int, row uint32) uint64 {
	return uint64(uint32(table))<<32 | uint64(row) // #nosec G115 -- table is one of a few small table numbers
}

// collectAttributes reads the CustomAttribute table (II.22.10) and keeps, for
// every type, field and method, the names of the attribute types applied to
// it. The arguments in the value blob are not decoded.
func (t *tables) collectAttributes() {
	t.attrs = make(map[uint64][]string)
	for row := uint32(1); row <= t.rowCount[0x0C]; row++ {
		parent := t.cell(0x0C, row, 0)
		var owner uint64
		switch parent & 31 { // HasCustomAttribute: 5 tag bits
		case 0:
			owner = attrKey(0x06, parent>>5)
		case 1:
			owner = attrKey(0x04, parent>>5)
		case 3:
			owner = attrKey(0x02, parent>>5)
		default:
			continue
		}
		if len(t.attrs[owner]) >= maxAttrsPerOwner {
			continue
		}
		name := t.attributeName(t.cell(0x0C, row, 1))
		if name == "" {
			continue
		}
		if args := t.attributeArgs(t.cell(0x0C, row, 1), t.cell(0x0C, row, 2)); args != "" {
			name += "(" + args + ")"
		}
		t.attrs[owner] = append(t.attrs[owner], name)
	}
}

// attributeName names the attribute type a CustomAttributeType coded token
// (II.24.2.6: MethodDef or MemberRef constructor) belongs to, without the
// "Attribute" suffix.
func (t *tables) attributeName(coded uint32) string {
	row := coded >> 3
	var typeName string
	switch coded & 7 {
	case 2:
		if row == 0 || row > t.rowCount[0x06] {
			return ""
		}
		typeName = t.typeDefName(t.methodOwner(row))
	case 3:
		if row == 0 || row > t.rowCount[0x0A] {
			return ""
		}
		typeName = t.memberParentName(t.cell(0x0A, row, 0))
	default:
		return ""
	}
	return strings.TrimSuffix(typeName, "Attribute")
}

// maxGenericParams bounds the parameters and the constraints kept per type
// or method.
const maxGenericParams = 64

// collectGenerics reads the GenericParam (II.22.20) and GenericParamConstraint
// (II.22.21) tables for the generic parameters of types and methods.
func (t *tables) collectGenerics() {
	t.generics = make(map[uint32][]GenericParam)
	t.methodGenerics = make(map[uint32][]GenericParam)
	type slot struct {
		lists map[uint32][]GenericParam // where the parameter was stored
		row   uint32
		idx   int
	}
	owners := make(map[uint32]slot) // GenericParam row -> where it was stored
	for row := uint32(1); row <= t.rowCount[0x2A]; row++ {
		owner := t.cell(0x2A, row, 2) // TypeOrMethodDef: 1 tag bit, 0 TypeDef, 1 MethodDef
		lists := t.generics
		if owner&1 != 0 {
			lists = t.methodGenerics
		}
		if len(lists[owner>>1]) >= maxGenericParams {
			continue
		}
		flags := t.cell(0x2A, row, 1)
		p := GenericParam{
			Name:   t.str(t.cell(0x2A, row, 3)),
			Class:  flags&0x04 != 0,
			Struct: flags&0x08 != 0,
			New:    flags&0x10 != 0,
		}
		switch flags & 0x03 {
		case 1:
			p.Variance = "out"
		case 2:
			p.Variance = "in"
		}
		owners[row] = slot{lists: lists, row: owner >> 1, idx: len(lists[owner>>1])}
		lists[owner>>1] = append(lists[owner>>1], p)
	}
	for row := uint32(1); row <= t.rowCount[0x2C]; row++ {
		where, ok := owners[t.cell(0x2C, row, 0)]
		if !ok {
			continue
		}
		list := where.lists[where.row]
		if len(list[where.idx].Constraints) >= maxGenericParams {
			continue
		}
		coded := t.cell(0x2C, row, 1)
		name := t.typeName(coded)
		if coded&3 == 2 {
			name = t.typeSpecName(coded >> 2)
		}
		list[where.idx].Constraints = append(list[where.idx].Constraints, name)
	}
}

func fill(info *Info, t *tables) {
	t.collectAttributes()
	t.collectGenerics()
	info.Attributes = make(map[string][]string)
	info.Generics = make(map[string][]GenericParam)
	if t.rowCount[0x20] > 0 {
		info.Name = t.str(t.cell(0x20, 1, 7))
		info.Version = Version{
			Major:    int(t.cell(0x20, 1, 1)),
			Minor:    int(t.cell(0x20, 1, 2)),
			Build:    int(t.cell(0x20, 1, 3)),
			Revision: int(t.cell(0x20, 1, 4)),
		}
		info.Culture = t.str(t.cell(0x20, 1, 8))
	}
	for row := uint32(1); row <= t.rowCount[0x23]; row++ {
		info.References = append(info.References, Ref{
			Name: t.str(t.cell(0x23, row, 6)),
			Version: Version{
				Major:    int(t.cell(0x23, row, 0)),
				Minor:    int(t.cell(0x23, row, 1)),
				Build:    int(t.cell(0x23, row, 2)),
				Revision: int(t.cell(0x23, row, 3)),
			},
		})
	}
	nested := make(map[uint32]bool, t.rowCount[0x29])
	enclosing := make(map[uint32]uint32, t.rowCount[0x29])
	for row := uint32(1); row <= t.rowCount[0x29]; row++ {
		nested[t.cell(0x29, row, 0)] = true
		if _, dup := enclosing[t.cell(0x29, row, 0)]; !dup {
			enclosing[t.cell(0x29, row, 0)] = t.cell(0x29, row, 1)
		}
	}
	info.Types = make(map[string][]string)
	info.Nested = make(map[string][]string)
	info.Members = make(map[string][]Member)
	for row := uint32(1); row <= t.rowCount[0x02]; row++ {
		if nested[row] {
			ns, name, ok := t.nestedName(row, enclosing)
			if !ok {
				continue
			}
			info.Nested[ns] = append(info.Nested[ns], name)
			key := name
			if ns != "" {
				key = ns + "." + name
			}
			info.Members[key] = append(info.Members[key], t.members(row, 0x04, 1, 4, "field")...)
			info.Members[key] = append(info.Members[key], t.members(row, 0x06, 3, 5, "method")...)
			if a := t.attrs[attrKey(0x02, row)]; len(a) > 0 {
				info.Attributes[key] = a
			}
			if g := t.generics[row]; len(g) > 0 {
				info.Generics[key] = g
			}
			continue
		}
		name := t.str(t.cell(0x02, row, 1))
		if name == "" || name == "<Module>" {
			continue
		}
		ns := t.str(t.cell(0x02, row, 2))
		info.Types[ns] = append(info.Types[ns], name)
		info.TypeCount++
		key := name
		if ns != "" {
			key = ns + "." + name
		}
		info.Members[key] = append(info.Members[key], t.members(row, 0x04, 1, 4, "field")...)
		info.Members[key] = append(info.Members[key], t.members(row, 0x06, 3, 5, "method")...)
		if a := t.attrs[attrKey(0x02, row)]; len(a) > 0 {
			info.Attributes[key] = a
		}
		if g := t.generics[row]; len(g) > 0 {
			info.Generics[key] = g
		}
	}
	for _, names := range info.Types {
		sort.Strings(names)
	}
	for _, names := range info.Nested {
		sort.Strings(names)
	}
	for row := uint32(1); row <= t.rowCount[0x28]; row++ {
		name := t.str(t.cell(0x28, row, 2))
		info.Resources = append(info.Resources, name)
		// Implementation 0 is a resource embedded in this file.
		info.resRefs = append(info.resRefs, resourceRef{name: name, offset: t.cell(0x28, row, 0), embedded: t.cell(0x28, row, 3) == 0})
	}
}

// Limits on the method bodies kept from one file.
const (
	maxBodySize  = 1 << 20
	maxBodyTotal = 64 << 20
)

// readBodies keeps the code of every method that has a body, within the
// limits above. A body that cannot be read is skipped: the method is still
// listed, only its IL is missing.
func (in *Info) readBodies(rvaRead func(rva, n uint32) ([]byte, error)) {
	in.readBodiesFrom(rvaRead, 0)
}

// readBodiesFrom is readBodies with bytes already spent against the total limit.
func (in *Info) readBodiesFrom(rvaRead func(rva, n uint32) ([]byte, error), total int) {
	for _, list := range in.Members {
		for i := range list {
			m := &list[i]
			if m.Kind != "method" || m.rva == 0 || total >= maxBodyTotal {
				continue
			}
			head, err := rvaRead(m.rva, 12)
			if err != nil {
				// A tiny header is one byte, so a body at the end of a section
				// can be shorter than a fat header.
				if head, err = rvaRead(m.rva, 1); err != nil {
					continue
				}
			}
			n := uint32(0)
			switch head[0] & 3 {
			case 2:
				n = 1 + uint32(head[0]>>2)
			case 3:
				if len(head) < 12 {
					continue
				}
				n = uint32(binary.LittleEndian.Uint16(head)>>12)*4 + binary.LittleEndian.Uint32(head[4:])
			default:
				continue
			}
			if n == 0 || n > maxBodySize {
				continue
			}
			if body, err := rvaRead(m.rva, n); err == nil {
				m.body = body
				total += len(body)
			}
		}
	}
}

// MethodIL returns the IL listing of the index-th member of the type key, or ""
// when that member is not a method or has no readable body.
func (in *Info) MethodIL(key string, index int) string {
	list := in.Members[key]
	if in.tab == nil || index < 0 || index >= len(list) || list[index].body == nil {
		return ""
	}
	return in.tab.disassemble(list[index].body)
}

// ReadyToRun images of the framework libraries for another operating system
// carry their machine type XORed with a per-OS constant (Linux, macOS, FreeBSD,
// NetBSD), which debug/pe does not know. The metadata inside is ordinary.
var readyToRunOSKeys = []uint16{0x7B79, 0x4644, 0xADC4, 0x1993}

var knownMachines = map[uint16]bool{0x14c: true, 0x8664: true, 0x1c4: true, 0xAA64: true, 0x5064: true, 0x6264: true}

// openPE opens r as a PE file; when the machine type is one of those XORed
// ReadyToRun values, the two bytes are read as the plain machine type.
func openPE(r io.ReaderAt) (*pe.File, error) {
	file, err := pe.NewFile(r)
	if err == nil {
		return file, nil
	}
	var head [4]byte
	if _, e := r.ReadAt(head[:], 0x3C); e != nil {
		return nil, err
	}
	machineAt := int64(binary.LittleEndian.Uint32(head[:])) + 4
	if machineAt < 4 || machineAt > 1<<20 {
		return nil, err
	}
	var raw [2]byte
	if _, e := r.ReadAt(raw[:], machineAt); e != nil {
		return nil, err
	}
	machine := binary.LittleEndian.Uint16(raw[:])
	for _, key := range readyToRunOSKeys {
		if plain := machine ^ key; knownMachines[plain] {
			return pe.NewFile(&machinePatch{r: r, at: machineAt, plain: plain})
		}
	}
	return nil, err
}

// machinePatch shows the machine type at offset at as plain.
type machinePatch struct {
	r     io.ReaderAt
	at    int64
	plain uint16
}

func (m *machinePatch) ReadAt(p []byte, off int64) (int, error) {
	n, err := m.r.ReadAt(p, off)
	var plain [2]byte
	binary.LittleEndian.PutUint16(plain[:], m.plain)
	for i, b := range plain {
		if pos := m.at + int64(i) - off; pos >= 0 && pos < int64(n) {
			p[pos] = b
		}
	}
	return n, err
}
