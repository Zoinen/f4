package filegroups

import (
	"encoding/hex"
	"fmt"
	"maps"
	"reflect"
	"strings"
	"unicode/utf8"
)

type paletteAlias struct {
	id, prefix, style string
	fields            map[string]string
}

// discardMigratedFactoryOrder is called only after a declared alias migrates.
// Old Apply saved every factory identity, freezing the obsolete listing. Empty
// factory patches can now inherit the reorganized defaults; edited records,
// command clears and independent user records retain their order and contents.
// Ordinary canonical-only documents never pass through this repair.
func discardMigratedFactoryOrder(defaults, overrides []Group) []Group {
	factory := make(map[string]bool, len(defaults))
	for _, group := range defaults {
		factory[group.ID] = true
	}
	result := make([]Group, 0, len(overrides))
	for _, group := range overrides {
		if factory[group.ID] && len(group.Fields) == 0 && group.Commands == nil {
			continue
		}
		result = append(result, group)
	}
	return result
}

// migratePaletteGroups uses declared ancestry only. An imported legacy.highlight
// record, or an unrelated style.* user record, is never inferred to be factory data.
func migratePaletteGroups(defaults, overrides []Group) ([]Group, bool, error) {
	aliases := map[string]paletteAlias{}
	for _, group := range defaults {
		for key, legacy := range group.Fields {
			if !strings.HasPrefix(key, "Palette.") || !strings.HasSuffix(key, ".LegacyID") || !strings.HasPrefix(legacy, "style.") {
				continue
			}
			prefix := strings.TrimSuffix(key, "LegacyID")
			raw := strings.TrimSuffix(strings.TrimPrefix(prefix, "Palette."), ".")
			name, err := hex.DecodeString(raw)
			if err != nil || len(name) == 0 || !utf8.Valid(name) {
				return nil, false, fmt.Errorf("file group %s has invalid palette identity %q", group.ID, raw)
			}
			target := paletteAlias{id: group.ID, prefix: prefix, style: string(name), fields: map[string]string{}}
			for field, value := range group.Fields {
				if strings.HasPrefix(field, prefix) {
					target.fields[strings.TrimPrefix(field, prefix)] = value
				}
			}
			delete(target.fields, "LegacyID")
			if old, exists := aliases[legacy]; exists && (old.id != target.id || old.prefix != prefix) {
				return nil, false, fmt.Errorf("file group %s has ambiguous palette replacements", legacy)
			}
			aliases[legacy] = target
		}
	}
	if len(aliases) == 0 {
		return overrides, false, nil
	}
	var result []Group
	if overrides != nil {
		result = make([]Group, 0, len(overrides))
	}
	positions := map[string]int{}
	appendPatch := func(patch Group) error {
		if index, exists := positions[patch.ID]; exists {
			previous := &result[index]
			for key, value := range patch.Fields {
				if old, present := previous.Fields[key]; present && old != value {
					return fmt.Errorf("cannot merge palette overrides for %s: conflicting field %s", patch.ID, key)
				}
			}
			if previous.Fields == nil {
				previous.Fields = map[string]string{}
			}
			maps.Copy(previous.Fields, patch.Fields)
			if patch.Commands != nil {
				if previous.Commands != nil && !reflect.DeepEqual(previous.Commands, patch.Commands) {
					return fmt.Errorf("cannot merge palette overrides for %s: conflicting commands", patch.ID)
				}
				previous.Commands = patch.Commands
			}
			return nil
		}
		positions[patch.ID] = len(result)
		result = append(result, patch)
		return nil
	}
	for _, original := range overrides {
		patch := original.Clone()
		target, mapped := aliases[patch.ID]
		if mapped && patch.Fields["Factory"] != "false" {
			if independentPaletteGroup(patch, target.style) {
				fields := maps.Clone(target.fields)
				fields["ColorStyle"] = target.style
				maps.Copy(fields, patch.Fields)
				// Factory=false makes materialization stable across Begin/Apply.
				fields["Factory"] = "false"
				patch.Fields = fields
				if err := appendPatch(patch); err != nil {
					return nil, false, err
				}
				patch = Group{ID: target.id, Fields: map[string]string{target.prefix + "Highlight.Enabled": "false"}}
			} else {
				fields := map[string]string{}
				for key, value := range patch.Fields {
					if key != "Factory" && key != "ColorStyle" && key != "LegacyID" {
						fields[target.prefix+key] = value
					}
				}
				patch = Group{ID: target.id, Fields: fields}
			}
		}
		if err := appendPatch(patch); err != nil {
			return nil, false, err
		}
	}
	return result, !reflect.DeepEqual(result, overrides), nil
}

func independentPaletteGroup(group Group, style string) bool {
	if group.Commands != nil {
		return true
	}
	for key, value := range group.Fields {
		if iconField(key) || key == "IconTheme" || key == "CommandOrder" || strings.HasPrefix(key, "Sort.") || strings.HasPrefix(key, "Associations.") {
			return true
		}
		if key == "ColorStyle" && value != style {
			return true
		}
	}
	return false
}
