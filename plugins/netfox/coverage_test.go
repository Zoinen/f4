package netfox

import (
	"testing"
)

func TestProxyHelpersMapModesAndPadLabels(t *testing.T) {
	if got := padProxyLabel("Host"); got != "Host " {
		t.Fatalf("padProxyLabel = %q, want trailing space", got)
	}
	for i, mode := range proxyModeOrder {
		if got := proxyModeIndex(mode); got != i {
			t.Fatalf("proxyModeIndex(%d) = %d, want %d", mode, got, i)
		}
	}
	if got := proxyModeIndex(-1); got != 0 {
		t.Fatalf("unknown proxy mode index = %d, want 0", got)
	}
	if got := proxyModeItems(); len(got) != len(proxyModeOrder) {
		t.Fatalf("proxyModeItems length = %d, want %d", len(got), len(proxyModeOrder))
	}
}
