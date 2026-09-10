package app

import (
	"context"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/vtui"
	"path/filepath"
	"testing"
)

func TestPlugRingDialog_Layout(t *testing.T) {
	// The dialog refreshes its catalog on a goroutine that outlives this test,
	// and plughost.FetchCatalog reads plughost.PlugRingCatalogURL, which
	// TestFetchCatalog_Success writes. This test wants the layout, not the
	// catalog, so it hands the dialog a fetch that touches neither.
	previousCatalog := plugRingCatalog
	plugRingCatalog = func(context.Context) ([]plughost.PlugRingItem, error) { return nil, nil }
	t.Cleanup(func() { plugRingCatalog = previousCatalog })

	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.SetDefaultPalette()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := panel.NewPanelsFrame()
	t.Cleanup(pf.Close)

	ActionPlugRing(pf)

	top := vtui.FrameManager.GetTopFrame()
	t.Cleanup(func() {
		if top != nil {
			top.Close()
			vtui.FrameManager.RemoveFrame(top)
		}
	})
	dlg, ok := top.(vtui.Container)
	if !ok {
		t.Fatalf("Expected vtui.Container, got %T", top)
	}

	vtui.AssertLayout(t, dlg)
}

func TestPlugRingRowColorsFollowDialogTheme(t *testing.T) {
	vtui.SetDefaultPalette()
	oldHighlight := vtui.Palette[vtui.ColDialogHighlightText]
	oldText := vtui.Palette[vtui.ColDialogText]
	t.Cleanup(func() {
		vtui.Palette[vtui.ColDialogHighlightText] = oldHighlight
		vtui.Palette[vtui.ColDialogText] = oldText
	})

	vtui.Palette[vtui.ColDialogHighlightText] = vtui.SetRGBFore(oldHighlight, 0x123456)
	vtui.Palette[vtui.ColDialogText] = vtui.SetRGBFore(oldText, 0xABCDEF)
	selected := vtui.SetRGBBoth(0, 0xFFFFFF, 0x654321)

	header := plugRingRow{header: "Category"}.GetCellAttr(0, selected)
	if got := vtui.GetRGBFore(header); got != 0x123456 {
		t.Fatalf("header foreground = %#x, want theme foreground", got)
	}
	if got := vtui.GetRGBBack(header); got != 0x654321 {
		t.Fatalf("header background = %#x, want selected background", got)
	}

	installed := plugRingRow{status: "Installed"}.GetCellAttr(0, selected)
	if got := vtui.GetRGBFore(installed); got != 0xABCDEF {
		t.Fatalf("installed foreground = %#x, want dialog text foreground", got)
	}
}

// The ID names the directory install replaces and remove deletes, and it comes
// from a remote catalog. Anything that resolves outside its own element, or to
// the plugring directory itself, has to be refused.
func TestSafePlugRingIDRejectsAnythingThatEscapesItsElement(t *testing.T) {
	for _, id := range []string{
		"",
		".",
		"..",
		"../evil",
		"a/b",
		`a\b`,
		"a\x00b",
		"./a",
		"a/",
	} {
		if safePlugRingID(id) {
			t.Errorf("accepted %q, which resolves to %q", id, filepath.Join("plugring", id))
		}
	}
}

func TestSafePlugRingIDAcceptsOrdinaryNames(t *testing.T) {
	for _, id := range []string{"netfox", "cloud-fox", "media_info", "plugin.v2", ".hidden"} {
		if !safePlugRingID(id) {
			t.Errorf("rejected %q, which is a plain directory name", id)
		}
	}
}
