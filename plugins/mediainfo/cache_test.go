package mediainfo

import (
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

type unkeyedMediaVFS struct {
	vfs.VFS
	values []int
}

func TestReportCacheLRUEviction(t *testing.T) {
	cache := newReportCache(2)
	cache.put("one", Report{General: General{Format: "one"}})
	cache.put("two", Report{General: General{Format: "two"}})
	if _, ok := cache.get("one"); !ok {
		t.Fatal("first entry missing")
	}
	cache.put("three", Report{General: General{Format: "three"}})
	if _, ok := cache.get("two"); ok {
		t.Fatal("least recently used entry was retained")
	}
	if report, ok := cache.get("one"); !ok || report.General.Format != "one" {
		t.Fatal("recently used entry was evicted")
	}
}

func TestReportCacheWeightedEviction(t *testing.T) {
	one := Report{Tags: []Field{{Name: "Title", Value: strings.Repeat("a", 256)}}}
	two := Report{Tags: []Field{{Name: "Artist", Value: strings.Repeat("b", 256)}}}
	oneWeight := cachedReportWeight("one", one)
	twoWeight := cachedReportWeight("two", two)
	cache := newReportCacheWithLimit(reportCacheCapacity, oneWeight+twoWeight-1)

	cache.put("one", one)
	cache.put("two", two)

	if _, ok := cache.get("one"); ok {
		t.Fatal("byte ceiling retained the least recently used entry")
	}
	if report, ok := cache.get("two"); !ok || report.Tags[0].Name != "Artist" {
		t.Fatal("byte ceiling evicted the newest entry")
	}
	if cache.usedBytes != twoWeight || cache.usedBytes > cache.maxBytes {
		t.Fatalf("used bytes = %d, want %d within %d", cache.usedBytes, twoWeight, cache.maxBytes)
	}
}

func TestReportCacheRejectsOversizedReplacement(t *testing.T) {
	small := Report{General: General{Format: "WAVE"}}
	smallWeight := cachedReportWeight("same", small)
	cache := newReportCacheWithLimit(reportCacheCapacity, smallWeight+64)
	cache.put("same", small)
	if _, ok := cache.get("same"); !ok {
		t.Fatal("small entry was not admitted")
	}

	large := Report{General: General{Format: strings.Repeat("x", int(cache.maxBytes)+1)}}
	cache.put("same", large)
	if _, ok := cache.get("same"); ok {
		t.Fatal("oversized replacement left a stale entry cached")
	}
	cache.put("other", large)
	if _, ok := cache.get("other"); ok {
		t.Fatal("oversized new entry was admitted")
	}
	if cache.usedBytes != 0 || cache.lru.Len() != 0 {
		t.Fatalf("rejected entries retained accounting: bytes=%d entries=%d", cache.usedBytes, cache.lru.Len())
	}
}

func TestReportCacheReplacementAndClearAccounting(t *testing.T) {
	cache := newReportCacheWithLimit(reportCacheCapacity, 1<<20)
	first := Report{General: General{Format: "FLAC"}}
	cache.put("item", first)
	if want := cachedReportWeight("item", first); cache.usedBytes != want {
		t.Fatalf("initial weight = %d, want %d", cache.usedBytes, want)
	}

	fields := make([]Field, 1, 32)
	fields[0] = Field{Name: "Comment", Value: strings.Repeat("v", 512)}
	replacement := Report{General: General{Format: "FLAC"}, Tags: fields}
	cache.put("item", replacement)
	if want := cachedReportWeight("item", replacement); cache.usedBytes != want {
		t.Fatalf("replacement weight = %d, want %d", cache.usedBytes, want)
	}
	if cache.lru.Len() != 1 {
		t.Fatalf("replacement created %d entries", cache.lru.Len())
	}

	cache.clear()
	if cache.usedBytes != 0 || cache.lru.Len() != 0 || len(cache.items) != 0 {
		t.Fatalf("clear retained accounting: bytes=%d entries=%d items=%d", cache.usedBytes, cache.lru.Len(), len(cache.items))
	}
}

func TestCachedReportWeightIncludesSliceCapacityAndStrings(t *testing.T) {
	baseline := cachedReportWeight("key", Report{})
	fields := make([]Field, 1, 64)
	fields[0] = Field{Name: "Comment", Value: strings.Repeat("z", 1024)}
	weighted := cachedReportWeight("key", Report{Tags: fields})
	if weighted <= baseline+int64(len(fields[0].Name)+len(fields[0].Value)) {
		t.Fatalf("weight %d does not include slice backing storage above baseline %d", weighted, baseline)
	}
}

func TestReportCacheKeyTracksRevisionAndMode(t *testing.T) {
	fs := vfs.NewOSVFS(t.TempDir())
	item := vfs.VFSItem{Name: "movie.mp4", Size: 10, SizeKnown: true, MTime: time.Unix(1, 2), Revision: "a"}
	fast := reportCacheKey(fs, "movie.mp4", item, ModeFast)
	item.Revision = "b"
	revised := reportCacheKey(fs, "movie.mp4", item, ModeFast)
	detailed := reportCacheKey(fs, "movie.mp4", item, ModeDetailed)
	if fast == revised {
		t.Fatal("revision did not change the cache key")
	}
	if revised == detailed {
		t.Fatal("analysis mode did not change the cache key")
	}
}

func TestReportCacheBypassesUnkeyedValueVFS(t *testing.T) {
	fs := unkeyedMediaVFS{values: []int{1}}
	if key := reportCacheKey(fs, "movie.mp4", vfs.VFSItem{Name: "movie.mp4", SizeKnown: true}, ModeFast); key != "" {
		t.Fatalf("unsafe cache key = %q", key)
	}
}

func TestReportCacheBypassesFilesWithoutRevisionSizeOrTime(t *testing.T) {
	fs := vfs.NewOSVFS(t.TempDir())
	if key := reportCacheKey(fs, "unknown.mp4", vfs.VFSItem{Name: "unknown.mp4"}, ModeFast); key != "" {
		t.Fatalf("unsafe unknown-file cache key = %q", key)
	}
}
