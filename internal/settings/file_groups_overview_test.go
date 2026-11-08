package settings

import (
	"context"
	"maps"
	"reflect"
	"testing"

	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

func TestFileGroupChildCancelAndAcceptAreDraftOnly(t *testing.T) {
	store := setupFileGroupSettings(t)
	d, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	c := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: d}})
	c.selectCategory("filegroups")
	v := c.fileGroupOverview
	d.Records["filegroups"][0].Values["group.Name"] = "Parent draft"
	child := v.childEditor(0)
	scratch := child.sessions[0].draft
	if scratch.Values["group.Enabled"] != "true" {
		t.Fatal("inherited Enabled default lost")
	}
	scratch.Values["group.Name"] = "Cancelled child"
	if err := scratch.PreviewFunc(scratch); err != nil {
		t.Fatal(err)
	}
	scratch.Close()
	if got := d.Records["filegroups"][0].Values["group.Name"]; got != "Parent draft" {
		t.Fatal(got)
	}
	child = v.childEditor(0)
	scratch = child.sessions[0].draft
	scratch.Values["group.Name"] = "Accepted child"
	result := scratch.Commit(context.Background())
	if len(result.Errors) > 0 {
		t.Fatal(result.Errors)
	}
	scratch.Close()
	if got := d.Records["filegroups"][0].Values["group.Name"]; got != "Accepted child" {
		t.Fatal(got)
	}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if tx.Groups()[0].Label() != "Factory" {
		t.Fatal("child saved to disk")
	}
}

func TestFileGroupsOverviewOmitsOriginColumnAndSeparators(t *testing.T) {
	setupFileGroupSettings(t)
	d, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.Records["filegroups"] = append(d.Records["filegroups"], groupsToRecords([]filegroups.Group{
		{ID: "legacy.highlight_5", Fields: map[string]string{"Name": "Executable"}},
		{ID: "user-test", Fields: map[string]string{"Name": "Personal"}},
	})["filegroups"]...)
	c := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: d}})
	c.selectCategory("filegroups")
	v := c.fileGroupOverview
	if len(v.rows) != 3 || v.rows[0].record.ID != "user-test" || v.rows[2].record.ID != "factory" {
		t.Fatalf("overview is not reversed: %+v", v.rows)
	}
	for _, row := range v.rows {
		if row.separator != "" {
			t.Fatalf("overview still contains a separator: %s", row.separator)
		}
		if len(row.cells) != 4 {
			t.Fatalf("overview has %d columns, want 4", len(row.cells))
		}
	}
	n := v.SemanticNode(&vtui.SemanticContext{})
	for _, child := range semantic.AppMapSlice(n["children"]) {
		if child["layoutRole"] == "overview-table" {
			columns, _ := child["columns"].([]map[string]any)
			if len(columns) != 4 {
				t.Fatalf("semantic table has %d columns", len(columns))
			}
		}
	}
}

func TestFileGroupsDeleteRemovesAnyDraftRowAndCommands(t *testing.T) {
	for _, id := range []string{"factory", "legacy.highlight_5", "user-test"} {
		t.Run(id, func(t *testing.T) {
			setupFileGroupSettings(t)
			d, err := (fileGroupsProvider{}).Begin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if id != "factory" {
				d.Records["filegroups"] = append(d.Records["filegroups"], f4settings.Record{ID: id, Values: map[string]string{"group.Name": "Personal"}})
			}
			d.Records["filegroups.commands"] = []f4settings.Record{
				{ID: id + "/open", Values: map[string]string{"command.Group": id, "command.Execute": "run"}},
				{ID: "other/open", Values: map[string]string{"command.Group": "other", "command.Execute": "keep"}},
			}
			c := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: d}})
			c.selectCategory("filegroups")
			showFileGroupsTestCenter(t, c)
			v := c.fileGroupOverview
			v.selectedID = id
			before := len(d.Records["filegroups"])
			v.remove()
			fileGroupsTestConfirm(t, 1)
			if len(d.Records["filegroups"]) != before {
				t.Fatal("cancelled Delete changed draft")
			}
			v.remove()
			fileGroupsTestConfirm(t, 0)
			for _, row := range d.Records["filegroups"] {
				if row.ID == id {
					t.Fatal("Delete disabled row instead of removing it")
				}
			}
			if len(d.Records["filegroups.commands"]) != 1 || d.Records["filegroups.commands"][0].ID != "other/open" {
				t.Fatal("Delete failed to remove only the selected group's commands")
			}
		})
	}
}

