package panel

import (
	"fmt"
	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"testing"
)

func TestFileGroupAllCommandSlotsAndOrdering(t *testing.T) {
	commands := []filegroups.CommandSet{{ID: "first", Name: "First"}, {ID: "second", Name: "Second"}}
	for i := range commands {
		for slot := range commands[i].Commands {
			commands[i].Commands[slot] = fmt.Sprintf("app%d-slot%d !.!", i, slot)
			commands[i].Enabled[slot] = true
		}
	}
	snapshot, err := filegroups.Compile([]filegroups.Group{{ID: "images", Fields: map[string]string{"Mask": "*.png", "Associations.Enabled": "true"}, Commands: commands}})
	if err != nil {
		t.Fatal(err)
	}
	for slot := AssocExecute; slot < AssocKindCount; slot++ {
		matches := matchingGroupAssociations(snapshot, &vfs.VFSItem{Name: "x.png"}, slot)
		if len(matches) != 2 || matches[0].Description != "First" || matches[1].Commands[slot] != commands[1].Commands[slot] {
			t.Fatalf("slot %d: %+v", slot, matches)
		}
		if len(matchingGroupAssociations(snapshot, &vfs.VFSItem{Name: "x.png", IsDir: true}, slot)) != 0 {
			t.Fatal("directory command intercepted navigation")
		}
	}
}

func TestFileGroupAssociationChoicesShowEverySource(t *testing.T) {
	first := filegroups.CommandSet{ID: "first", Name: "Viewer"}
	first.Commands[AssocView], first.Enabled[AssocView] = "view-one", true
	second := filegroups.CommandSet{ID: "second", Name: "Viewer"}
	second.Commands[AssocView], second.Enabled[AssocView] = "view-two", true
	blank := filegroups.CommandSet{ID: "blank"}
	blank.Commands[AssocView], blank.Enabled[AssocView] = "  ", true
	snapshot, err := filegroups.Compile([]filegroups.Group{
		{ID: "text", Fields: map[string]string{
			"Name": "Text files", "Mask": "*.txt", "Associations.Enabled": "true",
		}, Commands: []filegroups.CommandSet{first, blank}},
		{ID: "all", Fields: map[string]string{
			"Name": "All files", "Mask": "*", "Associations.Enabled": "true",
		}, Commands: []filegroups.CommandSet{second}},
	})
	if err != nil {
		t.Fatal(err)
	}
	matches := matchingGroupAssociations(snapshot, &vfs.VFSItem{Name: "notes.txt"}, AssocView)
	if len(matches) != 2 {
		t.Fatalf("View matches = %+v", matches)
	}
	if got := assocPickerLabel(matches[0]); got != "Viewer [Text files]" {
		t.Fatalf("first choice = %q", got)
	}
	if got := assocPickerLabel(matches[1]); got != "Viewer [All files]" {
		t.Fatalf("second choice = %q", got)
	}
	single := matchingGroupAssociations(snapshot, &vfs.VFSItem{Name: "photo.png"}, AssocView)
	if len(single) != 1 || single[0].Commands[AssocView] != "view-two" {
		t.Fatalf("single View match = %+v", single)
	}
}

