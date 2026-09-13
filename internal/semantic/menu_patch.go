package semantic

// CompactMenuRows keeps the accepted snapshot complete while sending only menu
// state when an owner guarantees that its immutable rows have not changed.
func CompactMenuRows(previous, rootSet map[string]any) map[string]any {
	menus, present := rootSet["menus"]
	if !present {
		return rootSet
	}
	oldMenus := AppMapSlice(previous["menus"])
	compact := make([]map[string]any, 0)
	for _, menu := range AppMapSlice(menus) {
		var retained bool
		if revision, ok := menu["itemsRevision"].(uint64); ok && revision != 0 {
			for _, old := range oldMenus {
				if old["id"] == menu["id"] && old["itemsRevision"] == revision {
					retained = true
					break
				}
			}
		}
		if retained {
			copyMenu := make(map[string]any, len(menu))
			for key, value := range menu {
				if key != "items" {
					copyMenu[key] = value
				}
			}
			copyMenu["itemsUnchanged"] = true
			menu = copyMenu
		}
		compact = append(compact, menu)
	}
	out := make(map[string]any, len(rootSet))
	for key, value := range rootSet {
		out[key] = value
	}
	out["menus"] = compact
	return out
}
