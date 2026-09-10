package testutil

import (
	"github.com/unxed/f4/sdk/f4rpc"
	"io"
	"testing"
)

// RPCSessionPair wires two F4-RPC sessions to each other over a pair of pipes
// and serves both: core is the side f4 owns, plugin the side a transport would
// be on. Registering a handler on one makes it callable from the other.
//
// Cleanup closes the write ends and waits for both Serve loops, so a session
// that fails to shut down fails the test that made it rather than a later one.
func RPCSessionPair(t *testing.T) (core, plugin *f4rpc.Session) {
	t.Helper()

	c2pR, c2pW := io.Pipe()
	p2cR, p2cW := io.Pipe()

	core = f4rpc.NewSession(p2cR, c2pW)
	plugin = f4rpc.NewSession(c2pR, p2cW)

	serveErrs := make(chan error, 2)
	go func() { serveErrs <- core.Serve() }()
	go func() { serveErrs <- plugin.Serve() }()
	t.Cleanup(func() {
		if err := c2pW.Close(); err != nil {
			t.Errorf("close core-to-plugin pipe: %v", err)
		}
		if err := p2cW.Close(); err != nil {
			t.Errorf("close plugin-to-core pipe: %v", err)
		}
		for range 2 {
			if err := <-serveErrs; err != nil {
				t.Errorf("serve test RPC session: %v", err)
			}
		}
	})
	return core, plugin
}
