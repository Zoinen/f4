package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestKeyBarSemanticRefreshCombinesIconsAndDisabledRows(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	fm.KeyBar = NewKeyBar()
	keys := &KeySet{}
	keys.Normal[0], keys.NormalIcons[0] = "Help", "help"
	keys.CtrlShift[0], keys.CtrlShiftIcons[0] = "Compare", "compare"
	keys.CtrlShiftDisabled[0] = true
	frame := &labelFrame{mockFrame: *newMockFrame(0, 0, 79, 23, false), labels: keys}
	fm.Push(frame)
	fm.refreshKeyBarState()
	fm.KeyBar.LatchModifiers(true, true, false)
	node := semanticKeyBar(fm.KeyBar)
	item := node["items"].([]map[string]any)[0]
	if node["modifier"] != "ctrl+shift" || item["text"] != "Compare" || item["icon"] != "compare" || item["disabled"] != true {
		t.Fatalf("combined native row lost metadata: %#v / %#v", node["modifier"], item)
	}
	keys.CtrlShift = KeyBarLabels{}
	keys.Shift[0], keys.ShiftIcons[0] = "View", "view"
	fm.refreshKeyBarState()
	node = semanticKeyBar(fm.KeyBar)
	item = node["items"].([]map[string]any)[0]
	if node["modifier"] != "shift" || item["icon"] != "view" || item["disabled"] != false {
		t.Fatalf("fallback kept combined metadata: %#v", item)
	}
}

func TestVMenuFilteredReplacementPreservesLazyChildAndSelection(t *testing.T) {
	previous := FrameManager
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	FrameManager = fm
	t.Cleanup(func() { FrameManager = previous })
	fm.Push(NewDesktop())
	menu := NewVMenu("Root")
	menu.SetPosition(10, 5, 30, 12)
	lazy := MenuItem{ID: "keep", Text: "Keep", Submenu: func() *VMenu {
		child := NewVMenu("Child")
		child.AddItem(MenuItem{Text: "Leaf"})
		return child
	}}
	menu.ReplaceItems([]MenuItem{{ID: "hidden", Text: "Hidden"}, lazy})
	fm.PushMenu(menu)
	menu.FilterOnType = true
	menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'k'})
	if !menu.OpenSubMenu(1) {
		t.Fatal("upstream spelling failed to open fork lazy submenu")
	}
	child := menu.childMenu
	_, y, _, _ := child.GetPosition()
	if y != menu.Y1+menu.MarginTop {
		t.Fatalf("filtered child anchored at y=%d instead of visible row", y)
	}
	menu.ReplaceItems([]MenuItem{{ID: "hidden", Text: "Hidden"}, lazy})
	if menu.childMenu != child || menu.SelectPos != 1 {
		t.Fatal("replacement lost stable selection or child")
	}
	menu.ReplaceItems([]MenuItem{{ID: "keep", Text: "Hidden", Submenu: lazy.Submenu}, {ID: "new", Text: "Keep new"}})
	if menu.SelectPos != 1 || menu.childMenu != nil || !child.IsDone() {
		t.Fatalf("filter retained hidden child/selection: selected=%d child=%p", menu.SelectPos, menu.childMenu)
	}
	menu.CloseChain()
	if menu.FilterText() != "" {
		t.Fatal("closing dynamic chain retained the upstream filter")
	}
}

func TestSemanticHelpPreservesEscapedMarkup(t *testing.T) {
	engine := NewHelpEngine(nil)
	engine.AddTopic(&HelpTopic{Name: "Contents", Lines: []string{"\x10# \x10~ ^"}})
	view := NewHelpView(engine, "Contents")
	view.ResizeConsole(80, 25)
	node := view.SemanticNode(nil)
	rows := node["helpLines"].([]map[string]any)
	spans := rows[0]["spans"].([]map[string]any)
	if len(spans) != 1 || spans[0]["text"] != "# ~ ^" || spans[0]["bold"] != false {
		t.Fatalf("escaped HLF text changed in native rendering: %#v", spans)
	}
}