func TestFileGroupsReversedOverviewPreservesPriorityAndMoveDirection(t *testing.T) {
	setupFileGroupSettings(t)
	d := f4settings.NewDraft(nil, groupsToRecords([]filegroups.Group{
		{ID: "text", Fields: map[string]string{
			"Name": "Text files", "Mask": "*.txt", "Highlight.Enabled": "true", "NormalColor": "foreground:#112233",
		}},
		{ID: "folders", Fields: map[string]string{
			"Name": "Folders", "IncludeAttributes": "Directory",
		}},
		{ID: "files", Fields: map[string]string{
			"Name": "Files", "Mask": "*", "Highlight.Enabled": "true", "NormalColor": "foreground:#AABBCC",
		}},
	}))
	defer d.Close()
	before := f4settings.NewDraft(nil, d.Records).Records
	c := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: d}})
	c.selectCategory("filegroups")
	v := c.fileGroupOverview
	if !reflect.DeepEqual(d.Records, before) {
		t.Fatal("opening reversed list rewrote persisted rule priority")
	}
	for i, id := range []string{"files", "folders", "text"} {
		if v.rows[i].record.ID != id || v.rows[i].separator != "" {
			t.Fatalf("row %d = %+v, want %s", i, v.rows[i], id)
		}
	}
	if got := v.previews["text"].style.Normal.Foreground; got != "#112233" {
		t.Fatalf("fallback overrode specialized colour: %s", got)
	}
	v.table.SetSelectPos(1)
	v.table.OnSelect(1)
	v.move(-1)
	if v.rows[0].record.ID != "folders" || v.selectedID != "folders" || v.recordIndex() != 2 {
		t.Fatal("Up did not move the selected visible group upward")
	}
	v.move(-1)
	if v.rows[0].record.ID != "folders" {
		t.Fatal("Up moved past the first visible row")
	}
	v.move(1)
	if !reflect.DeepEqual(d.Records, before) || v.rows[1].record.ID != "folders" || v.selectedID != "folders" {
		t.Fatal("Down failed to restore order and selection")
	}
}

