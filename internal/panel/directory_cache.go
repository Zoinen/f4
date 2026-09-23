package panel

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// The cache is deliberately small and bounded. It keeps the last useful
// snapshot for a remote navigation round-trip, while authoritative ReadDir
// remains the source of truth and replaces the snapshot before the spinner is
// cleared.
const (
	maxDirectoryCacheFolders = 32
	maxDirectoryCacheItems   = 64 * 1024
)

type directoryCacheEntry struct {
	items []vfs.VFSItem
	count int
}

type directoryListingCache struct {
	mu        sync.Mutex
	entries   map[string]directoryCacheEntry
	order     []string
	itemCount int
}

func newDirectoryListingCache() *directoryListingCache {
	return &directoryListingCache{entries: make(map[string]directoryCacheEntry)}
}

func directoryCacheIdentity(filesystem vfs.VFS) (string, bool) {
	if filesystem == nil {
		return "", false
	}
	var identity any
	if provider, ok := filesystem.(vfs.DirectoryCacheKeyProvider); ok {
		identity = provider.DirectoryCacheKey()
	}
	if identity == nil {
		if provider, ok := filesystem.(vfs.StableDirectoryIdentity); ok {
			identity = provider.StableDirectoryKey()
		}
	}
	if identity == nil {
		return "", false
	}
	typ := reflect.TypeOf(identity)
	if !typ.Comparable() {
		return "", false
	}
	// Providers normally use strings. Pointer identities are supported too,
	// but are rendered by address because dereferencing them could expose
	// mutable or sensitive provider state in a cache key/log.
	var rendered string
	switch reflect.ValueOf(identity).Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.UnsafePointer:
		rendered = fmt.Sprintf("%T:%p", identity, identity)
	default:
		rendered = fmt.Sprintf("%T:%v", identity, identity)
	}
	return rendered, true
}

func directoryCacheToken(filesystem vfs.VFS, path string) (string, bool) {
	identity, ok := directoryCacheIdentity(filesystem)
	if !ok || path == "" {
		return "", false
	}
	return identity + "\x00" + path, true
}

func (cache *directoryListingCache) get(filesystem vfs.VFS, path string) ([]vfs.VFSItem, bool) {
	if cache == nil {
		return nil, false
	}
	token, ok := directoryCacheToken(filesystem, path)
	if !ok {
		return nil, false
	}
	cache.mu.Lock()
	entry, found := cache.entries[token]
	if found {
		cache.touchLocked(token)
	}
	cache.mu.Unlock()
	if !found {
		return nil, false
	}
	items := append([]vfs.VFSItem(nil), entry.items...)
	return items, true
}

func (cache *directoryListingCache) store(filesystem vfs.VFS, path string, items []vfs.VFSItem) bool {
	if cache == nil {
		return false
	}
	token, ok := directoryCacheToken(filesystem, path)
	if !ok || len(items) > maxDirectoryCacheItems {
		return false
	}
	copyItems := append([]vfs.VFSItem(nil), items...)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if old, exists := cache.entries[token]; exists {
		cache.itemCount -= old.count
	}
	cache.entries[token] = directoryCacheEntry{items: copyItems, count: len(copyItems)}
	cache.itemCount += len(copyItems)
	cache.touchLocked(token)
	for len(cache.order) > maxDirectoryCacheFolders || cache.itemCount > maxDirectoryCacheItems {
		victim := cache.order[0]
		cache.order = cache.order[1:]
		if victim == token && len(cache.order) == 0 {
			break
		}
		if old, exists := cache.entries[victim]; exists {
			delete(cache.entries, victim)
			cache.itemCount -= old.count
		}
	}
	return true
}

func (cache *directoryListingCache) touchLocked(token string) {
	for index, current := range cache.order {
		if current == token {
			cache.order = append(cache.order[:index], cache.order[index+1:]...)
			break
		}
	}
	cache.order = append(cache.order, token)
}

func (fp *FileSystemPanel) restoreCachedDirectory(filesystem vfs.VFS, path string, showUpEntry bool) bool {
	if fp == nil || fp.directoryCache == nil {
		return false
	}
	items, ok := fp.directoryCache.get(filesystem, path)
	if !ok {
		return false
	}
	up := vfs.VFSItem{Name: "..", IsDir: true}
	fp.Entries = fp.freshDirectoryEntries(items, showUpEntry, up, filesystem, path)
	fp.catalogRefreshDelta = nil
	fp.mediaSourceEpoch++
	fp.markSemanticCatalogMutation()
	if target := fp.PendingSelection; target != "" {
		for index, entry := range fp.Entries {
			if entry == nil || entry.Name != target {
				continue
			}
			fp.SetCursorIndex(index)
			vtui.DebugLog("[FIX:directory-cache] applied pending selection path=%q target=%q index=%d", path, target, index)
			break
		}
	}
	fp.Refresh()
	vtui.DebugLog("[FIX:directory-cache] hit path=%q entries=%d", path, len(items))
	return true
}

func (fp *FileSystemPanel) storeDirectorySnapshot(filesystem vfs.VFS, path string, items []vfs.VFSItem) {
	if fp == nil || fp.directoryCache == nil || !fp.directoryCache.store(filesystem, path, items) {
		return
	}
	vtui.DebugLog("[FIX:directory-cache] stored path=%q entries=%d", path, len(items))
}
