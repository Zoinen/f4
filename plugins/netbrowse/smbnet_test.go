package netbrowse

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

// fakeServer is one SMB server as the smb:// provider hands it out: the
// shares "docs", "pub" and the administrative "C$" at its root.
type fakeServer struct {
	vfs.VFS
	p *fakeSMB
}

func (f fakeServer) ReadDir(_ context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	f.p.ops = append(f.p.ops, "readdir "+p)
	if p == "/" {
		onChunk([]vfs.VFSItem{{Name: "docs", IsDir: true}, {Name: "C$", IsDir: true, IsHidden: true}, {Name: "pub", IsDir: true}})
		return nil
	}
	onChunk([]vfs.VFSItem{{Name: "a.txt"}})
	return nil
}
func (f fakeServer) Stat(_ context.Context, p string) (vfs.VFSItem, error) {
	f.p.ops = append(f.p.ops, "stat "+p)
	if f.p.statErr != nil {
		return vfs.VFSItem{}, f.p.statErr
	}
	return vfs.VFSItem{Name: "x", IsDir: true}, nil
}
func (f fakeServer) Rename(_ context.Context, a, b string) error {
	f.p.ops = append(f.p.ops, "mv "+a+" "+b)
	return nil
}
func (f fakeServer) MkDir(context.Context, string) error                      { return nil }
func (f fakeServer) Remove(context.Context, string) error                     { return nil }
func (f fakeServer) SetAttributes(context.Context, string, vfs.VFSItem) error { return nil }
func (f fakeServer) Open(context.Context, string) (vfs.ReadAtCloser, error)   { return nil, nil }
func (f fakeServer) Create(context.Context, string) (io.WriteCloser, error)   { return nil, nil }
func (f fakeServer) Close() error                                             { f.p.closed++; return nil }

type fakeSMB struct {
	ops     []string
	dials   []string
	closed  int
	dialErr error
	statErr error
}

func (p *fakeSMB) Scheme() string { return "smb" }
func (p *fakeSMB) OpenURI(_ context.Context, _ vfs.VFS, uri string) (vfs.VFS, error) {
	p.dials = append(p.dials, uri)
	if p.dialErr != nil {
		return nil, p.dialErr
	}
	return fakeServer{p: p}, nil
}

func withFakeSMB(t *testing.T) *fakeSMB {
	t.Helper()
	p := &fakeSMB{}
	if err := vfs.RegisterURIProvider(p); err != nil {
		t.Fatal(err)
	}
	oldDiscover := discoverHosts
	discoverHosts = func() []string { return nil }
	resetSMBState := func() {
		closeSMBConns()
		discovery.Lock()
		discovery.at = time.Time{}
		discovery.hosts = nil
		discovery.Unlock()
		smbHosts.Lock()
		smbHosts.names = map[string]string{}
		smbHosts.Unlock()
	}
	resetSMBState()
	t.Cleanup(func() {
		discoverHosts = oldDiscover
		vfs.UnregisterURIProvider("smb")
		resetSMBState()
	})
	return p
}

