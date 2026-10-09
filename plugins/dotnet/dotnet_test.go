package dotnet

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	idotnet "github.com/unxed/f4/internal/dotnet"
	"github.com/unxed/f4/vfs"
)

func sampleInfo() *idotnet.Info {
	return &idotnet.Info{
		RuntimeVersion: "v4.0.30319",
		Name:           "Sample",
		Version:        idotnet.Version{Major: 1, Minor: 2},
		References:     []idotnet.Ref{{Name: "System.Runtime", Version: idotnet.Version{Major: 8}}},
		Types:          map[string][]string{"My.Ns": {"Zed", "Foo", "Foo"}, "": {"Global"}},
		TypeCount:      4,
		Resources:      []string{"a/b.resources", "linked.bin"},
		Blobs:          []idotnet.Blob{{Name: "a/b.resources", Data: []byte("RES")}},
		Nested:         map[string][]string{"My.Ns": {"Foo+Inner"}},
		Attributes:     map[string][]string{"My.Ns.Foo": {"System.Serializable"}},
		Generics: map[string][]idotnet.GenericParam{"My.Ns.Foo": {
			{Name: "T", Variance: "out", Class: true, Constraints: []string{"System.IDisposable"}},
			{Name: "K", Struct: true, New: true},
			{Name: "V", New: true},
			{Name: "X"},
		}},
		Members: map[string][]idotnet.Member{
			"My.Ns.Foo":       {{Kind: "field", Name: "Count", Attributes: []string{"System.NonSerialized"}}, {Kind: "method", Name: "Run", Generics: []idotnet.GenericParam{{Name: "M", Class: true}}}},
			"My.Ns.Foo+Inner": {{Kind: "field", Name: "Depth"}},
		},
	}
}

func listing(t *testing.T, v vfs.VFS, dir string) map[string]bool {
	t.Helper()
	got := map[string]bool{}
	if err := v.ReadDir(context.Background(), dir, func(items []vfs.VFSItem) {
		for _, it := range items {
			got[it.Name] = it.IsDir
		}
	}); err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	return got
}

func TestAssemblyTreeBrowsing(t *testing.T) {
	v := newAssemblyVFS(nil, "Sample.dll", sampleInfo())
	root := listing(t, v, "/")
	for name, dir := range map[string]bool{"assembly.md": false, "References": true, "Namespaces": true, "Resources": true} {
		if got, ok := root[name]; !ok || got != dir {
			t.Errorf("root lacks %q (dir=%v): %v", name, dir, root)
		}
	}
	if !v.IsAtRoot() {
		t.Error("a new panel is not at the root")
	}
	spaces := listing(t, v, "/Namespaces")
	if !spaces["My.Ns"] || !spaces["(global)"] {
		t.Errorf("namespaces = %v", spaces)
	}
	types := listing(t, v, "/Namespaces/My.Ns")
	// Foo and its twin are both listed, kept apart by a suffix.
	for _, name := range []string{"Foo", "Foo (2)", "Zed"} {
		if isDir, ok := types[name]; !ok || isDir {
			t.Errorf("types = %v, want file %q", types, name)
		}
	}
	if got := readAll(t, v, "/Namespaces/My.Ns/Foo+Inner"); !strings.Contains(got, "type Foo+Inner\n") || !strings.Contains(got, "field Depth\n") {
		t.Errorf("the nested type's file reads %q", got)
	}
	if got := readAll(t, v, "/Namespaces/My.Ns/Foo"); !strings.Contains(got, "[System.Serializable]\ntype Foo\n") || !strings.Contains(got, "[System.NonSerialized]\nfield Count\n") {
		t.Errorf("attributes are not in the type file: %q", got)
	}
	if got := readAll(t, v, "/Namespaces/My.Ns/Foo"); !strings.Contains(got, "generic out T : class, System.IDisposable\n") ||
		!strings.Contains(got, "generic K : struct\n") || !strings.Contains(got, "generic V : new()\n") || !strings.Contains(got, "generic X\n") {
		t.Errorf("generic parameters are not in the type file: %q", got)
	}
	if got := readAll(t, v, "/Namespaces/My.Ns/Foo"); !strings.Contains(got, "method Run\n  generic M : class\n") {
		t.Errorf("a method's generic parameters are not in the type file: %q", got)
	}
	if got := readAll(t, v, "/Namespaces/My.Ns/Foo"); !strings.Contains(got, "field Count\n") || !strings.Contains(got, "method Run\n") {
		t.Errorf("a type file lists %q", got)
	}
	if refs := listing(t, v, "/References"); len(refs) != 1 {
		t.Errorf("references = %v", refs)
	}
	res := listing(t, v, "/Resources")
	if isDir, ok := res["a_b.resources"]; !ok || isDir || len(res) != 2 {
		t.Errorf("resource names are not made safe: %v", res)
	}
	if got := readAll(t, v, "/Resources/a_b.resources"); got != "RES" {
		t.Errorf("resource bytes = %q", got)
	}
	if got := readAll(t, v, "/Resources/linked.bin"); !strings.Contains(got, "not stored") {
		t.Errorf("a resource without bytes reads %q", got)
	}
	if err := v.SetPath("/Namespaces/My.Ns"); err != nil || v.IsAtRoot() || v.GetPath() != "/Namespaces/My.Ns" {
		t.Errorf("SetPath: %v %q", err, v.GetPath())
	}
	if err := v.SetPath("/nowhere"); !errors.Is(err, os.ErrInvalid) {
		t.Errorf("SetPath into nothing = %v", err)
	}
	if err := v.SetPath("/assembly.md"); err == nil {
		t.Error("SetPath into a file succeeded")
	}
	if v.PanelTitle("/") == "" || v.GetTitle() != "Sample.dll" || v.ParentVFS() != nil {
		t.Error("titles or parent wrong")
	}
	if clone := v.Clone(); clone.GetPath() != v.GetPath() {
		t.Error("clone lost the path")
	}
}

