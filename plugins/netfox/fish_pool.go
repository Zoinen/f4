package netfox

import (
	"context"
	"path"
	"sync"
	"time"

	"github.com/unxed/f4/internal/netproxy"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// FishPoolIdleTimeout is how long a session is kept alive with nothing
// attached to it before it is finally closed. Reopening the same site
// within this window hands back the still-live connection instead of
// dialling and shaking hands again, so stepping out of a panel and back
// into it — the "other panel" swap, a quick trip to a local directory and
// back — costs nothing over a remote link that a fresh connection would
// have made the user wait for.
var FishPoolIdleTimeout = 15 * time.Minute

// fishPoolKey identifies a site closely enough that handing a session
// dialled for one request to another is safe. Host, port and user reach
// the same account; the proxy is part of the route there, so a session
// opened through one proxy — or none — must never be handed to a request
// that named a different one. Settings is a plain value of comparable
// fields, so this is usable as a map key as it stands.
type fishPoolKey struct {
	host, port, user string
	proxy            netproxy.Settings
}

// valid reports whether a key names a real site. The zero key, which a
// connection built on a bare stream (the test helpers, and anything else
// that has no host to reconnect to) carries, is never poolable — nothing
// tells such a connection apart from any other with no host of its own,
// and pooling it would hand one caller's session to an unrelated one.
func (k fishPoolKey) valid() bool { return k.host != "" }

type fishPoolEntry struct {
	conn  *fishConn
	timer *time.Timer
}

// fishPool holds sessions whose last view has closed but whose idle timer
// has not yet run out. A connection sits here fully alive — its keepalive
// loop keeps running, so a session parked for the ordinary few seconds
// between closing a panel and reopening it is indistinguishable, to the
// remote host, from one that was never idle at all.
type fishPool struct {
	mu      sync.Mutex
	entries map[fishPoolKey]*fishPoolEntry
}

var globalFishPool = &fishPool{entries: make(map[fishPoolKey]*fishPoolEntry)}

// take hands back a pooled connection for key, retained for the caller, or
// nil when there is none to give. Either way the key leaves the pool: a
// connection that is handed out is in use again, and one found dead has no
// business staying.
func (p *fishPool) take(key fishPoolKey) *fishConn {
	if !key.valid() {
		return nil
	}
	p.mu.Lock()
	e, ok := p.entries[key]
	if ok {
		delete(p.entries, key)
	}
	p.mu.Unlock()
	if !ok {
		return nil
	}
	e.timer.Stop()

	e.conn.mu.Lock()
	dead := e.conn.closed || e.conn.client.Session().Broken()
	if !dead {
		e.conn.refs++
	}
	e.conn.mu.Unlock()
	if dead {
		// The idle timer had not fired yet, but the far side is already
		// gone — a keepalive failure, the remote host rebooting. Finish
		// tearing it down rather than leaving it half closed.
		_ = e.conn.shutdown() // Dead pooled-session teardown is best effort.
		return nil
	}
	return e.conn
}

// park offers a connection whose last view just closed to the pool instead
// of shutting it down outright. Only ever called with refs already at
// zero, from fishConn.release.
func (p *fishPool) park(conn *fishConn, key fishPoolKey) {
	timer := time.AfterFunc(FishPoolIdleTimeout, func() {
		p.mu.Lock()
		e, ok := p.entries[key]
		if ok && e.conn == conn {
			delete(p.entries, key)
		}
		p.mu.Unlock()
		if ok {
			_ = conn.shutdown() // Idle-session teardown is best effort.
		}
	})

	p.mu.Lock()
	old, existed := p.entries[key]
	p.entries[key] = &fishPoolEntry{conn: conn, timer: timer}
	p.mu.Unlock()

	if existed {
		// Not expected in practice — a key is parked only once its refs
		// reach zero, and take() clears the entry before another view of
		// that connection can exist — but a session left behind by this
		// race is worth closing rather than leaking.
		old.timer.Stop()
		_ = old.conn.shutdown() // Superseded-session teardown is best effort.
	}
}

// closeAll tears every pooled session down without waiting for its idle
// timer, for f4's own shutdown: a connection kept alive for a reopen that
// is never coming must not outlive the process that pooled it.
func (p *fishPool) closeAll() {
	p.mu.Lock()
	entries := p.entries
	p.entries = make(map[fishPoolKey]*fishPoolEntry)
	p.mu.Unlock()
	for _, e := range entries {
		e.timer.Stop()
		_ = e.conn.shutdown() // Process-exit session teardown is best effort.
	}
}

// CloseIdlePool closes every FISH+ session the pool is holding onto. f4
// calls it once, during application shutdown.
func CloseIdlePool() {
	globalFishPool.closeAll()
}

// shutdown tears a connection down for good: stops its keepalive and
// closes the session under it. Safe to call more than once. Called from
// the pool, either when a parked connection's idle timer runs out or when
// the pool itself is closed at shutdown — never while a view still holds
// it, which is what fishConn.release and fishPool.take both arrange for
// before reaching here.
func (c *fishConn) shutdown() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()
	c.ka.Stop()
	return c.client.Session().Close()
}

// newFishVFSFromPooledConn wraps a connection taken back out of the pool in
// a fresh view, the way newFishVFSFromSession wraps a freshly dialled one.
// The working directory is asked of the remote shell again rather than
// remembered from before parking: the session is the same process, so it
// is wherever the panel that closed last left it, which is worth keeping
// rather than resetting to "/".
func newFishVFSFromPooledConn(parent vfs.VFS, conn *fishConn, host, port, user string) *FishVFS {
	title := host
	if user != "" {
		title = user + "@" + host
	}
	cwd := "/"
	if client := conn.current(); client != nil {
		if p, err := client.Pwd(context.Background()); err == nil && path.IsAbs(p) {
			cwd = p
		}
	}
	vtui.DebugLog("NET: FISH+ reusing a pooled connection to %s", title)
	return &FishVFS{
		parent: parent,
		conn:   conn,
		path:   cwd,
		title:  title,
		host:   host,
		port:   port,
		user:   user,
	}
}
