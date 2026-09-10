package app

import (
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/vtui"
	"testing"
)

func TestBootstrapStartupBackendLabels(t *testing.T) {
	labels := startupBackendLabels([]string{"", "tcell"})
	if len(labels) != 2 {
		t.Fatalf("startupBackendLabels returned %d labels, want 2", len(labels))
	}
	if labels[0] == "" {
		t.Fatal("automatic backend label is empty")
	}
	if labels[1] != "tcell" {
		t.Fatalf("explicit backend label = %q, want tcell", labels[1])
	}

	autoOnly := startupBackendLabels([]string{""})
	if len(autoOnly) != 1 || autoOnly[0] != labels[0] {
		t.Fatalf("automatic label changed between calls: %q vs %q", autoOnly, labels[0])
	}
}

func TestActionStartupSettingsBuildsDialog(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.SetDefaultPalette()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	actionStartupSettings(nil)
	top := vtui.FrameManager.GetTopFrame()
	if top == nil {
		t.Fatal("actionStartupSettings did not push a dialog")
	}
	if top.GetTitle() == "" {
		t.Fatal("startup settings dialog has an empty title")
	}
	container, ok := top.(vtui.Container)
	if !ok {
		t.Fatalf("startup settings frame has type %T, want container", top)
	}
	vtui.AssertLayout(t, container)
	vtui.FrameManager.Pop()
}
