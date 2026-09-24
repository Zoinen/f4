package app

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/vtui"
)

func compareDialogCheckbox(t *testing.T, dlg *vtui.Window, text string) *vtui.Checkbox {
	t.Helper()
	var found *vtui.Checkbox
	walkUI(dlg, func(el vtui.UIElement) bool {
		if checkbox, ok := el.(*vtui.Checkbox); ok && checkbox.GetText() == text {
			found = checkbox
			return false
		}
		return true
	})
	if found == nil {
		t.Fatalf("dialog has no checkbox %q", text)
	}
	return found
}

func compareDialogRadioGroup(t *testing.T, dlg *vtui.Window) *vtui.RadioGroup {
	t.Helper()
	var found *vtui.RadioGroup
	walkUI(dlg, func(el vtui.UIElement) bool {
		if radio, ok := el.(*vtui.RadioGroup); ok {
			found = radio
			return false
		}
		return true
	})
	if found == nil {
		t.Fatal("dialog has no ignore-mode radio group")
	}
	return found
}

func compareDialogEdit(t *testing.T, dlg *vtui.Window) *vtui.Edit {
	t.Helper()
	var found *vtui.Edit
	walkUI(dlg, func(el vtui.UIElement) bool {
		if edit, ok := el.(*vtui.Edit); ok {
			found = edit
			return false
		}
		return true
	})
	if found == nil {
		t.Fatal("dialog has no maximum-depth edit")
	}
	return found
}

func compareDialogButton(t *testing.T, dlg *vtui.Window, defaultButton bool) *vtui.Button {
	t.Helper()
	var found *vtui.Button
	walkUI(dlg, func(el vtui.UIElement) bool {
		if button, ok := el.(*vtui.Button); ok && button.IsDefault == defaultButton {
			found = button
			return false
		}
		return true
	})
	if found == nil {
		t.Fatalf("dialog has no button with IsDefault=%v", defaultButton)
	}
	return found
}

