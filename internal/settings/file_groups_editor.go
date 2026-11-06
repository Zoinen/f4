package settings

import (
	"context"
	"maps"
	"strings"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

// childEditor never writes the store: accepting it updates only the parent draft.
func (v *fileGroupsOverview) childEditor(index int) *settingsCenter {
	parent := v.session.draft
	originalScope := parent.Records["filegroups"][index].Values["group.IconTheme"]
	newRecord := v.pendingAddedID == parent.Records["filegroups"][index].ID
	scratch := f4settings.NewDraft(nil, parent.Records)
	record := &scratch.Records["filegroups"][index]
	scratch.Values = maps.Clone(record.Values)
	if selected, ok := parent.Values["filegroups.colorTheme"]; ok {
		scratch.Values["filegroups.colorTheme"] = selected
	}
	colors := newFileGroupEditorColors(scratch)
	if colors != nil {
		scratch.Values["filegroups.colorTheme"] = colors.themeID
	}
	if _, exists := scratch.Values["group.Enabled"]; !exists {
		scratch.Values["group.Enabled"] = "true"
		scratch.BaselineRecords["filegroups"][index].Values["group.Enabled"] = "true"
	}
	record.Values = fileGroupEditorRecordValues(scratch.Values, colors)
	scratch.BaselineRecords["filegroups"][index].Values = maps.Clone(record.Values)
	scratch.Baseline = maps.Clone(scratch.Values)
	cat := (fileGroupsProvider{}).Catalog()
	cat.ID = "filegroups.editor"
	cat.Categories = []f4settings.Category{}
	for _, section := range []string{"Conditions", "Appearance", "Sorting", "Associations"} {
		cat.Categories = append(cat.Categories, f4settings.Category{ID: "filegroup-" + section, Label: f4settings.Text{English: section}})
	}
	for _, collection := range cat.Collections {
		if collection.ID != "filegroups" {
			continue
		}
		fields := collection.Fields
		if colors == nil {
			fields = fileGroupPaletteFields(fields, record.Values)
		}
		for _, f := range fields {
			if f.ID == "group.IconTheme" && record.Values[f.ID] == "" {
				continue
			}
			if state := fileGroupColourState(f.ID); state >= 0 {
				f.Label = f4settings.Text{Key: []string{"FileGroups.Normal", "FileGroups.Marked",
					"FileGroups.Cursor", "FileGroups.MarkedCursor"}[state]}
				f.InputWidth = 44
			}
			section := fileGroupFieldSection(f.ID)
			if !strings.HasPrefix(f.Group, "Highlight conditions (") {
				f.Group = section
			}
			if section == "Highlighting" || section == "Icons" {
				section = "Appearance"
			}
			f.Category = "filegroup-" + section
			if f.ID == "group.IconTheme" {
				f.ChoicePresentation = "dropdown"
				f.Choices = []f4settings.Choice{{Value: "", Label: f4settings.Text{English: "Universal"}}}
				for _, set := range fileGroupDraftThemes(parent) {
					f.Choices = append(f.Choices, f4settings.Choice{Value: set.ID, Label: f4settings.Text{English: set.Name(), Literal: true}})
				}
			}
			cat.Fields = append(cat.Fields, f)
		}
	}
	advanced := recordField("filegroup.advanced", "Advanced conditions", "Attributes, size and date restrictions. Existing restrictions are always preserved.", f4settings.Boolean)
	advanced.Category = "filegroup-Conditions"
	advanced.Group = "Conditions"
	cat.Fields = append(cat.Fields, advanced)
	collections := []f4settings.Collection{}
	for _, col := range cat.Collections {
		if col.ID == "filegroups.commands" {
			col.Category = "filegroup-Associations"
			collections = append(collections, col)
		}
	}
	cat.Collections = collections
	accepted := false
	sample := newFileGroupEditorSample()
	scratch.PreviewFunc = func(d *f4settings.Draft) error {
		for _, field := range cat.Fields {
			if fileGroupColourState(field.ID) < 0 || d.Values[field.ID] == d.Baseline[field.ID] {
				continue
			}
			if err := theme.ValidateFarColor(d.Values[field.ID]); err != nil {
				sample.problem = err.Error()
				vtui.DebugLog("FILEGROUPS: colour validation failed field=%s", field.ID)
				return err
			}
		}
		colors.sync(d)
		d.Records["filegroups"][index].Values = fileGroupEditorRecordValues(d.Values, colors)
		themeID := d.Values["group.IconTheme"]
		if themeID == "" {
			themeID = v.themeID
		}
		_, previews, err := buildFileGroupPreviews(d, themeID, v.samples)
		if err != nil {
			sample.problem = err.Error()
			vtui.DebugLog("FILEGROUPS: editor preview validation failed")
			return err
		}
		sample.update(previews[record.ID])
		sample.problem = ""
		sample.catalog = ""
		for _, set := range fileGroupDraftThemes(d) {
			if set.ID == themeID {
				sample.catalog = set.Catalog()
			}
		}
		if parent.PreviewFunc != nil {
			return parent.PreviewFunc(d)
		}
		return nil
	}
	scratch.ValidateFunc = func(d *f4settings.Draft) map[string]error {
		if err := scratch.PreviewFunc(d); err != nil {
			return map[string]error{"group": err}
		}
		return nil
	}
	scratch.CommitFunc = func(_ context.Context, d *f4settings.Draft) f4settings.Result {
		if err := scratch.PreviewFunc(d); err != nil {
			return f4settings.Result{Errors: map[string]error{"group": err}}
		}
		parent.Records = f4settings.NewDraft(nil, d.Records).Records
		if originalScope != d.Values["group.IconTheme"] {
			v.center.moveFileGroupScope(v.session, record.ID, d.Values["group.IconTheme"])
		}
		accepted = true
		vtui.DebugLog("FILEGROUPS: child accepted group=%s", record.ID)
		return f4settings.Result{Applied: d.Changed()}
	}
	scratch.CloseFunc = func() {
		if !accepted {
			vtui.DebugLog("FILEGROUPS: child cancelled group=%s", record.ID)
			if newRecord {
				for i, r := range parent.Records["filegroups"] {
					if r.ID == record.ID {
						parent.Records["filegroups"] = append(parent.Records["filegroups"][:i], parent.Records["filegroups"][i+1:]...)
						break
					}
				}
				removeFileGroupCommands(parent, record.ID)
			}
		}
		if newRecord {
			v.pendingAddedID = ""
		}
		if parent.PreviewFunc != nil {
			_ = parent.PreviewFunc(parent)
		}
		v.changed()
	}
	child := newSettingsCenter([]*settingsSession{{catalog: cat, draft: scratch}})
	// Both dialogs remain rendered at once. Their chrome must not share event
	// and retained-focus identities, even though they use the same form class.
	for _, control := range []vtui.UIElement{child.search, child.sidebar, child.page,
		child.help, child.apply, child.ok, child.cancel, child.clearSearch, child.previous, child.next} {
		control.SetId(control.GetId() + ":filegroup:" + record.ID)
	}
	vtui.DebugLog("FILEGROUPS: nested editor controls scoped group=%s", record.ID)
	child.fileGroupEditor = true
	child.fileGroupEditorID = record.ID
	child.fileGroupEditorSample = sample
	child.AddItem(sample)
	for _, control := range []vtui.UIElement{child.search, child.sidebar, child.clearSearch, child.previous, child.next} {
		control.SetVisible(false)
		control.SetDisabled(true)
	}
	_ = scratch.PreviewFunc(scratch)
	child.SetId("filegroup-editor:" + record.ID)
	for key, value := range record.Values {
		if isAdvancedFileGroupCondition(key) && value != "" && value != "0" && value != "false" {
			child.fileGroupAdvanced = true
		}
	}
	child.offsets["record:filegroups"] = index
	child.apply.SetVisible(false)
	child.apply.SetDisabled(true)
	child.apply.IsDefault = false
	child.ok.IsDefault = true
	child.SetTitle(child.windowTitle())
	child.ok.OnClick = func() { child.commit(true) }
	for _, category := range cat.Categories {
		section := category.ID
		child.sessions[0].catalog.Commands = append(child.sessions[0].catalog.Commands, f4settings.Command{
			ID: "filegroup.reset." + section, Category: section, Group: "Reset", Label: f4settings.Text{English: "Restore section to standard values"}, Run: func(context.Context) error {
				for _, f := range cat.Fields {
					if f.Category == section {
						child.resetFileGroupEditorFieldValue(f.ID)
					}
				}
				if scratch.PreviewFunc != nil {
					_ = scratch.PreviewFunc(scratch)
				}
				child.rebuildCategory()
				return nil
			},
		})
	}
	child.rebuildCategory()
	child.SetFocusedItem(child.page)
	return child
}

func (v *fileGroupsOverview) openEditor(index int) {
	child := v.childEditor(index)
	v.editor = child
	if vtui.FrameManager != nil {
		vtui.FrameManager.Push(child)
	}
}

func (v *fileGroupsOverview) more() {
	menu := vtui.ShowMessageOn(v.center, "File groups", "Additional actions", []string{"Sample", "Use panel entry", "Reset group", "Themes", "Reload", "Cancel"})
	menu.OnResult = func(choice int) {
		switch choice {
		case 0:
			i := v.recordIndex()
			if i < 0 {
				return
			}
			r := v.session.draft.Records["filegroups"][i]
			v.openForm("Preview sample", map[string]string{"Preview.Name": v.previews[r.ID].item.Name, "Result": v.previews[r.ID].contributors}, []string{"Preview.Name", "Result"}, func(values map[string]string) {
				r.Values["group.Preview.Name"] = values["Preview.Name"]
				delete(v.samples, r.ID)
				v.changed()
			})
		case 1:
			v.usePanelSample()
		case 2:
			i := v.recordIndex()
			if i < 0 {
				return
			}
			v.center.resetFileGroup(v.session, i)
			v.changed()
		case 3:
			v.themeActions()
		case 4:
			confirm := vtui.ShowMessageOn(v.center, "Reload file groups", "Discard unapplied group edits and reload?", []string{"Reload", "Cancel"})
			confirm.OnResult = func(result int) {
				if result != 0 {
					return
				}
				d, err := (fileGroupsProvider{}).Begin(context.Background())
				if err != nil {
					v.center.status = err.Error()
					return
				}
				v.session.draft.Close()
				v.session.draft = d
				v.changed()
			}
		}
	}
}

func (v *fileGroupsOverview) openForm(title string, values map[string]string, keys []string, accept func(map[string]string)) {
	d := f4settings.NewDraft(values, nil)
	cat := f4settings.Catalog{ID: "filegroups.form", Categories: []f4settings.Category{{ID: "form", Label: f4settings.Text{English: title}}}}
	for _, key := range keys {
		f := recordField(key, key, "", f4settings.String)
		if key == "TintIcons" {
			f.Kind = f4settings.Boolean
			f.Label = f4settings.Text{English: "Tint icons with file colour"}
			if values["IconCatalog"] == "system" {
				f.Unavailable = "Native icons always keep their original colours."
			}
		}
		if key == "Result" {
			f.Kind = f4settings.Multiline
			f.Unavailable = "Read-only evaluation. Commands are never executed."
		}
		f.Category = "form"
		f.Group = title
		cat.Fields = append(cat.Fields, f)
	}
	d.CommitFunc = func(_ context.Context, d *f4settings.Draft) f4settings.Result {
		accept(d.Values)
		return f4settings.Result{Applied: d.Changed()}
	}
	c := newSettingsCenter([]*settingsSession{{catalog: cat, draft: d}})
	c.SetId("filegroup-form:" + filegroups.NewID())
	c.recordOnly = true
	c.recordTitle = title
	c.SetTitle(title)
	for _, item := range []vtui.UIElement{c.search, c.sidebar, c.clearSearch, c.previous, c.next, c.apply} {
		item.SetVisible(false)
		item.SetDisabled(true)
	}
	c.ok.IsDefault = true
	c.apply.IsDefault = false
	c.rebuildCategory()
	c.SetFocusedItem(c.page)
	if vtui.FrameManager != nil {
		vtui.FrameManager.Push(c)
	}
}

func (v *fileGroupsOverview) usePanelSample() {
	fm := vtui.FrameManager
	if fm == nil || fm.ActiveIdx < 0 || fm.ActiveIdx >= len(fm.Screens) {
		return
	}
	for _, frame := range fm.Screens[fm.ActiveIdx].Frames {
		pf, ok := frame.(*panel.PanelsFrame)
		if !ok {
			continue
		}
		fp := pf.GetActivePanel()
		if fp == nil {
			continue
		}
		index := fp.GetCursorIndex()
		if index < 0 || index >= len(fp.Entries) {
			continue
		}
		v.samples[v.selectedID] = fileGroupSampleData{item: fp.Entries[index].VFSItem, known: !fp.IsLoading}
		v.refresh()
		return
	}
}

func (v *fileGroupsOverview) themeActions() {
	menu := vtui.ShowMessageOn(v.center, "Theme sets", "Theme changes here affect editing and preview only.", []string{"New", "Duplicate", "Properties", "Delete/disable", "Cancel"})
	menu.OnResult = func(choice int) {
		sets := v.session.draft.Records["filegroups.themes"]
		index := -1
		for i, set := range sets {
			if set.ID == v.themeID {
				index = i
			}
		}
		if choice == 0 || choice == 1 {
			id := filegroups.NewID()
			fields := map[string]string{"Name": "New theme", "IconCatalog": ""}
			if choice == 1 && index >= 0 {
				fields = maps.Clone(sets[index].Values)
				fields["Name"] += " (copy)"
			}
			v.session.draft.Records["filegroups.themes"] = append(sets, f4settings.Record{ID: id, Values: fields})
			if choice == 1 {
				oldTheme := v.themeID
				for _, r := range append([]f4settings.Record{}, v.session.draft.Records["filegroups"]...) {
					if r.Values["group.IconTheme"] != oldTheme {
						continue
					}
					v.selectedID = r.ID
					v.duplicate()
					i := v.recordIndex()
					v.session.draft.Records["filegroups"][i].Values["group.IconTheme"] = id
				}
			}
			v.themeID = id
			v.changed()
			return
		}
		if index < 0 {
			return
		}
		switch choice {
		case 2:
			v.openForm("Theme properties", maps.Clone(sets[index].Values), []string{"Name", "IconCatalog", "TintIcons"}, func(values map[string]string) { sets[index].Values = maps.Clone(values); v.changed() })
		case 3:
			if v.themeID == fileGroupActiveTheme {
				v.center.status = "The application's active theme cannot be removed here."
				return
			}
			confirm := vtui.ShowMessageOn(v.center, "Disable theme", "Disable this theme and all its rules? Existing values are preserved.", []string{"Disable", "Cancel"})
			confirm.OnResult = func(result int) {
				if result != 0 {
					return
				}
				sets[index].Values["Enabled"] = "false"
				for _, r := range v.session.draft.Records["filegroups"] {
					if r.Values["group.IconTheme"] == v.themeID {
						r.Values["group.Enabled"] = "false"
					}
				}
				v.themeID = fileGroupActiveTheme
				v.changed()
			}
		}
	}
}

func fileGroupFeatureEnabled(values map[string]string, section string) bool {
	key := map[string]string{"Highlighting": "Highlight", "Icons": "Icons", "Sorting": "Sort", "Associations": "Associations"}[section]
	if strings.HasPrefix(section, "Highlight conditions (") {
		key = "Highlight"
	}
	if key == "Highlight" {
		if values["filegroups.colorTheme"] != "" {
			return values["group.Highlight.Enabled"] == "true"
		}
		group := fileGroupRecordGroup(f4settings.Record{Values: values})
		return group.Highlighting(config.App.ColorStyle).Enabled("Highlight")
	}
	return key == "" || values["group."+key+".Enabled"] == "true"
}

func isFileGroupFeatureToggle(id string) bool {
	return strings.HasSuffix(id, ".Enabled") && id != "group.Enabled"
}

func isAdvancedFileGroupCondition(id string) bool {
	id = "group." + fileGroupFieldKey(id)
	for _, key := range []string{"IncludeAttributes", "ExcludeAttributes", "SizeAbove", "SizeBelow", "DateType", "DateRelative", "DateAfter", "DateBefore"} {
		if id == "group."+key {
			return true
		}
	}
	return false
}

func (c *settingsCenter) resetFileGroupEditorFieldValue(id string) {
	if !c.fileGroupEditor || len(c.sessions) == 0 {
		return
	}
	d := c.sessions[0].draft
	if len(d.Records["filegroups.colorthemes"]) > 0 && fileGroupDraftColorThemeID(d) != "" && fileGroupColourState(id) >= 0 {
		key := fileGroupFieldKey(id)
		standard := fileGroupStandardColorValues(fileGroupDraftColorThemeID(d), d.Values["group.Name"])
		delete(d.Values, id)
		if value, present := standard[key]; present {
			d.Values[id] = value
		}
		return
	}
	delete(d.Values, id)
	for _, g := range currentFileGroupStore().StandardGroups() {
		if g.ID == c.fileGroupEditorID {
			if value, ok := g.Fields[strings.TrimPrefix(id, "group.")]; ok {
				d.Values[id] = value
			}
			break
		}
	}
}

func (c *settingsCenter) resetFileGroupEditorField(id string) {
	c.resetFileGroupEditorFieldValue(id)
	if c.sessions[0].draft.PreviewFunc != nil {
		_ = c.sessions[0].draft.PreviewFunc(c.sessions[0].draft)
	}
	c.rebuildCategory()
}
