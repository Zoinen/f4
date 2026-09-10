package app

import (
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/vtui"
	"testing"
)

func TestIsAIPanel(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := panel.NewPanelsFrame()
	defer pf.Close()

	if panel.IsAIPanel(pf.Panels[0]) {
		t.Error("OSVFS should not be identified as AI panel")
	}
}
