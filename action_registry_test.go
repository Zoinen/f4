package main

import (
	"testing"

	"github.com/unxed/vtui"
)

func TestActionRegistry(t *testing.T) {
	called := false
	testAction := Action{
		Name:        "Test.Action",
		Label:       "Test Label",
		Description: "Test Description",
		Handler: func() bool {
			called = true
			return true
		},
	}

	RegisterAction(testAction)

	// Test GetAction
	a, ok := GetAction("test.action")
	if !ok {
		t.Fatal("Expected to find Test.Action")
	}
	if a.Label != "Test Label" || a.Description != "Test Description" {
		t.Errorf("Action fields mismatch. Got %+v", a)
	}

	// Test GetActions
	actions := GetActions()
	found := false
	for _, act := range actions {
		if act.Name == "Test.Action" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Action not found in GetActions() result")
	}

	// Test RunAction
	if !RunAction("Test.action") {
		t.Error("RunAction failed")
	}
	if !called {
		t.Error("Action handler was not executed")
	}

	// Test missing action
	if RunAction("Missing.Action") {
		t.Error("RunAction should return false for missing action")
	}
}

func TestRunActionRequestsRedrawForConsumedAction(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	defer vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	RegisterAction(Action{
		Name:    "Test.ConsumedRedraw",
		Handler: func() bool { return true },
	})
	for {
		select {
		case <-vtui.FrameManager.RedrawChan:
		default:
			goto drained
		}
	}

drained:
	if !RunAction("Test.ConsumedRedraw") {
		t.Fatal("RunAction did not execute the test action")
	}
	select {
	case <-vtui.FrameManager.RedrawChan:
	default:
		t.Fatal("successful action did not request a redraw")
	}
}

func TestPanelViewIconsActionChangesLayoutAndRequestsRedraw(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame()
	t.Cleanup(func() {
		pf.Close()
		vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	})
	pf.activeIdx = 0
	vtui.FrameManager.Push(pf)

	for {
		select {
		case <-vtui.FrameManager.RedrawChan:
		default:
			goto drained
		}
	}

drained:
	fsp := pf.panels[0].(*FileSystemPanel)
	if !RunAction("Panel.ViewIcons") {
		t.Fatal("Panel.ViewIcons did not run")
	}
	if got := fsp.effectiveGalleryLayoutMode(); got != GalleryLayoutIcons {
		t.Fatalf("Panel.ViewIcons selected %q, want %q", got, GalleryLayoutIcons)
	}
	select {
	case <-vtui.FrameManager.RedrawChan:
	default:
		t.Fatal("Panel.ViewIcons did not request a redraw")
	}
}

func TestRegistry_HexModeAndWorkspaceActions(t *testing.T) {
	a, ok := GetAction("Editor.HexMode")
	if !ok {
		t.Fatal("Editor.HexMode should be registered")
	}
	if a.MenuPath != "Options" {
		t.Errorf("MenuPath = %q, want Options", a.MenuPath)
	}

	_, ok = GetAction("Workspace.New")
	if !ok {
		t.Fatal("Workspace.New should be registered")
	}
}

func TestHotkeyManager_BookmarksDefault(t *testing.T) {
	hm := NewHotkeyManager("")
	hm.initDefaults()

	if got := hm.GetAction("Shell", "CtrlShiftVK_DC"); got != "Panel.Bookmarks" {
		t.Fatalf("Shell/CtrlShiftVK_DC = %q, want Panel.Bookmarks", got)
	}
}

func TestHotkeyManager_PanelPathDefaults(t *testing.T) {
	hm := NewHotkeyManager("")
	hm.initDefaults()

	if got := hm.GetAction("Shell", "CtrlD"); got != "Panel.CopyPath" {
		t.Fatalf("Shell/CtrlD = %q, want Panel.CopyPath", got)
	}
	if got := hm.GetAction("Shell", "CtrlF"); got != "Panel.InsertPath" {
		t.Fatalf("Shell/CtrlF = %q, want Panel.InsertPath", got)
	}
}

func TestHotkeyManager_ViewerEditorSearchDirections(t *testing.T) {
	hm := NewHotkeyManager("")
	hm.initDefaults()

	cases := []struct {
		area, key, want string
	}{
		{"Editor", "CtrlEnter", "Editor.SearchForward"},
		{"Editor", "CtrlShiftEnter", "Editor.SearchPrevious"},
		{"Viewer", "CtrlEnter", "Viewer.SearchNext"},
		{"Viewer", "CtrlShiftEnter", "Viewer.SearchPrevious"},
	}
	for _, tc := range cases {
		if got := hm.GetAction(tc.area, tc.key); got != tc.want {
			t.Errorf("%s/%s = %q, want %q", tc.area, tc.key, got, tc.want)
		}
	}
}

func TestAction_PanelToggleHidden(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)

	original := AppConfig.ShowHiddenFiles
	defer func() { AppConfig.ShowHiddenFiles = original }()

	if !RunAction("Panel.ToggleHidden") {
		t.Fatal("Panel.ToggleHidden did not run")
	}
	if AppConfig.ShowHiddenFiles == original {
		t.Errorf("Panel.ToggleHidden did not flip ShowHiddenFiles (was %v, still %v)", original, AppConfig.ShowHiddenFiles)
	}

	if !RunAction("Panel.ToggleHidden") {
		t.Fatal("Panel.ToggleHidden did not run on second call")
	}
	if AppConfig.ShowHiddenFiles != original {
		t.Errorf("Panel.ToggleHidden second call did not restore ShowHiddenFiles (want %v, got %v)", original, AppConfig.ShowHiddenFiles)
	}
}

func TestActionPanelToggleTargetsActiveWorkspace(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	t.Cleanup(func() { vtui.FrameManager.Init(vtui.NewSilentScreenBuf()) })

	first := &PanelsFrame{showPanels: true, showLeftPanel: true, showRightPanel: true}
	active := &PanelsFrame{showPanels: true, showLeftPanel: true, showRightPanel: true}
	vtui.FrameManager.Screens = []*vtui.AppScreen{
		{Number: 1, Frames: []vtui.Frame{first}},
		{Number: 2, Frames: []vtui.Frame{active}},
	}
	vtui.FrameManager.ActiveIdx = 1

	if !RunAction("Panel.Toggle") {
		t.Fatal("Panel.Toggle did not run")
	}
	if active.showPanels {
		t.Fatal("Panel.Toggle did not hide panels in the active workspace")
	}
	if !first.showPanels {
		t.Fatal("Panel.Toggle changed panels in the first, inactive workspace")
	}
}

func TestPanelViewShortcutsReserveCtrl4ForWide(t *testing.T) {
	want := map[string]string{
		"Panel.ViewMedium":   "Ctrl1",
		"Panel.ViewBrief":    "Ctrl2",
		"Panel.ViewDetailed": "Ctrl3",
		"Panel.ViewWide":     "Ctrl4",
		"Panel.ViewIcons":    "Ctrl5",
		"Panel.ViewGrid":     "Ctrl6",
		"Panel.ViewGallery":  "Ctrl7",
	}
	for name, key := range want {
		action, ok := GetAction(name)
		if !ok {
			t.Fatalf("missing action %s", name)
		}
		if len(action.DefaultKeys) != 1 || action.DefaultKeys[0] != key {
			t.Errorf("%s keys = %v, want [%s]", name, action.DefaultKeys, key)
		}
	}
}
