package settings

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

func fileGroupRecordLabel(record f4settings.Record) string {
	name := record.Values["group.Name"]
	if name == "" {
		name = record.ID
	}
	if style := record.Values["group.ColorStyle"]; style != "" {
		name += " (" + style + ")"
	}
	features := []string{}
	for _, feature := range []string{"Highlight", "Icons", "Sort", "Associations"} {
		enabled := record.Values["group."+feature+".Enabled"] == "true"
		if feature == "Highlight" {
			enabled = fileGroupFeatureEnabled(record.Values, "Highlighting")
		}
		if enabled {
			features = append(features, feature)
		}
	}
	scope := "Universal"
	if id := record.Values["group.IconTheme"]; id != "" {
		scope = fileGroupThemeName(id)
	}
	return name + " [" + scope + "] " + strings.Join(features, ", ")
}

func fileGroupFieldSection(id string) string {
	id = fileGroupFieldKey(id)
	switch {
	case id == "Order":
		return "Highlighting"
	case id == "IconTheme":
		return "Conditions"
	case strings.HasPrefix(id, "Icon"), id == "OverrideNativeIcon":
		return "Icons"
	case strings.HasPrefix(id, "Sort."):
		return "Sorting"
	case strings.HasPrefix(id, "Associations."):
		return "Associations"
	case strings.HasSuffix(id, "Color"), id == "Mark", id == "ContinueProcessing", id == "UseDefaults", strings.HasPrefix(id, "Highlight."):
		return "Highlighting"
	default:
		return "Conditions"
	}
}

