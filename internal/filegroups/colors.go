package filegroups

import (
	"fmt"
	"maps"
	"strings"
)

// ColorTheme contains appearance only; matching and cascade policy belong to groups.
type ColorTheme struct {
	ID     string
	Fields map[string]string
}

var colorFields = []string{"NormalColor", "SelectedColor", "CursorColor", "SelectedCursorColor"}

func (t ColorTheme) Name() string {
	if name := t.Fields["Name"]; name != "" {
		return name
	}
	return t.ID
}

func (t ColorTheme) Colors(name string) map[string]string {
	result := map[string]string{}
	prefix := "Color." + IconName(name) + "."
	for _, field := range colorFields {
		if value, present := t.Fields[prefix+field]; present {
			result[field] = value
		}
	}
	return result
}

func cloneColorThemes(themes []ColorTheme) []ColorTheme {
	result := make([]ColorTheme, 0, len(themes))
	for _, theme := range themes {
		result = append(result, ColorTheme{ID: theme.ID, Fields: maps.Clone(theme.Fields)})
	}
	return result
}

func ValidateColorThemes(themes []ColorTheme) error {
	seen := map[string]bool{}
	for _, theme := range themes {
		if err := validID(theme.ID); err != nil {
			return err
		}
		if seen[theme.ID] {
			return fmt.Errorf("duplicate file colour theme %q", theme.ID)
		}
		seen[theme.ID] = true
	}
	return nil
}

func FactoryColorThemes() []ColorTheme {
	data, err := factoryFiles.ReadFile("colors.default.ini")
	if err != nil {
		return nil
	}
	doc, err := Parse(data)
	if err != nil {
		return nil
	}
	themes, _ := doc.ColorThemes()
	return themes
}

func (d *Document) ColorThemes() ([]ColorTheme, error) {
	result := []ColorTheme{}
	ordered := []Group{}
	for _, section := range d.SectionNames() {
		if id, ok := strings.CutPrefix(section, "FileColorTheme."); ok {
			ordered = append(ordered, Group{ID: id, Fields: d.Section(section)})
		}
	}
	for _, group := range orderGroups(ordered, d.Section("FileGroups")["ColorThemeOrder"]) {
		result = append(result, ColorTheme{ID: group.ID, Fields: group.Fields})
	}
	return result, ValidateColorThemes(result)
}

func mergeColorThemes(defaults, overrides []ColorTheme) []ColorTheme {
	result := cloneColorThemes(defaults)
	indices := map[string]int{}
	for i, theme := range result {
		indices[theme.ID] = i
	}
	for _, theme := range overrides {
		if i, found := indices[theme.ID]; found {
			result[i].Fields = OverlayFields(result[i].Fields, theme.Fields)
		} else {
			indices[theme.ID] = len(result)
			result = append(result, ColorTheme{ID: theme.ID, Fields: maps.Clone(theme.Fields)})
		}
	}
	return result
}

// WithColorThemes shares every compiled predicate and icon reference. The fresh
// root owns its view cache, so icon selection cannot recover an older colour set.
func (s *Snapshot) WithColorThemes(themes []ColorTheme, selected string) (*Snapshot, error) {
	if s == nil {
		return nil, fmt.Errorf("cannot compose colours on a nil snapshot")
	}
	if err := ValidateColorThemes(themes); err != nil {
		return nil, err
	}
	if selected != "" {
		if err := validID(selected); err != nil {
			return nil, err
		}
	}
	root := s
	if s.source != nil {
		root = s.source
	}
	composed := &Snapshot{groups: root.groups, themes: root.themes, colorThemes: cloneColorThemes(themes), colorTheme: selected}
	if _, err := finishSnapshot(composed, root.Groups()); err != nil {
		return nil, err
	}
	if s.source != nil {
		return composed.ForIconTheme(s.iconTheme), nil
	}
	return composed, nil
}

func (s *Snapshot) ColorThemes() []ColorTheme {
	if s == nil {
		return nil
	}
	return cloneColorThemes(s.colorThemes)
}

func (s *Snapshot) ColorThemeID() string {
	if s == nil {
		return ""
	}
	return s.colorTheme
}

func (s *Snapshot) colorHighlightGroups() []Group {
	var selected ColorTheme
	for _, theme := range s.colorThemes {
		if theme.ID == s.colorTheme {
			selected = theme
			break
		}
	}
	groups := make([]Group, 0, len(s.groups))
	for _, compiled := range s.groups {
		group := compiled.group.Highlighting("")
		delete(group.Fields, "ColorStyle")
		for _, field := range colorFields {
			delete(group.Fields, field)
		}
		for _, alias := range []string{"NormalFileName", "SelectedFileName", "NormalColorUnderCursor", "FileNameUnderCursor", "SelectedColorUnderCursor", "FileNameSelectedUnderCursor"} {
			delete(group.Fields, alias)
		}
		colors := selected.Colors(compiled.group.Label())
		maps.Copy(group.Fields, colors)
		if len(colors) == 0 && group.Fields["Mark"] == "" {
			group.Fields["Highlight.Enabled"] = "false"
		} else {
			if _, explicit := compiled.group.Fields["Highlight.Enabled"]; !explicit {
				group.Fields["Highlight.Enabled"] = "true"
			}
			if len(colors) == 0 {
				group.Fields["ContinueProcessing"] = "true"
			}
		}
		groups = append(groups, group)
	}
	return groups
}
