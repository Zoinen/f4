package settings

import (
	"maps"
	"strconv"
	"strings"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/sdk/f4settings"
)

var fileGroupEditorColorKeys = []string{"NormalColor", "SelectedColor", "CursorColor", "SelectedCursorColor"}

// The form keeps four ordinary controls, but their storage belongs to the
// selected colour theme. Each child owns its routing state and theme records.
type fileGroupEditorColors struct {
	themeID, name string
	loaded        map[string]string
}

func newFileGroupEditorColors(d *f4settings.Draft) *fileGroupEditorColors {
	if len(d.Records["filegroups.colorthemes"]) == 0 || fileGroupDraftColorThemeID(d) == "" {
		return nil
	}
	r := &fileGroupEditorColors{}
	r.load(d)
	return r
}

func (r *fileGroupEditorColors) load(d *f4settings.Draft) {
	r.themeID = fileGroupDraftColorThemeID(d)
	r.name = filegroups.IconName(d.Values["group.Name"])
	r.loaded = map[string]string{}
	for _, key := range fileGroupEditorColorKeys {
		delete(d.Values, "group."+key)
	}
	for _, record := range d.Records["filegroups.colorthemes"] {
		if record.ID != r.themeID {
			continue
		}
		for _, key := range fileGroupEditorColorKeys {
			if value, ok := record.Values["Color."+r.name+"."+key]; ok {
				d.Values["group."+key] = value
				r.loaded[key] = value
			}
		}
		break
	}
}

func (r *fileGroupEditorColors) sync(d *f4settings.Draft) {
	if r == nil {
		return
	}
	for _, key := range fileGroupEditorColorKeys {
		value, present := d.Values["group."+key]
		old, existed := r.loaded[key]
		if present == existed && value == old {
			continue
		}
		records := d.Records["filegroups.colorthemes"]
		index := -1
		for i := range records {
			if records[i].ID == r.themeID {
				index = i
				break
			}
		}
		if index < 0 {
			records = append(records, f4settings.Record{ID: r.themeID, Values: map[string]string{}})
			index = len(records) - 1
		}
		if records[index].Values == nil {
			records[index].Values = map[string]string{}
		}
		field := "Color." + r.name + "." + key
		if present {
			records[index].Values[field] = value
			r.loaded[key] = value
		} else {
			delete(records[index].Values, field)
			delete(r.loaded, key)
		}
		d.Records["filegroups.colorthemes"] = records
	}
	// A rename selects another entry; it never moves or copies the old one.
	if r.name != filegroups.IconName(d.Values["group.Name"]) || r.themeID != fileGroupDraftColorThemeID(d) {
		r.load(d)
	}
}

func fileGroupEditorRecordValues(values map[string]string, colors *fileGroupEditorColors) map[string]string {
	result := maps.Clone(values)
	for key := range result {
		if !strings.HasPrefix(key, "group.") || colors != nil && fileGroupColourState(key) >= 0 {
			delete(result, key)
		}
	}
	return result
}

// Palette controls edit their real backing fields. Other palettes stay in the
// draft unchanged, and field reset can use the ordinary factory inheritance.
func fileGroupPalettePrefix(values map[string]string) string {
	prefix := "group." + filegroups.PalettePrefix(config.App.ColorStyle)
	for key := range values {
		if strings.HasPrefix(key, prefix) {
			return prefix
		}
	}
	return ""
}

func fileGroupFieldKey(id string) string {
	id = strings.TrimPrefix(id, "group.")
	if strings.HasPrefix(id, "Palette.") {
		parts := strings.SplitN(id, ".", 3)
		if len(parts) == 3 {
			return parts[2]
		}
	}
	return id
}

func fileGroupHighlightField(id string) bool {
	switch fileGroupFieldKey(id) {
	case "Highlight.Enabled", "NormalColor", "SelectedColor", "CursorColor", "SelectedCursorColor",
		"Mark", "ContinueProcessing", "UseDefaults":
		return true
	}
	return false
}

func fileGroupConditionField(id string) bool {
	switch fileGroupFieldKey(id) {
	case "Mask", "MaskSyntax", "Case", "IncludeAttributes", "ExcludeAttributes",
		"SizeAbove", "SizeBelow", "DateType", "DateRelative", "DateAfter", "DateBefore":
		return true
	}
	return false
}

func fileGroupPaletteFields(fields []f4settings.Field, values map[string]string) []f4settings.Field {
	prefix := fileGroupPalettePrefix(values)
	if prefix == "" {
		return fields
	}
	result := []f4settings.Field{}
	for _, field := range fields {
		if fileGroupHighlightField(field.ID) {
			// An explicit universal override remains universal and editable.
			if _, global := values[field.ID]; !global {
				field.ID = prefix + fileGroupFieldKey(field.ID)
			}
		}
		result = append(result, field)
	}
	order := groupField(strings.TrimPrefix(prefix, "group.")+"Order", "Highlight priority",
		"Order inside the current colour scheme's cascade. Lower numbers are processed first.", f4settings.Integer)
	order.Group = "Highlighting"
	result = append(result, order)
	for _, field := range fields {
		if !fileGroupConditionField(field.ID) {
			continue
		}
		field.ID = prefix + fileGroupFieldKey(field.ID)
		field.Label = f4settings.Text{English: "Highlight: " + field.Label.English}
		field.Description = f4settings.Text{English: "Matching for the current colour scheme's highlighting only. " +
			"The group's main conditions still control icons, sorting and associations."}
		field.Group = "Highlight conditions (" + config.App.ColorStyle + ")"
		result = append(result, field)
	}
	return result
}

func fileGroupRecordGroup(record f4settings.Record) filegroups.Group {
	fields := map[string]string{}
	for key, value := range record.Values {
		if name, ok := strings.CutPrefix(key, "group."); ok {
			fields[name] = value
		}
	}
	return filegroups.Group{ID: record.ID, Fields: fields}
}

func fileGroupRecordConditions(record f4settings.Record) string {
	base := fileGroupCondition(record.Values)
	group := fileGroupRecordGroup(record)
	highlight := group.Highlighting(config.App.ColorStyle)
	if !highlight.Enabled("Highlight") {
		return base
	}
	values := map[string]string{}
	for key, value := range highlight.Fields {
		values["group."+key] = value
	}
	condition := fileGroupCondition(values)
	if condition != base {
		return base + " · Highlight: " + condition
	}
	return base
}

func reorderFileGroupPalette(records []f4settings.Record, scope string) {
	prefix := "group." + filegroups.PalettePrefix(config.App.ColorStyle)
	order := 0
	for _, record := range records {
		if record.Values["group.IconTheme"] != scope || fileGroupPalettePrefix(record.Values) == "" {
			continue
		}
		record.Values[prefix+"Order"] = strconv.Itoa(order)
		order++
	}
}
