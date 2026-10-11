//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package vtui

import (
	"encoding/binary"
	"testing"

	"github.com/jezek/xgb/xproto"
)

func netWMStateValue(atoms ...xproto.Atom) []byte {
	v := make([]byte, 4*len(atoms))
	for i, a := range atoms {
		binary.LittleEndian.PutUint32(v[4*i:], uint32(a))
	}
	return v
}

// The toggle restores only a window the window manager lists as maximized in
// both directions.
func TestNetWMStateMaximized(t *testing.T) {
	const vert, horz, fullscreen, focused xproto.Atom = 301, 302, 303, 304
	tests := []struct {
		name  string
		value []byte
		want  bool
	}{
		{"empty", nil, false},
		{"both", netWMStateValue(vert, horz), true},
		{"both among others", netWMStateValue(focused, horz, fullscreen, vert), true},
		{"vertical only", netWMStateValue(vert, focused), false},
		{"horizontal only", netWMStateValue(horz), false},
		{"trailing partial atom", append(netWMStateValue(vert, horz), 1, 2), true},
	}
	for _, tt := range tests {
		if got := netWMStateMaximized(tt.value, vert, horz); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
	var _ windowMaximizer = (*X11Renderer)(nil)
}