func TestFileGroupsRestoreStandardSetIsConfirmedAndDraftOnly(t *testing.T) {
	store := setupFileGroupSettings(t)
	store.Defaults[0].Commands = []filegroups.CommandSet{{ID: "open", Name: "Open", Commands: [6]string{"one", "two", "three", "four", "five", "six"}, Enabled: [6]bool{true, false, true, false, true, true}}}
	store.Defaults = append(store.Defaults, filegroups.Group{ID: "second", Fields: map[string]string{"Name": "Second", "Mask": "*.txt", "IconTheme": "hidden"}})
	ConfigureFileGroups(store, fileGroupRuntime)
	d, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.Records["filegroups"] = []f4settings.Record{{ID: "personal", Values: map[string]string{"group.Name": "Personal"}}}
	d.Records["filegroups.commands"] = []f4settings.Record{{ID: "personal/open", Values: map[string]string{"command.Group": "personal", "command.Execute": "personal"}}}
	d.Records["filegroups.themes"] = append(d.Records["filegroups.themes"], f4settings.Record{ID: "personal-theme", Values: map[string]string{"Name": "Personal theme", "IconCatalog": "future", "Vendor.Keep": "unchanged"}})
	var themesBefore []f4settings.Record
	for _, record := range d.Records["filegroups.themes"] {
		record.Values = maps.Clone(record.Values)
		themesBefore = append(themesBefore, record)
	}
	c := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: d}})
	c.selectCategory("filegroups")
	showFileGroupsTestCenter(t, c)
	v := c.fileGroupOverview
	v.themeID = "other"
	var restore *vtui.Button
	for _, button := range v.actions.buttons {
		if button.GetCaption() == "Restore standard set" {
			restore = button
		}
	}
	if restore == nil {
		t.Fatal("Restore standard set action missing")
	}
	restore.OnClick()
	fileGroupsTestConfirm(t, 1)
	if len(d.Records["filegroups"]) != 1 || d.Records["filegroups"][0].ID != "personal" {
		t.Fatal("cancelled Restore changed draft")
	}
	restore.OnClick()
	fileGroupsTestConfirm(t, 0)
	want := groupsToRecords(store.Defaults)
	if !reflect.DeepEqual(d.Records["filegroups"], want["filegroups"]) || !reflect.DeepEqual(d.Records["filegroups.commands"], want["filegroups.commands"]) {
		t.Fatal("Restore failed to replace all groups, commands and order")
	}
	if v.themeID != "other" || fileGroupActiveTheme != "generic" {
		t.Fatal("Restore changed the selected edit theme or application theme")
	}
	if !reflect.DeepEqual(d.Records["filegroups.themes"], themesBefore) {
		t.Fatal("Restore changed theme descriptors")
	}
	d.Records["filegroups"][0].Values["group.Name"] = "Edited after restore"
	if store.Defaults[0].Label() != "Factory" {
		t.Fatal("Restore shared mutable defaults with draft")
	}
	if len(CommittedFileGroups().Groups()) != 1 || CommittedFileGroups().Groups()[0].Label() != "Factory" {
		t.Fatal("Restore applied before Apply")
	}
	d.Records["filegroups"][0].Values["group.Name"] = "Factory"
	result := d.Commit(context.Background())
	if len(result.Errors) > 0 {
		t.Fatal(result.Errors)
	}
	d.Accept(result)
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	groups := tx.Groups()
	if len(groups) != 2 || groups[0].ID != "factory" || groups[1].ID != "second" || len(groups[0].Commands) != 1 {
		t.Fatalf("Restore failed to save all groups and order: %+v", groups)
	}
	command := groups[0].Commands[0]
	wantCommand := store.Defaults[0].Commands[0]
	if command.Commands != wantCommand.Commands || command.Enabled != wantCommand.Enabled {
		t.Fatal("Restore changed six command slots or enable bits")
	}
	retainedTheme := false
	for _, theme := range tx.Themes() {
		if theme.ID == "personal-theme" && theme.Fields["Vendor.Keep"] == "unchanged" && theme.Catalog() == "future" {
			retainedTheme = true
		}
	}
	if !retainedTheme {
		t.Fatal("Apply lost a preserved theme descriptor")
	}
	d.Records["filegroups"] = nil
	d.Records["filegroups.commands"] = nil
	d.Close()
	if groups := CommittedFileGroups().Groups(); len(groups) != 1 || groups[0].ID != "factory" {
		t.Fatal("Cancel rolled back applied Restore")
	}
	tx, err = store.Begin()
	if err != nil || len(tx.Groups()) != 2 {
		t.Fatal("Cancel lost the restored group in a hidden theme", err)
	}
}

func TestFileGroupsDeleteApplyReloadRetainsRemoval(t *testing.T) {
	store := setupFileGroupSettings(t)
	d, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	c := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: d}})
	c.selectCategory("filegroups")
	showFileGroupsTestCenter(t, c)
	v := c.fileGroupOverview
	v.selectedID = "factory"
	v.remove()
	fileGroupsTestConfirm(t, 0)
	result := d.Commit(context.Background())
	if len(result.Errors) > 0 {
		t.Fatal(result.Errors)
	}
	d.Accept(result)
	d.Close()
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if len(tx.Groups()) != 0 || len(CommittedFileGroups().Groups()) != 0 {
		t.Fatal("deleted row returned after Apply/reload")
	}
	reloaded, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	if len(reloaded.Records["filegroups"]) != 0 {
		t.Fatal("settings recreated a deleted row")
	}
}

