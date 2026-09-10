package app

import (
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/appcmd"
	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type staticDirectActionsAITestVFS struct {
	vfs.VFS
}

func (*staticDirectActionsAITestVFS) GetTitle() string { return "ai" }

func TestStaticDirectActionsRegistered(t *testing.T) {
	for _, side := range fixedPanelSideActionSpecs {
		for _, view := range fixedPanelViewActionSpecs {
			name := "Panel." + side.id + "." + view.id
			action, ok := GetAction(name)
			if !ok {
				t.Errorf("%s is not registered", name)
				continue
			}
			if action.Area != "Shell" || action.MenuPath != side.menuPath || !action.HideFromMenu {
				t.Errorf("%s metadata = area %q, menu %q, hidden %v", name, action.Area, action.MenuPath, action.HideFromMenu)
			}
			if action.LabelKey != view.labelKey || action.DescKey != view.descKey {
				t.Errorf("%s localization = (%q, %q), want (%q, %q)", name, action.LabelKey, action.DescKey, view.labelKey, view.descKey)
			}
			if action.Visible == nil || action.Checked == nil || action.Handler == nil {
				t.Errorf("%s is missing Visible, Checked, or Handler", name)
			}
		}

		for _, sortMode := range fixedPanelSortActionSpecs {
			name := "Panel." + side.id + "." + sortMode.id
			action, ok := GetAction(name)
			if !ok {
				t.Errorf("%s is not registered", name)
				continue
			}
			if action.MenuPath != side.menuPath || !action.HideFromMenu {
				t.Errorf("%s menu metadata = (%q, %v)", name, action.MenuPath, action.HideFromMenu)
			}
			if action.LabelKey != sortMode.labelKey || action.DescKey != sortMode.descKey {
				t.Errorf("%s localization = (%q, %q), want (%q, %q)", name, action.LabelKey, action.DescKey, sortMode.labelKey, sortMode.descKey)
			}
			if action.Visible == nil || action.Checked == nil || action.Handler == nil {
				t.Errorf("%s is missing Visible, Checked, or Handler", name)
			}
		}

		for _, aiView := range fixedAIViewActionSpecs {
			name := "AI." + side.id + "." + aiView.id
			action, ok := GetAction(name)
			if !ok {
				t.Errorf("%s is not registered", name)
				continue
			}
			if action.MenuPath != side.menuPath || !action.HideFromMenu || action.Visible == nil {
				t.Errorf("%s menu/visibility metadata = (%q, %v, %v)", name, action.MenuPath, action.HideFromMenu, action.Visible != nil)
			}
			if action.LabelKey != aiView.labelKey || action.DescKey != aiView.descKey {
				t.Errorf("%s localization = (%q, %q), want (%q, %q)", name, action.LabelKey, action.DescKey, aiView.labelKey, aiView.descKey)
			}
		}
	}

	viewerGoTo, ok := GetAction("Viewer.GoTo")
	if !ok {
		t.Fatal("Viewer.GoTo is not registered")
	}
	if viewerGoTo.Area != "Viewer" || viewerGoTo.LabelKey != "KeyBar.ViewerAltF8" || len(viewerGoTo.DefaultKeys) != 1 || viewerGoTo.DefaultKeys[0] != "AltF8" {
		t.Errorf("Viewer.GoTo metadata = %#v", viewerGoTo)
	}
	if got := keymap.NewHotkeyManager("").GetAction("Viewer", "AltF8"); got != "Viewer.GoTo" {
		t.Errorf("Viewer AltF8 binding = %q, want Viewer.GoTo", got)
	}

	editorGoTo, ok := GetAction("Editor.GoTo")
	if !ok {
		t.Fatal("Editor.GoTo is not registered")
	}
	if editorGoTo.Area != "Editor" || editorGoTo.LabelKey != "KeyBar.EditorAltF8" || len(editorGoTo.DefaultKeys) != 1 || editorGoTo.DefaultKeys[0] != "AltF8" {
		t.Errorf("Editor.GoTo metadata = %#v", editorGoTo)
	}
	if got := keymap.NewHotkeyManager("").GetAction("Editor", "AltF8"); got != "Editor.GoTo" {
		t.Errorf("Editor AltF8 binding = %q, want Editor.GoTo", got)
	}

	background, ok := GetAction("App.Background")
	if !ok {
		t.Fatal("App.Background is not registered")
	}
	if background.Area != "Shell" || background.LabelKey != "FileOp.BtnBackground" || background.Handler == nil {
		t.Errorf("App.Background metadata = %#v", background)
	}

	arkanoid, ok := GetAction("App.Arkanoid")
	if !ok {
		t.Fatal("App.Arkanoid is not registered")
	}
	if arkanoid.Area != "Shell" || arkanoid.LabelKey != "Action.App.Arkanoid" ||
		arkanoid.MenuPath != "" || !arkanoid.HideFromMenu || arkanoid.Handler == nil || arkanoid.Visible == nil {
		t.Errorf("App.Arkanoid metadata = %#v", arkanoid)
	}
	if len(arkanoid.DefaultKeys) != 0 || len(arkanoid.NativeKeys) != 1 || arkanoid.NativeKeys[0] != "CtrlAltA" {
		t.Errorf("App.Arkanoid shortcuts = defaults %v, native %v", arkanoid.DefaultKeys, arkanoid.NativeKeys)
	}
}

func TestArkanoidActionKeepsPhysicalShortcutFrameworkOwned(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	previous := keymap.GlobalHotkeysMgr
	keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	t.Cleanup(func() { keymap.GlobalHotkeysMgr = previous })

	arkanoid, ok := GetAction("App.Arkanoid")
	if !ok {
		t.Fatal("App.Arkanoid is not registered")
	}
	if got := keymap.GlobalHotkeysMgr.GetAction("Shell", "CtrlAltA"); got != "" {
		t.Fatalf("CtrlAltA was installed into configurable defaults as %q", got)
	}
	if got := keymap.NativeShortcutsForAction("Shell", arkanoid); len(got) != 1 || got[0] != "Ctrl+Alt+A" {
		t.Fatalf("App.Arkanoid native shortcuts = %v", got)
	}
}

func TestFixedSideActionsUseTheAddressedPanelState(t *testing.T) {
	left := &panel.FileSystemPanel{ViewMode: panel.ViewModeBrief, SortMode: panel.SortTime}
	right := &panel.FileSystemPanel{
		Vfs: &staticDirectActionsAITestVFS{VFS: vfs.NewNullVFS(0)},
	}
	pf := &panel.PanelsFrame{Panels: [2]panel.Panel{left, right}}
	t.Cleanup(testutil.SetFrameManagerScreens(t, []*vtui.AppScreen{{Number: 1, Frames: []vtui.Frame{pf}}}, 0))

	leftBrief, _ := GetAction("Panel.Left.ViewBrief")
	leftWide, _ := GetAction("Panel.Left.ViewWide")
	rightBrief, _ := GetAction("Panel.Right.ViewBrief")
	leftSortTime, _ := GetAction("Panel.Left.SortByTime")
	leftAI, _ := GetAction("AI.Left.ViewChat")
	rightAI, _ := GetAction("AI.Right.ViewChat")

	if !leftBrief.Visible() || !leftBrief.Checked() || !leftSortTime.Checked() {
		t.Error("left regular-panel actions did not reflect the left panel")
	}
	if rightBrief.Visible() {
		t.Error("regular right-panel action is visible for an AI panel")
	}
	if leftAI.Visible() || !rightAI.Visible() {
		t.Error("fixed AI action visibility did not follow its addressed side")
	}

	pf.Wide, pf.WidePanel = true, 0
	if leftBrief.Checked() || !leftWide.Checked() {
		t.Error("wide mode did not replace the left panel's ordinary view checkmark")
	}
}

func TestCustomSideMenuCommandsResolveToRegisteredActions(t *testing.T) {
	regular := &panel.PanelsFrame{Panels: [2]panel.Panel{&panel.FileSystemPanel{}, &panel.FileSystemPanel{}}}
	aiVFS := func() vfs.VFS {
		return &staticDirectActionsAITestVFS{VFS: vfs.NewNullVFS(0)}
	}
	ai := &panel.PanelsFrame{Panels: [2]panel.Panel{
		&panel.FileSystemPanel{Vfs: aiVFS()},
		&panel.FileSystemPanel{Vfs: aiVFS()},
	}}

	menus := []vtui.MenuBarItem{
		regular.LeftMenu(), regular.RightMenu(),
		ai.LeftMenu(), ai.RightMenu(),
	}
	for _, menu := range menus {
		for _, item := range menu.SubItems {
			if item.Separator {
				continue
			}
			actionName, ok := panel.CommandToActionName[item.Command]
			if !ok {
				t.Errorf("menu %q command %d (%q) has no action mapping", menu.Label, item.Command, item.Text)
				continue
			}
			if _, ok := GetAction(actionName); !ok {
				t.Errorf("menu %q command %d maps to unregistered %q", menu.Label, item.Command, actionName)
			}
		}
	}

	wantExact := map[int]string{
		appcmd.CmLeftBrief:      "Panel.Left.ViewBrief",
		appcmd.CmRightWide:      "Panel.Right.ViewWide",
		appcmd.CmLeftSortExt:    "Panel.Left.SortByExt",
		appcmd.CmRightSortSize:  "Panel.Right.SortBySize",
		appcmd.CmLeftAIContext:  "AI.Left.ViewContext",
		appcmd.CmRightAIMem:     "AI.Right.ViewMem",
		appcmd.CmBackground:     "App.Background",
		vtui.CmQuit:             "App.Quit",
		appcmd.CmWorkspaceNew:   "Workspace.New",
		appcmd.CmWorkspaceClose: "Workspace.Close",
	}
	for command, want := range wantExact {
		if got := panel.CommandToActionName[command]; got != want {
			t.Errorf("command %d maps to %q, want %q", command, got, want)
		}
	}
}

func TestFixedSideMenuKeepsActivePanelShortcutHints(t *testing.T) {
	oldHotkeys := keymap.GlobalHotkeysMgr
	keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	defer func() { keymap.GlobalHotkeysMgr = oldHotkeys }()

	pf := &panel.PanelsFrame{
		Panels:     [2]panel.Panel{&panel.FileSystemPanel{}, &panel.FileSystemPanel{}},
		ShowPanels: true,
		MenuBar:    vtui.NewMenuBar(nil),
	}
	pf.MenuBar.Items = pf.BuildMenuItems()
	pf.UpdateMenuCheckmarks()

	for _, menuIndex := range []int{0, len(pf.MenuBar.Items) - 1} {
		for itemIndex, want := range []string{"Ctrl+1", "Ctrl+2", "Ctrl+3", "Ctrl+4"} {
			if got := pf.MenuBar.Items[menuIndex].SubItems[itemIndex].Shortcut; got != want {
				t.Errorf("menu %d view item %d shortcut = %q, want %q", menuIndex, itemIndex, got, want)
			}
		}
	}
}

func TestFixedSidePaletteEntriesUseLocalizedSideCategories(t *testing.T) {
	oldHotkeys := keymap.GlobalHotkeysMgr
	defer func() {
		keymap.GlobalHotkeysMgr = oldHotkeys
	}()
	keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")

	pf := &panel.PanelsFrame{CmdLine: cmdline.NewCommandLine(""), Panels: [2]panel.Panel{&panel.FileSystemPanel{}, &panel.FileSystemPanel{}}}
	t.Cleanup(testutil.SetFrameManagerScreens(t, []*vtui.AppScreen{{Number: 1, Frames: []vtui.Frame{pf}}}, 0))

	want := map[string]string{
		"Panel.Left.ViewBrief":   action.PlainLabel(i18n.Msg("Menu.Left")),
		"Panel.Right.SortByName": action.PlainLabel(i18n.Msg("Menu.Right")),
	}
	for _, entry := range commandPaletteActionEntries("Shell") {
		category, ok := want[entry.ID]
		if !ok {
			continue
		}
		if entry.Category != category {
			t.Errorf("%s category = %q, want %q", entry.ID, entry.Category, category)
		}
		if strings.TrimSpace(entry.Label) == "" {
			t.Errorf("%s has an empty localized label", entry.ID)
		}
		delete(want, entry.ID)
	}
	for id := range want {
		t.Errorf("%s is missing from the Shell command palette", id)
	}
}
