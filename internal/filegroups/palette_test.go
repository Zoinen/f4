package filegroups

import (
	"reflect"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestPaletteProjectionAndCompiledConditions(t *testing.T) {
	prefix := PalettePrefix("Тёмная.style")
	if prefix != "Palette.d0a2d191d0bcd0bdd0b0d18f2e7374796c65." {
		t.Fatal(prefix)
	}
	g := Group{ID: "type.executables", Fields: map[string]string{
		"Mask": "*.exe", "Highlight.Mask": "tool", "NormalColor": "",
		prefix + "Mask": "*", prefix + "IncludeAttributes": "Executable",
		prefix + "NormalColor": "red", prefix + "Highlight.Enabled": "true",
		prefix + "Future": "retained",
	}}
	effective := g.Highlighting("Тёмная.style")
	if effective.Fields["Mask"] != "tool" || effective.Fields["NormalColor"] != "" || effective.Fields["ColorStyle"] != "Тёмная.style" || effective.Fields["Future"] != "retained" {
		t.Fatal(effective.Fields)
	}
	snapshot, err := Compile([]Group{g})
	if err != nil {
		t.Fatal(err)
	}
	p := snapshot.HighlightPredicate(g.ID, "Тёмная.style")
	if p == nil || p != snapshot.HighlightPredicate(g.ID, "Тёмная.style") {
		t.Fatal("predicate was not precompiled")
	}
	if !p.Match(&vfs.VFSItem{Name: "tool", IsExecutable: true}, true, time.Time{}) || p.Match(&vfs.VFSItem{Name: "other", IsExecutable: true}, true, time.Time{}) {
		t.Fatal("highlight overrides were not compiled")
	}
	if !snapshot.Predicate(g.ID).Match(&vfs.VFSItem{Name: "other.exe"}, true, time.Time{}) {
		t.Fatal("classification changed")
	}
	g.Fields["Highlight.Enabled"] = "false"
	if g.Highlighting("Тёмная.style").Fields["ColorStyle"] != "" {
		t.Fatal("global highlight override stayed in theme layer")
	}
}

func TestPaletteOrderAndValidation(t *testing.T) {
	prefix := PalettePrefix("A")
	groups := []Group{
		{ID: "late", Fields: map[string]string{prefix + "Order": "8", prefix + "Highlight.Enabled": "true"}},
		{ID: "user", Fields: map[string]string{"Highlight.Enabled": "true"}},
		{ID: "early", Fields: map[string]string{prefix + "Order": "1", prefix + "Highlight.Enabled": "true"}},
	}
	snapshot, err := Compile(groups)
	if err != nil {
		t.Fatal(err)
	}
	got := snapshot.HighlightGroups("A")
	if got[0].ID != "user" || got[1].ID != "early" || got[2].ID != "late" {
		t.Fatal(got)
	}
	groups[0].Fields[prefix+"Mask"] = "["
	if _, err := Compile(groups); err == nil {
		t.Fatal("invalid palette condition accepted")
	}
}

func TestPaletteAllHighlightConditionsAndUniversalOverrides(t *testing.T) {
	prefix := PalettePrefix("A")
	fields := map[string]string{
		"Mask": "*.base", "IncludeAttributes": "Hidden", "Highlight.Enabled": "true",
		prefix + "Highlight.Enabled": "false", prefix + "Name": "Palette label",
		"Name": "Canonical label", "ContinueProcessing": "", prefix + "ContinueProcessing": "true",
	}
	for _, key := range HighlightConditionKeys {
		fields[prefix+key] = "palette:" + key
		fields["Highlight."+key] = "override:" + key
	}
	g := Group{ID: "all", Fields: fields}
	original := g.Clone()
	effective := g.Highlighting("A")
	for _, key := range HighlightConditionKeys {
		if got := effective.Fields[key]; got != "override:"+key {
			t.Fatalf("%s = %s", key, got)
		}
	}
	if effective.Fields["Name"] != "Canonical label" || effective.Fields["ColorStyle"] != "" || !effective.Enabled("Highlight") || effective.Fields["ContinueProcessing"] != "" {
		t.Fatal(effective)
	}
	effective.Fields[prefix+"Name"] = "mutated"
	if !reflect.DeepEqual(g, original) {
		t.Fatal("projection mutated original")
	}
	delete(fields, "Highlight.Enabled")
	if g.Highlighting("A").Enabled("Highlight") {
		t.Fatal("palette disable ignored")
	}
	if g.Highlighting("Other").Enabled("Highlight") {
		t.Fatal("inactive palette enabled highlighting")
	}
}

func TestPalettePredicateClearsClassificationRestrictions(t *testing.T) {
	prefix := PalettePrefix("A")
	g := Group{ID: "g", Fields: map[string]string{
		"Mask": "*.base", "IncludeAttributes": "Hidden", "SizeAbove": "100", "DateAfter": "2026-01-01 00:00:00",
		prefix + "Mask": "*.palette", prefix + "Highlight.Enabled": "true",
	}}
	snapshot, err := Compile([]Group{g})
	if err != nil {
		t.Fatal(err)
	}
	item := &vfs.VFSItem{Name: "a.palette"}
	if !snapshot.HighlightPredicate(g.ID, "A").Match(item, false, time.Time{}) {
		t.Fatal("palette inherited classification restrictions")
	}
	if snapshot.HighlightPredicate(g.ID, "unknown") != snapshot.HighlightPredicate(g.ID, "") {
		t.Fatal("missing palette should share universal predicate")
	}
	if snapshot.Predicate(g.ID).Match(item, true, time.Time{}) {
		t.Fatal("classification inherited palette mask")
	}
}
