package settings

import (
	"testing"

	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestFileGroupChildSinglePageContainsEverySection(t *testing.T) {
	_, view := setupFileGroupColorChild(t)
	child := view.childEditor(0)
	defer child.sessions[0].draft.Close()
	child.SetPosition(0, 0, 99, 29)
	assertSections := func() {
		t.Helper()
		seen := map[string]bool{}
		headers := 0
		fields := map[string]bool{}
		for _, row := range child.page.rows {
			seen[row.field.Category] = true
			fields[row.field.ID] = true
			if row.checkableHeader {
				headers++
			}
		}
		for _, section := range []string{"Conditions", "Appearance", "Sorting", "Associations"} {
			if !seen["filegroup-"+section] {
				t.Errorf("single page omits %s", section)
			}
		}
		for _, id := range []string{"group.Name", "group.NormalColor", "group.Sort.Number", "collection:filegroups.commands"} {
			if id == "collection:filegroups.commands" {
				found := false
				for _, row := range child.page.rows {
					found = found || row.control != nil && row.control.GetId() == id
				}
				if !found {
					t.Error("applications list missing from single page")
				}
			} else if !fields[id] {
				t.Errorf("single page omits %s", id)
			}
		}
		if headers != 4 {
			t.Fatalf("single page has %d feature headers, want 4", headers)
		}
	}
	assertSections()
	if child.page.total <= child.page.Y2-child.page.Y1+1 {
		t.Fatal("long property page does not scroll")
	}
	for _, section := range []string{"Appearance", "Sorting", "Associations", "Conditions"} {
		child.selectCategory("filegroup-" + section)
		assertSections()
	}
	if view.center.sidebar.IsDisabled() {
		t.Fatal("child changed the parent category navigation")
	}
}

func TestFileGroupChildSinglePageSemanticChromeAndPinnedPreview(t *testing.T) {
	_, view := setupFileGroupColorChild(t)
	child := view.childEditor(0)
	defer child.sessions[0].draft.Close()
	child.SetPosition(3, 2, 102, 31)
	for _, control := range []vtui.UIElement{child.search, child.sidebar, child.previous, child.next, child.clearSearch} {
		if !control.IsDisabled() || control.IsVisible() {
			t.Fatalf("hidden navigation remains interactive: %s", control.GetId())
		}
	}
	node := child.SemanticNode(&vtui.SemanticContext{})
	if node["settingsSectionsOnly"] != true {
		t.Fatal("child allowed native settings categories")
	}
	var page, sample map[string]any
	for _, control := range semantic.AppMapSlice(node["children"]) {
		switch semantic.String(control["layoutRole"]) {
		case "navigation", "search", "search-clear", "search-label", "search-previous", "search-next", "search-matches", "content-title":
			t.Fatalf("child exported navigation chrome: %+v", control)
		case "content":
			page = control
		case "editor-preview":
			sample = control
		}
	}
	if len(page) == 0 || page["scrollable"] != true || len(sample) == 0 || sample["fileGroupSample"] != true {
		t.Fatal("missing scrollable page or pinned preview")
	}
	if child.page.X1 != child.X1+2 || child.page.X2 != child.X2-2 || child.fileGroupEditorSample.X1 != child.page.X1 {
		t.Fatal("child still reserves space for the sidebar")
	}
	if child.fileGroupEditorSample.Y2 >= child.page.Y1 {
		t.Fatal("preview overlaps the property page")
	}
	beforeX, beforeY := child.fileGroupEditorSample.X1, child.fileGroupEditorSample.Y1
	if !child.page.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(child.page), "action": "control.scroll", "value": 20,
	}) {
		t.Fatal("single page rejected scrolling")
	}
	if child.page.scroll == 0 || child.fileGroupEditorSample.X1 != beforeX || child.fileGroupEditorSample.Y1 != beforeY {
		t.Fatal("scrolling moved the pinned preview")
	}
}

func TestFileGroupChildSinglePageKeyboardNeverFocusesHiddenNavigation(t *testing.T) {
	_, view := setupFileGroupColorChild(t)
	child := view.childEditor(0)
	defer child.sessions[0].draft.Close()
	child.SetPosition(0, 0, 99, 29)
	child.SetFocusedItem(child.page)
	for _, key := range []uint16{vtinput.VK_LEFT, vtinput.VK_UP, vtinput.VK_PRIOR} {
		child.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: key})
		if child.GetFocusedItem() != child.page {
			t.Fatalf("key %d escaped to hidden navigation", key)
		}
	}
	child.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_F, ControlKeyState: vtinput.LeftCtrlPressed})
	if child.GetFocusedItem() != child.page {
		t.Fatal("Ctrl+F focused hidden search")
	}
	for _, event := range []vtinput.InputEvent{
		{KeyDown: true, VirtualKeyCode: vtinput.VK_TAB},
		{KeyDown: true, VirtualKeyCode: vtinput.VK_TAB, ControlKeyState: vtinput.ShiftPressed},
	} {
		for range 30 {
			child.ProcessKey(&event)
			if focused := child.GetFocusedItem(); focused != child.page && focused != child.ok && focused != child.cancel {
				t.Fatalf("Tab focused hidden chrome: %T", focused)
			}
		}
	}
}
