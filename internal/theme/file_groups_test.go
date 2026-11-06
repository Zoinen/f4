package theme

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/vfs"
)

func TestThemeIconTintInvalidatesSemanticCache(t *testing.T) {
	highlighter := &FileHighlighter{}
	sets := []filegroups.ThemeSet{{ID: "zoin", Fields: map[string]string{"IconCatalog": "zoin", "TintIcons": "true"}}}
	var previousID string
	for _, tint := range []bool{true, false, true} {
		sets[0].Fields["TintIcons"] = map[bool]string{true: "true", false: "false"}[tint]
		snapshot, err := filegroups.Compile(nil, sets)
		if err != nil {
			t.Fatal(err)
		}
		if err := highlighter.LoadFileGroups(snapshot.ForIconTheme("zoin")); err != nil {
			t.Fatal(err)
		}
		id, style := highlighter.SemanticStyle(&vfs.VFSItem{Name: "example.txt"}, true)
		if style.TintIcon == nil || *style.TintIcon != tint {
			t.Fatal("stale semantic tint policy")
		}
		if id == previousID {
			t.Fatal("style identity did not change with tint policy")
		}
		previousID = id
	}
}

func TestDeferredHighlighterClonePreservesGroupsAndAttributeOverrides(t *testing.T) {
	groups := []filegroups.Group{{ID: "hidden", Fields: map[string]string{
		"Mask": "*", "IncludeAttributes": "Hidden", "UseDefaults": "true", "Highlight.Enabled": "true",
		"NormalColor": "foreground:#123456", "Icons.Enabled": "true", "Icon.*": "asset:old",
	}}}
	snapshot, err := filegroups.Compile(groups)
	if err != nil {
		t.Fatal(err)
	}
	source := &FileHighlighter{}
	if err := source.LoadFileGroups(snapshot.ForIconTheme("zoin")); err != nil {
		t.Fatal(err)
	}
	item := &vfs.VFSItem{Name: ".hidden", IsHidden: true}
	_, expected := source.SemanticStyle(item, true)
	clone := source.Clone()
	groups[0].Fields["Icon.*"] = "asset:new"
	groups[0].Fields["NormalColor"] = "foreground:#abcdef"
	updated, err := filegroups.Compile(groups)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.LoadFileGroups(updated.ForIconTheme("lucide")); err != nil {
		t.Fatal(err)
	}
	_, actual := clone.SemanticStyle(item, true)
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("deferred clone lost frozen groups/UseDefaults: got %+v want %+v", actual, expected)
	}
	if actual.IconChoices["*"].Value != "old" {
		t.Fatal("clone borrowed mutable runtime")
	}
}

