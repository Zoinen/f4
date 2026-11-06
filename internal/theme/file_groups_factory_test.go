package theme

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestBroadFactoryCanonicalSet(t *testing.T) {
	groups := colorStyleFileGroups(loadStylesFromFS(builtInStyles, "styles/*.ini"))
	if len(groups) != 46 {
		t.Fatalf("factory groups = %d, want 46", len(groups))
	}
	labels := map[string]bool{}
	assets := []string{}
	for _, group := range groups {
		if labels[strings.ToLower(group.Label())] {
			t.Fatalf("repeated standard group label: %+v", group)
		}
		labels[strings.ToLower(group.Label())] = true
		if group.Fields["Icons.Enabled"] == "true" {
			assets = append(assets, filegroups.IconName(group.Label()))
		}
	}
	snapshot, err := filegroups.Compile(groups, []filegroups.ThemeSet{{ID: "test", Fields: map[string]string{
		"IconCatalog": "test", "GroupIcons": "true", "CatalogAssets": strings.Join(assets, ","),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Predicate("type.scripts").Match(&vfs.VFSItem{Name: "app.vbs"}, true, time.Time{}) {
		t.Fatal("shared Scripts base mask was not expanded")
	}
	cases := map[string]string{
		"app.pdb": "type.debug-symbols", "app.pro": "type.projects", "app.pri": "type.projects",
		"app.vcxproj": "type.projects", "CMakeLists.txt": "type.projects", "app.sln": "type.projects",
		"app.csproj": "type.projects", "Makefile": "type.projects", "app.dll": "type.libraries",
		"app.a": "type.libraries", "app.lib": "type.libraries", "libx.so.2": "type.libraries", "libx.dylib": "type.libraries",
		"book.bbdoc": "type.books", "archive.r01": "type.archives", "archive.a02": "type.archives",
		"archive.tbz": "type.archives", "archive.iso": "type.archives", "archive.pk3": "type.archives",
		"music.ape": "type.audio", "music.mod": "type.audio", "music.ac3": "type.audio",
		"photo.cr3": "type.images", "photo.psd": "type.images", "font.ttc": "type.fonts", "font.ttf": "type.fonts",
		"BuildLog.htm": "type.temporary-files", "app.obj": "type.temporary-files", "app.old": "type.temporary-files",
		"page.mht": "type.web-pages", "movie.m2ts": "type.video", "movie.ogm": "type.video",
		"note.diz": "type.text-files", "note.rtf": "type.text-files", "note.ttl": "type.text-files",
		"main.pas": "type.source-code", "main.pyw": "type.python", "main.go": "type.go-source",
		"app.vbs": "type.executables", "app.cpl": "type.executables", "app.msp": "type.executables",
		"main.js":  "type.javascript",
		"icon.svg": "type.vector-images", "data.yaml": "type.yaml", "data.csv": "type.tables",
	}
	for name, id := range cases {
		t.Run(name, func(t *testing.T) {
			item := &vfs.VFSItem{Name: name}
			matches := snapshot.Matching(item, true, "Icons", time.Time{})
			if len(matches) == 0 || matches[0].ID != id {
				t.Fatalf("%s first classifier = %+v, want %s", name, matches, id)
			}
			if choice := snapshot.IconChoices(item, true, time.Time{})["test"]; choice.GroupID != id {
				t.Fatalf("icon choice = %+v, want %s", choice, id)
			}
			if id == "type.fonts" && snapshot.Predicate("type.images").Match(item, true, time.Time{}) {
				t.Fatal("font classified as image")
			}
		})
	}
}

func TestModernBroadFactoryPalette(t *testing.T) {
	previousCorrection := config.App.EnforceColorCorrection
	config.App.EnforceColorCorrection = false
	t.Cleanup(func() { config.App.EnforceColorCorrection = previousCorrection })
	groups := colorStyleFileGroups(loadStylesFromFS(builtInStyles, "styles/*.ini"))
	snapshot, err := filegroups.Compile(groups)
	if err != nil {
		t.Fatal(err)
	}
	highlighter := &FileHighlighter{activeColorStyle: "Modern"}
	if err := highlighter.LoadFileGroups(snapshot); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		item            vfs.VFSItem
		id, color, mark string
	}{
		{item: vfs.VFSItem{Name: "..", IsDir: true}, id: "type.parent-folder", color: "#B4B4B4"},
		{item: vfs.VFSItem{Name: ".folder", IsHidden: true, IsDir: true}, id: "attribute.hidden-folders", color: "#6E6E6E"},
		{item: vfs.VFSItem{Name: ".file", IsHidden: true}, id: "attribute.hidden-files", color: "#5A5A5A"},
		{item: vfs.VFSItem{Name: "app.pdb"}, id: "type.debug-symbols", color: "#ECE9D8"},
		{item: vfs.VFSItem{Name: "app.exe"}, id: "type.scripts", color: "#72EE48", mark: "*"},
		{item: vfs.VFSItem{Name: "tool", IsExecutable: true}, id: "type.executables", color: "#72EE48", mark: "*"},
		{item: vfs.VFSItem{Name: "archive.r01"}, id: "type.archives", color: "#CB78EB"},
		{item: vfs.VFSItem{Name: "music.ape"}, id: "type.audio", color: "#68A318", mark: "♫"},
		{item: vfs.VFSItem{Name: "main.pas"}, id: "type.source-code", color: "#68A318"},
		{item: vfs.VFSItem{Name: "photo.cr3"}, id: "type.images", color: "#FF9238"},
		{item: vfs.VFSItem{Name: "app.obj"}, id: "type.temporary-files", color: "#A39276"},
		{item: vfs.VFSItem{Name: "page.mht"}, id: "type.web-pages", color: "#0082FF"},
		{item: vfs.VFSItem{Name: "page.html"}, id: "type.web-pages", color: "#0082FF"},
		{item: vfs.VFSItem{Name: "movie.m2ts"}, id: "type.video", color: "#AAF046"},
		{item: vfs.VFSItem{Name: "app.pro"}, id: "type.projects", color: "#BEB9FF"},
		{item: vfs.VFSItem{Name: "module.cmake"}, id: "type.projects", color: "#BEB9FF"},
		{item: vfs.VFSItem{Name: "note.rtf"}, id: "type.text-files", color: "#ECE9D8"},
		{item: vfs.VFSItem{Name: "libx.so.2"}, id: "type.libraries", color: "#BEB9FF"},
		{item: vfs.VFSItem{Name: "book.bbdoc"}, id: "type.books", color: "#88C7E1"},
		{item: vfs.VFSItem{Name: "file.unknown"}, id: "type.files", color: "#AAAAAA"},
		{item: vfs.VFSItem{Name: "folder", IsDir: true}, id: "type.folders", color: "#FFFFFF"},
	}
	for _, test := range cases {
		t.Run(test.item.Name, func(t *testing.T) {
			var projected filegroups.Group
			for _, group := range groups {
				if group.ID == test.id {
					projected = group.Highlighting("Modern")
					break
				}
			}
			if projected.Fields["NormalColor"] != "foreground:"+test.color || projected.Fields["CursorColor"] != "foreground:"+test.color || projected.Fields["Mark"] != test.mark {
				t.Fatal("shipped four-state palette data changed", projected)
			}
			selected := "foreground:#000000"
			if test.id == "type.libraries" {
				selected = ""
			}
			if projected.Fields["SelectedColor"] != selected || projected.Fields["SelectedCursorColor"] != selected {
				t.Fatal("selected state inheritance changed", projected)
			}
			if !snapshot.HighlightPredicate(test.id, "Modern").Match(&test.item, true, time.Time{}) {
				t.Fatal("canonical palette predicate did not match")
			}
			// Parent colours are deliberately suppressed by both renderers;
			// semantic rendering also suppresses hidden-only presentation rules.
			// Verify their data/predicates above and legacy hidden rendering here.
			if test.item.Name == ".." {
				return
			}
			want, err := strconv.ParseUint(strings.TrimPrefix(test.color, "#"), 16, 32)
			if err != nil {
				t.Fatal(err)
			}
			if got := vtui.GetRGBFore(highlighter.GetColor(&test.item, 7, false, false)); uint64(got) != want {
				t.Fatalf("legacy foreground=%06X want=%06X", got, want)
			}
			if test.item.IsHidden {
				return
			}
			_, style := highlighter.SemanticStyle(&test.item, true)
			if style.Normal.Foreground != test.color || highlighter.GetMarker(&test.item) != test.mark {
				t.Fatalf("style=%+v marker=%q, want %s %q", style, highlighter.GetMarker(&test.item), test.color, test.mark)
			}
		})
	}
}

func TestModernFactoryDeclaredLegacyAncestry(t *testing.T) {
	styles := loadStylesFromFS(builtInStyles, "styles/modern.ini")
	if len(styles) != 1 {
		t.Fatal("missing Modern data")
	}
	old := map[string]int{
		"attribute.symlink": 0, "attribute.hidden-files": 1, "attribute.hidden-folders": 2,
		"attribute.system": 3, "type.folders": 4, "type.scripts": 5,
		"type.archives": 6, "type.temporary-files": 7, "type.executables": 8,
	}
	seen := 0
	for section, fields := range styles[0].ini.Sections() {
		if !strings.HasPrefix(strings.ToLower(section), "highlight_") {
			continue
		}
		seen++
		declared, present := fields["LegacyID"]
		if !present {
			t.Fatalf("%s lacks explicit ancestry", section)
		}
		want := ""
		if index, original := old[fields["FileGroup"]]; original {
			want = fmt.Sprintf("style.%x.Highlight_%d", sha256.Sum256([]byte("Modern")), index)
		}
		if declared != want {
			t.Fatalf("%s alias=%q want=%q", fields["FileGroup"], declared, want)
		}
	}
	if seen != 21 {
		t.Fatalf("Modern rules=%d want=21", seen)
	}
	groups := colorStyleFileGroups(styles)
	for _, group := range groups {
		prefix := filegroups.PalettePrefix("Modern")
		if _, projected := group.Fields[prefix+"Highlight.Enabled"]; !projected {
			continue
		}
		want := ""
		if index, original := old[group.ID]; original {
			want = fmt.Sprintf("style.%x.Highlight_%d", sha256.Sum256([]byte("Modern")), index)
		}
		if got := group.Fields[prefix+"LegacyID"]; got != want {
			t.Fatalf("adapter changed %s declared alias=%q to %q", group.ID, want, got)
		}
	}
}
