package filegroups

import (
	"fmt"
	"maps"
	"reflect"
	"strings"
)

// migrateCatalogGroups folds explicitly listed factory aliases into shared
// groups. Customized predicates or non-icon features remain independent user
// rules. The inputs are never modified; publication belongs to Store.Commit.
func migrateCatalogGroups(defaults, overrides []Group) ([]Group, bool, error) {
	targets := map[string]Group{}
	for _, group := range defaults {
		for _, raw := range strings.Split(group.Fields["LegacyIDs"], ",") {
			id := strings.TrimSpace(raw)
			if id == "" || id == group.ID {
				continue
			}
			if previous, exists := targets[id]; exists && previous.ID != group.ID {
				return nil, false, fmt.Errorf("file group %s has ambiguous catalog replacements %s and %s", id, previous.ID, group.ID)
			}
			targets[id] = group
		}
	}

	var result []Group
	if overrides != nil {
		result = make([]Group, 0, len(overrides))
	}
	positions := map[string]int{}
	// Reserve current IDs too: a mapped alias must merge with an existing
	// canonical patch, regardless of which one occurs first in user order.
	canonical := map[string]bool{}
	for _, target := range targets {
		canonical[target.ID] = true
	}
	for _, original := range overrides {
		patch := original.Clone()
		target, mapped := targets[patch.ID]
		if mapped && !(patch.Fields["Factory"] == "false" && patch.IconTheme() == "") {
			catalog := legacyCatalog(patch)
			if catalogCustomized(patch, target, catalog) {
				patch = catalogUserGroup(target, patch, catalog)
			} else {
				patch.ID = target.ID
				patch.Fields = catalogPatchFields(patch.Fields, catalog)
			}
		}
		if canonical[patch.ID] {
			if index, exists := positions[patch.ID]; exists {
				for key, value := range patch.Fields {
					if old, exists := result[index].Fields[key]; exists && old != value {
						return nil, false, fmt.Errorf("cannot merge catalog overrides for %s: conflicting field %s", patch.ID, key)
					}
				}
				if patch.Commands != nil && result[index].Commands != nil && !reflect.DeepEqual(patch.Commands, result[index].Commands) {
					return nil, false, fmt.Errorf("cannot merge catalog overrides for %s: conflicting commands", patch.ID)
				}
				maps.Copy(result[index].Fields, patch.Fields)
				if patch.Commands != nil {
					result[index].Commands = patch.Commands
				}
				continue
			}
			positions[patch.ID] = len(result)
			if patch.Fields == nil {
				patch.Fields = map[string]string{}
			}
		}
		result = append(result, patch)
	}
	return result, !reflect.DeepEqual(result, overrides), nil
}

// Catalog inference is used only after an exact LegacyIDs lookup, never to
// decide whether an arbitrary qt.* user ID is a factory alias.
func legacyCatalog(group Group) string {
	parts := strings.Split(group.ID, ".")
	if len(parts) >= 3 && parts[0] == "qt" {
		return parts[1]
	}
	return group.IconTheme()
}

func catalogCustomized(patch, target Group, catalog string) bool {
	if patch.Commands != nil || patch.Fields["Factory"] == "false" {
		return true
	}
	for key, value := range patch.Fields {
		switch key {
		case "IconTheme":
			if value != "" && value != catalog {
				return true
			}
		case "Name":
			if value != target.Label() {
				return true
			}
		case "Enabled", "Factory", "FactoryFallback", "LegacyID", "LegacyIDs":
			continue
		default:
			if iconField(key) {
				continue
			}
			if old, exists := target.Fields[key]; !exists || old != value {
				return true
			}
		}
	}
	return false
}

func catalogPatchFields(fields map[string]string, catalog string) map[string]string {
	result := maps.Clone(fields)
	if result == nil {
		result = map[string]string{}
	}
	for _, key := range []string{"IconTheme", "Factory", "FactoryFallback", "LegacyID", "LegacyIDs"} {
		delete(result, key)
	}
	// Preserve effective scope: a generic override in an old Zoin-only group
	// must not start hiding icons in Lucide/Native after the group is shared.
	if catalog != "" {
		if _, specific := result["Icon."+catalog]; !specific {
			if raw, present := result["Icon.*"]; present {
				result["Icon."+catalog] = raw
			} else if raw, present := result["Icon"]; present {
				result["Icon."+catalog] = raw
			}
		}
		delete(result, "Icon.*")
		delete(result, "Icon")
	}
	return result
}

func catalogUserGroup(target, patch Group, catalog string) Group {
	fields := map[string]string{}
	for key, value := range target.Fields {
		if catalogConditionField(key) || iconField(key) {
			fields[key] = value
		}
	}
	maps.Copy(fields, catalogPatchFields(patch.Fields, catalog))
	if _, exists := fields["Name"]; !exists {
		fields["Name"] = target.Label()
	}
	// The canonical name supplies the automatic catalog association. Do not
	// synthesize Icon.*: a catalog without this artwork must remain free to
	// resolve a later group, and explicit per-catalog overrides still win.
	// Keep the old stable ID and mark this as a materialized user rule. This
	// prevents a later Begin from folding it back into factory inheritance.
	fields["Factory"] = "false"
	patch.Fields = fields
	return patch
}

func catalogConditionField(key string) bool {
	switch key {
	case "Mask", "MaskSyntax", "Case", "IncludeAttributes", "ExcludeAttributes", "SizeAbove", "SizeBelow", "DateType", "DateAfter", "DateBefore", "DateRelative":
		return true
	}
	return false
}