func (c *settingsCenter) addFileGroupTools(s *settingsSession) {
	bar := &settingsButtonRow{Group: vtui.NewGroup(0, 0, 20, 1)}
	button := func(label string, run func()) {
		b := vtui.NewButton(0, 0, label)
		b.OnClick = run
		bar.AddItem(b)
		bar.buttons = append(bar.buttons, b)
	}
	button("Duplicate", func() {
		c.fileGroupSearch = ""
		groups := s.draft.Records["filegroups"]
		if len(groups) == 0 {
			return
		}
		original := groups[min(c.offsets["record:filegroups"], len(groups)-1)]
		copy := f4settings.Record{ID: filegroups.NewID(), Values: maps.Clone(original.Values)}
		copy.Values["group.Name"] += " (copy)"
		delete(copy.Values, "group.Factory")
		delete(copy.Values, "group.ColorStyle")
		delete(copy.Values, "group.LegacyID")
		delete(copy.Values, "group.FactoryFallback")
		c.appendFileGroup(s, copy)
		for _, command := range slices.Clone(s.draft.Records["filegroups.commands"]) {
			if command.Values["command.Group"] != original.ID {
				continue
			}
			command.ID = copy.ID + "/" + filegroups.NewID()
			command.Values = maps.Clone(command.Values)
			command.Values["command.Group"] = copy.ID
			s.draft.Records["filegroups.commands"] = append(s.draft.Records["filegroups.commands"], command)
		}
		c.rebuildCategory()
	})
	button("Reload from disk", func() {
		confirm := vtui.ShowMessageOn(c, "Reload file groups", "Discard unapplied file-group edits and reload the external file?", []string{"Reload", "Cancel"})
		confirm.OnResult = func(choice int) {
			if choice != 0 {
				return
			}
			draft, err := (fileGroupsProvider{}).Begin(context.Background())
			if err != nil {
				c.status = err.Error()
				return
			}
			s.draft.Close()
			s.draft = draft
			c.rebuildCategory()
		}
	})
	c.page.AddItem(bar)
	c.page.rows = append(c.page.rows, &settingsRow{field: f4settings.Field{Category: "filegroups", Group: "File groups", Label: f4settings.Text{English: "Group operations"}}, session: s, control: bar, match: true})
	add := func(field f4settings.Field, read func() string, write func(string)) {
		field.Category = "filegroups"
		if field.Group == "" {
			field.Group = "Test match"
		}
		r := &settingsRow{field: field, session: s, match: true, read: read, write: write}
		r.control = c.makeControl(r)
		r.control.SetId("setting:" + field.ID)
		c.page.AddItem(r.control)
		c.page.rows = append(c.page.rows, r)
	}
	add(recordField("filegroups.sample", "Sample file name", "Read-only evaluation; preview never executes a command.", f4settings.String), func() string { return c.fileGroupSample }, func(value string) { c.fileGroupSample = value })
	test := vtui.NewButton(0, 0, i18n.Msg("FileGroups.TestMatch"))
	test.OnClick = func() {
		groups, err := recordsToGroups(s.draft.Records)
		if err != nil {
			c.fileGroupMatchResult = err.Error()
		} else {
			c.fileGroupMatchResult, err = previewMatchText(groups, c.fileGroupSample)
			if err != nil {
				c.fileGroupMatchResult = err.Error()
			}
		}
		c.rebuildCategory()
	}
	c.page.AddItem(test)
	c.page.rows = append(c.page.rows, &settingsRow{field: f4settings.Field{Category: "filegroups", Group: "Test match", Label: f4settings.Text{English: "Evaluate"}}, session: s, control: test, match: true})
	selectedTest := vtui.NewButton(0, 0, i18n.Msg("FileGroups.TestSelectedEntry"))
	selectedTest.OnClick = func() {
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
			item := fp.Entries[index].VFSItem
			groups, err := recordsToGroups(s.draft.Records)
			if err == nil {
				c.fileGroupSample = item.Name
				c.fileGroupMatchResult, err = previewMatchEntry(groups, &item, !fp.IsLoading)
			}
			if err != nil {
				c.fileGroupMatchResult = err.Error()
			}
			c.rebuildCategory()
			return
		}
	}
	c.page.AddItem(selectedTest)
	c.page.rows = append(c.page.rows, &settingsRow{field: f4settings.Field{Category: "filegroups", Group: "Test match", Label: f4settings.Text{English: "Use available panel metadata"}}, session: s, control: selectedTest, match: true})
	result := recordField("filegroups.match", "Matches", "Commands are listed, never executed.", f4settings.Multiline)
	result.Unavailable = "Read-only match results"
	add(result, func() string { return c.fileGroupMatchResult }, func(string) {})
	resetChoice := recordField("filegroups.resetSection", "Reset section", "Restore all fields in a feature or condition section to standard values.", f4settings.ChoiceKind)
	resetChoice.Group = "Reset"
	for _, section := range []string{"Conditions", "Highlighting", "Icons", "Sorting", "Associations"} {
		resetChoice.Choices = append(resetChoice.Choices, f4settings.Choice{Value: section, Label: f4settings.Text{English: section}})
	}
	if c.fileGroupResetSection == "" {
		c.fileGroupResetSection = "Conditions"
	}
	add(resetChoice, func() string { return c.fileGroupResetSection }, func(value string) { c.fileGroupResetSection = value })
	resetField := recordField("filegroups.resetField", "Reset field", "Choose one field to restore to its standard value; entering an empty value clears it.", f4settings.ChoiceKind)
	resetField.Group = "Reset"
	for _, collection := range s.catalog.Collections {
		if collection.ID != "filegroups" {
			continue
		}
		for _, field := range collection.Fields {
			resetField.Choices = append(resetField.Choices, f4settings.Choice{Value: field.ID, Label: field.Label})
		}
	}
	if c.fileGroupResetField == "" {
		c.fileGroupResetField = "group.Name"
	}
	add(resetField, func() string { return c.fileGroupResetField }, func(value string) { c.fileGroupResetField = value })
	resetBar := &settingsButtonRow{Group: vtui.NewGroup(0, 0, 20, 1)}
	for _, fieldOnly := range []bool{false, true} {
		label := "Reset section"
		if fieldOnly {
			label = "Reset field"
		}
		button := vtui.NewButton(0, 0, label)
		button.OnClick = func() { c.resetFileGroupFields(s, fieldOnly); c.rebuildCategory() }
		resetBar.AddItem(button)
		resetBar.buttons = append(resetBar.buttons, button)
	}
	c.page.AddItem(resetBar)
	c.page.rows = append(c.page.rows, &settingsRow{field: f4settings.Field{Category: "filegroups", Group: "Reset", Label: f4settings.Text{English: "Restore standard values"}}, session: s, control: resetBar, match: true})
}

func (c *settingsCenter) resetFileGroupFields(s *settingsSession, fieldOnly bool) {
	records := s.draft.Records["filegroups"]
	if len(records) == 0 {
		return
	}
	record := &records[min(c.offsets["record:filegroups"], len(records)-1)]
	defaults := map[string]string{}
	for _, g := range currentFileGroupStore().StandardGroups() {
		if g.ID == record.ID {
			defaults = groupsToRecords([]filegroups.Group{g})["filegroups"][0].Values
			break
		}
	}
	for _, collection := range s.catalog.Collections {
		if collection.ID != "filegroups" {
			continue
		}
		for _, field := range collection.Fields {
			if fieldOnly && field.ID != c.fileGroupResetField || !fieldOnly && fileGroupFieldSection(field.ID) != c.fileGroupResetSection {
				continue
			}
			if value, ok := defaults[field.ID]; ok {
				record.Values[field.ID] = value
			} else {
				delete(record.Values, field.ID)
			}
		}
	}
	if !fieldOnly && c.fileGroupResetSection == "Associations" {
		removeFileGroupCommands(s.draft, record.ID)
		for _, group := range currentFileGroupStore().StandardGroups() {
			if group.ID == record.ID {
				s.draft.Records["filegroups.commands"] = append(s.draft.Records["filegroups.commands"], groupsToRecords([]filegroups.Group{group})["filegroups.commands"]...)
				break
			}
		}
	}
}

