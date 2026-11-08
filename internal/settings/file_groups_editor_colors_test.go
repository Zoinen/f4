package settings

import (
	"context"
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/sdk/f4settings"
)

func setupFileGroupColorChild(t *testing.T) (*f4settings.Draft, *fileGroupsOverview) {
	t.Helper()
	store := setupFileGroupSettings(t)
	oldCorrection, oldStyle := config.App.EnforceColorCorrection, config.App.ColorStyle
	config.App.EnforceColorCorrection, config.App.ColorStyle = false, "Unrelated application palette"
	t.Cleanup(func() { config.App.EnforceColorCorrection, config.App.ColorStyle = oldCorrection, oldStyle })
	store.Defaults = []filegroups.Group{{ID: "arbitrary-id", Fields: map[string]string{
		"Name": "Text files", "Mask": "*.txt", "MaskSyntax": "far", "Highlight.Enabled": "true",
		"NormalColor": "foreground:#DEAD00", "Mark": "!", "ContinueProcessing": "true", "UseDefaults": "true",
		filegroups.PalettePrefix(config.App.ColorStyle) + "NormalColor": "foreground:#DEAD01",
	}}}
	store.ColorDefaults = []filegroups.ColorTheme{{ID: "chosen", Fields: map[string]string{
		"Name": "Chosen standard", "Color.text-files.NormalColor": "foreground:#010203",
		"Color.text-files.CursorColor": "foreground:#030405",
	}}}
	ConfigureFileGroups(store, fileGroupRuntime)
	d := f4settings.NewDraft(map[string]string{"filegroups.colorTheme": "chosen"}, groupsToRecords(store.Defaults))
	d.Records["filegroups.colorthemes"] = []f4settings.Record{
		{ID: "other", Values: map[string]string{"Name": "Other", "Color.text-files.NormalColor": "foreground:#999999"}},
		{ID: "chosen", Values: map[string]string{
			"Name": "Chosen", "Color.text-files.NormalColor": "foreground:#112233",
			"Color.text-files.SelectedColor": "foreground:#223344", "Color.text-files.CursorColor": "foreground:#334455",
			"Color.text-files.SelectedCursorColor": "foreground:#445566", "Color.images.NormalColor": "foreground:#ABCDEF",
		}},
	}
	t.Cleanup(d.Close)
	c := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: d}})
	c.selectCategory("filegroups")
	return d, c.fileGroupOverview
}

func assertNoFileGroupColorShadows(t *testing.T, values map[string]string) {
	t.Helper()
	for key := range values {
		if fileGroupColourState(key) >= 0 || !strings.HasPrefix(key, "group.") {
			t.Fatalf("form-only value escaped into group: %s", key)
		}
	}
}

func TestFileGroupChildColorsRouteToSelectedTheme(t *testing.T) {
	parent, view := setupFileGroupColorChild(t)
	before := f4settings.NewDraft(parent.Values, parent.Records)
	child := view.childEditor(0)
	scratch := child.sessions[0].draft
	defer scratch.Close()
	if scratch.Values["filegroups.colorTheme"] != "chosen" {
		t.Fatal("child lost the selected colour theme")
	}
	for i, want := range []string{"foreground:#112233", "foreground:#223344", "foreground:#334455", "foreground:#445566"} {
		if got := scratch.Values[fileGroupColourFields[i]]; got != want {
			t.Fatalf("state %d loaded %q, want %q", i, got, want)
		}
	}
	states := 0
	for _, f := range child.sessions[0].catalog.Fields {
		if fileGroupColourState(f.ID) >= 0 {
			states++
			if strings.Contains(f.ID, "Palette.") || f.InputWidth != 44 {
				t.Fatalf("colour matrix still uses application palette controls: %+v", f)
			}
		}
	}
	if states != 4 {
		t.Fatalf("colour matrix has %d states", states)
	}
	for i, value := range []string{"foreground:#556677", "foreground:#667788", "foreground:#778899", "foreground:#8899AA"} {
		scratch.Values[fileGroupColourFields[i]] = value
	}
	if err := scratch.PreviewFunc(scratch); err != nil {
		t.Fatal(err)
	}
	for i, patch := range fileGroupColourPatches(child.fileGroupEditorSample.preview.style) {
		if want := []string{"#556677", "#667788", "#778899", "#8899AA"}[i]; patch.Foreground != want {
			t.Fatalf("preview state %d=%s, want %s", i, patch.Foreground, want)
		}
	}
	assertNoFileGroupColorShadows(t, scratch.Records["filegroups"][0].Values)
	if !reflect.DeepEqual(parent.Records, before.Records) {
		t.Fatal("preview modified the parent draft")
	}
	if result := scratch.Commit(context.Background()); len(result.Errors) != 0 {
		t.Fatal(result.Errors)
	}
	if got := parent.Records["filegroups.colorthemes"][0]; !reflect.DeepEqual(got, before.Records["filegroups.colorthemes"][0]) {
		t.Fatal("editing chosen theme modified another theme")
	}
	for i, key := range fileGroupEditorColorKeys {
		if got := parent.Records["filegroups.colorthemes"][1].Values["Color.text-files."+key]; got != scratch.Values[fileGroupColourFields[i]] {
			t.Fatalf("state %s not saved in chosen theme: %s", key, got)
		}
	}
	group := parent.Records["filegroups"][0].Values
	assertNoFileGroupColorShadows(t, group)
	for _, key := range []string{"Mark", "ContinueProcessing", "UseDefaults", "Highlight.Enabled", "Mask"} {
		if group["group."+key] != before.Records["filegroups"][0].Values["group."+key] {
			t.Fatalf("colour routing changed group field %s", key)
		}
	}
}

