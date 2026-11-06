package filegroups

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestThemeSnapshotsReusePredicatesAndOrder(t *testing.T) {
	groups := []Group{}
	for _, scope := range []string{"lucide", "zoin", "system", "", "unknown"} {
		for i := range 2 {
			groups = append(groups, Group{ID: fmt.Sprintf("%s-%d", scope, i), Fields: map[string]string{
				"IconTheme": scope, "Mask": "*.txt", "Highlight.Enabled": "true", "Sort.Enabled": "true",
				"Icons.Enabled": "true", "Icon.*": "asset:file", "Associations.Enabled": "true",
			}, Commands: []CommandSet{{ID: "cmd", Commands: [6]string{"1", "2", "3", "4", "5", "6"}, Enabled: [6]bool{true, true, true, true, true, true}}}})
		}
	}
	snapshot, err := Compile(groups)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"lucide", "zoin", "system", "lucide", "unknown", "generic"} {
		t.Run(scope, func(t *testing.T) {
			view := snapshot.ForIconTheme(scope)
			want := []string{"-0", "-1"}
			if scope != "generic" {
				want = append(want, scope+"-0", scope+"-1")
			}
			for _, facet := range []string{"Highlight", "Icons", "Sort", "Associations"} {
				ids := []string{}
				for _, group := range view.Matching(&vfs.VFSItem{Name: "x.txt"}, true, facet, time.Now()) {
					ids = append(ids, group.ID)
					if view.Predicate(group.ID) != snapshot.Predicate(group.ID) {
						t.Fatal("predicate recompiled")
					}
					if facet == "Associations" && group.Commands[0].Commands[5] != "6" {
						t.Fatal("alternate slot lost")
					}
				}
				if !reflect.DeepEqual(ids, want) {
					t.Fatalf("%s ids=%v want %v", facet, ids, want)
				}
			}
			if view != snapshot.ForIconTheme(scope) {
				t.Fatal("view not reused")
			}
			choices := view.IconChoices(&vfs.VFSItem{Name: "x.txt"}, true, time.Now())
			if choices["*"].GroupID != "-0" {
				t.Fatal("themed icons overrode universal choice")
			}
			for range 10000 {
				_ = view.IconChoices(&vfs.VFSItem{Name: "x.txt"}, true, time.Now())
			}
		})
	}
}

func TestFactoryClassificationIsSharedAcrossIconCatalogs(t *testing.T) {
	groups, err := FactoryGroups()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := Compile(groups)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.ForIconTheme("").Groups()) < 3 {
		t.Fatal("missing shared groups")
	}
	for _, scope := range []string{"lucide", "zoin", "system", "unknown"} {
		if len(snapshot.ForIconTheme(scope).Groups()) != len(groups) {
			t.Fatal("icon theme changed group definitions", scope)
		}
	}
}

func TestThemeChangeDuringOwnedPreview(t *testing.T) {
	groups := []Group{{ID: "universal", Fields: map[string]string{"Name": "saved"}},
		{ID: "lucide", Fields: map[string]string{"IconTheme": "lucide"}},
		{ID: "zoin", Fields: map[string]string{"IconTheme": "zoin"}}}
	runtime, err := NewRuntime(groups, nil)
	if err != nil {
		t.Fatal(err)
	}
	runtime.SetIconTheme("lucide")
	session := runtime.BeginPreview()
	groups[0].Fields["Name"] = "draft"
	snapshot, err := Compile(groups)
	if err != nil {
		t.Fatal(err)
	}
	session.Publish(snapshot)
	runtime.SetIconTheme("zoin")
	if runtime.Appearance().Groups()[0].Label() != "draft" || runtime.Committed().Groups()[0].Label() != "saved" {
		t.Fatal("draft/commands baseline mixed")
	}
	session.Cancel()
	if runtime.IconTheme() != "zoin" || runtime.Appearance().Groups()[1].ID != "zoin" {
		t.Fatal("Cancel restored stale theme")
	}
	if err := runtime.Apply(groups); err != nil {
		t.Fatal(err)
	}
	runtime.SetIconTheme("lucide")
	runtime.SetIconTheme("zoin")
	if runtime.Committed().Groups()[0].Label() != "draft" {
		t.Fatal("Apply lost hidden edits")
	}
}
