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
	child.selectCategory("filegroup-Appearance")
	count := 0
	for _, row := range child.page.rows {
		if fileGroupColourState(row.field.ID) >= 0 {
			count++
			if row.controlY != 0 || row.controlX == 0 || row.height != 1 {
				t.Fatalf("colour row is not compact/inline: %+v", row)
			}
		}
	}
	if count != 4 {
		t.Fatal("missing colour states")
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
