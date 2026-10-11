package netbrowse

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
)

// The network off Windows (f4#1702, part 3): there is no system call that
// lists it, so the "servers" are the SMB hosts this session has reached (by
// smb:// or \\host, or by typing a host name in the Network drive), and the
// shares of a server are what it reports over SMB. Every level goes through
// the smb:// provider the NetFox plugin registers, so this file needs no
// protocol code, and what smb:// can log on to, the network browser can list.

// smbHosts is the set of SMB servers known to this session.
var smbHosts = struct {
	sync.Mutex
	names map[string]string // lower-case name -> name as first written
}{names: map[string]string{}}

// RememberHost adds host to the servers the Network view lists at its top.
func RememberHost(host string) {
	host = strings.TrimSpace(host)
	if host == "" {
		return
	}
	smbHosts.Lock()
	defer smbHosts.Unlock()
	key := strings.ToLower(host)
	if _, ok := smbHosts.names[key]; !ok {
		smbHosts.names[key] = host
	}
}

func knownHosts() []string {
	smbHosts.Lock()
	defer smbHosts.Unlock()
	out := make([]string, 0, len(smbHosts.names))
	for _, h := range smbHosts.names {
		out = append(out, h)
	}
	return sortedHosts(out)
}

func sortedHosts(out []string) []string {
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

const smbDialTimeout = 20 * time.Second

var errNoSMB = errors.New("SMB is not available in this build")

// openSMBHost opens smb://host through the registered provider (the anonymous
// logon: a share that needs a login is opened by its smb://user:password@host
// address instead).
func openSMBHost(host string) (vfs.VFS, error) {
	provider := vfs.FindURIProvider("smb://")
	if provider == nil {
		return nil, errNoSMB
	}
	ctx, cancel := context.WithTimeout(context.Background(), smbDialTimeout)
	defer cancel()
	return provider.OpenURI(ctx, nil, "smb://"+host)
}

// splitUNC splits \\host\share\rest.
func splitUNC(unc string) (host, rest string) {
	unc = strings.TrimLeft(unc, `\`)
	host, rest, _ = strings.Cut(unc, `\`)
	return host, rest
}

// mergeHosts is the union of two host lists, without repeats that differ only
// in case, in name order.
func mergeHosts(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, h := range append(append([]string(nil), a...), b...) {
		if k := strings.ToLower(h); !seen[k] {
			seen[k] = true
			out = append(out, h)
		}
	}
	return sortedHosts(out)
}

// smbConns holds one connection per server, shared by every request for it
// until it fails.
var smbConns = struct {
	sync.Mutex
	byHost map[string]vfs.VFS
}{byHost: map[string]vfs.VFS{}}

func smbConn(host string) (vfs.VFS, error) {
	key := strings.ToLower(host)
	smbConns.Lock()
	defer smbConns.Unlock()
	if v, ok := smbConns.byHost[key]; ok {
		return v, nil
	}
	v, err := openSMBHost(host)
	if err != nil {
		return nil, err
	}
	smbConns.byHost[key] = v
	RememberHost(host)
	return v, nil
}

// dropSMBConn forgets (and closes) a connection that has failed, so the next
// request dials again.
func dropSMBConn(host string, v vfs.VFS) {
	key := strings.ToLower(host)
	smbConns.Lock()
	if smbConns.byHost[key] == v {
		delete(smbConns.byHost, key)
	}
	smbConns.Unlock()
	_ = v.Close()
}

// closeSMBConns closes every open server connection (plugin shutdown).
func closeSMBConns() {
	smbConns.Lock()
	conns := smbConns.byHost
	smbConns.byHost = map[string]vfs.VFS{}
	smbConns.Unlock()
	for _, v := range conns {
		_ = v.Close()
	}
}

// enumerateSMB lists the network's top (the known servers) or one server's
// shares.
func enumerateSMB(parent *resource) ([]resource, error) {
	if parent == nil {
		hosts := mergeHosts(knownHosts(), discoveredHosts())
		out := make([]resource, 0, len(hosts))
		for _, h := range hosts {
			out = append(out, resource{Remote: `\\` + h, Provider: "SMB", Display: displayServer, Container: true})
		}
		return out, nil
	}
	host, _ := splitUNC(parent.Remote)
	v, err := smbConn(host)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", host, err)
	}
	var shares []resource
	err = v.ReadDir(context.Background(), "/", func(items []vfs.VFSItem) {
		for _, it := range items {
			// Administrative shares (C$, IPC$) are left out, as Explorer does.
			if it.Name == ".." || strings.HasSuffix(it.Name, "$") {
				continue
			}
			shares = append(shares, resource{Remote: `\\` + host + `\` + it.Name, Provider: "SMB", Display: displayShare})
		}
	})
	if err != nil {
		dropSMBConn(host, v)
		return nil, fmt.Errorf("%s: %w", host, err)
	}
	return shares, nil
}

// smbShareVFS is a server's SMB file system seen through UNC names: every
// method that takes a path takes \\host\share\dir and hands the server
// /share/dir.
type smbShareVFS struct {
	vfs.VFS
	host string
}

// openSMBShare is the file system at a share's UNC name.
func openSMBShare(unc string) vfs.VFS {
	host, _ := splitUNC(unc)
	v, err := smbConn(host)
	if err != nil {
		return failedVFS{err: err}
	}
	return smbShareVFS{VFS: v, host: host}
}

// smbPath turns \\host\share\a\b into /share/a/b.
func smbPath(unc string) string {
	_, rest := splitUNC(unc)
	return "/" + strings.ReplaceAll(rest, `\`, "/")
}

// note drops the pooled connection when err says it is broken rather than
// that the file is missing or not allowed, so the next request dials again.
func (s smbShareVFS) note(err error) error {
	if err != nil && !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, fs.ErrPermission) && !errors.Is(err, fs.ErrExist) {
		dropSMBConn(s.host, s.VFS)
	}
	return err
}

func (s smbShareVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	return s.note(s.VFS.ReadDir(ctx, smbPath(p), onChunk))
}
func (s smbShareVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	item, err := s.VFS.Stat(ctx, smbPath(p))
	return item, s.note(err)
}
func (s smbShareVFS) MkDir(ctx context.Context, p string) error {
	return s.note(s.VFS.MkDir(ctx, smbPath(p)))
}
func (s smbShareVFS) Remove(ctx context.Context, p string) error {
	return s.note(s.VFS.Remove(ctx, smbPath(p)))
}
func (s smbShareVFS) Rename(ctx context.Context, a, b string) error {
	return s.note(s.VFS.Rename(ctx, smbPath(a), smbPath(b)))
}
func (s smbShareVFS) SetAttributes(ctx context.Context, p string, it vfs.VFSItem) error {
	return s.note(s.VFS.SetAttributes(ctx, smbPath(p), it))
}
func (s smbShareVFS) Open(ctx context.Context, p string) (vfs.ReadAtCloser, error) {
	f, err := s.VFS.Open(ctx, smbPath(p))
	return f, s.note(err)
}
func (s smbShareVFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	w, err := s.VFS.Create(ctx, smbPath(p))
	return w, s.note(err)
}

// failedVFS is what a share that could not be reached opens as: every
// operation reports why.
type failedVFS struct {
	vfs.VFS
	err error
}

func (f failedVFS) ReadDir(context.Context, string, func([]vfs.VFSItem)) error { return f.err }
func (f failedVFS) Stat(context.Context, string) (vfs.VFSItem, error) {
	return vfs.VFSItem{}, f.err
}
func (f failedVFS) MkDir(context.Context, string) error                      { return f.err }
func (f failedVFS) Remove(context.Context, string) error                     { return f.err }
func (f failedVFS) Rename(context.Context, string, string) error             { return f.err }
func (f failedVFS) SetAttributes(context.Context, string, vfs.VFSItem) error { return f.err }
func (f failedVFS) Open(context.Context, string) (vfs.ReadAtCloser, error) {
	return nil, f.err
}
func (f failedVFS) Create(context.Context, string) (io.WriteCloser, error) { return nil, f.err }

// Close leaves the shared connection open: it belongs to the pool.
func (s smbShareVFS) Close() error { return nil }
