package dotnet

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

type builder struct{ bytes.Buffer }

// The builder writes small test values; the conversions cannot overflow.
func (b *builder) u16(v int) { _ = binary.Write(b, binary.LittleEndian, uint16(v)) } //nolint:gosec // test data
func (b *builder) u32(v int) { _ = binary.Write(b, binary.LittleEndian, uint32(v)) } //nolint:gosec // test data

// heap builds a #Strings heap and hands out indexes into it.
type heap struct {
	data []byte
}

func (h *heap) add(s string) int {
	if len(h.data) == 0 {
		h.data = []byte{0}
	}
	at := len(h.data)
	h.data = append(h.data, s...)
	h.data = append(h.data, 0)
	return at
}

// sampleMetadata is a small but real metadata root: one module, three
// TypeDefs (<Module>, My.Ns.Foo, and Nested inside Foo), one assembly, one
// reference and one resource.
func sampleMetadata() []byte {
	var h heap
	h.add("")
	modName := h.add("Sample.dll")
	moduleType := h.add("<Module>")
	foo := h.add("Foo")
	ns := h.add("My.Ns")
	nested := h.add("Nested")
	asmName := h.add("Sample")
	refName := h.add("System.Runtime")
	resName := h.add("Sample.strings.resources")
	fieldCount := h.add("Count")
	fieldHidden := h.add("Hidden")
	methodCtor := h.add(".ctor")
	methodRun := h.add("Run")
	methodStop := h.add("Stop")
	sysNs := h.add("System")
	obsName := h.add("ObsoleteAttribute")
	paramT := h.add("T")
	paramU := h.add("U")

	var t builder
	// Module: Generation, Name, Mvid, EncId, EncBaseId.
	t.u16(0)
	t.u16(modName)
	t.u16(0)
	t.u16(0)
	t.u16(0)
	// TypeRef x1: ResolutionScope (AssemblyRef 1), Name, Namespace.
	t.u16(6)
	t.u16(obsName)
	t.u16(sysNs)
	// TypeDef x3: Flags, Name, Namespace, Extends, FieldList, MethodList.
	// Foo owns field 1 and methods 1-2; Nested owns field 2 and method 3.
	for _, row := range [][4]int{{moduleType, 0, 1, 1}, {foo, ns, 1, 1}, {nested, 0, 2, 3}} {
		t.u32(0)
		t.u16(row[0])
		t.u16(row[1])
		t.u16(0)
		t.u16(row[2])
		t.u16(row[3])
	}
	// Field x2: Flags, Name, Signature.
	for _, name := range []int{fieldCount, fieldHidden} {
		t.u16(0)
		t.u16(name)
		t.u16(0)
	}
	// Method x3: RVA, ImplFlags, Flags, Name, Signature, ParamList.
	for _, name := range []int{methodCtor, methodRun, methodStop} {
		t.u32(0)
		t.u16(0)
		t.u16(0)
		t.u16(name)
		t.u16(0)
		t.u16(1)
	}
	// MemberRef x1: Class (TypeRef 1), Name, Signature.
	t.u16(9)
	t.u16(methodCtor)
	t.u16(0)
	// CustomAttribute x3: Parent, Type, Value. Foo carries [System.Obsolete]
	// (through a MemberRef), its field Count carries an attribute whose
	// constructor is Foo's own .ctor (a MethodDef), and its method Run carries
	// [System.Obsolete] again.
	for _, row := range [][2]int{{2<<5 | 3, 1<<3 | 3}, {1<<5 | 1, 1<<3 | 2}, {2 << 5, 1<<3 | 3}} {
		t.u16(row[0])
		t.u16(row[1])
		t.u16(0)
	}
	// Assembly: HashAlgId, version, Flags, PublicKey, Name, Culture.
	t.u32(0x8004)
	t.u16(1)
	t.u16(2)
	t.u16(3)
	t.u16(4)
	t.u32(0)
	t.u16(0)
	t.u16(asmName)
	t.u16(0)
	// AssemblyRef: version, Flags, PublicKeyOrToken, Name, Culture, HashValue.
	t.u16(8)
	t.u16(0)
	t.u16(0)
	t.u16(0)
	t.u32(0)
	t.u16(0)
	t.u16(refName)
	t.u16(0)
	t.u16(0)
	// ManifestResource: Offset, Flags, Name, Implementation.
	t.u32(0)
	t.u32(1)
	t.u16(resName)
	t.u16(0)
	// NestedClass: NestedClass, EnclosingClass.
	t.u16(3)
	t.u16(2)

	// GenericParam x3: Number, Flags, Owner (TypeOrMethodDef), Name. On Foo,
	// T is covariant with class and U has new(); on the method Run (MethodDef
	// 2), T is a struct.
	for _, row := range [][4]int{{0, 0x01 | 0x04, 2 << 1, paramT}, {1, 0x10, 2 << 1, paramU}, {0, 0x08, 2<<1 | 1, paramT}} {
		t.u16(row[0])
		t.u16(row[1])
		t.u16(row[2])
		t.u16(row[3])
	}
	// GenericParamConstraint x2: Owner (GenericParam), Constraint (TypeRef 1).
	for _, owner := range []int{1, 3} {
		t.u16(owner)
		t.u16(1<<2 | 1)
	}

	var s builder
	s.u32(0)
	s.WriteByte(2)
	s.WriteByte(0)
	s.WriteByte(0)
	s.WriteByte(1)
	valid := uint64(1<<0 | 1<<1 | 1<<2 | 1<<4 | 1<<6 | 1<<0x0A | 1<<0x0C | 1<<0x20 | 1<<0x23 | 1<<0x28 | 1<<0x29 | 1<<0x2A | 1<<0x2C)
	_ = binary.Write(&s, binary.LittleEndian, valid)
	_ = binary.Write(&s, binary.LittleEndian, uint64(0))
	for _, n := range []int{1, 1, 3, 2, 3, 1, 3, 1, 1, 1, 1, 3, 2} {
		s.u32(n)
	}
	s.Write(t.Bytes())
	stream := s.Bytes()

	const version = "v4.0.30319\x00\x00"
	head := 16 + len(version) + 4
	dir1 := 8 + 4
	dir2 := 8 + 12
	tableOff := head + dir1 + dir2
	strOff := tableOff + len(stream)

	var r builder
	r.u32(0x424A5342)
	r.u16(1)
	r.u16(1)
	r.u32(0)
	r.u32(len(version))
	r.WriteString(version)
	r.u16(0)
	r.u16(2)
	r.u32(tableOff)
	r.u32(len(stream))
	r.WriteString("#~\x00\x00")
	r.u32(strOff)
	r.u32(len(h.data))
	r.WriteString("#Strings\x00\x00\x00\x00")
	r.Write(stream)
	r.Write(h.data)
	return r.Bytes()
}

