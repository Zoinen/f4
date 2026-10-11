//go:build windows

// The netfox "wsl" site type (f4#1494): a saved connection whose transport
// is wsl_dialer_windows.go's wslFishDialer instead of SSH. A WSL target has
// no host, port, user or password of its own -- wsl.exe starts the
// distribution and speaks to it directly -- so the site configuration's
// Host field is repurposed as the distribution name ("wsl.exe -l -q"
// enumerates the choices; an empty Host dials whatever wsl.exe treats as
// the default), and Port/User/Pass/KeyPath go unused.
//
// This is deliberately the "FISH+ site with a local transport flag" shape
// from the issue, not a new wsl:// URI scheme: an ordinary FISH+ site is
// what lets it reuse everything netfox already has -- the connection
// dialog, the session pool, reconnect -- with nothing new beyond the
// dialer itself.
package netfox

import (
	"context"
	"os"
	"path"
	"strconv"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// wslPoolKeyPrefix keeps a WSL site's pool key in its own namespace so a
// distribution named the same as some unrelated SSH host never shares a
// pooled session with it. fishPoolKey has no field of its own for "this is
// a local transport", so the prefix does that job instead.
const wslPoolKeyPrefix = "wsl-local-transport:"

// wslTitle is what the panel border and folder history show for a WSL
// session -- there is no host or user to build one from the way NewFishVFS
// does.
func wslTitle(distro string) string {
	if distro == "" {
		return "WSL"
	}
	return "WSL:" + distro
}

// newWSLVFSFromPooledConn wraps a connection taken back out of the pool,
// the WSL counterpart of newFishVFSFromPooledConn (fish_pool.go). It
// deliberately leaves host/port/user unset, so FishVFS.ConnectionInfo keeps
// reporting no connection info for this session: a WSL distribution is not
// a second hop any scp-based server-to-server transfer could reach.
func newWSLVFSFromPooledConn(parent vfs.VFS, conn *fishConn, distro string) *FishVFS {
	title := wslTitle(distro)
	cwd := "/"
	if client := conn.current(); client != nil {
		if p, err := client.Pwd(context.Background()); err == nil && path.IsAbs(p) {
			cwd = p
		}
	}
	vtui.DebugLog("NET: WSL FISH+ reusing a pooled connection to %s", title)
	return &FishVFS{parent: parent, conn: conn, path: cwd, title: title}
}

// NewWSLVFS opens a WSL distribution's filesystem through a locally
// spawned wsl.exe, the way NewFishVFS opens a site over SSH. timeoutSeconds
// bounds only the handshake with the helper the dialer bootstraps; once up,
// the session is pooled like any other FISH+ connection (fish_pool.go), so
// a cold distribution's startup cost -- the issue's own honest concern --
// is paid once per idle window rather than once per operation.
func NewWSLVFS(parent vfs.VFS, distro string, timeoutSeconds int) (*FishVFS, error) {
	key := fishPoolKey{host: wslPoolKeyPrefix + distro}
	if conn := globalFishPool.take(key); conn != nil {
		return newWSLVFSFromPooledConn(parent, conn, distro), nil
	}

	title := wslTitle(distro)
	vtui.DebugLog("NET: Initiating WSL FISH+ connection via wsl.exe (distro: %q)", distro)
	ctx, cancel := context.WithTimeout(context.Background(), sshTimeout(timeoutSeconds))
	defer cancel()
	v, err := NewFishVFSOnDialer(ctx, parent, wslFishDialer(distro), title)
	if err != nil {
		return nil, err
	}
	v.conn.mu.Lock()
	v.conn.key = key
	v.conn.mu.Unlock()
	vtui.DebugLog("NET: WSL FISH+ session established, features: %s", v.client().Session().Features().Raw)
	return v, nil
}

// wslProvider implements vfs.Provider for the "wsl" site type.
type wslProvider struct{}

func (p *wslProvider) Name() string  { return "NetFox-WSL" }
func (p *wslProvider) Priority() int { return 100 }

func (p *wslProvider) CanOpen(ctx context.Context, parent vfs.VFS, pth string) bool {
	cfg, ok := netFoxConfigAt(ctx, parent, pth)
	return ok && cfg.Type == "wsl"
}

func (p *wslProvider) Open(ctx context.Context, parent vfs.VFS, pth string) (vfs.VFS, error) {
	cfg, ok := netFoxConfigAt(ctx, parent, pth)
	if !ok {
		return nil, os.ErrInvalid
	}
	timeout := 15
	if cfg.Timeout != "" {
		if t, err := strconv.Atoi(cfg.Timeout); err == nil && t > 0 {
			timeout = t
		}
	}
	// Explicit nil, not a bare return of NewWSLVFS's own result: a failed
	// dial hands back a nil *FishVFS, and returning that directly here
	// would wrap it in a non-nil vfs.VFS interface value -- the asynchronous
	// panel opener would then call Close on something that looks open but
	// is not (see TestNetFoxProvidersFailedDialReturnPlainNil).
	res, err := NewWSLVFS(parent, cfg.Host, timeout)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// wslProtocolHandler implements ProtocolHandler for the "wsl" site type.
// DefaultPort is empty because wsl.exe has no port to offer, and
// BuildExtraUI adds nothing beyond the fields the connection dialog already
// shows -- the same "nil, no-op" answer sftpProtocolHandler gives -- since
// the Host field alone (the distribution name) is all a WSL site needs.
type wslProtocolHandler struct{}

func (ph *wslProtocolHandler) Prefix() string      { return "wsl" }
func (ph *wslProtocolHandler) DefaultPort() string { return "" }
func (ph *wslProtocolHandler) BuildExtraUI(cfg *NetFoxConfig, x, y, w, h int) (vtui.UIElement, func()) {
	return nil, func() {}
}

func init() {
	vfs.RegisterProvider(&wslProvider{})
	RegisterProtocol(&wslProtocolHandler{})
}
