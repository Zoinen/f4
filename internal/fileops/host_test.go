package fileops

import (
	"github.com/unxed/vtui"
	"testing"
)

// The fallback is silent by design — a background operation whose seam is
// unwired simply runs headless — so the branch needs a test of its own to say
// which way it went.
func TestBackgroundScreen(t *testing.T) {
	previous := BackgroundWorkspace
	t.Cleanup(func() { BackgroundWorkspace = previous })

	workspace := vtui.NewWindow(0, 0, 10, 5, "workspace")

	BackgroundWorkspace = nil
	if got := BackgroundScreen(1); got != nil {
		t.Errorf("unwired seam returned %T, want nil so the dialog runs headless", got)
	}

	BackgroundWorkspace = func() vtui.Frame { return workspace }
	if got := BackgroundScreen(1); got != workspace {
		t.Errorf("background mode returned %v, want the workspace the seam supplies", got)
	}
	if got := BackgroundScreen(0); got != nil {
		t.Errorf("foreground mode returned %T, want nil: only mode 1 runs behind a copy", got)
	}

	// A run with no panels to copy answers nil, and that answer has to survive
	// the trip: a typed nil inside the interface would pass a != nil test and
	// put an empty screen on the frame manager.
	BackgroundWorkspace = func() vtui.Frame { return nil }
	if got := BackgroundScreen(1); got != nil {
		t.Errorf("seam with nothing to copy returned %T, want nil", got)
	}
}
