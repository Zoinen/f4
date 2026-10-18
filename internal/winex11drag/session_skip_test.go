//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package winex11drag

import (
	"testing"

	"github.com/jezek/xgb/xproto"
)

// TestSameClientSkip: ids from the client that owns the sample window are
// skipped, other clients' and the session's own are not.
func TestSameClientSkip(t *testing.T) {
	const mask = 0x1fffff
	wine := xproto.Window(0xa00001)
	ours := xproto.Window(0xc00000)

	skip := sameClientSkip(mask, wine, ours)
	if skip == nil {
		t.Fatal("no predicate for a foreign client")
	}
	for _, w := range []xproto.Window{0xa00001, 0xa00042, 0xa1ffff} {
		if !skip(w) {
			t.Errorf("window %#x of the sampled client was not skipped", w)
		}
	}
	for _, w := range []xproto.Window{0x600001, 0xc00000, 0xc00007, 0x1e} {
		if skip(w) {
			t.Errorf("window %#x of another client was skipped", w)
		}
	}
}

// TestSameClientSkipNeverOwnClient: when the window under the pointer is the
// session's own, nothing is skipped.
func TestSameClientSkipNeverOwnClient(t *testing.T) {
	if skip := sameClientSkip(0x1fffff, 0xc00005, 0xc00000); skip != nil {
		t.Fatal("the session's own client must not be skipped")
	}
}
