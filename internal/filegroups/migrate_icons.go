package filegroups

import (
	"strings"

	"github.com/unxed/vtui"
)

// Only repair the duplicated legacy Icon/Icon.* pair created by ImportLegacy.
// Catalog ownership is declared by the frontend, never inferred from a user
// group's label or from arbitrary file paths. Explicit custom overrides survive.
func migrateLegacyIconScopes(groups []Group, themes []ThemeSet) bool {
	changed := false
	for i := range groups {
		g := &groups[i]
		if !strings.HasPrefix(g.ID, "legacy.highlight_") {
			continue
		}
		raw := g.Fields["Icon"]
		if raw == "" || g.Fields["Icon.*"] != raw {
			continue
		}
		for _, set := range themes {
			prefix := set.Fields["LegacyIconPrefix"]
			catalog := set.Catalog()
			if prefix == "" || catalog == "" || !strings.HasPrefix(raw, prefix) {
				continue
			}
			// This duplicated bundled reference was an imported default, not a
			// user-authored override. Let the group's name select theme artwork.
			delete(g.Fields, "Icon.*")
			delete(g.Fields, "Icon")
			changed = true
			vtui.DebugLog("[FIX:file-group-icon-scope] imported group=%s catalog=%s", g.ID, catalog)
			break
		}
	}
	return changed
}
