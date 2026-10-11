package netbrowse

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

// fakeShareFS records what the file system of a share is asked, and answers
// as a share with one directory "sub" and one file "a.txt" would.
type fakeShareFS struct {
	vfs.VFS
	ops *[]string
}

func (f fakeShareFS) ReadDir(_ context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	*f.ops = append(*f.ops, "readdir "+p)
	onChunk([]vfs.VFSItem{{Name: "a.txt", Size: 3}, {Name: "sub", IsDir: true}})
	return nil
}

func (f fakeShareFS) Stat(_ context.Context, p string) (vfs.VFSItem, error) {
	*f.ops = append(*f.ops, "stat "+p)
	if strings.HasSuffix(p, "a.txt") {
		return vfs.VFSItem{Name: "a.txt", Size: 3}, nil
	}
	return vfs.VFSItem{Name: "d", IsDir: true}, nil
}

func (f fakeShareFS) MkDir(_ context.Context, p string) error {
	*f.ops = append(*f.ops, "mkdir "+p)
	return nil
}
func (f fakeShareFS) Remove(_ context.Context, p string) error {
	*f.ops = append(*f.ops, "rm "+p)
	return nil
}
func (f fakeShareFS) Rename(_ context.Context, a, b string) error {
	*f.ops = append(*f.ops, "mv "+a+" "+b)
	return nil
}
func (f fakeShareFS) SetAttributes(_ context.Context, p string, _ vfs.VFSItem) error {
	*f.ops = append(*f.ops, "attr "+p)
	return nil
}
func (f fakeShareFS) Open(_ context.Context, p string) (vfs.ReadAtCloser, error) {
	*f.ops = append(*f.ops, "open "+p)
	return nil, nil
}
func (f fakeShareFS) Create(_ context.Context, p string) (io.WriteCloser, error) {
	*f.ops = append(*f.ops, "create "+p)
	return nil, nil
}

func newFakeNetVFS(ops *[]string) *networkVFS {
	return newNetworkVFS(fakeNetwork, func(string) vfs.VFS { return fakeShareFS{ops: ops} })
}

func list(t *testing.T, v vfs.VFS, p string) []string {
	t.Helper()
	var names []string
	if err := v.ReadDir(context.Background(), p, func(items []vfs.VFSItem) {
		for _, i := range items {
			names = append(names, i.Name)
		}
	}); err != nil {
		t.Fatalf("ReadDir(%q): %v", p, err)
	}
	return names
}

func TestSegmentOf(t *testing.T) {
	for _, c := range []struct{ remote, want string }{
		{"Microsoft Windows Network", "Microsoft Windows Network"}, {`\\alpha`, "alpha"}, {`\\alpha\docs`, "docs"}, {`\\`, `\\`},
	} {
		if got := segmentOf(resource{Remote: c.remote}); got != c.want {
			t.Errorf("segmentOf(%q) = %q, want %q", c.remote, got, c.want)
		}
	}
}

// TestNetworkVFSWalksTheTreeAndEntersShares: the tree levels list the network,
// and a path inside a share is handed to the file system at the share's UNC name.
func TestNetworkVFSWalksTheTreeAndEntersShares(t *testing.T) {
	var ops []string
	v := newFakeNetVFS(&ops)
	if v.GetTitle() != "Network" || !v.IsAtRoot() || v.GetPath() != "/" || !v.IsAbs("/x") || v.IsAbs("x") {
		t.Fatalf("fresh VFS: title %q root %v path %q", v.GetTitle(), v.IsAtRoot(), v.GetPath())
	}
	if got := list(t, v, "/"); len(got) != 1 || got[0] != "Microsoft Windows Network" {
		t.Errorf("top = %v", got)
	}
	if got := list(t, v, "/Microsoft Windows Network/workgroup"); strings.Join(got, ",") != "alpha,beta" {
		t.Errorf("servers = %v (case-insensitive segment)", got)
	}
	if got := list(t, v, "/Microsoft Windows Network/WORKGROUP/alpha"); strings.Join(got, ",") != "docs,pub" {
		t.Errorf("shares = %v", got)
	}
	if got := list(t, v, "/Microsoft Windows Network/WORKGROUP/alpha/docs/sub"); strings.Join(got, ",") != "a.txt,sub" {
		t.Errorf("inside a share = %v", got)
	}
	if want := `readdir \\alpha\docs\sub`; len(ops) != 1 || ops[0] != want {
		t.Errorf("share file system saw %v, want %q", ops, want)
	}

	if err := v.SetPath("/Microsoft Windows Network/WORKGROUP/alpha/docs"); err != nil {
		t.Fatal(err)
	}
	if v.IsAtRoot() || v.GetPath() != "/Microsoft Windows Network/WORKGROUP/alpha/docs" {
		t.Errorf("path = %q", v.GetPath())
	}
	if abs, _ := v.Abs("sub"); abs != "/Microsoft Windows Network/WORKGROUP/alpha/docs/sub" {
		t.Errorf("Abs = %q", abs)
	}
	if err := v.SetPath("/Microsoft Windows Network/WORKGROUP/alpha/docs/a.txt"); err == nil {
		t.Error("SetPath into a file succeeded")
	}
	if err := v.SetPath("/nowhere"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("SetPath to a missing entry: %v", err)
	}
	if v.Join("a", "b") != "a/b" || v.Base("/a/b") != "b" || v.Dir("/a/b") != "/a" {
		t.Error("path helpers")
	}
}

