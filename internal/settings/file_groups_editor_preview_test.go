package settings

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

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
		if patches[i].Foreground != want || patches[i].Background == "" {
			t.Fatalf("state %d lost resolved palette: %+v", i, patches[i])
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
