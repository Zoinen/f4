package terminal

import "sync/atomic"

// clientOwnsSession is whether the client now attached started the session it
// is attached to. It is true in every process that is its own session -- the
// Windows console, the GUI, FreeBSD's attached TTY -- and on Unix the daemon
// sets it for each attach from what the client said (see attachFreshLine).
var clientOwnsSession atomic.Bool

func init() { clientOwnsSession.Store(true) }

// ClientOwnsSession reports whether the session lives only for the client
// that is attached: closing what that client opened may then end the session,
// where a client that joined a running session must leave it running.
func ClientOwnsSession() bool { return clientOwnsSession.Load() }