func TestNetworkVFSStat(t *testing.T) {
	var ops []string
	v := newFakeNetVFS(&ops)
	ctx := context.Background()
	if it, err := v.Stat(ctx, "/"); err != nil || !it.IsDir {
		t.Errorf("Stat(/) = %+v, %v", it, err)
	}
	if it, err := v.Stat(ctx, "/Microsoft Windows Network/WORKGROUP/alpha"); err != nil || !it.IsDir || it.Name != "alpha" {
		t.Errorf("Stat(server) = %+v, %v", it, err)
	}
	if it, err := v.Stat(ctx, "/Microsoft Windows Network/WORKGROUP/alpha/docs/a.txt"); err != nil || it.IsDir || it.Size != 3 {
		t.Errorf("Stat(file) = %+v, %v", it, err)
	}
	if _, err := v.Stat(ctx, "/nope"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat(missing) = %v", err)
	}
	if _, err := v.Stat(ctx, "/Microsoft Windows Network/WORKGROUP/beta/x"); err == nil {
		t.Error("Stat under a server that cannot be listed succeeded")
	}
}

// TestNetworkVFSChangesOnlyInsideShares: files inside a share are changed
// through the share's file system, the network tree itself is read-only.
func TestNetworkVFSChangesOnlyInsideShares(t *testing.T) {
	var ops []string
	v := newFakeNetVFS(&ops)
	ctx := context.Background()
	base := "/Microsoft Windows Network/WORKGROUP/alpha/docs"
	if !v.GetCapabilities().HasWrite || !v.GetCapabilities().HasRandomAccess {
		t.Errorf("capabilities = %+v", v.GetCapabilities())
	}
	if err := v.MkDir(ctx, base+"/new"); err != nil {
		t.Fatal(err)
	}
	if err := v.Remove(ctx, base+"/a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := v.Rename(ctx, base+"/a.txt", base+"/b.txt"); err != nil {
		t.Fatal(err)
	}
	if err := v.SetAttributes(ctx, base+"/a.txt", vfs.VFSItem{}); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Open(ctx, base+"/a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Create(ctx, base+"/c.txt"); err != nil {
		t.Fatal(err)
	}
	want := `mkdir \\alpha\docs\new|rm \\alpha\docs\a.txt|mv \\alpha\docs\a.txt \\alpha\docs\b.txt|attr \\alpha\docs\a.txt|open \\alpha\docs\a.txt|create \\alpha\docs\c.txt`
	if got := strings.Join(ops, "|"); got != want {
		t.Errorf("share file system saw\n%s\nwant\n%s", got, want)
	}

	tree := "/Microsoft Windows Network/WORKGROUP"
	for i, err := range []error{
		v.MkDir(ctx, tree+"/x"), v.Remove(ctx, tree), v.Rename(ctx, tree, tree+"/y"),
		v.Rename(ctx, base+"/a.txt", tree), v.SetAttributes(ctx, "/", vfs.VFSItem{}),
	} {
		if !errors.Is(err, errNotInShare) {
			t.Errorf("change of the tree %d: %v, want errNotInShare", i, err)
		}
	}
	if _, err := v.Open(ctx, tree); !errors.Is(err, errNotInShare) {
		t.Errorf("Open of a container: %v", err)
	}
	if _, err := v.Create(ctx, tree+"/z"); !errors.Is(err, errNotInShare) {
		t.Errorf("Create in the tree: %v", err)
	}
	if ch, err := v.Search(ctx, "/", "x"); ch != nil || err != nil {
		t.Errorf("Search = %v, %v", ch, err)
	}
	if v.ParentVFS() != nil || v.Close() != nil {
		t.Error("ParentVFS or Close")
	}
	c := v.Clone()
	if c.GetPath() != v.GetPath() {
		t.Errorf("clone path %q", c.GetPath())
	}
	if openUNC(`\\x\y`) == nil {
		t.Error("openUNC returned nil")
	}
}

func TestNetworkURIOpensTheNetworkAtAnAddress(t *testing.T) {
	var ops []string
	p := &uriProvider{enum: fakeNetwork, open: func(string) vfs.VFS { return fakeShareFS{ops: &ops} }}
	if p.Scheme() != "network" {
		t.Fatalf("scheme = %q", p.Scheme())
	}
	ctx := context.Background()
	for raw, want := range map[string]string{
		"network://": "/", "network:": "/",
		"network://Microsoft%20Windows%20Network/WORKGROUP/alpha": "/Microsoft Windows Network/WORKGROUP/alpha",
		"network:///Microsoft Windows Network":                    "/Microsoft Windows Network",
	} {
		v, err := p.OpenURI(ctx, nil, raw)
		if err != nil {
			t.Fatalf("OpenURI(%q): %v", raw, err)
		}
		if v.GetPath() != want {
			t.Errorf("OpenURI(%q) opens at %q, want %q", raw, v.GetPath(), want)
		}
	}
	if _, err := p.OpenURI(ctx, nil, "network://nowhere"); err == nil {
		t.Error("an address that is not in the network opened")
	}
}