func TestCompareFoldersDialogOptionDependenciesAndSubmit(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.SetDefaultPalette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(140, 35)
	vtui.FrameManager.Init(scr)

	oldConfig := config.App
	t.Cleanup(func() { config.App = oldConfig })
	config.App.AutoSaveDialogSettings = false
	config.App.Compare = config.CompareOptions{
		Recursive:   false,
		LimitDepth:  true,
		MaxDepth:    7,
		MarkedOnly:  true,
		ByTime:      false,
		TimeSlack:   true,
		IgnoreZones: true,
		BySize:      false,
		ByContent:   false,
		Ignore:      true,
		IgnoreMode:  config.CompareIgnoreSpaces,
		ReportEqual: true,
	}

	ShowCompareFoldersDialog(&panel.PanelsFrame{})
	dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want compare dialog", vtui.FrameManager.GetTopFrame())
	}
	vtui.AssertLayout(t, dlg)
	if !dlg.ShowClose {
		t.Fatal("compare dialog does not expose its close button")
	}

	cbRecursive := compareDialogCheckbox(t, dlg, i18n.Msg("Compare.Recursive"))
	cbDepth := compareDialogCheckbox(t, dlg, i18n.Msg("Compare.MaxDepth"))
	cbTime := compareDialogCheckbox(t, dlg, i18n.Msg("Compare.ByTime"))
	cbSlack := compareDialogCheckbox(t, dlg, i18n.Msg("Compare.TimeSlack"))
	cbZones := compareDialogCheckbox(t, dlg, i18n.Msg("Compare.IgnoreZones"))
	cbContent := compareDialogCheckbox(t, dlg, i18n.Msg("Compare.ByContent"))
	cbIgnore := compareDialogCheckbox(t, dlg, i18n.Msg("Compare.Ignore"))
	cbReport := compareDialogCheckbox(t, dlg, i18n.Msg("Compare.ReportEqual"))
	edDepth := compareDialogEdit(t, dlg)
	rgIgnore := compareDialogRadioGroup(t, dlg)

	if cbRecursive.State != 0 || !cbDepth.IsDisabled() || !edDepth.IsDisabled() {
		t.Fatalf("recursive dependency = state %d, depth disabled %v, edit disabled %v", cbRecursive.State, cbDepth.IsDisabled(), edDepth.IsDisabled())
	}
	if !cbSlack.IsDisabled() || !cbZones.IsDisabled() || !cbIgnore.IsDisabled() || !rgIgnore.IsDisabled() {
		t.Fatalf("inactive compare dependencies were left enabled")
	}
	if cbReport.State != 1 || rgIgnore.Selected != 1 || edDepth.GetText() != "7" {
		t.Fatalf("initial values = report %d, ignore mode %d, depth %q", cbReport.State, rgIgnore.Selected, edDepth.GetText())
	}

	// With every criterion off, OK must keep the dialog open and show the
	// validation message instead of starting a name-only comparison.
	compareDialogButton(t, dlg, true).OnClick()
	if dlg.IsDone() {
		t.Fatal("OK closed the dialog without a comparison criterion")
	}
	message := vtui.FrameManager.GetTopFrame()
	if message == nil || message == dlg {
		t.Fatal("missing no-criteria validation message")
	}
	message.Close()
	vtui.FrameManager.RemoveFrame(message)

	cbRecursive.Toggle()
	if cbDepth.IsDisabled() || edDepth.IsDisabled() {
		t.Fatal("enabling recursion did not enable the depth controls")
	}
	cbDepth.Toggle()
	if !edDepth.IsDisabled() {
		t.Fatal("disabling depth limiting left the depth edit enabled")
	}
	cbDepth.Toggle()
	if edDepth.IsDisabled() {
		t.Fatal("re-enabling depth limiting left the depth edit disabled")
	}
	cbTime.Toggle()
	if cbSlack.IsDisabled() || cbZones.IsDisabled() {
		t.Fatal("enabling time comparison did not enable its sub-options")
	}
	cbContent.Toggle()
	if cbIgnore.IsDisabled() || rgIgnore.IsDisabled() {
		t.Fatal("enabling content comparison did not enable ignore options")
	}
	cbIgnore.Toggle()
	if !rgIgnore.IsDisabled() {
		t.Fatal("disabling content filtering left its radio group enabled")
	}

	compareDialogButton(t, dlg, true).OnClick()
	if !dlg.IsDone() {
		t.Fatal("valid OK did not close the compare dialog")
	}
	if got := config.App.Compare; !got.Recursive || !got.LimitDepth || got.MaxDepth != 7 || !got.ByTime || !got.TimeSlack || !got.IgnoreZones || !got.ByContent || got.Ignore || got.IgnoreMode != config.CompareIgnoreSpaces || !got.ReportEqual {
		t.Fatalf("saved compare options = %#v", got)
	}
	if message = vtui.FrameManager.GetTopFrame(); message == nil || message == dlg {
		t.Fatal("valid submit did not report missing panels")
	}
	message.Close()
	vtui.FrameManager.RemoveFrame(message)
	vtui.FrameManager.RemoveFrame(dlg)

	compareDialogButton(t, dlg, false).OnClick()
	if !dlg.IsDone() {
		t.Fatal("Cancel did not close the compare dialog")
	}
}

func TestCompareFoldersGuards(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	if panelCanCompareFolders() {
		t.Fatal("empty frame manager reported comparable panels")
	}
	ShowCompareFoldersDialog(nil)
	if top := vtui.FrameManager.GetTopFrame(); top != nil {
		t.Fatalf("nil panels frame opened a dialog: %T", top)
	}
	runCompareFolders(nil, config.CompareOptions{})
	if top := vtui.FrameManager.GetTopFrame(); top != nil {
		t.Fatalf("nil compare frame opened a dialog: %T", top)
	}

	runCompareFolders(&panel.PanelsFrame{}, config.CompareOptions{})
	top := vtui.FrameManager.GetTopFrame()
	if top == nil {
		t.Fatal("empty panels frame did not report missing panels")
	}
	top.Close()
	vtui.FrameManager.RemoveFrame(top)
}