func TestFileGroupsDeletedRowsStayAbsentInPreview(t *testing.T) {
	setupFileGroupSettings(t)
	oldManager := vtui.FrameManager
	vtui.FrameManager = nil
	t.Cleanup(func() { vtui.FrameManager = oldManager })
	d, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	d.Records["filegroups"] = nil
	if err := d.PreviewFunc(d); err != nil {
		t.Fatal(err)
	}
	if len(fileGroupRuntime.Appearance().Groups()) != 0 {
		t.Fatal("preview reintroduced a deleted standard row")
	}
	d.Close()
	if len(fileGroupRuntime.Appearance().Groups()) != 1 {
		t.Fatal("Cancel failed to restore committed groups")
	}
}

func TestFileGroupsActionsStayReachableOnNarrowPage(t *testing.T) {
	setupFileGroupSettings(t)
	d, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	c := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: d}})
	c.selectCategory("filegroups")
	v := c.fileGroupOverview
	v.SetPosition(0, 0, 69, 24)
	for _, button := range v.actions.buttons {
		if button.X1 < v.X1 || button.X2 > v.X2 || button.X1 > button.X2 || button.Y1 <= v.table.Y2 || button.Y2 > v.Y2 {
			t.Fatalf("action %q is clipped or overlaps table: (%d,%d)-(%d,%d)", button.GetCaption(), button.X1, button.Y1, button.X2, button.Y2)
		}
	}
}

func showFileGroupsTestCenter(t *testing.T, c *settingsCenter) {
	t.Helper()
	oldManager := vtui.FrameManager
	vtui.FrameManager = vtui.NewFrameManager()
	t.Cleanup(func() { vtui.FrameManager = oldManager })
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(140, 40)
	c.Show(screen)
}

func fileGroupsTestConfirm(t *testing.T, choice int) {
	t.Helper()
	confirm, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok || confirm.OnResult == nil {
		t.Fatalf("confirmation missing: %T", vtui.FrameManager.GetTopFrame())
	}
	confirm.OnResult(choice)
	vtui.FrameManager.RemoveFrame(confirm)
}

func TestFileGroupPreviewUsesOrderedCascadeAndFourStates(t *testing.T) {
	setupFileGroupSettings(t)
	groups := []filegroups.Group{
		{ID: "one", Fields: map[string]string{"Name": "First", "Mask": "*.txt", "MaskSyntax": "far", "Highlight.Enabled": "true", "NormalColor": "foreground:#112233", "SelectedColor": "foreground:#223344", "CursorColor": "foreground:#334455", "SelectedCursorColor": "foreground:#445566"}},
		{ID: "two", Fields: map[string]string{"Name": "Second", "Mask": "*.txt", "MaskSyntax": "far", "Highlight.Enabled": "true", "NormalColor": "foreground:#aabbcc"}},
	}
	d := f4settings.NewDraft(nil, groupsToRecords(groups))
	defer d.Close()
	_, previews, err := buildFileGroupPreviews(d, "generic")
	if err != nil {
		t.Fatal(err)
	}
	p := previews["two"]
	if p.style.Normal.Foreground != "#112233" || p.style.Selected.Foreground != "#223344" || p.style.Cursor.Foreground != "#334455" || p.style.SelectedCursor.Foreground != "#445566" {
		t.Fatal(p.style)
	}
	d.Records["filegroups"][0], d.Records["filegroups"][1] = d.Records["filegroups"][1], d.Records["filegroups"][0]
	_, previews, err = buildFileGroupPreviews(d, "generic")
	if err != nil {
		t.Fatal(err)
	}
	if previews["one"].style.Normal.Foreground != "#AABBCC" {
		t.Fatalf("preview ignored reordered rules: %+v", previews["one"].style)
	}
}