// wrapPE puts metadata into a minimal PE32 image with a CLR header.
func wrapPE(metadata []byte, clr bool) []byte { return wrapPEWith(metadata, nil, clr) }

// wrapPEWith also places a resource area behind the metadata and points the
// CLR header at it.
func wrapPEWith(metadata, resources []byte, clr bool) []byte {
	const (
		peOff    = 0x40
		optOff   = peOff + 4 + 20
		optSize  = 224
		secOff   = optOff + optSize
		rawOff   = 0x200
		virtAddr = 0x2000
		corSize  = 72
		metaOff  = corSize
	)
	image := make([]byte, rawOff+corSize+len(metadata)+len(resources))
	le := binary.LittleEndian
	image[0], image[1] = 'M', 'Z'
	le.PutUint32(image[0x3c:], peOff)
	copy(image[peOff:], "PE\x00\x00")
	le.PutUint16(image[peOff+4:], 0x14c)
	le.PutUint16(image[peOff+6:], 1)
	le.PutUint16(image[peOff+20:], optSize)
	le.PutUint16(image[peOff+22:], 0x2102)
	le.PutUint16(image[optOff:], 0x10b)
	le.PutUint32(image[optOff+92:], 16)
	if clr {
		le.PutUint32(image[optOff+96+14*8:], virtAddr)
		le.PutUint32(image[optOff+96+14*8+4:], corSize)
	}
	copy(image[secOff:], ".text")
	le.PutUint32(image[secOff+8:], uint32(corSize+len(metadata)+len(resources))) //nolint:gosec // test data
	le.PutUint32(image[secOff+12:], virtAddr)
	le.PutUint32(image[secOff+16:], uint32(corSize+len(metadata)+len(resources))) //nolint:gosec // test data
	le.PutUint32(image[secOff+20:], rawOff)
	le.PutUint32(image[rawOff:], corSize)
	le.PutUint32(image[rawOff+8:], virtAddr+metaOff)
	le.PutUint32(image[rawOff+12:], uint32(len(metadata))) //nolint:gosec // test data
	copy(image[rawOff+corSize:], metadata)
	if len(resources) > 0 {
		le.PutUint32(image[rawOff+24:], virtAddr+corSize+uint32(len(metadata))) //nolint:gosec // test data
		le.PutUint32(image[rawOff+28:], uint32(len(resources)))                 //nolint:gosec // test data
		copy(image[rawOff+corSize+len(metadata):], resources)
	}
	return image
}

