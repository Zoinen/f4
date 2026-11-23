package settings

import (
	"context"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestHiddenDirectoryEditorSampleUsesPanelAppearance(t *testing.T) {
	setupFileGroupSettings(t)
	groups := []filegroups.Group{
		{ID: "hidden", Fields: map[string]string{"Name": "Hidden folders", "Mask": "*",
			"IncludeAttributes": "Directory, Hidden", "Highlight.Enabled": "true", "NormalColor": "foreground:#123456"}},
		{ID: "folders", Fields: map[string]string{"Name": "Folders", "Mask": "*",
			"IncludeAttributes": "Directory", "Highlight.Enabled": "true", "NormalColor": "foreground:#654321"}},
	}
	draft := f4settings.NewDraft(nil, groupsToRecords(groups))
	t.Cleanup(draft.Close)
	center := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: draft}})
	center.selectCategory("filegroups")
	editor := center.fileGroupOverview.childEditor(0)
	t.Cleanup(editor.sessions[0].draft.Close)
	sample := editor.fileGroupEditorSample
	if !sample.preview.item.IsDir || !sample.preview.item.IsHidden {
		t.Fatalf("sample does not satisfy the directory and hidden predicates: %+v", sample.preview.item)
	}
	patch := sample.preview.style.Normal
	// The Qt panel fades hidden entries, using the underlying directory color.
	if patch.Foreground != "#654321" || patch.Background != "" {
		t.Fatalf("expected the panel's raw highlight patch, got %+v", patch)
	}
}

func TestFileGroupConsolePreviewReachesColorEdits(t *testing.T) {
	_, overview := setupFileGroupColorChild(t)
	backend, details := vtui.ActiveBackend(), vtui.BackendDetails()
	t.Cleanup(func() { vtui.SetActiveBackend(backend, details...) })
	vtui.SetActiveBackend("console")
	editor := overview.childEditor(0)
	t.Cleanup(editor.sessions[0].draft.Close)
	editor.SetPosition(0, 0, 119, 39)
	sample := editor.fileGroupEditorSample

	editor.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_TAB, ControlKeyState: vtinput.ShiftPressed})
	if editor.GetFocusedItem() != sample {
		t.Fatal("Shift+Tab from the first setting did not focus the preview")
	}
	editor.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT})
	editor.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})
	assertColorFocus := func(state int) {
		t.Helper()
		focused := editor.page.GetFocusedItem()
		if editor.GetFocusedItem() != editor.page || focused == nil ||
			focused.GetId() != "setting:"+fileGroupColourFields[state] {
			t.Fatalf("state %d did not focus its color field; focused %v", state, focused)
		}
		_, y, _, _ := focused.GetPosition()
		if y < editor.page.Y1 || y > editor.page.Y2 {
			t.Fatalf("color field is outside the visible page: y=%d", y)
		}
	}
	assertColorFocus(1)

	editor.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed, MouseX: int16(sample.X2), MouseY: int16(sample.Y1 + 1)})
	assertColorFocus(3)
	editor.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_A, ControlKeyState: vtinput.LeftCtrlPressed})
	for _, ch := range "foreground:#778899" {
		editor.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: ch})
	}
	if got := sample.preview.style.SelectedCursor.Foreground; got != "#778899" {
		t.Fatalf("editing the focused color updated the preview to %q", got)
	}
}