func TestScopedAppearanceRefreshesLargePagedCatalogWithoutReload(t *testing.T) {
	oldRows := semantic.SetPanelCatalogRowsEnabled(true)
	oldMetadata := semantic.SetPanelCatalogMetadataEnabled(true)
	oldHighlighter := theme.GlobalFileHighlighter
	t.Cleanup(func() {
		semantic.SetPanelCatalogRowsEnabled(oldRows)
		semantic.SetPanelCatalogMetadataEnabled(oldMetadata)
		theme.GlobalFileHighlighter = oldHighlighter
	})
	highlighter := &theme.FileHighlighter{}
	theme.GlobalFileHighlighter = highlighter
	groups := []filegroups.Group{}
	for _, scope := range []string{"lucide", "zoin", "system"} {
		groups = append(groups, filegroups.Group{ID: scope, Fields: map[string]string{
			"IconTheme": scope, "Mask": "*", "Highlight.Enabled": "true", "NormalColor": "foreground:#123456",
			"Icons.Enabled": "true", "Icon.*": "asset:" + scope,
		}})
	}
	runtime, err := filegroups.NewRuntime(groups, func(snapshot *filegroups.Snapshot) { _ = highlighter.LoadFileGroups(snapshot) })
	if err != nil {
		t.Fatal(err)
	}
	runtime.SetIconTheme("lucide")
	entries := make([]*FileEntry, 10000)
	for i := range entries {
		entries[i] = &FileEntry{VFSItem: vfs.VFSItem{Name: fmt.Sprintf("entry-%d", i)}}
	}
	entries[42].Selected = true
	fp := &FileSystemPanel{Vfs: vfs.NewNullVFS(1), Table: vtui.NewTable(0, 0, 80, 40, nil), Entries: entries, SelectedItems: map[string]bool{entries[42].Name: true}}
	t.Cleanup(fp.unpublishSemanticMetadataSnapshot)
	fp.SetCursorIndex(4123)
	fp.Table.TopPos = 4000
	initial := fp.SemanticPanelModel(nil, 0, true)
	for _, scope := range []string{"zoin", "system", "lucide"} {
		runtime.SetIconTheme(scope)
		model := fp.SemanticPanelModel(nil, 0, true)
		if model.CatalogRevision != initial.CatalogRevision {
			t.Fatal("theme resent/replaced catalog")
		}
		if model.MetadataRevision == initial.MetadataRevision {
			t.Fatal("theme failed to invalidate deferred appearance metadata")
		}
		if len(model.Entries) > initialPanelCatalogRowsLimit {
			t.Fatal("full 10000-entry catalog materialized")
		}
		chunk, ok := BuildLivePanelCatalogMetadataChunk(model.ID, model.Path, model.CatalogRevision, model.MetadataRevision, 42, 1)
		if !ok {
			t.Fatal("metadata rejected after theme switch")
		}
		rows := semantic.AppMapSlice(chunk["entries"])
		styles := semantic.AppMap(chunk["highlightStyles"])
		style := semantic.AppMap(styles[semantic.String(rows[0]["highlightStyleId"])])
		choice := semantic.AppMap(semantic.AppMap(style["iconChoices"])["*"])
		if choice["value"] != scope {
			t.Fatalf("stale theme metadata: %+v", choice)
		}
		if fp.Entries[42] != entries[42] || !entries[42].Selected || fp.GetCursorIndex() != 4123 || fp.Table.TopPos != 4000 {
			t.Fatal("theme lost panel state")
		}
		initial = model
	}
}

func TestScopedAssociationSlotsUseOnlyActiveCommittedSet(t *testing.T) {
	groups := []filegroups.Group{}
	for _, scope := range []string{"lucide", "zoin", "system", ""} {
		command := filegroups.CommandSet{ID: "app", Name: scope + " app"}
		for slot := range 6 {
			command.Commands[slot] = fmt.Sprintf("%s-slot%d", scope, slot)
			command.Enabled[slot] = true
		}
		groups = append(groups, filegroups.Group{ID: "group" + scope, Fields: map[string]string{
			"IconTheme": scope, "Mask": "*", "Associations.Enabled": "true",
		}, Commands: []filegroups.CommandSet{command}})
	}
	runtime, err := filegroups.NewRuntime(groups, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"lucide", "zoin", "system", "lucide"} {
		runtime.SetIconTheme(scope)
		for slot := AssocExecute; slot < AssocKindCount; slot++ {
			matches := matchingGroupAssociations(runtime.Committed(), &vfs.VFSItem{Name: "x"}, slot)
			if len(matches) != 2 || matches[0].Commands[slot] != fmt.Sprintf("-slot%d", slot) || matches[1].Commands[slot] != fmt.Sprintf("%s-slot%d", scope, slot) {
				t.Fatalf("%s slot %d: %+v", scope, slot, matches)
			}
		}
	}
	groups[3].Commands[0].Commands[0] = "draft-only"
	if err := runtime.Preview(groups); err != nil {
		t.Fatal(err)
	}
	if matchingGroupAssociations(runtime.Committed(), &vfs.VFSItem{Name: "x"}, AssocExecute)[0].Commands[0] == "draft-only" {
		t.Fatal("preview command activated")
	}
}
