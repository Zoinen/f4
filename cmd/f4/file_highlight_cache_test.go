package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func installTestFileHighlighter(t *testing.T, iniText string) *FileHighlighter {
	t.Helper()
	oldHighlighter := GlobalFileHighlighter
	oldConfig := AppConfig
	highlighter := &FileHighlighter{}
	highlighter.LoadFromIni(ParseIni(strings.NewReader(iniText)))
	GlobalFileHighlighter = highlighter
	AppConfig.EnforceColorCorrection = false
	t.Cleanup(func() {
		GlobalFileHighlighter = oldHighlighter
		AppConfig = oldConfig
	})
	return highlighter
}

func TestFileEntryHighlightCacheInvalidation(t *testing.T) {
	highlighter := installTestFileHighlighter(t, `[Highlight_0]
Mask = *.go
Mark = G
NormalColor = foreground:#010203
SelectedColor = foreground:#040506
`)
	entry := &fileEntry{VFSItem: vfs.VFSItem{Name: "main.go", Size: 10}}
	defaultOne := ParseFarColor("foreground:#111111 | background:#121212", 0)
	defaultTwo := ParseFarColor("foreground:#111111 | background:#343434", 0)

	first := entry.highlightColor(defaultOne, false, false)
	wantFirst := highlighter.GetColor(&entry.VFSItem, defaultOne, false, false)
	if first != wantFirst {
		t.Fatalf("first cached color = %#x, want %#x", first, wantFirst)
	}
	if !entry.highlightCache.colors[0].valid {
		t.Fatal("normal-state color was not cached")
	}
	if got := entry.highlightColor(defaultOne, false, false); got != first {
		t.Fatalf("cache hit color = %#x, want %#x", got, first)
	}

	// A palette change arrives through the exact default attribute passed by
	// the table. It must not reuse a result whose inherited background came
	// from the previous palette value.
	second := entry.highlightColor(defaultTwo, false, false)
	wantSecond := highlighter.GetColor(&entry.VFSItem, defaultTwo, false, false)
	if second != wantSecond || second == first {
		t.Fatalf("color after default change = %#x, want %#x (old %#x)", second, wantSecond, first)
	}

	selected := entry.highlightColor(defaultTwo, true, false)
	wantSelected := highlighter.GetColor(&entry.VFSItem, defaultTwo, true, false)
	if selected != wantSelected || !entry.highlightCache.colors[1].valid {
		t.Fatalf("selected color = %#x, want %#x; selected cache valid=%v", selected, wantSelected, entry.highlightCache.colors[1].valid)
	}

	AppConfig.EnforceColorCorrection = true
	wantCorrected := highlighter.GetColor(&entry.VFSItem, defaultTwo, false, false)
	if got := entry.highlightColor(defaultTwo, false, false); got != wantCorrected || !entry.highlightCache.colors[0].contrast {
		t.Fatalf("color after contrast setting change = %#x, want %#x; contrast key=%v", got, wantCorrected, entry.highlightCache.colors[0].contrast)
	}
	AppConfig.EnforceColorCorrection = false

	if marker := entry.highlightMarker(); marker != "G" {
		t.Fatalf("marker = %q, want G", marker)
	}
	if !entry.highlightCache.markerValid {
		t.Fatal("marker was not cached")
	}

	oldRevision := entry.highlightCache.revision
	highlighter.LoadFromIni(ParseIni(strings.NewReader(`[Highlight_0]
Mask = *.go
Mark = N
NormalColor = foreground:#abcdef
`)))
	if highlighter.revision <= oldRevision {
		t.Fatalf("rule revision did not advance: old=%d new=%d", oldRevision, highlighter.revision)
	}
	wantReloaded := highlighter.GetColor(&entry.VFSItem, defaultTwo, false, false)
	if got := entry.highlightColor(defaultTwo, false, false); got != wantReloaded || got == second {
		t.Fatalf("color after rules reload = %#x, want %#x (old %#x)", got, wantReloaded, second)
	}
	if marker := entry.highlightMarker(); marker != "N" {
		t.Fatalf("marker after rules reload = %q, want N", marker)
	}

	// Size participates in rule matching and can change after the asynchronous
	// directory-size calculation. The fingerprint must invalidate without an
	// explicit cache-reset call from that worker.
	highlighter.LoadFromIni(ParseIni(strings.NewReader(`[Highlight_0]
Mask = *.go
SizeAbove = 100
NormalColor = foreground:#fedcba
`)))
	if got := entry.highlightColor(defaultTwo, false, false); got != defaultTwo {
		t.Fatalf("color below size threshold = %#x, want default %#x", got, defaultTwo)
	}
	entry.Size = 101
	wantAfterSize := highlighter.GetColor(&entry.VFSItem, defaultTwo, false, false)
	if got := entry.highlightColor(defaultTwo, false, false); got != wantAfterSize || got == defaultTwo {
		t.Fatalf("color after size mutation = %#x, want %#x", got, wantAfterSize)
	}
}

func TestFileEntryHighlightCacheSkipsRelativeDateRules(t *testing.T) {
	installTestFileHighlighter(t, `[Highlight_0]
Mask = *
DateRelative = 1
DateAfter = 1s
NormalColor = foreground:#010203
`)
	entry := &fileEntry{VFSItem: vfs.VFSItem{Name: "moving.txt", MTime: time.Now()}}
	entry.highlightColor(0, false, false)
	entry.highlightMarker()

	if entry.highlightCache.colors[0].valid || entry.highlightCache.markerValid {
		t.Fatalf("relative-date result was cached: %+v", entry.highlightCache)
	}
}

func BenchmarkFileEntryHighlightColor(b *testing.B) {
	var ini strings.Builder
	for rule := 0; rule < 18; rule++ {
		fmt.Fprintf(&ini, "[Highlight_%d]\n", rule)
		fmt.Fprintf(&ini, "Mask = *.mask%d-a,*.mask%d-b,*.mask%d-c,*.mask%d-d,*.go\n", rule, rule, rule, rule)
		fmt.Fprintln(&ini, "NormalColor = foreground:#123456")
		fmt.Fprintln(&ini, "ContinueProcessing = 1")
	}
	highlighter := &FileHighlighter{}
	highlighter.LoadFromIni(ParseIni(strings.NewReader(ini.String())))
	entry := &fileEntry{VFSItem: vfs.VFSItem{Name: "main.go", Size: 10}}
	defaultAttr := ParseFarColor("foreground:#ffffff | background:#000000", 0)
	oldHighlighter := GlobalFileHighlighter
	oldContrast := AppConfig.EnforceColorCorrection
	GlobalFileHighlighter = highlighter
	AppConfig.EnforceColorCorrection = false
	b.Cleanup(func() {
		GlobalFileHighlighter = oldHighlighter
		AppConfig.EnforceColorCorrection = oldContrast
	})

	b.Run("cached-entry", func(b *testing.B) {
		entry.highlightColor(defaultAttr, false, false)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = entry.highlightColor(defaultAttr, false, false)
		}
	})
	b.Run("direct-rule-scan", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = highlighter.GetColor(&entry.VFSItem, defaultAttr, false, false)
		}
	})
}
