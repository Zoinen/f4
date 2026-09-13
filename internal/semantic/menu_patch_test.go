package semantic

import "testing"

func TestCompactMenuRowsRequiresMatchingIdentityAndRevision(t *testing.T) {
	rows := []any{map[string]any{"text": "retained"}}
	old := map[string]any{"id": "history", "itemsRevision": uint64(3), "items": rows}
	previous := map[string]any{"menus": []map[string]any{old}}
	for _, tc := range []struct {
		name, id string
		revision uint64
		compact  bool
	}{
		{"page", "history", 3, true}, {"filter", "history", 4, false},
		{"reopen", "new-history", 3, false}, {"unmanaged", "history", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			menu := map[string]any{"id": tc.id, "itemsRevision": tc.revision, "items": rows, "selected": 1}
			root := map[string]any{"menus": []map[string]any{menu}}
			wire := CompactMenuRows(previous, root)
			result := AppMapSlice(wire["menus"])[0]
			if Bool(result["itemsUnchanged"]) != tc.compact {
				t.Fatal(result)
			}
			if _, exists := result["items"]; exists == tc.compact {
				t.Fatal(result)
			}
			if menu["items"] == nil || old["items"] == nil {
				t.Fatal("mutated accepted snapshot")
			}
		})
	}
}
