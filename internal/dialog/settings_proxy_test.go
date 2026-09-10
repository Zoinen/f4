package dialog

import (
	"testing"
)

func TestProxyModeItems(t *testing.T) {
	items := proxyModeItems()
	if len(items) != len(proxyModeOrder) {
		t.Fatalf("proxyModeItems() returned %d items, want %d", len(items), len(proxyModeOrder))
	}
}

func TestProxyModeIndex(t *testing.T) {
	for want, mode := range proxyModeOrder {
		if got := proxyModeIndex(mode); got != want {
			t.Errorf("proxyModeIndex(%d) = %d, want %d", mode, got, want)
		}
	}
	if got := proxyModeIndex(-1); got != 0 {
		t.Errorf("proxyModeIndex(unknown) = %d, want 0", got)
	}
}

func TestPadProxyLabel(t *testing.T) {
	if got := padProxyLabel("Mode"); got != "Mode " {
		t.Errorf("padProxyLabel() = %q, want %q", got, "Mode ")
	}
}
