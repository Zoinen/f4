package filegroups

import (
	"fmt"
	"sort"
	"strings"
)

// Contribution is a frontend-neutral defaults/catalog document. Frontends send
// it once at startup, never with panel frames. Unknown themes stay editable.
type Contribution struct {
	Groups    []Group
	Themes    []IconTheme
	ThemeSets []ThemeSet
}

func ParseContribution(data []byte) (Contribution, error) {
	document, err := Parse(data)
	if err != nil {
		return Contribution{}, err
	}
	groups, err := document.Groups()
	if err != nil {
		return Contribution{}, err
	}
	if _, err = Compile(groups); err != nil {
		return Contribution{}, err
	}
	result := Contribution{Groups: groups}
	result.ThemeSets, err = document.Themes()
	if err != nil {
		return Contribution{}, err
	}
	for _, section := range document.SectionNames() {
		if !strings.HasPrefix(section, "IconTheme.") {
			continue
		}
		id := strings.TrimPrefix(section, "IconTheme.")
		if err := validID(id); err != nil {
			return Contribution{}, err
		}
		fields := document.Section(section)
		theme := IconTheme{ID: id, Name: fields["Name"], Fallbacks: map[string]IconReference{}}
		for key, raw := range fields {
			if strings.HasPrefix(key, "Asset.") {
				theme.Assets = append(theme.Assets, strings.TrimPrefix(key, "Asset."))
			}
			if strings.HasPrefix(key, "Fallback.") {
				ref, err := ParseIconReference(raw)
				if err != nil {
					return Contribution{}, fmt.Errorf("%s: %w", section, err)
				}
				theme.Fallbacks[strings.TrimPrefix(key, "Fallback.")] = ref
			}
		}
		sort.Strings(theme.Assets)
		result.Themes = append(result.Themes, theme)
		// Carry only compact catalog metadata into immutable matching snapshots.
		// Artwork URLs and pixels remain owned by the frontend.
		for i := range result.ThemeSets {
			if result.ThemeSets[i].Catalog() == id {
				result.ThemeSets[i].Fields["CatalogAssets"] = strings.Join(theme.Assets, ",")
			}
		}
	}
	return result, nil
}