func TestFileGroupEditorRetainsPinnedPreviewAndCompactColourRows(t *testing.T) {
	setupFileGroupSettings(t)
	correction := config.App.EnforceColorCorrection
	config.App.EnforceColorCorrection = false
	t.Cleanup(func() { config.App.EnforceColorCorrection = correction })
	groups := []filegroups.Group{{ID: "user", Fields: map[string]string{
		"Name": "Example", "Mask": "*.txt", "MaskSyntax": "far", "Highlight.Enabled": "true",
		"NormalColor": "foreground:#112233", "SelectedColor": "foreground:#223344",
		"CursorColor": "foreground:#334455", "SelectedCursorColor": "foreground:#445566",
	}}}
	d := f4settings.NewDraft(nil, groupsToRecords(groups))
	defer d.Close()
	c := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: d}})
	c.selectCategory("filegroups")
	child := c.fileGroupOverview.childEditor(0)
	defer child.sessions[0].draft.Close()
	child.SetPosition(0, 0, 119, 39)
	sample := child.fileGroupEditorSample
	before := sample.Y1
	patches := fileGroupColourPatches(sample.preview.style)
	for i, want := range []string{"#112233", "#223344", "#334455", "#445566"} {
		if patches[i].Foreground != want || patches[i].Background != "" {
			t.Fatalf("state %d lost the raw highlight patch: %+v", i, patches[i])
		}
	}
	child.selectCategory("filegroup-Highlighting")
	var states [4]*settingsRow
	for _, row := range child.page.rows {
		if state := fileGroupColourState(row.field.ID); state >= 0 {
			states[state] = row
			if row.controlY != 0 || row.controlX == 0 || row.height != 1 {
				t.Fatalf("color row is not compact/inline: %+v", row)
			}
		}
	}
	for _, state := range states {
		if state == nil {
			t.Fatal("missing color state")
		}
	}
	normal, marked, cursor, markedCursor := states[0], states[1], states[2], states[3]
	if normal.y != cursor.y || marked.y != markedCursor.y || normal.y >= marked.y ||
		normal.labelX != marked.labelX || cursor.labelX != markedCursor.labelX ||
		normal.controlX+normal.controlWidth >= cursor.labelX {
		t.Fatal("color states should form two columns with the cursor states on the right")
	}
	child.page.scroll = 10
	child.page.positionRows()
	child.selectCategory("filegroup-Associations")
	if child.fileGroupEditorSample != sample || sample.Y1 != before || sample.Y2 >= child.page.Y1 {
		t.Fatal("preview moved into the scrollable property page")
	}
	scratch := child.sessions[0].draft
	scratch.Values["group.NormalColor"] = "foreground:#ABCDEF"
	if err := scratch.PreviewFunc(scratch); err != nil {
		t.Fatal(err)
	}
	if sample.preview.style.Normal.Foreground != "#ABCDEF" {
		t.Fatal("preview did not update after a valid edit")
	}
	scratch.Values["group.NormalColor"] = "foreground:#oops"
	if err := scratch.PreviewFunc(scratch); err == nil || sample.problem == "" {
		t.Fatal("invalid colour was accepted")
	}
	if sample.preview.style.Normal.Foreground != "#ABCDEF" {
		t.Fatal("invalid colour replaced the last valid sample")
	}
	scratch.Values["group.NormalColor"] = "foreground:#ABCDEF"
	scratch.Values["group.Mask"] = "/[/"
	if err := scratch.PreviewFunc(scratch); err == nil {
		t.Fatal("invalid mask was accepted")
	}
	if sample.problem == "" || sample.preview.style.Normal.Foreground != "#ABCDEF" {
		t.Fatal("invalid edit replaced last valid preview")
	}
	node := child.SemanticNode(&vtui.SemanticContext{})
	found := false
	for _, n := range semantic.AppMapSlice(node["children"]) {
		if n["layoutRole"] == "editor-preview" {
			found = n["fileGroupSample"] == true
		}
	}
	if !found || d.Records["filegroups"][0].Values["group.NormalColor"] != "foreground:#112233" {
		t.Fatal("preview missing or scratch edit escaped into parent")
	}
}

func TestFileGroupQtFileNameColorIsOptIn(t *testing.T) {
	parent, overview := setupFileGroupColorChild(t)
	backend, details := vtui.ActiveBackend(), vtui.BackendDetails()
	t.Cleanup(func() { vtui.SetActiveBackend(backend, details...) })
	const id = "group.Highlight.ColorizeFileName"
	vtui.SetActiveBackend("console")
	console := overview.childEditor(0)
	for _, row := range console.page.rows {
		if row.field.ID == id {
			t.Fatal("Qt option is visible in the console")
		}
	}
	console.sessions[0].draft.Close()

	vtui.SetActiveBackend("qt")
	editor := overview.childEditor(0)
	draft := editor.sessions[0].draft
	defer draft.Close()
	var toggle vtui.UIElement
	for _, row := range editor.page.rows {
		if row.field.ID == id {
			toggle = row.control
			break
		}
	}
	if toggle == nil || toggle.IsDisabled() {
		t.Fatal("Qt file name color control is unavailable")
	}
	preview := editor.fileGroupEditorSample
	if patch := preview.preview.style.Normal; patch.Foreground == "" || patch.FileNameForeground != "" {
		t.Fatalf("default should provide an icon color only: %+v", patch)
	}
	if !editor.HandleSemanticAction(map[string]any{"target": vtui.SemanticID(toggle), "action": "control.toggle"}) {
		t.Fatal("could not enable file name color")
	}
	if patch := preview.preview.style.Normal; patch.FileNameForeground != "#112233" {
		t.Fatalf("file name color did not reach preview: %+v", patch)
	}
	if result := draft.Commit(context.Background()); len(result.Errors) != 0 {
		t.Fatal(result.Errors)
	}
	if parent.Records["filegroups"][0].Values[id] != "true" {
		t.Fatal("file name color was not saved on the group")
	}
}