func TestAssemblyFilesAndReadOnly(t *testing.T) {
	v := newAssemblyVFS(nil, "Sample.dll", sampleInfo())
	ctx := context.Background()
	item, err := v.Stat(ctx, "/assembly.md")
	if err != nil || item.IsDir || item.Size == 0 || !item.SizeKnown {
		t.Fatalf("Stat = %+v, %v", item, err)
	}
	if root, err := v.Stat(ctx, "/"); err != nil || !root.IsDir || root.Name != "Sample.dll" {
		t.Errorf("root Stat = %+v, %v", root, err)
	}
	if _, err := v.Stat(ctx, "/missing"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat missing = %v", err)
	}
	f, err := v.Open(ctx, "/assembly.md")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	all, err := io.ReadAll(readerOf{f, ctx})
	if err != nil || len(all) != int(f.Size()) {
		t.Errorf("read %d bytes of %d: %v", len(all), f.Size(), err)
	}
	if n, err := f.ReadAt(ctx, make([]byte, 4), 1<<20); n != 0 || !errors.Is(err, io.EOF) {
		t.Errorf("ReadAt past the end = %d, %v", n, err)
	}
	if _, err := v.Open(ctx, "/References"); err == nil {
		t.Error("a folder opened as a file")
	}
	if err := v.MkDir(ctx, "/x"); err == nil {
		t.Error("MkDir succeeded")
	}
	if err := v.Remove(ctx, "/assembly.md"); err == nil {
		t.Error("Remove succeeded")
	}
	if err := v.Rename(ctx, "/a", "/b"); err == nil {
		t.Error("Rename succeeded")
	}
	if _, err := v.Create(ctx, "/n"); err == nil {
		t.Error("Create succeeded")
	}
	if err := v.SetAttributes(ctx, "/a", vfs.VFSItem{}); err == nil {
		t.Error("SetAttributes succeeded")
	}
	if v.GetCapabilities().HasRandomAccess != true {
		t.Error("random access is not advertised")
	}
	if ch, err := v.Search(ctx, "/", "x"); ch != nil || err != nil {
		t.Error("Search is not a no-op")
	}
	if err := v.Close(); err != nil {
		t.Error(err)
	}
}

