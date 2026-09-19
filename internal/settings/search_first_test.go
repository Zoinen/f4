package settings

import (
	"testing"

	"context"
	"github.com/unxed/vtui"
)

func TestSearchFirstOptionsAvailability(t *testing.T) {
	provider := coreSettingsProvider{}
	draft, err := provider.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(draft.Close)
	center := newSettingsCenter([]*settingsSession{{catalog: provider.Catalog(), draft: draft}})
	center.selectCategory("panels")
	center.SetPosition(0, 0, 119, 49)
	for _, mode := range []string{"0", "1", "2", "0"} {
		draft.Values["NavigationMode"] = mode
		center.refreshAvailability()
		found := 0
		for _, row := range center.page.rows {
			if row.field.ID != "SearchCommandStayFocused" && row.field.ID != "SearchCommandHideUnfocused" {
				continue
			}
			found++
			disabled := mode != "2"
			if row.control.IsDisabled() != disabled {
				t.Fatalf("%s disabled=%v in mode %s", row.field.ID, row.control.IsDisabled(), mode)
			}
			semanticFound := false
			for _, node := range settingsSemanticNodes(center.SemanticNode(&vtui.SemanticContext{Width: 120, Height: 50})) {
				if node["id"] == vtui.SemanticID(row.control) {
					semanticFound = true
					if node["disabled"] != disabled {
						t.Fatalf("Qt semantic disabled state disagrees: %v", node)
					}
				}
			}
			if !semanticFound {
				t.Fatalf("missing semantic checkbox %s", row.field.ID)
			}
		}
		if found != 2 {
			t.Fatalf("found %d search options", found)
		}
	}
}
