package filegroups

import (
	"github.com/unxed/f4/vfs"
	"os"
	"testing"
	"time"
)

func TestCatalogsUseSharedGroupNamesWithoutRules(t *testing.T) {
	data, err := os.ReadFile("../../qt/host/icons/filegroups.default.ini")
	if err != nil {
		t.Fatal(err)
	}
	contribution, err := ParseContribution(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(contribution.Groups) != 0 {
		t.Fatal("icon catalogs must not supply matching rules")
	}
	groups, err := FactoryGroups()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, group := range groups {
		names[IconName(group.Label())] = true
	}
	for _, catalog := range contribution.Themes {
		for _, asset := range catalog.Assets {
			if !names[asset] {
				t.Errorf("catalog %s has artwork without a shared group: %s", catalog.ID, asset)
			}
		}
	}
	snapshot, err := Compile(groups, contribution.ThemeSets)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, catalog, asset string }{
		{"test.go", "zoin", "go-source"}, {"test.go", "lucide", "source-code"},
		{"test.zip", "zoin", "archives"}, {"test.zip", "lucide", "archives"},
		{"test.unrecognized", "zoin", "files"}, {"test.unrecognized", "lucide", "files"},
		{"settings.yaml", "zoin", "yaml"}, {"settings.yml", "zoin", "yaml"},
		{"table.csv", "zoin", "tables"}, {"drawing.svg", "zoin", "vector-images"},
		{"settings.cfg", "lucide", "configuration"},
	} {
		t.Run(tc.name+"/"+tc.catalog, func(t *testing.T) {
			choice := snapshot.ForIconTheme(tc.catalog).IconChoices(&vfs.VFSItem{Name: tc.name}, true, time.Now())[tc.catalog]
			if choice.Reference.Value != tc.asset {
				t.Fatalf("got %+v want %s", choice, tc.asset)
			}
		})
	}
}

func TestImplicitGroupIconsRespectRenameAndOptOut(t *testing.T) {
	groups := []Group{{ID: "custom", Fields: map[string]string{"Name": "Images", "Mask": "*.png", "Highlight.Enabled": "true"}}}
	sets := []ThemeSet{{ID: "zoin", Fields: map[string]string{"IconCatalog": "zoin", "GroupIcons": "true", "CatalogAssets": "images,files"}}}
	item := &vfs.VFSItem{Name: "sample.png"}
	snapshot, err := Compile(groups, sets)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.IconChoices(item, true, time.Now())["zoin"].Reference.Value != "images" {
		t.Fatal("normal group did not supply its named artwork")
	}
	groups[0].Fields["Name"] = "Unknown"
	changed, err := Compile(groups, sets)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.IconChoices(item, true, time.Now())) != 0 {
		t.Fatal("rename retained old artwork")
	}
	groups[0].Fields["Name"] = "Images"
	groups[0].Fields["Icons.Enabled"] = "false"
	disabled, err := Compile(groups, sets)
	if err != nil {
		t.Fatal(err)
	}
	if len(disabled.IconChoices(item, true, time.Now())) != 0 {
		t.Fatal("explicit opt-out ignored")
	}
}

func TestIconRulesSeparateFromHighlightAndCommands(t *testing.T) {
	s, err := Compile([]Group{
		{ID: "icons", Fields: map[string]string{"Mask": "*.go", "Icons.Enabled": "true", "Icon.zoin": "asset:code", "OverrideNativeIcon": "true"}},
		{ID: "color", Fields: map[string]string{"Highlight.Enabled": "true"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	item := &vfs.VFSItem{Name: "main.go"}
	choice := s.IconChoices(item, true, time.Now())["zoin"]
	if choice.Reference.Value != "code" || !choice.OverrideNative || choice.GroupID != "icons" {
		t.Fatalf("%+v", choice)
	}
	if len(s.Matching(item, true, "Highlight", time.Now())) != 1 || len(s.Matching(item, true, "Associations", time.Now())) != 0 {
		t.Fatal("facets coupled")
	}
}
