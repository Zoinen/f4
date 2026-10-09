package panel

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unxed/vtui"
)

// The expanded directories of the tree panel survive a restart of f4 (f4#1602:
// the owner named far2l as the reference, and far2l keeps its tree between
// runs). They are kept in one text file, one absolute path per line, next to
// the other settings; the in-memory treeExpandCache stays the working copy and
// this file is its mirror.
//
// It is off until the application names a file (EnableTreeExpandPersistence),
// so nothing that builds a tree panel without an application around it -- a
// test, a tool -- reads or writes anybody's configuration. Writes are
// coalesced: an expand or collapse (and the replay of a whole cache when the
// tree opens) schedules one write a moment later, which drops the paths that
// no longer exist, so a renamed or deleted directory does not stay in the file
// for ever.

const (
	// treeExpandSaveDelay is how long a change waits for further ones before
	// the file is rewritten.
	treeExpandSaveDelay = 500 * time.Millisecond
	// treeExpandPersistCap bounds the file: a tree that big is not a tree
	// anybody expanded by hand.
	treeExpandPersistCap = 5000
)

var (
	treePersistMu     sync.Mutex
	treePersistPath   string
	treePersistLoaded bool
	treePersistTimer  *time.Timer
)

// EnableTreeExpandPersistence names the file the expanded directories are
// kept in. Calling it again with another path (or "") switches the mirror.
func EnableTreeExpandPersistence(path string) {
	treePersistMu.Lock()
	defer treePersistMu.Unlock()
	treePersistPath = path
	treePersistLoaded = false
}

// treeLoadPersisted merges the file into treeExpandCache, once per
// EnableTreeExpandPersistence.
func treeLoadPersisted() {
	treePersistMu.Lock()
	path := treePersistPath
	if path == "" || treePersistLoaded {
		treePersistMu.Unlock()
		return
	}
	treePersistLoaded = true
	treePersistMu.Unlock()

	f, err := os.Open(path) //nolint:gosec // the application's own settings file
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	var paths []string
	sc := bufio.NewScanner(f)
	for sc.Scan() && len(paths) < treeExpandPersistCap {
		if p := strings.TrimSpace(sc.Text()); p != "" && filepath.IsAbs(p) {
			paths = append(paths, p)
		}
	}
	treeExpandCacheMu.Lock()
	for _, p := range paths {
		treeExpandCache[p] = struct{}{}
	}
	treeExpandCacheMu.Unlock()
}

// treeExpandChanged schedules the mirror to be rewritten.
func treeExpandChanged() {
	treePersistMu.Lock()
	defer treePersistMu.Unlock()
	if treePersistPath == "" {
		return
	}
	if treePersistTimer != nil {
		treePersistTimer.Reset(treeExpandSaveDelay)
		return
	}
	treePersistTimer = time.AfterFunc(treeExpandSaveDelay, saveTreeExpanded)
}

// saveTreeExpanded rewrites the file from the cache, keeping the paths that
// still name a directory.
func saveTreeExpanded() {
	treePersistMu.Lock()
	path := treePersistPath
	treePersistTimer = nil
	treePersistMu.Unlock()
	if path == "" {
		return
	}
	treeExpandCacheMu.Lock()
	paths := make([]string, 0, len(treeExpandCache))
	for p := range treeExpandCache {
		paths = append(paths, p)
	}
	treeExpandCacheMu.Unlock()

	kept := paths[:0]
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			kept = append(kept, p)
		}
	}
	sort.Strings(kept)
	if len(kept) > treeExpandPersistCap {
		kept = kept[:treeExpandPersistCap]
	}
	var b strings.Builder
	for _, p := range kept {
		if strings.ContainsAny(p, "\r\n") {
			continue
		}
		b.WriteString(p)
		b.WriteByte('\n')
	}
	tmp := path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return
	}
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		vtui.DebugLog("TREE: cannot save the expanded directories: %v", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		vtui.DebugLog("TREE: cannot save the expanded directories: %v", err)
	}
}