func TestFileGroupFactoryStylesCompile(t *testing.T) {
	groups := FileGroupDefaults()
	if len(groups) == 0 {
		t.Fatal("no INI colour contributions")
	}
	if _, err := filegroups.Compile(groups); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalPaletteLabelsAndLegacyParity(t *testing.T) {
	factory, err := filegroups.FactoryGroups()
	if err != nil {
		t.Fatal(err)
	}
	groups := FileGroupDefaults()
	for _, original := range factory {
		for _, group := range groups {
			if group.ID != original.ID {
				continue
			}
			for _, key := range append([]string{"Name"}, filegroups.HighlightConditionKeys...) {
				if group.Fields[key] != original.Fields[key] {
					t.Fatalf("%s classification %s changed", group.ID, key)
				}
			}
		}
	}
	snapshot, err := filegroups.Compile(groups)
	if err != nil {
		t.Fatal(err)
	}
	for _, style := range loadStylesFromFS(builtInStyles, "styles/*.ini") {
		t.Run(style.Name, func(t *testing.T) {
			labels := map[string]bool{}
			for _, group := range snapshot.HighlightGroups(style.Name) {
				if !group.Enabled("Highlight") {
					continue
				}
				if strings.HasPrefix(group.ID, "style.") || labels[group.Label()] {
					t.Fatalf("repeated or independent palette row: %s %s", group.ID, group.Label())
				}
				labels[group.Label()] = true
			}
			old := &FileHighlighter{}
			old.LoadThemeRules(style.ini)
			current := &FileHighlighter{activeColorStyle: style.Name}
			if err := current.LoadFileGroups(snapshot); err != nil {
				t.Fatal(err)
			}
			items := []vfs.VFSItem{
				{Name: "a.old"}, {Name: "a.lha"}, {Name: "a.zip"}, {Name: "a.tmp"},
				{Name: "a.sh"}, {Name: "tool", IsExecutable: true}, {Name: "plain"},
				{Name: "folder", IsDir: true}, {Name: ".hidden", IsHidden: true},
				{Name: ".hidden", IsHidden: true, IsDir: true},
				{Name: "link", IsSymlink: true}, {Name: "link", IsSymlink: true, IsDir: true},
			}
			for _, item := range items {
				if got, want := current.GetMarker(&item), old.GetMarker(&item); got != want {
					t.Fatalf("%+v marker=%q want=%q", item, got, want)
				}
				for _, selected := range []bool{false, true} {
					for _, cursor := range []bool{false, true} {
						want := old.GetColor(&item, 7, selected, cursor)
						got := current.GetColor(&item, 7, selected, cursor)
						if got != want {
							t.Fatalf("%+v selected=%v cursor=%v got=%x want=%x", item, selected, cursor, got, want)
						}
					}
				}
			}
		})
	}
}

func TestCanonicalPaletteClassificationOrder(t *testing.T) {
	groups := FileGroupDefaults()
	wantTail := []string{"type.parent-folder", "type.folders", "type.files"}
	for i, id := range wantTail {
		if got := groups[len(groups)-3+i].ID; got != id {
			t.Fatalf("fallback %d = %s, want %s", i, got, id)
		}
	}
	factory, err := filegroups.FactoryGroups()
	if err != nil {
		t.Fatal(err)
	}
	position := 0
	for _, group := range groups {
		if position < len(factory) && group.ID == factory[position].ID {
			position++
		}
	}
	if position != len(factory) {
		t.Fatal("core classification order changed")
	}
	snapshot, err := filegroups.Compile(groups)
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range []*filegroups.Snapshot{snapshot, snapshot.ForIconTheme("generic")} {
		palette := view.HighlightGroups("Modern")
		last := -1
		for _, group := range palette {
			if group.Fields["ColorStyle"] != "Modern" {
				continue
			}
			order, err := strconv.Atoi(group.Fields[filegroups.PalettePrefix("Modern")+"Order"])
			if err != nil || order <= last {
				t.Fatal("historical highlighting order changed", palette)
			}
			last = order
			if view.HighlightPredicate(group.ID, "Modern") != snapshot.HighlightPredicate(group.ID, "Modern") {
				t.Fatal("theme view recompiled highlighting")
			}
		}
	}
}

func TestInstalledPaletteIdentityAndUnknownFields(t *testing.T) {
	source := ini.Parse(strings.NewReader(`[Highlight_0]
Name = Archives
Mask = *.odd
NormalColor = foreground:#123456
FutureField = preserve me
[Highlight_1]
FileGroup = custom.explicit
Name = Explicit
Mask = *.explicit
`))
	styles := []ColorStyle{{Name: "Installed", ini: source}}
	groups := colorStyleFileGroups(styles)
	if !reflect.DeepEqual(groups, colorStyleFileGroups(styles)) {
		t.Fatal("unstable installed style identity")
	}
	found := 0
	for _, group := range groups {
		prefix := filegroups.PalettePrefix("Installed")
		if _, ok := group.Fields[prefix+"LegacyID"]; !ok {
			continue
		}
		found++
		if group.ID == "type.archives" {
			t.Fatal("label inferred a shared canonical identity")
		}
		if group.Fields["Name"] == "Archives" {
			if group.Highlighting("Installed").Fields["FutureField"] != "preserve me" || group.Fields["Mask"] != "*.odd" {
				t.Fatal(group)
			}
			want := fmt.Sprintf("style.%x.Highlight_0", sha256.Sum256([]byte("Installed")))
			if group.Fields[prefix+"LegacyID"] != want {
				t.Fatal("legacy alias changed", group)
			}
		} else if group.ID != "custom.explicit" {
			t.Fatal("explicit FileGroup identity ignored", group)
		}
	}
	if found != 2 {
		t.Fatal("missing installed style contributions", found)
	}
}

func TestPaletteIndependentLayersAndFourStateCascade(t *testing.T) {
	previous := config.App.HighlightPriority
	t.Cleanup(func() { config.App.HighlightPriority = previous })
	paletteINI := ini.Parse(strings.NewReader(`[Highlight_0]
FileGroup = test.first
Mask = *.go
NormalColor = foreground:#123456
SelectedColor = foreground:#234567
CursorColor = foreground:#345678
SelectedCursorColor = foreground:#456789
ContinueProcessing = 1
[Highlight_1]
FileGroup = test.fill
Mask = *
NormalColor = background:#102030
SelectedColor = background:#203040
CursorColor = background:#304050
SelectedCursorColor = background:#405060
`))
	userSource := `[Highlight_0]
Mask = *.go
NormalColor = foreground:#abcdef
ContinueProcessing = 1
[Highlight_1]
UseDefaults = 1
IncludeAttributes = Hidden
CursorColor = foreground:#654321
`
	userINI := ini.Parse(strings.NewReader(userSource))
	document, err := filegroups.Parse([]byte(userSource))
	if err != nil {
		t.Fatal(err)
	}
	groups := append(filegroups.ImportLegacy(document, nil), colorStyleFileGroups([]ColorStyle{{Name: "Synthetic", ini: paletteINI}})...)
	snapshot, err := filegroups.Compile(groups)
	if err != nil {
		t.Fatal(err)
	}
	for _, priority := range []int{0, 1} {
		config.App.HighlightPriority = priority
		old := &FileHighlighter{}
		old.LoadUserRules(userINI)
		old.LoadThemeRules(paletteINI)
		current := &FileHighlighter{activeColorStyle: "Synthetic"}
		if err := current.LoadFileGroups(snapshot); err != nil {
			t.Fatal(err)
		}
		if len(current.UserRules) != 2 || len(current.ThemeRules) != 2 {
			t.Fatal("layers collapsed")
		}
		for _, item := range []vfs.VFSItem{{Name: "main.go"}, {Name: "main.go", IsHidden: true}, {Name: "text"}} {
			for _, selected := range []bool{false, true} {
				for _, cursor := range []bool{false, true} {
					if got, want := current.GetColor(&item, 7, selected, cursor), old.GetColor(&item, 7, selected, cursor); got != want {
						t.Fatalf("priority=%d %+v selected=%v cursor=%v got=%x want=%x", priority, item, selected, cursor, got, want)
					}
				}
			}
		}
	}
}

func TestGloballyDisabledPaletteDoesNotRestoreLegacyTheme(t *testing.T) {
	prefix := filegroups.PalettePrefix("Disabled")
	snapshot, err := filegroups.Compile([]filegroups.Group{{ID: "disabled", Fields: map[string]string{
		"Highlight.Enabled": "false", prefix + "Highlight.Enabled": "true", prefix + "NormalColor": "foreground:#123456",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	highlighter := &FileHighlighter{activeColorStyle: "Disabled"}
	highlighter.LoadThemeRules(ini.Parse(strings.NewReader("[Highlight_0]\nNormalColor = foreground:#abcdef\n")))
	if err := highlighter.LoadFileGroups(snapshot); err != nil {
		t.Fatal(err)
	}
	if len(highlighter.ThemeRules) != 0 || len(highlighter.UserRules) != 0 {
		t.Fatal("disabled palette restored legacy rules")
	}
}

func TestFileGroupLegacyColourParityAndExplicitClear(t *testing.T) {
	source := `[Highlight_0]
Mask=*.go
NormalFileName=foreground:#123456
ContinueProcessing=1
[Highlight_1]
Mask=*
SelectedColor=foreground:#abcdef
[Highlight_2]
UseDefaults=1
IncludeAttributes=Hidden
CursorColor=foreground:#456789
`
	legacy := ini.Parse(strings.NewReader(source))
	old := &FileHighlighter{}
	old.LoadFromIni(legacy)
	document, err := filegroups.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	groups := filegroups.ImportLegacy(document, nil)
	groups[0].Fields["ContinueProcessing"] = "true"
	groups[2].Fields["UseDefaults"] = "true"
	snapshot, err := filegroups.Compile(groups)
	if err != nil {
		t.Fatal(err)
	}
	current := &FileHighlighter{}
	if err := current.LoadFileGroups(snapshot); err != nil {
		t.Fatal(err)
	}
	for _, item := range []vfs.VFSItem{{Name: "main.go"}, {Name: "main.go", IsHidden: true}, {Name: "note.txt"}, {Name: "folder", IsDir: true}} {
		for _, selected := range []bool{false, true} {
			for _, cursor := range []bool{false, true} {
				want, got := old.GetColor(&item, 7, selected, cursor), current.GetColor(&item, 7, selected, cursor)
				if got != want {
					t.Fatalf("%+v selected=%v cursor=%v got=%x want=%x", item, selected, cursor, got, want)
				}
			}
		}
	}
	groups[0].Fields["NormalColor"] = ""
	snapshot, err = filegroups.Compile(groups)
	if err != nil {
		t.Fatal(err)
	}
	if err := current.LoadFileGroups(snapshot); err != nil {
		t.Fatal(err)
	}
	_, style := current.SemanticStyle(&vfs.VFSItem{Name: "main.go"}, true)
	if style.Normal.Foreground != "" {
		t.Fatal("legacy alias undid explicit clear", style)
	}
}

func TestFileGroupIndependentIcons(t *testing.T) {
	snapshot, err := filegroups.Compile([]filegroups.Group{
		{ID: "icon", Fields: map[string]string{"Mask": "*.go", "Icons.Enabled": "true", "Icon.zoin": "asset:code"}},
		{ID: "color", Fields: map[string]string{"Mask": "*", "Highlight.Enabled": "true", "NormalColor": "foreground:#123456"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fh := &FileHighlighter{}
	if err := fh.LoadFileGroups(snapshot); err != nil {
		t.Fatal(err)
	}
	_, goStyle := fh.SemanticStyle(&vfs.VFSItem{Name: "a.go"}, true)
	_, txtStyle := fh.SemanticStyle(&vfs.VFSItem{Name: "a.txt"}, true)
	if goStyle.IconChoices["zoin"].Value != "code" || len(txtStyle.IconChoices) != 0 {
		t.Fatalf("icon cache conflated matches: %+v %+v", goStyle, txtStyle)
	}
	if goStyle.Normal.Foreground != "#123456" {
		t.Fatalf("icon-only rule blocked colour: %+v", goStyle)
	}
}

func TestFileGroupPreviewUsesActivePaletteStyle(t *testing.T) {
	snapshot, err := filegroups.Compile([]filegroups.Group{
		{ID: "original", Fields: map[string]string{"ColorStyle": "Original", "Highlight.Enabled": "true", "NormalColor": "foreground:#123456"}},
		{ID: "preview", Fields: map[string]string{"ColorStyle": "Preview", "Highlight.Enabled": "true", "NormalColor": "foreground:#abcdef"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	highlighter := &FileHighlighter{activeColorStyle: "Preview"}
	if err := highlighter.LoadFileGroups(snapshot); err != nil {
		t.Fatal(err)
	}
	_, style := highlighter.SemanticStyle(&vfs.VFSItem{Name: "file"}, true)
	if style.Normal.Foreground != "#ABCDEF" {
		t.Fatal("preview restored a stale style", style)
	}
	highlighter.activeColorStyle = "Original"
	if err := highlighter.LoadFileGroups(snapshot); err != nil {
		t.Fatal(err)
	}
	_, style = highlighter.SemanticStyle(&vfs.VFSItem{Name: "file"}, true)
	if style.Normal.Foreground != "#123456" {
		t.Fatal("rollback did not compose with palette", style)
	}
}