func TestReadAssembly(t *testing.T) {
	image := wrapPE(sampleMetadata(), true)
	info, err := Read(bytes.NewReader(image), int64(len(image)))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if info.RuntimeVersion != "v4.0.30319" {
		t.Errorf("RuntimeVersion = %q", info.RuntimeVersion)
	}
	if info.Name != "Sample" || info.Version.String() != "1.2.3.4" {
		t.Errorf("assembly = %q %s", info.Name, info.Version)
	}
	if len(info.References) != 1 || info.References[0].Name != "System.Runtime" || info.References[0].Version.String() != "8.0.0.0" {
		t.Errorf("References = %+v", info.References)
	}
	// <Module> and the nested type are left out.
	if info.TypeCount != 1 || len(info.Types["My.Ns"]) != 1 || info.Types["My.Ns"][0] != "Foo" {
		t.Errorf("Types = %v (%d)", info.Types, info.TypeCount)
	}
	if got := info.Namespaces(); len(got) != 1 || got[0] != "My.Ns" {
		t.Errorf("Namespaces = %v", got)
	}
	// The nested type is listed apart, as Foo+Nested in Foo's namespace, with
	// its own members.
	if got := info.Nested["My.Ns"]; len(got) != 1 || got[0] != "Foo+Nested" {
		t.Errorf("Nested = %v", info.Nested)
	}
	// The custom attributes: on the type, on a field and on a method.
	if got := info.Attributes["My.Ns.Foo"]; len(got) != 1 || got[0] != "System.Obsolete" {
		t.Errorf("attributes of Foo = %v", info.Attributes)
	}
	foo := info.Members["My.Ns.Foo"]
	if len(foo) < 3 || len(foo[0].Attributes) != 1 || foo[0].Attributes[0] != "My.Ns.Foo" ||
		len(foo[2].Attributes) != 1 || foo[2].Attributes[0] != "System.Obsolete" || len(foo[1].Attributes) != 0 {
		t.Errorf("member attributes = %+v", foo)
	}
	// Foo<T, U>: T is covariant with a class constraint and one type
	// constraint, U has new().
	gen := info.Generics["My.Ns.Foo"]
	if len(gen) != 2 || gen[0].Name != "T" || gen[0].Variance != "out" || !gen[0].Class || len(gen[0].Constraints) != 1 ||
		gen[0].Constraints[0] != "System.ObsoleteAttribute" || gen[1].Name != "U" || !gen[1].New || gen[1].Class || len(gen[1].Constraints) != 0 {
		t.Errorf("generics of Foo = %+v", info.Generics)
	}
	// The method Run has its own <T> where T : struct, ObsoleteAttribute.
	run := info.Members["My.Ns.Foo"][2]
	if len(run.Generics) != 1 || run.Generics[0].Name != "T" || !run.Generics[0].Struct || len(run.Generics[0].Constraints) != 1 ||
		len(info.Members["My.Ns.Foo"][0].Generics) != 0 {
		t.Errorf("generics of Run = %+v", run.Generics)
	}
	nestedMembers := info.Members["My.Ns.Foo+Nested"]
	if len(nestedMembers) != 2 || nestedMembers[0].Name != "Hidden" || nestedMembers[1].Name != "Stop" {
		t.Errorf("members of the nested type = %+v", nestedMembers)
	}
	// Foo's field and its first two methods; the nested type's own members are
	// not attached to it.
	wantMembers := []Member{{Kind: "field", Name: "Count"}, {Kind: "method", Name: ".ctor"}, {Kind: "method", Name: "Run"}}
	got := info.Members["My.Ns.Foo"]
	sameMembers := len(got) == len(wantMembers)
	for i := 0; sameMembers && i < len(got); i++ {
		sameMembers = got[i].Kind == wantMembers[i].Kind && got[i].Name == wantMembers[i].Name
	}
	if !sameMembers {
		t.Errorf("Members = %+v, want %+v", info.Members, wantMembers)
	}
	if len(info.Resources) != 1 || info.Resources[0] != "Sample.strings.resources" {
		t.Errorf("Resources = %v", info.Resources)
	}
}

