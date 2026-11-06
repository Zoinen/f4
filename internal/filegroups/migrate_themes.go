package filegroups

import (
	"fmt"
	"strings"

	"github.com/unxed/vtui"
)

// LegacyID is an explicit factory migration table carried by defaults, not a
// naming convention. An unrelated user group named qt.zoin.* remains universal.
func migrateThemeScopes(defaults, overrides []Group) ([]Group, error) {
	targets := map[string][]Group{}
	occupied := map[string]bool{}
	for _, group := range defaults {
		if legacy := group.Fields["LegacyID"]; legacy != "" {
			targets[legacy] = append(targets[legacy], group)
		}
		occupied[group.ID] = true
	}
	for _, group := range overrides {
		occupied[group.ID] = true
	}
	result := []Group{}
	for _, patch := range overrides {
		replacements := targets[patch.ID]
		if len(replacements) == 0 {
			result = append(result, patch.Clone())
			continue
		}
		// Non-icon features added to a formerly universal factory icon rule
		// stay universal. Extract once, even when the old fallback splits into
		// several themes. Conditions and all six command slots are retained.
		if hasUniversalFeatures(patch) {
			id := patch.ID + ".universal"
			if occupied[id] {
				return nil, fmt.Errorf("cannot migrate file group %s: identity %s already exists", patch.ID, id)
			}
			occupied[id] = true
			universal := Merge([]Group{replacements[0]}, []Group{{ID: replacements[0].ID, Fields: patch.Fields, Commands: patch.Commands}})[0]
			universal.ID = id
			for key := range universal.Fields {
				if iconField(key) || key == "Factory" || key == "LegacyID" || key == "IconTheme" || key == "FactoryFallback" {
					delete(universal.Fields, key)
				}
			}
			result = append(result, universal)
		}
		for _, replacement := range replacements {
			scoped := patch.Clone()
			scoped.ID = replacement.ID
			scoped.Commands = nil
			for key := range scoped.Fields {
				if universalFeatureField(key) || key == "CommandOrder" {
					delete(scoped.Fields, key)
				}
			}
			scoped.Fields["IconTheme"] = replacement.IconTheme()
			result = append(result, scoped)
		}
		vtui.DebugLog("FILEGROUPS: migrated factory scope id=%s targets=%d universal=%v", patch.ID, len(replacements), hasUniversalFeatures(patch))
	}
	return result, nil
}

func iconField(key string) bool {
	return key == "Icon" || strings.HasPrefix(key, "Icon.") || key == "Icons.Enabled" || key == "OverrideNativeIcon"
}

func universalFeatureField(key string) bool {
	if strings.HasPrefix(key, "Highlight.") || strings.HasPrefix(key, "Sort.") || strings.HasPrefix(key, "Associations.") {
		return true
	}
	switch key {
	case "NormalColor", "SelectedColor", "CursorColor", "SelectedCursorColor", "Mark", "UseDefaults", "ContinueProcessing",
		"NormalFileName", "SelectedFileName", "NormalColorUnderCursor", "FileNameUnderCursor",
		"SelectedColorUnderCursor", "FileNameSelectedUnderCursor", "MarkChar", "ColorStyle":
		return true
	}
	return false
}

func hasUniversalFeatures(group Group) bool {
	if len(group.Commands) > 0 {
		return true
	}
	for key := range group.Fields {
		if universalFeatureField(key) {
			return true
		}
	}
	return false
}
