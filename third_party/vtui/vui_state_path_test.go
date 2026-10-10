package vtui

import (
	"strings"
	"testing"
)

func loadStateTestDialog(t *testing.T, doc string) *Window {
	t.Helper()
	SetDefaultPalette()
	dlg, err := LoadDialog(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("LoadDialog: %v", err)
	}
	return dlg
}

func statefulEdits(root UIElement) []*Edit {
	var edits []*Edit
	walk(root, func(el UIElement) bool {
		if e, ok := el.(*Edit); ok {
			edits = append(edits, e)
		}
		return true
	})
	return edits
}

// Two unnamed edits in different containers get the same generated id, so the
// state of a reloaded window is keyed by the path from the root (vtui#174,
// item 5): neither overwrites the other.
func TestHotReloadStateIsKeyedByStructuralPath(t *testing.T) {
	const template = `{
		"vuiVersion": 1,
		"root": {
			"type": "Dialog", "id": "dlg", "props": { "title": " T " },
			"children": [
				{ "type": "GroupBox", "id": "left",  "children": [ { "type": "Edit", "props": { "text": "" } } ] },
				{ "type": "GroupBox", "id": "right", "children": [ { "type": "Edit", "props": { "text": "" } } ] }
			]
		}
	}`
	before := loadStateTestDialog(t, template)
	edits := statefulEdits(before)
	if len(edits) != 2 {
		t.Fatalf("found %d edits, want 2", len(edits))
	}
	edits[0].SetText("left text")
	edits[1].SetText("right text")

	states := captureWidgetStates(before)
	after := loadStateTestDialog(t, template)
	states.restore(after)

	got := statefulEdits(after)
	if got[0].GetText() != "left text" || got[1].GetText() != "right text" {
		t.Fatalf("restored texts = %q, %q; want each edit to get its own", got[0].GetText(), got[1].GetText())
	}
}

// A control with an explicit id that moved to another container in the edited
// template is still matched by that id; one with a generated id is not.
func TestHotReloadStateFollowsAnExplicitIDToAnotherContainer(t *testing.T) {
	const before = `{
		"vuiVersion": 1,
		"root": {
			"type": "Dialog", "id": "dlg", "props": { "title": " T " },
			"children": [
				{ "type": "GroupBox", "id": "a", "children": [
					{ "type": "Edit", "id": "named", "props": { "text": "" } },
					{ "type": "Edit", "props": { "text": "" } } ] },
				{ "type": "GroupBox", "id": "b", "children": [] }
			]
		}
	}`
	const after = `{
		"vuiVersion": 1,
		"root": {
			"type": "Dialog", "id": "dlg", "props": { "title": " T " },
			"children": [
				{ "type": "GroupBox", "id": "a", "children": [] },
				{ "type": "GroupBox", "id": "b", "children": [
					{ "type": "Edit", "id": "named", "props": { "text": "" } },
					{ "type": "Edit", "props": { "text": "fresh" } } ] }
			]
		}
	}`
	old := loadStateTestDialog(t, before)
	oldEdits := statefulEdits(old)
	oldEdits[0].SetText("kept by id")
	oldEdits[1].SetText("lost with its path")

	states := captureWidgetStates(old)
	moved := loadStateTestDialog(t, after)
	states.restore(moved)

	got := statefulEdits(moved)
	if got[0].GetText() != "kept by id" {
		t.Errorf("the named edit = %q, want its text carried over by id", got[0].GetText())
	}
	if got[1].GetText() != "fresh" {
		t.Errorf("the unnamed edit = %q, want it left alone (its path changed)", got[1].GetText())
	}
}

func TestWalkWithPathReportsThePathOfIDs(t *testing.T) {
	dlg := loadStateTestDialog(t, `{
		"vuiVersion": 1,
		"root": { "type": "Dialog", "id": "dlg", "props": { "title": " T " },
			"children": [ { "type": "GroupBox", "id": "box", "children": [ { "type": "Edit", "id": "e", "props": {} } ] } ] }
	}`)
	var paths []string
	walkWithPath(dlg, "", func(path string, el UIElement) { paths = append(paths, path) })
	joined := strings.Join(paths, " ")
	if !strings.Contains(joined, "/dlg/box/e") {
		t.Fatalf("paths = %v, want one ending in /dlg/box/e", paths)
	}
}
