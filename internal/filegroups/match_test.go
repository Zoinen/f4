package filegroups

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestCompiledPredicates(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, mask, syntax, filename string
		want                         bool
	}{
		{"suffix", "*.png", "far", "IMAGE.PNG", true},
		{"exclude", "*.png|thumb*", "far", "thumb.png", false},
		{"separator", "*.png;*.jpg", "far", "a.jpg", true},
		{"regex", `/^IMG_[0-9]+\.jpg$/`, "far", "IMG_13.jpg", true},
		{"regex alternation list", `/^(a|b),c$/;*.png|thumb*`, "far", "a,c", true},
		{"regex alternation excluded", `/^(a|b),c$/;*.png|thumb*`, "far", "thumb.png", false},
		{"legacy Unicode lower, not fold", "*.s", "legacy-glob", "file.ſ", false},
		{"Far glob Unicode lower, not fold", "*.s", "far", "file.ſ", false},
		{"legacy literal exclusion", "*.png|thumb*", "legacy-glob", "a.png", false},
		{"legacy lists", "*.png,*.jpg", "legacy-glob", "a.jpg", true},
		{"directory separator", "*", "far", filepath.Join("dir", "file"), false},
		{"dot star", "*.*", "far", "LICENSE", true},
		{"class", "file[0-9].txt", "far", "file2.txt", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := CompilePredicate(map[string]string{"Mask": tc.mask, "MaskSyntax": tc.syntax})
			if err != nil {
				t.Fatal(err)
			}
			if got := p.Match(&vfs.VFSItem{Name: tc.filename}, true, now); got != tc.want {
				t.Errorf("match=%v want %v", got, tc.want)
			}
		})
	}
}

func TestPredicatesMetadataAndDates(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	p, err := CompilePredicate(map[string]string{"Mask": "*.log", "SizeAbove": "10", "SizeBelow": "30", "DateRelative": "true", "DateAfter": "2d", "DateBefore": "1h", "ExcludeAttributes": "Directory"})
	if err != nil {
		t.Fatal(err)
	}
	item := &vfs.VFSItem{Name: "app.log", Size: 20, MTime: now.Add(-2 * time.Hour)}
	if p.Match(item, false, now) || !p.Match(item, true, now) {
		t.Fatal("deferred predicate evaluated incorrectly")
	}
	item.IsDir = true
	if p.Match(item, true, now) {
		t.Fatal("directory matched")
	}
	item.IsDir = false
	item.MTime = now
	if p.Match(item, true, now) {
		t.Fatal("relative before ignored")
	}
}

func TestIndependentFacets(t *testing.T) {
	s, err := Compile([]Group{
		{ID: "icon", Fields: map[string]string{"Mask": "*.png", "Icons.Enabled": "true"}},
		{ID: "colour", Fields: map[string]string{"Mask": "*.png", "Highlight.Enabled": "true"}},
		{ID: "open", Fields: map[string]string{"Mask": "*.png", "Associations.Enabled": "true"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for facet, id := range map[string]string{"Icons": "icon", "Highlight": "colour", "Associations": "open"} {
		got := s.Matching(&vfs.VFSItem{Name: "file.png"}, true, facet, time.Now())
		if len(got) != 1 || got[0].ID != id {
			t.Fatalf("%s: %#v", facet, got)
		}
	}
}

func TestInvalidPredicates(t *testing.T) {
	for _, fields := range []map[string]string{{"Mask": "["}, {"Mask": "/[/"}, {"SizeAbove": "-1"}, {"SizeAbove": "100", "SizeBelow": "10"}, {"DateAfter": "tomorrow"}, {"DateRelative": "true", "DateBefore": "999999999999999999999d"}, {"IncludeAttributes": "unknown"}} {
		if _, err := CompilePredicate(fields); err == nil {
			t.Errorf("accepted %#v", fields)
		}
	}
}

func BenchmarkCompiledTenThousandFiles(b *testing.B) {
	p, err := CompilePredicate(map[string]string{"Mask": "*.png,*.jpg,*.jpeg,*.webp|thumb*"})
	if err != nil {
		b.Fatal(err)
	}
	items := make([]vfs.VFSItem, 10000)
	for i := range items {
		items[i].Name = fmt.Sprintf("image%d.jpg", i)
	}
	now := time.Now()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		for i := range items {
			p.Match(&items[i], true, now)
		}
	}
}