func TestReadRejectsWhatIsNotAnAssembly(t *testing.T) {
	native := wrapPE(sampleMetadata(), false)
	for name, data := range map[string][]byte{
		"text":   []byte("hello, world"),
		"native": native,
		"empty":  {},
	} {
		if _, err := Read(bytes.NewReader(data), int64(len(data))); !errors.Is(err, ErrNotAssembly) {
			t.Errorf("%s: err = %v, want ErrNotAssembly", name, err)
		}
	}
}

func TestReadSurvivesDamage(t *testing.T) {
	meta := sampleMetadata()
	// Every truncation and every single-byte corruption must end in an error
	// or a result, never a panic or a runaway allocation.
	for cut := 0; cut < len(meta); cut++ {
		image := wrapPE(meta[:cut], true)
		_, _ = Read(bytes.NewReader(image), int64(len(image)))
	}
	for i := range meta {
		damaged := append([]byte(nil), meta...)
		damaged[i] ^= 0xff
		image := wrapPE(damaged, true)
		_, _ = Read(bytes.NewReader(image), int64(len(image)))
	}
	short := wrapPE(sampleMetadata(), true)
	if _, err := Read(bytes.NewReader(short), int64(len(short)-10)); err == nil {
		t.Error("a file shorter than its metadata was accepted")
	}
}