func readAll(t *testing.T, v vfs.VFS, p string) string {
	t.Helper()
	ctx := context.Background()
	f, err := v.Open(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	all, err := io.ReadAll(readerOf{f, ctx})
	if err != nil {
		t.Fatal(err)
	}
	return string(all)
}

type readerOf struct {
	f   vfs.ReadAtCloser
	ctx context.Context
}

func (r readerOf) Read(p []byte) (int, error) { return r.f.Read(r.ctx, p) }

func TestProviderRefusesWhatIsNotAnAssembly(t *testing.T) {
	dir := t.TempDir()
	text := filepath.Join(dir, "notes.txt")
	fake := filepath.Join(dir, "fake.dll")
	for _, p := range []string{text, fake} {
		if err := os.WriteFile(p, []byte("plain text"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	local := vfs.NewOSVFS(dir)
	p := &provider{}
	ctx := context.Background()
	if p.CanOpen(ctx, local, text) || p.CanOpen(ctx, local, fake) || p.CanOpen(ctx, local, filepath.Join(dir, "missing.dll")) {
		t.Error("a file without .NET metadata was claimed")
	}
	if p.CanOpen(ctx, nil, fake) {
		t.Error("a non-local parent was claimed")
	}
	if _, err := p.Open(ctx, local, fake); !errors.Is(err, idotnet.ErrNotAssembly) {
		t.Errorf("Open = %v, want ErrNotAssembly", err)
	}
	if _, err := p.Open(ctx, local, text); !errors.Is(err, idotnet.ErrNotAssembly) {
		t.Errorf("Open(text) = %v", err)
	}
	if p.PanelEnterAllowed(ctx, local, fake) || p.Name() != "dotnet" || p.Priority() == 0 {
		t.Error("provider identity or Enter policy wrong")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if p.CanOpen(cancelled, local, fake) {
		t.Error("a cancelled context still claimed a file")
	}
	if !assemblyFile("X.DLL") || assemblyFile("x.txt") {
		t.Error("assemblyFile misjudges names")
	}
}

func TestPluginLifecycle(t *testing.T) {
	p := NewPlugin()
	if p.GetName() == "" {
		t.Error("no name")
	}
	if err := p.Init(nil); err == nil {
		t.Error("Init(nil) succeeded")
	}
	if err := p.Close(); err != nil {
		t.Error(err)
	}
}

func TestReferencesAreFollowedNextToTheFile(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"Main.dll", "Dep.dll", "Broken.exe"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := loadAssembly
	defer func() { loadAssembly = old }()
	loadAssembly = func(path string) (*idotnet.Info, error) {
		switch filepath.Base(path) {
		case "Dep.dll":
			// Dep references Main again: a cycle, which must not be followed.
			return &idotnet.Info{Name: "Dep", References: []idotnet.Ref{{Name: "Main"}}}, nil
		}
		return nil, errors.New("cannot read")
	}
	main := &idotnet.Info{Name: "Main", References: []idotnet.Ref{
		{Name: "Dep", Version: idotnet.Version{Major: 1}},
		{Name: "Missing"},
		{Name: "../Dep"},
		{Name: "Broken"},
	}}
	v := newAssemblyVFSIn(nil, "Main.dll", main, dir, []string{filepath.Join(dir, "Main.dll")})
	refs := listing(t, v, "/References")
	if !refs["Dep 1.0.0.0"] || !refs["Broken 0.0.0.0"] || refs["Missing 0.0.0.0"] || refs["../Dep 0.0.0.0"] || refs[".._Dep 0.0.0.0"] {
		t.Fatalf("references = %v", refs)
	}
	inside := listing(t, v, "/References/Dep 1.0.0.0")
	if isDir, ok := inside["assembly.md"]; !ok || isDir || !inside["References"] || !inside["Namespaces"] {
		t.Fatalf("a followed reference lists %v", inside)
	}
	// The cycle back to Main is only a note.
	if again := listing(t, v, "/References/Dep 1.0.0.0/References"); again["Main 0.0.0.0"] {
		t.Errorf("a reference back to an entered assembly was followed: %v", again)
	}
	if got := readAll(t, v, "/References/Dep 1.0.0.0/assembly.md"); !strings.Contains(got, "Dep") {
		t.Errorf("the followed assembly's report = %q", got)
	}
	// A reference that cannot be read is an empty folder, not an error.
	if len(listing(t, v, "/References/Broken 0.0.0.0")) != 0 {
		t.Error("an unreadable reference is not empty")
	}
	if err := v.SetPath("/References/Dep 1.0.0.0/Namespaces"); err != nil || v.GetPath() != "/References/Dep 1.0.0.0/Namespaces" {
		t.Errorf("SetPath into a reference: %v", err)
	}
	if findAssembly("", "Dep") != "" || findAssembly(dir, "") != "" || findAssembly(dir, "a\\b") != "" {
		t.Error("findAssembly accepted a bad name")
	}
	if findAssembly(dir, "Broken") == "" || findAssembly(dir, "Dep") == "" || findAssembly(dir, "Nope") != "" {
		t.Error("findAssembly did not find the files that exist")
	}
	deep := []string{"1", "2", "3", "4", "5", "6"}
	limited := buildTree(main, "Main.dll", dir, deep)
	if isDir := limited.kids["References"].kids["Dep 1.0.0.0"].dir; isDir {
		t.Error("references were followed past the depth limit")
	}
}
