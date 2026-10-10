package theme

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/vfs"
)

func TestSemanticUseDefaultsDeferredMetadataAndReload(t *testing.T) {
	fh := useDefaultsHighlighter(t, `[Highlight_10]
Mask = *.go
NormalColor = foreground:#112233
Mark = B
Icon = qrc:/icons/lucide/file.svg

[Highlight_20]
UseDefaults = 1
SizeAbove = 100
NormalColor = foreground:#445566
Mark = O
Icon = qrc:/icons/lucide/archive.svg
`)
	item := vfs.VFSItem{Name: "main.go", Size: 200}
	_, provisional := fh.SemanticStyle(&item, false)
	if provisional.Normal.Foreground != "#112233" || provisional.Marker != "B" ||
		provisional.IconKey != "file" {
		t.Fatalf("provisional style applied deferred override: %+v", provisional)
	}
	id, complete := fh.SemanticStyle(&item, true)
	if complete.Normal.Foreground != "#445566" || complete.Marker != "O" ||
		complete.IconKey != "archive" || len(complete.Groups) != 2 {
		t.Fatalf("complete style lost override after terminating rule: %+v", complete)
	}
	if complete.Cursor.Foreground != "" || complete.SelectedCursor.Foreground != "" {
		t.Fatalf("unspecified cursor states inherited normal color: %+v", complete)
	}
	revision := fh.Revision
	fh.UserRules[1].NormalStr = "foreground:#778899"
	fh.CombineRules()
	newID, reloaded := fh.SemanticStyle(&item, true)
	if fh.Revision == revision || newID == id || reloaded.Normal.Foreground != "#778899" {
		t.Fatalf("override reload left a stale semantic style: %+v", reloaded)
	}
}

func TestHighlightCacheTracksLinkAndJunctionIdentity(t *testing.T) {
	fh := useDefaultsHighlighter(t, `[Highlight_0]
UseDefaults = 1
IncludeAttributes = Junction
Mark = J

[Highlight_1]
UseDefaults = 1
IncludeAttributes = Symlink
Mark = L
`)
	item := vfs.VFSItem{Name: "same", IsDir: true}
	if marker := fh.GetMarker(&item); marker != "" {
		t.Fatalf("ordinary directory marker = %q", marker)
	}
	item.IsSymlink = true
	if marker := fh.GetMarker(&item); marker != "L" {
		t.Fatalf("symlink marker = %q, want L", marker)
	}
	item.ReparseTag = vfs.ReparseTagMountPoint
	if marker := fh.GetMarker(&item); marker != "J" {
		t.Fatalf("junction marker = %q, want J", marker)
	}
}

func TestHighlightRelativeOverrideBypassesMatchCache(t *testing.T) {
	previous := filepathMatchFn
	calls := 0
	filepathMatchFn = func(pattern, name string) (bool, error) {
		calls++
		return filepath.Match(pattern, name)
	}
	t.Cleanup(func() { filepathMatchFn = previous })
	fh := &FileHighlighter{}
	fh.LoadUserRules(ini.Parse(strings.NewReader(`[Highlight_0]
UseDefaults = 1
Mask = *.[g]o
DateRelative = 1
DateAfter = 1h
Mark = R
`)))
	item := vfs.VFSItem{Name: "main.go", MTime: time.Now()}
	for range 2 {
		if marker := fh.GetMarker(&item); marker != "R" {
			t.Fatalf("recent file marker = %q", marker)
		}
	}
	if calls != 2 || len(fh.matchCache) != 0 {
		t.Fatalf("relative override was cached: glob calls=%d, cache entries=%d", calls, len(fh.matchCache))
	}
}
