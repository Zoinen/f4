package theme

import (
	"reflect"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
)

// This in-memory adapter regression uses the real compiled domain snapshot and
// highlighter, without a profile, filesystem metadata reads, or a running UI.
func TestFileColorThemesWithColorThemesAttributeCascade(t *testing.T) {
	previous := config.App
	config.App.EnforceColorCorrection = false
	t.Cleanup(func() { config.App = previous })
	states := []struct {
		name, field      string
		selected, cursor bool
	}{
		{name: "normal", field: "NormalColor"},
		{name: "selected", field: "SelectedColor", selected: true},
		{name: "cursor", field: "CursorColor", cursor: true},
		{name: "selected-cursor", field: "SelectedCursorColor", selected: true, cursor: true},
	}
	baseColors := [4]string{
		"foreground:#112233 | background:#010203",
		"foreground:#223344 | background:#020304",
		"foreground:#334455 | background:#030405",
		"foreground:#445566 | background:#040506",
	}
	colorFields := map[string]string{"Name": "Independent"}
	for _, label := range []string{"Link base", "Hidden base", "Folder base", "File base"} {
		for i, state := range states {
			colorFields["Color."+filegroups.IconName(label)+"."+state.field] = baseColors[i]
		}
	}
	overrideColors := map[string]map[string]string{
		"folder-override": {
			"NormalColor": "foreground:#AA1100", "SelectedColor": "background:#050607",
			"CursorColor": "", "SelectedCursorColor": "foreground:#AA2200 | background:#060708",
		},
		"link-override": {
			"NormalColor": "foreground:#00AA11", "SelectedColor": "background:#111213",
			"CursorColor": "foreground:#00AA22 | background:#141516",
		},
		"junction-override": {
			"NormalColor": "foreground:#0011AA", "SelectedColor": "foreground:#0022AA | background:#212223",
			"CursorColor": "background:#242526", "SelectedCursorColor": "foreground:#0033AA",
		},
		"hidden-override": {
			"NormalColor": "foreground:#AAAA11", "SelectedColor": "foreground:#AAAA22",
			"CursorColor": "background:#313233", "SelectedCursorColor": "",
		},
	}
	for name, colors := range overrideColors {
		for field, value := range colors {
			colorFields["Color."+name+"."+field] = value
		}
	}
	ordinary := []filegroups.Group{
		{ID: "links", Fields: map[string]string{"Name": "Link base", "IncludeAttributes": "Symlink", "Highlight.Enabled": "true"}},
		{ID: "hidden", Fields: map[string]string{"Name": "Hidden base", "IncludeAttributes": "Hidden", "Highlight.Enabled": "true"}},
		{ID: "folders", Fields: map[string]string{"Name": "Folder base", "IncludeAttributes": "Directory", "Highlight.Enabled": "true"}},
		{ID: "files", Fields: map[string]string{"Name": "File base", "ExcludeAttributes": "Directory", "Highlight.Enabled": "true"}},
	}
	overrides := []filegroups.Group{
		{ID: "folder-override", Fields: map[string]string{"Name": "Folder override", "IncludeAttributes": "Directory", "UseDefaults": "true", "Highlight.Enabled": "true", "Mark": ">"}},
		{ID: "hidden-override", Fields: map[string]string{"Name": "Hidden override", "IncludeAttributes": "Hidden", "UseDefaults": "true", "Highlight.Enabled": "true", "Mark": "H"}},
		{ID: "link-override", Fields: map[string]string{"Name": "Link override", "IncludeAttributes": "Symlink", "UseDefaults": "true", "Highlight.Enabled": "true", "Mark": "L"}},
		{ID: "junction-override", Fields: map[string]string{"Name": "Junction override", "IncludeAttributes": "Junction", "UseDefaults": "true", "Highlight.Enabled": "true", "Mark": "J"}},
	}
	expected := map[string][4]string{
		"folder": {
			"foreground:#AA1100 | background:#010203", "foreground:#223344 | background:#050607",
			"foreground:#334455 | background:#030405", "foreground:#AA2200 | background:#060708",
		},
		"link": {
			"foreground:#00AA11 | background:#010203", "foreground:#223344 | background:#111213",
			"foreground:#00AA22 | background:#141516", "foreground:#445566 | background:#040506",
		},
		"junction": {
			"foreground:#0011AA | background:#010203", "foreground:#0022AA | background:#212223",
			"foreground:#334455 | background:#242526", "foreground:#0033AA | background:#040506",
		},
		"hidden": {
			"foreground:#AAAA11 | background:#010203", "foreground:#AAAA22 | background:#020304",
			"foreground:#334455 | background:#313233", "foreground:#445566 | background:#040506",
		},
		"file": baseColors,
	}
	cases := []struct {
		name, kind, marker string
		item               vfs.VFSItem
	}{
		{name: "tree-folder", kind: "folder", marker: ">", item: vfs.VFSItem{Name: "folder", IsDir: true}},
		{name: "tree-hidden-folder", kind: "hidden", marker: "H", item: vfs.VFSItem{Name: ".folder", IsDir: true, IsHidden: true}},
		{name: "tree-symlink", kind: "link", marker: "L", item: vfs.VFSItem{Name: "link", IsDir: true, IsSymlink: true}},
		{name: "tree-hidden-symlink", kind: "link", marker: "L", item: vfs.VFSItem{Name: ".link", IsDir: true, IsSymlink: true, IsHidden: true}},
		{name: "tree-junction", kind: "junction", marker: "J", item: vfs.VFSItem{Name: "junction", IsDir: true, IsSymlink: true, ReparseTag: vfs.ReparseTagMountPoint}},
		{name: "tree-hidden-junction", kind: "junction", marker: "J", item: vfs.VFSItem{Name: ".junction", IsDir: true, IsSymlink: true, IsHidden: true, ReparseTag: vfs.ReparseTagMountPoint}},
		{name: "hidden-file", kind: "hidden", marker: "H", item: vfs.VFSItem{Name: ".file", IsHidden: true}},
		{name: "plain-file", kind: "file", item: vfs.VFSItem{Name: "file"}},
	}
	for _, order := range []string{"overrides-before-base", "overrides-after-base"} {
		t.Run(order, func(t *testing.T) {
			groups := append(append([]filegroups.Group{}, overrides...), ordinary...)
			if order == "overrides-after-base" {
				groups = append(append([]filegroups.Group{}, ordinary...), overrides...)
			}
			base, err := filegroups.Compile(groups)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := base.WithColorThemes([]filegroups.ColorTheme{{ID: "independent", Fields: colorFields}}, "independent")
			if err != nil {
				t.Fatal(err)
			}
			highlighter := &FileHighlighter{}
			for _, uiStyle := range []string{"Modern", "Classic", "MC Dark", "Modern"} {
				t.Run(uiStyle, func(t *testing.T) {
					config.App.ColorStyle = uiStyle
					// Simulate a UI palette update before the independent file
					// theme is reprojected. It must not recover these legacy reds.
					highlighter.LoadThemeRules(ini.Parse(strings.NewReader("[Highlight_0]\nNormalColor=foreground:#FF0000\nSelectedColor=foreground:#FF0000\nCursorColor=foreground:#FF0000\nSelectedCursorColor=foreground:#FF0000\n")))
					if err := highlighter.LoadFileGroups(snapshot); err != nil {
						t.Fatal(err)
					}
					if snapshot.ColorThemeID() != "independent" {
						t.Fatal("UI style changed the file theme selection")
					}
					for _, test := range cases {
						t.Run(test.name, func(t *testing.T) {
							for i, state := range states {
								t.Run(state.name, func(t *testing.T) {
									// The tree calls this exact GetColor path with
									// selected=false and directory/link metadata.
									got := highlighter.GetColor(&test.item, 7, state.selected, state.cursor)
									want := ParseFarColor(expected[test.kind][i], 7)
									if got != want {
										t.Fatalf("colour=%#x want=%#x (%s)", got, want, expected[test.kind][i])
									}
								})
							}
							if got := highlighter.GetMarker(&test.item); got != test.marker {
								t.Fatalf("marker=%q want=%q", got, test.marker)
							}
							if !test.item.IsHidden {
								_, style := highlighter.SemanticStyle(&test.item, true)
								patches := [4]extui.HighlightColorPatchModel{style.Normal, style.Selected, style.Cursor, style.SelectedCursor}
								for i, patch := range patches {
									if want := semanticColorPatch(expected[test.kind][i]); !reflect.DeepEqual(patch, want) {
										t.Fatalf("semantic %s=%+v want=%+v", states[i].name, patch, want)
									}
								}
							}
						})
					}
				})
			}
		})
	}
}