func TestReportListsWhatWasRead(t *testing.T) {
	image := wrapPE(sampleMetadata(), true)
	info, err := Read(bytes.NewReader(image), int64(len(image)))
	if err != nil {
		t.Fatal(err)
	}
	text := Report(info, "Sample.dll")
	for _, want := range []string{"`Sample.dll`", "`Sample`", "`1.2.3.4`", "`System.Runtime`", "`My.Ns`", "`Foo`", "`Sample.strings.resources`", "`v4.0.30319`"} {
		if !strings.Contains(text, want) {
			t.Errorf("report lacks %s:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Nested") {
		t.Error("a nested type was listed")
	}
	if code("a`b") != "`a'b`" {
		t.Error("a backtick inside a code span was not neutralized")
	}
}

func TestReportCapsLongLists(t *testing.T) {
	info := &Info{RuntimeVersion: "v4", Types: map[string][]string{}}
	for i := 0; i < maxReportRefs+5; i++ {
		info.References = append(info.References, Ref{Name: "R"})
	}
	for i := 0; i < maxReportTypes+5; i++ {
		info.Types["N"] = append(info.Types["N"], "T")
		info.TypeCount++
	}
	for i := 0; i < maxReportResources+5; i++ {
		info.Resources = append(info.Resources, "res")
	}
	text := Report(info, "big.dll")
	if n := strings.Count(text, "- `R`"); n != maxReportRefs {
		t.Errorf("references listed = %d", n)
	}
	if n := strings.Count(text, "- `T`"); n != maxReportTypes {
		t.Errorf("types listed = %d", n)
	}
	if n := strings.Count(text, "- `res`"); n != maxReportResources {
		t.Errorf("resources listed = %d", n)
	}
}

func TestReadResourceBytes(t *testing.T) {
	// The sample's one resource sits at offset 0 of the resource area: a
	// 4-byte length and its bytes.
	area := []byte{5, 0, 0, 0, 'h', 'e', 'l', 'l', 'o'}
	image := wrapPEWith(sampleMetadata(), area, true)
	info, err := Read(bytes.NewReader(image), int64(len(image)))
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Blobs) != 1 || info.Blobs[0].Name != "Sample.strings.resources" || string(info.Blobs[0].Data) != "hello" {
		t.Fatalf("blobs = %+v", info.Blobs)
	}
	// A length that runs past the area, or an area missing altogether, leaves
	// the rest of the result intact.
	for name, bad := range map[string][]byte{"too long": {200, 0, 0, 0, 'x'}, "too short": {1, 2}} {
		image := wrapPEWith(sampleMetadata(), bad, true)
		info, err := Read(bytes.NewReader(image), int64(len(image)))
		if err != nil || len(info.Blobs) != 0 || info.Name != "Sample" {
			t.Errorf("%s: %+v, %v", name, info, err)
		}
	}
	plain := wrapPE(sampleMetadata(), true)
	if info, err := Read(bytes.NewReader(plain), int64(len(plain))); err != nil || len(info.Blobs) != 0 {
		t.Errorf("no resource area: %+v, %v", info, err)
	}
	if got := blobsOf([]resourceRef{{name: "linked", embedded: false}, {name: "far", offset: 1 << 30, embedded: true}}, area); len(got) != 0 {
		t.Errorf("linked or out-of-range resources yielded %+v", got)
	}
}

func TestNestedNameFollowsAndBoundsTheChain(t *testing.T) {
	image := wrapPE(sampleMetadata(), true)
	info, err := Read(bytes.NewReader(image), int64(len(image)))
	if err != nil {
		t.Fatal(err)
	}
	tab := info.tab
	// Row 3 is nested in row 2 (Foo, My.Ns).
	if ns, name, ok := tab.nestedName(3, map[uint32]uint32{3: 2}); !ok || ns != "My.Ns" || name != "Foo+Nested" {
		t.Fatalf("nestedName = %q %q %v", ns, name, ok)
	}
	// A chain that loops back on itself is refused instead of followed for ever.
	if _, _, ok := tab.nestedName(3, map[uint32]uint32{3: 2, 2: 3}); ok {
		t.Error("a looping NestedClass chain was accepted")
	}
	// So is one that points outside the TypeDef table.
	if _, _, ok := tab.nestedName(3, map[uint32]uint32{3: 999}); ok {
		t.Error("an enclosing row outside the table was accepted")
	}
}

func TestAttributeNameRefusesBadTokens(t *testing.T) {
	image := wrapPE(sampleMetadata(), true)
	info, err := Read(bytes.NewReader(image), int64(len(image)))
	if err != nil {
		t.Fatal(err)
	}
	for _, coded := range []uint32{0<<3 | 2, 999<<3 | 2, 0<<3 | 3, 999<<3 | 3, 1<<3 | 5} {
		if got := info.tab.attributeName(coded); got != "" {
			t.Errorf("attributeName(%#x) = %q, want none", coded, got)
		}
	}
}
