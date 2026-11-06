package filegroups

import (
	"fmt"
	"maps"
	"strings"
)

// ThemeSet scopes complete rules independently of the artwork catalog it uses.
// Unknown fields are retained just like group fields.
type ThemeSet struct {
	ID     string
	Fields map[string]string
}

func (t ThemeSet) Name() string {
	if name := t.Fields["Name"]; name != "" {
		return name
	}
	return t.ID
}

func (t ThemeSet) Catalog() string { return t.Fields["IconCatalog"] }
func (t ThemeSet) Enabled() bool   { return booleanValue(t.Fields["Enabled"], true) }

// Native artwork always keeps its original colours, even with a stale override.
func (t ThemeSet) TintIcons() bool {
	return t.Catalog() != "system" && booleanValue(t.Fields["TintIcons"], false)
}

func (d *Document) Themes() ([]ThemeSet, error) {
	result := []ThemeSet{}
	for _, section := range d.SectionNames() {
		if !strings.HasPrefix(section, "FileGroupTheme.") {
			continue
		}
		id := strings.TrimPrefix(section, "FileGroupTheme.")
		if err := validID(id); err != nil {
			return nil, err
		}
		result = append(result, ThemeSet{ID: id, Fields: d.Section(section)})
	}
	return result, nil
}

func FactoryThemes() []ThemeSet {
	data, err := factoryFiles.ReadFile("highlight.default.ini")
	if err != nil {
		return nil
	}
	d, err := Parse(data)
	if err != nil {
		return nil
	}
	themes, _ := d.Themes()
	return themes
}

func MergeThemes(defaults, overrides []ThemeSet, groups []Group) []ThemeSet {
	base := []Group{}
	patch := []Group{}
	for _, t := range defaults {
		base = append(base, Group{ID: t.ID, Fields: t.Fields})
	}
	for _, t := range overrides {
		patch = append(patch, Group{ID: t.ID, Fields: t.Fields})
	}
	result := []ThemeSet{}
	seen := map[string]bool{}
	for _, g := range Merge(base, patch) {
		result = append(result, ThemeSet{ID: g.ID, Fields: maps.Clone(g.Fields)})
		seen[g.ID] = true
	}
	// Referenced but unavailable sets remain visible/editable, not discarded.
	for _, g := range groups {
		id := g.IconTheme()
		if id == "" || seen[id] {
			continue
		}
		result = append(result, ThemeSet{ID: id, Fields: map[string]string{"Name": id}})
		seen[id] = true
	}
	return result
}

func ValidateThemes(themes []ThemeSet) error {
	seen := map[string]bool{}
	for _, t := range themes {
		if err := validID(t.ID); err != nil {
			return err
		}
		if seen[t.ID] {
			return fmt.Errorf("duplicate file-group theme %q", t.ID)
		}
		seen[t.ID] = true
		switch t.Fields["TintIcons"] {
		case "", "true", "false", "0", "1":
		default:
			return fmt.Errorf("theme %s: invalid TintIcons boolean", t.ID)
		}
		if id := t.Catalog(); id != "" {
			if err := validID(id); err != nil {
				return err
			}
		}
	}
	return nil
}

func themeDifferences(defaults, themes []ThemeSet) []ThemeSet {
	base, current := []Group{}, []Group{}
	for _, t := range defaults {
		base = append(base, Group{ID: t.ID, Fields: t.Fields})
	}
	for _, t := range themes {
		current = append(current, Group{ID: t.ID, Fields: t.Fields})
	}
	result := []ThemeSet{}
	for _, g := range Differences(base, current) {
		result = append(result, ThemeSet{ID: g.ID, Fields: g.Fields})
	}
	return result
}
