package filegroups

import (
	"strconv"
	"strings"
)

var migrationColorAliases = map[string][]string{
	"NormalColor":         {"NormalFileName"},
	"SelectedColor":       {"SelectedFileName"},
	"CursorColor":         {"NormalColorUnderCursor", "FileNameUnderCursor"},
	"SelectedCursorColor": {"SelectedColorUnderCursor", "FileNameSelectedUnderCursor"},
}

func migrationColorField(key string) bool {
	for canonical, aliases := range migrationColorAliases {
		if key == canonical {
			return true
		}
		for _, alias := range aliases {
			if key == alias {
				return true
			}
		}
	}
	return false
}

func migrationColors(fields map[string]string) map[string]string {
	colors := map[string]string{}
	for canonical, aliases := range migrationColorAliases {
		if value, present := fields[canonical]; present {
			// Presence, including an explicit clear, takes precedence over aliases.
			colors[canonical] = value
			continue
		}
		for _, alias := range aliases {
			if value, present := fields[alias]; present {
				colors[canonical] = value
				if value != "" {
					break
				}
			}
		}
	}
	return colors
}

func preserveMigrationColor(fields map[string]string, originalKey, value string) {
	base := "Legacy.Color." + originalKey
	for suffix := 0; ; suffix++ {
		key := base
		if suffix != 0 {
			key += "." + strconv.Itoa(suffix)
		}
		if old, present := fields[key]; !present {
			fields[key] = value
			return
		} else if old == value {
			return
		}
	}
}

func captureCurrentColors(groups []Group, style string) ColorTheme {
	current := ColorTheme{ID: "current", Fields: map[string]string{"Name": "Current"}}
	owners := map[string]int{}
	for _, original := range groups {
		effective := original.Highlighting(style)
		colors := migrationColors(effective.Fields)
		if len(colors) == 0 && !effective.Enabled("Highlight") {
			continue
		}
		priority := 0
		if !effective.Enabled("Highlight") {
			priority += 2
		}
		if scope := original.Fields["ColorStyle"]; scope != "" && scope != style {
			priority++
		}
		name := IconName(original.Label())
		if owner, present := owners[name]; present && owner <= priority {
			continue
		}
		owners[name] = priority
		prefix := "Color." + name + "."
		for _, field := range colorFields {
			delete(current.Fields, prefix+field)
			if value, present := colors[field]; present {
				current.Fields[prefix+field] = value
			}
		}
	}
	return current
}

// NormalizeColorGroups strips legacy appearance data without changing the
// caller's selected colour theme. Already normalized records are cloned intact.
func NormalizeColorGroups(groups []Group, style string) []Group {
	result := make([]Group, 0, len(groups))
	for _, group := range groups {
		raw := false
		for key := range group.Fields {
			raw = raw || strings.HasPrefix(key, "Palette.") || migrationColorField(key)
		}
		if raw {
			normalized, _ := captureGroupColors([]Group{group}, style)
			result = append(result, normalized[0])
		} else {
			result = append(result, group.Clone())
		}
	}
	return result
}

func (s Store) StandardGroups() []Group {
	if s.ColorDefaults != nil {
		return NormalizeColorGroups(s.Defaults, s.ColorStyle)
	}
	result := make([]Group, 0, len(s.Defaults))
	for _, group := range s.Defaults {
		result = append(result, group.Clone())
	}
	return result
}

// captureGroupColors migrates a materialized working set, never a sparse patch.
// The active legacy predicate remains group-owned; only its four colours move.
func captureGroupColors(groups []Group, style string) ([]Group, ColorTheme) {
	result := make([]Group, 0, len(groups))
	current := captureCurrentColors(groups, style)
	for _, original := range groups {
		group := original.Clone()
		if group.Fields == nil {
			group.Fields = map[string]string{}
		}
		effective := original.Highlighting(style)
		for key := range original.Fields {
			if migrationColorField(key) {
				preserveMigrationColor(group.Fields, key, original.Fields[key])
				delete(group.Fields, key)
			}
			if strings.HasPrefix(key, "Palette.") {
				// Preserve extension data outside the retired palette namespace.
				field := paletteFieldKey(key)
				known := field == "Name" || field == "LegacyID" || field == "Order" || field == "Enabled" || field == "ColorStyle" || migrationColorField(field)
				for _, name := range append(append([]string{}, HighlightConditionKeys...), HighlightFields...) {
					known = known || field == name
				}
				if known {
					if migrationColorField(field) {
						preserveMigrationColor(group.Fields, key, original.Fields[key])
					}
					delete(group.Fields, key)
				} else {
					if _, collision := group.Fields["Legacy."+key]; !collision {
						group.Fields["Legacy."+key] = original.Fields[key]
						delete(group.Fields, key)
					}
				}
			}
		}
		for _, key := range HighlightConditionKeys {
			group.Fields["Highlight."+key] = effective.Fields[key]
		}
		for _, key := range []string{"Mark", "ContinueProcessing", "UseDefaults"} {
			if value, present := effective.Fields[key]; present {
				group.Fields[key] = value
			}
		}
		if raw, explicit := effective.Fields["Highlight.Enabled"]; explicit {
			enabled := booleanValue(raw, false)
			// A group-wide disable must not rewrite its independent feature bit.
			// A palette-only disable still applies to that palette's highlighting.
			if _, global := original.Fields["Enabled"]; !global && !booleanValue(effective.Fields["Enabled"], true) {
				enabled = false
			}
			group.Fields["Highlight.Enabled"] = strconv.FormatBool(enabled)
		}
		if scope := original.Fields["ColorStyle"]; scope != "" && scope != style {
			group.Fields["Highlight.Enabled"] = "false"
		}
		delete(group.Fields, "ColorStyle")
		result = append(result, group)
	}
	return result, current
}