func TestFileGroupOverviewSemanticAndHeaderControls(t *testing.T) {
	setupFileGroupSettings(t)
	d, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	c := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: d}})
	c.selectCategory("filegroups")
	v := c.fileGroupOverview
	n := v.SemanticNode(&vtui.SemanticContext{})
	if n["fileGroupsOverview"] != true {
		t.Fatal("overview projection missing")
	}
	var table map[string]any
	for _, child := range semantic.AppMapSlice(n["children"]) {
		if child["layoutRole"] == "overview-table" {
			table = child
		}
	}
	if table == nil {
		t.Fatal("native overview table absent")
	}
	rows := semantic.AppMapSlice(table["rows"])
	if len(rows) != 1 || rows[0]["separator"] != "" || len(semantic.AppMap(rows[0]["appearance"])) == 0 {
		t.Fatal(rows)
	}
	if !v.table.HandleSemanticAction(map[string]any{"action": "control.select", "index": 0}) || v.selectedID != "factory" {
		t.Fatal("first group cannot take cursor")
	}
	child := v.childEditor(0)
	defer child.sessions[0].draft.Close()
	child.selectCategory("filegroup-Highlighting")
	projection := child.page.SemanticNode(&vtui.SemanticContext{})
	var headers []map[string]any
	for _, box := range semantic.AppMapSlice(projection["children"]) {
		if box["layoutRole"] != "section-header" {
			continue
		}
		if h := semantic.AppMap(box["headerControl"]); len(h) > 0 {
			headers = append(headers, h)
		}
	}
	if len(headers) != 4 {
		t.Fatalf("single page has %d checkable feature headers", len(headers))
	}
	if !child.HandleSemanticAction(map[string]any{"target": headers[0]["id"], "action": "control.toggle"}) {
		t.Fatal("header toggle not routed")
	}
}

func TestFileGroupFilteredMarkingUsesVisibleIdentity(t *testing.T) {
	setupFileGroupSettings(t)
	d, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.Records["filegroups"] = append(d.Records["filegroups"], f4settings.Record{ID: "second", Values: map[string]string{"group.Name": "Second", "group.Mask": "*.txt"}})
	c := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: d}})
	c.selectCategory("filegroups")
	v := c.fileGroupOverview
	v.table.SetSearchText("Second")
	if v.table.ItemCount != 1 {
		t.Fatal(v.table.ItemCount)
	}
	v.toggleMark(0)
	if !v.marked["second"] || v.marked["factory"] {
		t.Fatal("marked raw rather than visible row")
	}
	if v.selectedID != "second" {
		t.Fatal("search left stale selection")
	}
}

func TestFileGroupNewChildCancelRemovesProvisionalRecord(t *testing.T) {
	setupFileGroupSettings(t)
	d, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	c := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: d}})
	c.selectCategory("filegroups")
	v := c.fileGroupOverview
	before := len(d.Records["filegroups"])
	d.Records["filegroups"] = append(d.Records["filegroups"], f4settings.Record{ID: "new", Values: map[string]string{"group.Name": "New"}})
	v.pendingAddedID = "new"
	child := v.childEditor(before)
	child.sessions[0].draft.Close()
	if len(d.Records["filegroups"]) != before || v.pendingAddedID != "" {
		t.Fatal("cancel left provisional group in parent draft")
	}
}

func TestFileGroupOverviewThemeContextDoesNotChangeApplicationTheme(t *testing.T) {
	setupFileGroupSettings(t)
	applicationTheme := fileGroupActiveTheme
	d := f4settings.NewDraft(nil, groupsToRecords([]filegroups.Group{
		{ID: "universal", Fields: map[string]string{"Name": "Universal", "Mask": "*.txt"}},
		{ID: "other", Fields: map[string]string{"Name": "Other", "IconTheme": "other", "Mask": "*.txt"}},
	}))
	defer d.Close()
	d.Records["filegroups.themes"] = []f4settings.Record{
		{ID: applicationTheme, Values: map[string]string{"Name": "App"}},
		{ID: "other", Values: map[string]string{"Name": "Other", "IconCatalog": "zoin"}},
	}
	c := newSettingsCenter([]*settingsSession{{catalog: (fileGroupsProvider{}).Catalog(), draft: d}})
	c.selectCategory("filegroups")
	v := c.fileGroupOverview
	for i, id := range v.themeIDs {
		if id == "other" {
			v.themes.Menu.OnAction(i)
			break
		}
	}
	if v.themeID != "other" || fileGroupActiveTheme != applicationTheme {
		t.Fatal("editing context changed application theme")
	}
	if v.catalog() != "zoin" {
		t.Fatal("rule scope was mistaken for artwork catalog")
	}
	if len(v.rows) != 2 || v.rows[0].record.ID != "other" || v.rows[1].record.ID != "universal" {
		t.Fatal("active sets not projected in reverse order without separators")
	}
}