func TestEnumerateSMBListsKnownServersThenShares(t *testing.T) {
	p := withFakeSMB(t)
	top, err := enumerateSMB(nil)
	if err != nil || len(top) != 0 {
		t.Fatalf("empty session: %v, %v", top, err)
	}
	RememberHost("Beta")
	RememberHost("alpha")
	RememberHost("ALPHA") // the same server
	RememberHost(" ")
	top, err = enumerateSMB(nil)
	if err != nil || len(top) != 2 || top[0].Remote != `\\alpha` || top[1].Remote != `\\Beta` || !top[0].Container {
		t.Fatalf("servers = %+v, %v", top, err)
	}
	shares, err := enumerateSMB(&top[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 2 || shares[0].Remote != `\\alpha\docs` || shares[1].Remote != `\\alpha\pub` || shares[0].Container {
		t.Fatalf("shares = %+v", shares)
	}
	if _, err := enumerateSMB(&top[0]); err != nil || len(p.dials) != 1 || p.dials[0] != "smb://alpha" {
		t.Errorf("connection not reused: dials = %v, err = %v", p.dials, err)
	}
}

func TestEnumerateSMBReportsAServerThatCannotBeReached(t *testing.T) {
	p := withFakeSMB(t)
	p.dialErr = errors.New("connection refused")
	_, err := enumerateSMB(&resource{Remote: `\\down`, Container: true})
	if err == nil || !strings.Contains(err.Error(), "down") || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("error = %v", err)
	}
	if hosts := knownHosts(); len(hosts) != 0 {
		t.Errorf("an unreachable server was remembered: %v", hosts)
	}
	if _, err := openSMBShare(`\\down\docs`).Stat(context.Background(), `\\down\docs`); err == nil {
		t.Error("Stat on an unreachable share succeeded")
	}
	p.dialErr = nil
	if _, err := enumerateSMB(&resource{Remote: `\\down`, Container: true}); err != nil {
		t.Errorf("no retry after the server came back: %v", err)
	}
}

func TestSMBShareVFSTranslatesUNCNamesAndKeepsTheConnection(t *testing.T) {
	p := withFakeSMB(t)
	v := openSMBShare(`\\srv\docs\a\b`)
	ctx := context.Background()
	if _, err := v.Stat(ctx, `\\srv\docs\a\b`); err != nil {
		t.Fatal(err)
	}
	if err := v.Rename(ctx, `\\srv\docs\a`, `\\srv\docs\c`); err != nil {
		t.Fatal(err)
	}
	if err := v.ReadDir(ctx, `\\srv\docs`, func([]vfs.VFSItem) {}); err != nil {
		t.Fatal(err)
	}
	want := "stat /docs/a/b|mv /docs/a /docs/c|readdir /docs"
	if got := strings.Join(p.ops, "|"); got != want {
		t.Errorf("server saw %q, want %q", got, want)
	}
	_ = v.Close()
	if p.closed != 0 {
		t.Error("closing a share closed the pooled connection")
	}
	if len(p.dials) != 1 {
		t.Errorf("dials = %v", p.dials)
	}
	if hosts := knownHosts(); len(hosts) != 1 || hosts[0] != "srv" {
		t.Errorf("known servers = %v", hosts)
	}
}

func TestSMBShareVFSRedialsAfterABrokenConnectionOnly(t *testing.T) {
	p := withFakeSMB(t)
	ctx := context.Background()
	p.statErr = os.ErrNotExist
	if _, err := openSMBShare(`\\srv\docs`).Stat(ctx, `\\srv\docs\x`); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if len(p.dials) != 1 || p.closed != 0 {
		t.Errorf("a missing file dropped the connection: dials %v, closed %d", p.dials, p.closed)
	}
	p.statErr = errors.New("broken pipe")
	if _, err := openSMBShare(`\\srv\docs`).Stat(ctx, `\\srv\docs\x`); err == nil {
		t.Fatal("no error")
	}
	if p.closed != 1 {
		t.Errorf("a broken connection was kept (closed %d)", p.closed)
	}
	p.statErr = nil
	if _, err := openSMBShare(`\\srv\docs`).Stat(ctx, `\\srv\docs\x`); err != nil || len(p.dials) != 2 {
		t.Errorf("no redial: dials %v, err %v", p.dials, err)
	}
}

func TestNetworkDriveOpensATypedServerOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" || !smbBuilt {
		t.Skip("servers are only guessed over SMB")
	}
	p := withFakeSMB(t)
	v := newNetworkVFS(enumerateSMB, openSMBShare)
	v.guess = guessResource
	if err := v.SetPath("/srv/docs"); err != nil {
		t.Fatal(err)
	}
	if got := v.GetPath(); got != "/srv/docs" {
		t.Errorf("path = %q", got)
	}
	if len(p.dials) != 1 {
		t.Errorf("dials = %v", p.dials)
	}
	if err := v.SetPath("/srv/nope"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a share the server lacks: %v", err)
	}
	for _, bad := range []string{`a b`, `a\b`, ``} {
		if guessResource(nil, bad) != nil {
			t.Errorf("guessResource(%q) guessed", bad)
		}
	}
	if guessResource(&resource{}, "x") != nil {
		t.Error("guessed a share inside a server")
	}
}
