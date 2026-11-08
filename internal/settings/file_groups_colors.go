package settings

import (
	"fmt"
	"maps"
	"strings"

	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/sdk/f4settings"
)

const fileGroupColorThemeKey = "filegroups.colorTheme"

func fileGroupDraftColorThemes(d *f4settings.Draft) []filegroups.ColorTheme {
	result := []filegroups.ColorTheme{}
	for _, record := range d.Records["filegroups.colorthemes"] {
		result = append(result, filegroups.ColorTheme{ID: record.ID, Fields: maps.Clone(record.Values)})
	}
	return result
}

func fileGroupDraftColorThemeID(d *f4settings.Draft) string {
	if id := d.Values[fileGroupColorThemeKey]; id != "" {
		return id
	}
	for _, record := range d.Records["filegroups.colorthemes"] {
		return record.ID
	}
	return ""
}

func fileGroupStandardColorValues(themeID, groupName string) map[string]string {
	for _, set := range currentFileGroupStore().ColorDefaults {
		if set.ID == themeID {
			return set.Colors(groupName)
		}
	}
	return map[string]string{}
}

func compileFileGroupDraft(d *f4settings.Draft) (*filegroups.Snapshot, error) {
	groups, err := recordsToGroups(d.Records)
	if err != nil {
		return nil, err
	}
	snapshot, err := filegroups.Compile(groups, fileGroupDraftThemes(d))
	if err != nil {
		return nil, err
	}
	sets := fileGroupDraftColorThemes(d)
	if len(sets) == 0 {
		return snapshot, nil
	}
	for _, set := range sets {
		for key, value := range set.Fields {
			_, field, valid := strings.Cut(strings.TrimPrefix(key, "Color."), ".")
			known := field == "NormalColor" || field == "SelectedColor" || field == "CursorColor" || field == "SelectedCursorColor"
			if !strings.HasPrefix(key, "Color.") || !valid || !known {
				continue
			}
			if err := theme.ValidateFarColor(value); err != nil {
				return nil, fmt.Errorf("color theme %s, %s: %w", set.Name(), key, err)
			}
		}
	}
	return snapshot.WithColorThemes(sets, fileGroupDraftColorThemeID(d))
}