func TestFileGroupChildColorsRenameAndReorderUseNormalizedName(t *testing.T) {
	parent, view := setupFileGroupColorChild(t)
	child := view.childEditor(0)
	scratch := child.sessions[0].draft
	scratch.Values["group.NormalColor"] = "foreground:#556677"
	if err := scratch.PreviewFunc(scratch); err != nil {
		t.Fatal(err)
	}
	scratch.Values["group.Name"] = "Images"
	if err := scratch.PreviewFunc(scratch); err != nil {
		t.Fatal(err)
	}
	if scratch.Values["group.NormalColor"] != "foreground:#ABCDEF" || scratch.Values["group.SelectedColor"] != "" {
		t.Fatal("rename carried colours from the old name")
	}
	scratch.Values["group.Name"] = "New category"
	if err := scratch.PreviewFunc(scratch); err != nil {
		t.Fatal(err)
	}
	if _, exists := scratch.Values["group.NormalColor"]; exists {
		t.Fatal("unknown name inherited the old entry")
	}
	scratch.Values["group.Name"] = "TEXT--FILES!"
	if err := scratch.PreviewFunc(scratch); err != nil {
		t.Fatal(err)
	}
	if scratch.Values["group.NormalColor"] != "foreground:#556677" {
		t.Fatal("normalized name did not recover its own edited colour")
	}
	if result := scratch.Commit(context.Background()); len(result.Errors) != 0 {
		t.Fatal(result.Errors)
	}
	scratch.Close()
	colors := maps.Clone(parent.Records["filegroups.colorthemes"][1].Values)
	if colors["Color.images.NormalColor"] != "foreground:#ABCDEF" {
		t.Fatal("rename replaced the target's existing entry")
	}
	parent.Records["filegroups"] = append([]f4settings.Record{{ID: "different-id-same-name", Values: map[string]string{
		"group.Name": "text files", "group.Mask": "*.different", "group.Highlight.Enabled": "true",
	}}}, parent.Records["filegroups"]...)
	child = view.childEditor(0)
	scratch = child.sessions[0].draft
	defer scratch.Close()
	if scratch.Values["group.NormalColor"] != "foreground:#556677" {
		t.Fatal("colours depended on group ID, predicates or row order")
	}
	if !maps.Equal(colors, parent.Records["filegroups.colorthemes"][1].Values) {
		t.Fatal("opening reordered group changed theme entries")
	}
}

func TestFileGroupChildColorsCancelRestoresParentAndPreview(t *testing.T) {
	parent, view := setupFileGroupColorChild(t)
	before := f4settings.NewDraft(parent.Values, parent.Records)
	var preview *f4settings.Draft
	parent.PreviewFunc = func(d *f4settings.Draft) error { preview = d; return nil }
	child := view.childEditor(0)
	scratch := child.sessions[0].draft
	scratch.Values["group.NormalColor"] = "foreground:#556677"
	if err := scratch.PreviewFunc(scratch); err != nil {
		t.Fatal(err)
	}
	if preview != scratch {
		t.Fatal("live preview did not receive scratch theme edits")
	}
	scratch.Close()
	if !reflect.DeepEqual(parent.Records, before.Records) || !maps.Equal(parent.Values, before.Values) {
		t.Fatal("Cancel leaked child edits into parent")
	}
	if preview != parent {
		t.Fatal("Cancel failed to restore parent preview")
	}
}

func TestFileGroupChildColorsResetUsesChosenThemeStandard(t *testing.T) {
	parent, view := setupFileGroupColorChild(t)
	child := view.childEditor(0)
	scratch := child.sessions[0].draft
	defer scratch.Close()
	child.resetFileGroupEditorFieldValue("group.NormalColor")
	child.resetFileGroupEditorFieldValue("group.SelectedColor")
	if scratch.Values["group.NormalColor"] != "foreground:#010203" {
		t.Fatal("reset used group defaults or the application palette")
	}
	if _, exists := scratch.Values["group.SelectedColor"]; exists {
		t.Fatal("reset retained a state absent from the standard theme")
	}
	if err := scratch.PreviewFunc(scratch); err != nil {
		t.Fatal(err)
	}
	values := scratch.Records["filegroups.colorthemes"][1].Values
	if values["Color.text-files.NormalColor"] != "foreground:#010203" {
		t.Fatal("reset did not update scratch theme")
	}
	if _, exists := values["Color.text-files.SelectedColor"]; exists {
		t.Fatal("reset did not remove the overridden state")
	}
	if parent.Records["filegroups.colorthemes"][1].Values["Color.text-files.NormalColor"] != "foreground:#112233" {
		t.Fatal("reset escaped child draft before OK")
	}
}

