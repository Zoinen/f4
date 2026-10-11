package wincondrag

import (
	"testing"

	"github.com/unxed/vtui"
)

func TestPickHostWantsTheWindowUnderThePointer(t *testing.T) {
	console := Host{Handle: 1, Rect: Rect{26, 26, 1002, 545}, Visible: true}
	pseudo := Host{Handle: 2, Rect: Rect{0, 0, 0, 0}, Visible: true}
	terminal := Host{Handle: 3, Rect: Rect{0, 0, 1920, 1080}, Visible: true, Topmost: true}

	if h, ok := PickHost(100, 100, console, terminal); !ok || h.Handle != 1 {
		t.Fatalf("pointer inside conhost: got %v %v, want the console", h, ok)
	}
	// Windows Terminal: the console window is a 0x0 helper, the terminal
	// in the foreground is the host.
	if h, ok := PickHost(100, 100, pseudo, terminal); !ok || h.Handle != 3 || !h.Topmost {
		t.Fatalf("pseudoconsole: got %v %v, want the foreground terminal", h, ok)
	}
	// The field log of #1604: the pointer had already left the console
	// (1082 > 1002) when the drag began. A tool window over the console
	// would not be under it, so there is no host.
	if h, ok := PickHost(1082, 423, console, Host{}); ok {
		t.Fatalf("pointer outside every window: got %v, want none", h)
	}
	hidden := console
	hidden.Visible = false
	if _, ok := PickHost(100, 100, hidden); ok {
		t.Fatal("an invisible window cannot host the tool window")
	}
	// Right and bottom are exclusive, as in a Windows RECT.
	if _, ok := PickHost(1002, 100, console); ok {
		t.Fatal("the right edge is outside the rectangle")
	}
}

func TestEffectsRoundTrip(t *testing.T) {
	if got := ActionToEffects(vtui.DropCopy | vtui.DropMove | vtui.DropLink); got != 7 {
		t.Fatalf("all actions: got 0x%X, want 0x7", got)
	}
	if got := ActionToEffects(vtui.DropCopy); got != dropEffectCopy {
		t.Fatalf("copy: got 0x%X", got)
	}
	cases := map[uint32]vtui.DropAction{
		0:                               vtui.DropNone,
		dropEffectCopy:                  vtui.DropCopy,
		dropEffectMove | dropEffectCopy: vtui.DropMove,
		dropEffectLink:                  vtui.DropLink,
	}
	for eff, want := range cases {
		if got := EffectToAction(eff); got != want {
			t.Errorf("effect 0x%X: got %s, want %s", eff, got, want)
		}
	}
}

func TestPrimaryButtonsFollowTheSwap(t *testing.T) {
	if b := PrimaryButtons(false); b.VirtualKey != vkLButton || b.Down != mouseEventLeftDown || b.Up != mouseEventLeftUp {
		t.Fatalf("unswapped: %+v", b)
	}
	if b := PrimaryButtons(true); b.VirtualKey != vkRButton || b.Down != mouseEventRightDown || b.Up != mouseEventRightUp {
		t.Fatalf("swapped: %+v", b)
	}
}
