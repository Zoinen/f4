package filegroups

import "github.com/unxed/vtui"

// migrateCurrentFolders moves the former Qt folder blue into the Current theme.
// Each complete old four-state entry is an explicit migration signature. Any
// missing or customized state opts that entry out; unrelated theme data survives.
func migrateCurrentFolders(defaults, colors []ColorTheme) ([]ColorTheme, bool) {
	var standard ColorTheme
	for _, theme := range defaults {
		if theme.ID == "current" {
			standard = theme
			break
		}
	}
	result := cloneColorThemes(colors)
	changed := false
	for i := range result {
		if result[i].ID != "current" {
			continue
		}
		for _, entry := range []struct{ name, normal string }{
			{name: "folders", normal: "foreground:#FFFFFF"},
			{name: "parent-folder", normal: "foreground:#B4B4B4"},
		} {
			updated, present := standard.Fields["Color."+entry.name+".NormalColor"]
			if !present || updated != "foreground:#5AB2F1" {
				continue
			}
			old := map[string]string{
				"NormalColor": entry.normal, "CursorColor": entry.normal,
				"SelectedColor": "foreground:#000000", "SelectedCursorColor": "foreground:#000000",
			}
			prefix := "Color." + entry.name + "."
			exact := true
			for field, expected := range old {
				if value, exists := result[i].Fields[prefix+field]; !exists || value != expected {
					exact = false
					break
				}
			}
			if !exact {
				continue
			}
			result[i].Fields[prefix+"NormalColor"] = updated
			changed = true
			vtui.DebugLog("[FIX:file-group-folder-colors] current group=%s normal=%s -> %s (in memory)", entry.name, entry.normal, updated)
		}
	}
	return result, changed
}