func (c *settingsCenter) collectionRecordIndices(s *settingsSession, collection string) []int {
	parentID := ""
	if collection == "filegroups.commands" {
		parents := s.draft.Records["filegroups"]
		if len(parents) == 0 {
			return nil
		}
		selected := min(c.offsets["record:filegroups"], len(parents)-1)
		if selected < 0 || (!c.fileGroupEditor && !slices.Contains(c.collectionRecordIndices(s, "filegroups"), selected)) {
			return nil
		}
		parentID = parents[selected].ID
	}
	indices := []int{}
	if collection == "filegroups" {
		for _, scope := range []string{"", fileGroupActiveTheme} {
			if c.fileGroupScopeFilter == "universal" && scope != "" || c.fileGroupScopeFilter == "theme" && scope == "" {
				continue
			}
			for i, record := range s.draft.Records[collection] {
				if record.Values["group.IconTheme"] == scope {
					indices = append(indices, i)
				}
			}
		}
		return indices
	}
	for i, record := range s.draft.Records[collection] {
		if parentID == "" || record.Values["command.Group"] == parentID {
			indices = append(indices, i)
		}
	}
	return indices
}

func (c *settingsCenter) addFileGroupContext(s *settingsSession) {
	field := recordField("filegroups.scopeFilter", "Active icon theme: "+fileGroupThemeName(fileGroupActiveTheme),
		"Switch the application's icon theme to edit another thematic set. Hidden drafts are retained.", f4settings.ChoiceKind)
	field.Category, field.Group = "filegroups", "File groups"
	for _, choice := range []struct{ value, label string }{{"", "All active"}, {"universal", "Universal"}, {"theme", "Current theme"}} {
		field.Choices = append(field.Choices, f4settings.Choice{Value: choice.value, Label: f4settings.Text{English: choice.label}})
	}
	r := &settingsRow{field: field, session: s, match: true,
		read: func() string { return c.fileGroupScopeFilter }, write: func(value string) { c.fileGroupScopeFilter = value }}
	r.control = c.makeControl(r)
	r.control.SetId("setting:" + field.ID)
	c.page.AddItem(r.control)
	c.page.rows = append(c.page.rows, r)
}

// Moving a scope is an ordering operation, not duplication of the record.
func (c *settingsCenter) moveFileGroupScope(s *settingsSession, id, scope string) {
	records := s.draft.Records["filegroups"]
	index := slices.IndexFunc(records, func(r f4settings.Record) bool { return r.ID == id })
	if index < 0 {
		return
	}
	record := records[index]
	record.Values["group.IconTheme"] = scope
	records = slices.Delete(records, index, index+1)
	s.draft.Records["filegroups"] = records
	c.appendFileGroup(s, record)
}

// Shipped catch-alls remain the tail when adding to a thematic set. Inserting
// a new override after the generic file fallback would make it unreachable.
func (c *settingsCenter) appendFileGroup(s *settingsSession, record f4settings.Record) {
	records := s.draft.Records["filegroups"]
	index := len(records)
	for i, existing := range records {
		if existing.Values["group.IconTheme"] == record.Values["group.IconTheme"] &&
			existing.Values["group.FactoryFallback"] == "true" {
			index = i
			break
		}
	}
	s.draft.Records["filegroups"] = slices.Insert(records, index, record)
	c.offsets["record:filegroups"] = index
}

func removeFileGroupCommands(draft *f4settings.Draft, groupID string) {
	draft.Records["filegroups.commands"] = slices.DeleteFunc(draft.Records["filegroups.commands"], func(r f4settings.Record) bool { return r.Values["command.Group"] == groupID })
}

func (c *settingsCenter) resetFileGroup(s *settingsSession, index int) {
	records := s.draft.Records["filegroups"]
	if index < 0 || index >= len(records) {
		return
	}
	id := records[index].ID
	removeFileGroupCommands(s.draft, id)
	for _, group := range currentFileGroupStore().StandardGroups() {
		if group.ID != id {
			continue
		}
		rows := groupsToRecords([]filegroups.Group{group})
		records[index].Values = rows["filegroups"][0].Values
		s.draft.Records["filegroups.commands"] = append(s.draft.Records["filegroups.commands"], rows["filegroups.commands"]...)
		return
	}
	records[index].Values = map[string]string{"group.Enabled": "true", "group.Mask": "*", "group.MaskSyntax": "far", "group.Case": "insensitive"}
}