func TestFileGroupChildColorsExplicitClearAndInvalidEdit(t *testing.T) {
	parent, view := setupFileGroupColorChild(t)
	child := view.childEditor(0)
	scratch := child.sessions[0].draft
	defer scratch.Close()
	scratch.Values["group.NormalColor"] = "foreground:#oops"
	if err := scratch.PreviewFunc(scratch); err == nil {
		t.Fatal("invalid colour accepted")
	}
	if result := scratch.Commit(context.Background()); len(result.Errors) == 0 {
		t.Fatal("invalid colour committed")
	}
	if parent.Records["filegroups.colorthemes"][1].Values["Color.text-files.NormalColor"] != "foreground:#112233" {
		t.Fatal("invalid commit changed parent theme")
	}
	scratch.Values["group.NormalColor"] = ""
	if result := scratch.Commit(context.Background()); len(result.Errors) != 0 {
		t.Fatal(result.Errors)
	}
	value, present := parent.Records["filegroups.colorthemes"][1].Values["Color.text-files.NormalColor"]
	if !present || value != "" {
		t.Fatal("explicit clear was lost")
	}
	assertNoFileGroupColorShadows(t, parent.Records["filegroups"][0].Values)
}

func TestFileGroupChildColorsWithoutThemesRetainLegacyPaletteRouting(t *testing.T) {
	parent, view := setupFileGroupColorChild(t)
	delete(parent.Records, "filegroups.colorthemes")
	delete(parent.Values, "filegroups.colorTheme")
	values := parent.Records["filegroups"][0].Values
	delete(values, "group.NormalColor")
	prefix := "group." + filegroups.PalettePrefix(config.App.ColorStyle)
	values[prefix+"Highlight.Enabled"] = "true"
	child := view.childEditor(0)
	scratch := child.sessions[0].draft
	defer scratch.Close()
	found := false
	for _, field := range child.sessions[0].catalog.Fields {
		found = found || field.ID == prefix+"NormalColor"
	}
	if !found {
		t.Fatal("legacy draft lost its palette colour control")
	}
	scratch.Values[prefix+"NormalColor"] = "foreground:#556677"
	if result := scratch.Commit(context.Background()); len(result.Errors) != 0 {
		t.Fatal(result.Errors)
	}
	if parent.Records["filegroups"][0].Values[prefix+"NormalColor"] != "foreground:#556677" {
		t.Fatal("legacy edit was routed outside the group")
	}
	if _, exists := parent.Records["filegroups.colorthemes"]; exists {
		t.Fatal("legacy edit synthesized a colour theme")
	}
}

func TestFileGroupChildColorsEmptyThemeRecordsRetainLegacyPaletteControls(t *testing.T) {
	parent, view := setupFileGroupColorChild(t)
	parent.Records["filegroups.colorthemes"] = []f4settings.Record{}
	parent.Values["filegroups.colorTheme"] = ""
	values := parent.Records["filegroups"][0].Values
	delete(values, "group.NormalColor")
	delete(values, "group.Highlight.Enabled")
	prefix := "group." + filegroups.PalettePrefix(config.App.ColorStyle)
	values[prefix+"Highlight.Enabled"] = "true"
	child := view.childEditor(0)
	scratch := child.sessions[0].draft
	defer scratch.Close()
	if !fileGroupFeatureEnabled(scratch.Values, "Highlighting") {
		t.Fatal("empty selected theme disabled legacy highlighting")
	}
	found := false
	for _, field := range child.sessions[0].catalog.Fields {
		found = found || field.ID == prefix+"NormalColor"
	}
	if !found {
		t.Fatal("empty theme records selected the new colour routing")
	}
	scratch.Values[prefix+"NormalColor"] = "foreground:#556677"
	if result := scratch.Commit(context.Background()); len(result.Errors) != 0 {
		t.Fatal(result.Errors)
	}
	if len(parent.Records["filegroups.colorthemes"]) != 0 || parent.Records["filegroups"][0].Values[prefix+"NormalColor"] != "foreground:#556677" {
		t.Fatal("legacy edit escaped into colour themes")
	}
}

func TestFileGroupColorPickerUpdatesDraftAndClearsOverride(t *testing.T) {
	parent, view := setupFileGroupColorChild(t)
	child := view.childEditor(0)
	defer child.sessions[0].draft.Close()
	scratch := child.sessions[0].draft
	for _, value := range []string{"foreground:#ff0000", ""} {
		if !child.HandleSemanticAction(map[string]any{
			"target": "id:setting:group.NormalColor", "action": "control.setText", "text": value,
		}) {
			t.Fatal("color picker update was not handled")
		}
		if got := scratch.Values["group.NormalColor"]; got != value {
			t.Fatalf("draft value = %q, want %q", got, value)
		}
	}
	if got := parent.Records["filegroups.colorthemes"][1].Values["Color.text-files.NormalColor"]; got != "foreground:#112233" {
		t.Fatalf("picker modified the saved theme before applying: %q", got)
	}
}
