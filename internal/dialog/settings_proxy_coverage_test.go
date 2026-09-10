package dialog

import (
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
)

func TestActionProxySettingsBuildsDialog(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(120, 30)
	vtui.FrameManager.Init(screen)

	ActionProxySettings()
	if vtui.FrameManager.GetTopFrame() == nil {
		t.Fatal("proxy settings did not push a dialog")
	}
}
