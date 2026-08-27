package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"time"

	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// fileEntry implements vtui.TableRow for display in a table.
type fileEntry struct {
	vfs.VFSItem
	Selected       bool
	PrevSelected   bool // snapshot of Selected taken by SaveSelection; swapped in by RestoreSelection (Ctrl+M)
	SizeCalculated bool
	IsCached       bool
}
type mediumRow struct {
	fp *FileSystemPanel
	r  int
}

type panelEntryRow struct {
	fp    *FileSystemPanel
	entry *fileEntry
}

type panelMatchSpan struct {
	start int
	width int
}

func (r *panelEntryRow) GetCellText(col int) string {
	if col == 0 && len(r.fp.table.Columns) > 0 {
		return formatPanelFileName(r.entry, r.fp.table.Columns[0].Width)
	}
	return r.entry.GetCellText(col)
}

func (r *panelEntryRow) IsSelected() bool {
	return r.entry.IsSelected()
}

func (r *panelEntryRow) GetCellAttr(col int, defaultAttr uint64) uint64 {
	return r.entry.GetCellAttr(col, defaultAttr)
}

func (m *mediumRow) GetCellText(col int) string {
	H := m.fp.table.ViewHeight
	if H <= 0 {
		H = 1
	}
	idx := m.r + col*H
	if idx >= len(m.fp.entries) {
		return ""
	}
	e := m.fp.entries[idx]
	width := 0
	if col >= 0 && col < len(m.fp.table.Columns) {
		width = m.fp.table.Columns[col].Width
	}
	return formatPanelFileName(e, width)
}

func (f *fileEntry) displayName(name string) string {
	if f.Name == ".." {
		return ".."
	}
	marker := ""
	if AppConfig.ShowHighlightMarks {
		marker = GlobalFileHighlighter.GetMarker(&f.VFSItem)
	}
	prefix := ""
	if f.IsDir {
		if AppConfig.ShowDirPrefix {
			if marker == "/" {
				marker = ""
			}
			prefix = "/"
		} else {
			if marker == "/" {
				marker = ""
			}
		}
	}
	if marker != "" {
		name = marker + " " + name
	}
	return prefix + name
}

func splitFileExtension(name string) (string, string) {
	lastDot := strings.LastIndex(name, ".")
	if lastDot <= 0 || lastDot == len(name)-1 {
		return name, ""
	}
	return name[:lastDot], name[lastDot+1:]
}

func shouldSeparatePanelExtension(entry *fileEntry) bool {
	return AppConfig.SeparateFileExtensions && !entry.IsDir && !entry.NoExtension && entry.Name != ".."
}

func formatPanelFileName(entry *fileEntry, width int) string {
	if !shouldSeparatePanelExtension(entry) || width <= 0 {
		return entry.displayName(entry.Name)
	}
	base, extension := splitFileExtension(entry.Name)
	if extension == "" {
		return entry.displayName(entry.Name)
	}

	extensionWidth := runewidth.StringWidth(extension)
	extensionFieldWidth := extensionWidth
	if extensionFieldWidth < 3 {
		extensionFieldWidth = 3
	}
	extensionText := extension + strings.Repeat(" ", extensionFieldWidth-extensionWidth)
	if extensionFieldWidth >= width {
		return runewidth.Truncate(extensionText, width, "")
	}
	left := runewidth.Truncate(entry.displayName(base), width-extensionFieldWidth-1, "")
	padding := width - runewidth.StringWidth(left) - extensionFieldWidth
	return left + strings.Repeat(" ", padding) + extensionText
}

func clippedPanelMatchSpan(start, width, cellWidth int) (panelMatchSpan, bool) {
	if start < 0 {
		width += start
		start = 0
	}
	if start >= cellWidth || width <= 0 {
		return panelMatchSpan{}, false
	}
	if start+width > cellWidth {
		width = cellWidth - start
	}
	return panelMatchSpan{start: start, width: width}, width > 0
}

func panelFileNameMatchSpans(entry *fileEntry, width, matchStartRunes, matchedRunes int) []panelMatchSpan {
	if matchStartRunes < 0 || matchedRunes <= 0 || width <= 0 {
		return nil
	}
	nameRunes := []rune(entry.Name)
	if matchStartRunes >= len(nameRunes) {
		return nil
	}
	matchEndRunes := matchStartRunes + matchedRunes
	if matchEndRunes > len(nameRunes) {
		matchEndRunes = len(nameRunes)
	}
	prefixWidth := 0
	if entry.Name != ".." {
		prefixWidth = runewidth.StringWidth(entry.displayName(""))
	}

	if !shouldSeparatePanelExtension(entry) {
		if span, ok := clippedPanelMatchSpan(
			prefixWidth+runewidth.StringWidth(string(nameRunes[:matchStartRunes])),
			runewidth.StringWidth(string(nameRunes[matchStartRunes:matchEndRunes])), width,
		); ok {
			return []panelMatchSpan{span}
		}
		return nil
	}

	base, extension := splitFileExtension(entry.Name)
	if extension == "" {
		if span, ok := clippedPanelMatchSpan(
			prefixWidth+runewidth.StringWidth(string(nameRunes[:matchStartRunes])),
			runewidth.StringWidth(string(nameRunes[matchStartRunes:matchEndRunes])), width,
		); ok {
			return []panelMatchSpan{span}
		}
		return nil
	}

	baseRunes := []rune(base)
	extensionRunes := []rune(extension)
	extensionFieldWidth := runewidth.StringWidth(extension)
	if extensionFieldWidth < 3 {
		extensionFieldWidth = 3
	}

	spans := make([]panelMatchSpan, 0, 2)
	baseMatchStart := matchStartRunes
	if baseMatchStart < 0 {
		baseMatchStart = 0
	}
	baseMatchEnd := matchEndRunes
	if baseMatchEnd > len(baseRunes) {
		baseMatchEnd = len(baseRunes)
	}
	if baseMatchStart < baseMatchEnd && extensionFieldWidth < width {
		leftWidth := width - extensionFieldWidth - 1
		if span, ok := clippedPanelMatchSpan(
			prefixWidth+runewidth.StringWidth(string(baseRunes[:baseMatchStart])),
			runewidth.StringWidth(string(baseRunes[baseMatchStart:baseMatchEnd])), leftWidth,
		); ok {
			spans = append(spans, span)
		}
	}

	// The separating dot is intentionally hidden when extensions are aligned.
	// Continue highlighting at the right-aligned extension after that dot.
	extensionNameStart := len(baseRunes) + 1
	extensionMatchStart := matchStartRunes - extensionNameStart
	if extensionMatchStart < 0 {
		extensionMatchStart = 0
	}
	extensionMatchEnd := matchEndRunes - extensionNameStart
	if extensionMatchEnd > len(extensionRunes) {
		extensionMatchEnd = len(extensionRunes)
	}
	if extensionMatchStart < extensionMatchEnd {
		extensionStart := width - extensionFieldWidth
		if extensionStart < 0 {
			extensionStart = 0
		}
		if span, ok := clippedPanelMatchSpan(
			extensionStart+runewidth.StringWidth(string(extensionRunes[:extensionMatchStart])),
			runewidth.StringWidth(string(extensionRunes[extensionMatchStart:extensionMatchEnd])), width,
		); ok {
			spans = append(spans, span)
		}
	}
	return spans
}

func (m *mediumRow) IsColSelected(col int) bool {
	H := m.fp.table.ViewHeight
	if H <= 0 {
		H = 1
	}
	idx := m.r + col*H
	if idx >= len(m.fp.entries) {
		return false
	}
	return m.fp.entries[idx].Selected
}
func (m *mediumRow) GetCellAttr(col int, defaultAttr uint64) uint64 {
	H := m.fp.table.ViewHeight
	if H <= 0 {
		H = 1
	}
	idx := m.r + col*H
	if idx >= len(m.fp.entries) {
		return defaultAttr
	}
	e := m.fp.entries[idx]
	attr := defaultAttr
	isCursor := (defaultAttr == vtui.Palette[ColPanelCursor] || defaultAttr == vtui.Palette[ColPanelSelectedCursor] || defaultAttr == vtui.Palette[ColPanelInactiveCursor] || defaultAttr == vtui.Palette[ColPanelInactiveSelectedCursor])

	attr = GlobalFileHighlighter.GetColor(&e.VFSItem, attr, e.Selected, isCursor)

	return attr
}

type ViewMode int

const (
	ViewModeMedium ViewMode = iota
	ViewModeDetailed
	ViewModeBrief
	ViewModeWide
)

const (
	panelSizeColumnWidth      = 11
	panelModifiedColumnWidth  = 14
	panelDragScrollInterval   = 75 * time.Millisecond
	panelLoadingPulseInterval = 100 * time.Millisecond
)

var panelLoadingPulse = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// GalleryLayoutMode selects the geometry/delegate strategy used by the
// reusable ZoinGallery renderer. It remains independent from ViewMode: the
// terminal frontend keeps its compact table layouts, while native frontends
// always use this unified renderer.
type GalleryLayoutMode string

const (
	GalleryLayoutMasonry GalleryLayoutMode = "masonry"
	GalleryLayoutColumns GalleryLayoutMode = "columns"
	GalleryLayoutDetails GalleryLayoutMode = "details"
	GalleryLayoutGrid    GalleryLayoutMode = "grid"
	GalleryLayoutIcons   GalleryLayoutMode = "icons"
)

const (
	defaultGalleryColumnCount = 2
	minGalleryColumnCount     = 2
	maxGalleryColumnCount     = 3
)

var galleryLayoutModes = [...]GalleryLayoutMode{
	GalleryLayoutMasonry,
	GalleryLayoutColumns,
	GalleryLayoutDetails,
	GalleryLayoutGrid,
	GalleryLayoutIcons,
}

func parseGalleryLayoutMode(value string) (GalleryLayoutMode, bool) {
	mode := GalleryLayoutMode(strings.ToLower(strings.TrimSpace(value)))
	for _, candidate := range galleryLayoutModes {
		if mode == candidate {
			return mode, true
		}
	}
	return GalleryLayoutMasonry, false
}

func galleryDensityLimits(mode GalleryLayoutMode) (defaultValue, minimum, maximum int) {
	switch mode {
	case GalleryLayoutColumns, GalleryLayoutDetails:
		// Zero asks the QML host to derive the untouched default from its font.
		return 0, 22, 72
	case GalleryLayoutGrid:
		return 160, 96, 320
	case GalleryLayoutIcons:
		return 64, 18, 256
	default:
		return 150, 30, 500
	}
}

func clampGalleryDensity(mode GalleryLayoutMode, density int) int {
	_, minimum, maximum := galleryDensityLimits(mode)
	if density < minimum {
		return minimum
	}
	if density > maximum {
		return maximum
	}
	return density
}

type SortMode int

const (
	SortName SortMode = iota
	SortExt
	SortTime
	SortSize
	SortUnsorted
)

func (f *fileEntry) IsSelected() bool {
	return f.Selected
}

func (f *fileEntry) GetCellText(col int) string {
	switch col {
	case 0:
		return f.displayName(f.Name)
	case 1:
		if f.IsDir {
			if f.SizeCalculated {
				return formatIntWithSpaces(f.Size)
			}
			if f.Name == ".." {
				return Msg("Panel.UpDir")
			}
			return ""
		}
		return formatIntWithSpaces(f.Size)
	case 2:
		if f.MTime.IsZero() {
			return ""
		}
		return f.MTime.Format("02.01.06 15:04")
	}
	return ""
}
func (f *fileEntry) GetCellAttr(col int, defaultAttr uint64) uint64 {
	attr := defaultAttr
	isCursor := (defaultAttr == vtui.Palette[ColPanelCursor] || defaultAttr == vtui.Palette[ColPanelSelectedCursor] || defaultAttr == vtui.Palette[ColPanelInactiveCursor] || defaultAttr == vtui.Palette[ColPanelInactiveSelectedCursor])

	attr = GlobalFileHighlighter.GetColor(&f.VFSItem, attr, f.Selected, isCursor)

	return attr
}

// FileSystemPanel is a panel displaying files on disk.
const maxDirCache = 50

type dirCacheEntry struct {
	items       []vfs.VFSItem
	time        time.Time
	showUpEntry bool
	// upItem preserves the parent-directory metadata used by the synthetic
	// ".." row. Without it, a cache hit publishes a zero-time row and the
	// completed refresh immediately replaces it with the parent's real MTime,
	// making an otherwise identical semantic catalog look changed.
	upItem vfs.VFSItem
}

// dirCacheKey qualifies a path with the filesystem it belongs to. Remote
// roots are commonly all named "/"; using the path alone briefly showed one
// Android device's cached directory while another device was being loaded.
// A shared-session identity preserves the fast preview across pooled FISH+
// views. Other comparable VFS implementations are scoped to their exact
// instance, giving a safe cache miss rather than a false hit after reopening.
type dirCacheKey struct {
	identity      any
	qualifiedPath string
}

type directoryLoadRequest struct {
	load       func()
	benchmark  *navigationBenchmarkTrace
	enqueuedNs int64
}

type FileSystemPanel struct {
	vtui.ScreenObject
	table                 *vtui.Table
	scrollBar             *vtui.ScrollBar
	scrollMouseActive     bool
	minimalScrollDragGap  int
	headerMouseActive     bool
	frame                 *vtui.BorderedFrame
	vfs                   vfs.VFS
	entries               []*fileEntry
	selectedItems         map[string]bool
	previousSelection     map[string]bool
	previousSelectionVFS  vfs.VFS
	previousSelectionPath string
	selectionEpoch        map[string]uint64
	selectionEpochNext    uint64
	directoryEpoch        uint64
	viewMode              ViewMode
	wide                  bool
	cursorIdx             int
	lastRightClickedIdx   int
	rightDragActive       bool
	rightDragSelect       bool
	semanticRightIndex    int
	semanticRightState    bool
	semanticPriorIndex    int
	semanticPriorState    bool
	rowDragButton         uint32
	dragScrollDirection   int
	dragScrollTimer       *time.Timer
	dragScrollGeneration  uint64
	galleryLayoutMode     GalleryLayoutMode
	galleryColumnCount    int
	galleryDensities      map[GalleryLayoutMode]int
	galleryLayoutRevision int64

	loadCtx    context.Context
	cancelLoad context.CancelFunc
	// loadGeneration binds every asynchronous base/metadata callback to the
	// exact ReadDirectory request that created it. Context cancellation usually
	// rejects stale work first; the generation also covers providers which
	// finish a callback concurrently with cancellation.
	loadGeneration uint64
	isLoading      bool
	// catalogProvisional marks the synthetic cold-load placeholder (normally
	// just ".."). Native frontends keep the destination hidden until the first
	// authoritative catalog arrives instead of briefly painting an empty list.
	catalogProvisional bool
	// semanticCachedCatalogReady distinguishes a complete cache replacement
	// from a cold/phased base while ReadDir is still running. The cached rows are
	// already interactive and have a coherent metadata snapshot, so a capable
	// native frontend need not remain in its loading gate during revalidation.
	semanticCachedCatalogReady bool
	// semanticCachedCatalogExported proves that the complete cached model was
	// constructed for the negotiated frontend. Completion may omit its own
	// redraw only after this transition; otherwise the cache redraw might still
	// be pending behind an exceptionally fast ReadDir.
	semanticCachedCatalogExported bool
	loadingTimer                  *time.Timer
	loadingFrame                  int
	loadingGeneration             uint64
	loadQueueMu                   sync.Mutex
	loadWorkerActive              bool
	pendingDirectoryLoad          *directoryLoadRequest
	benchmarkLoadTrace            *navigationBenchmarkTrace
	providerOpenTask              *vtui.TaskContext
	providerOpenDialog            *vtui.Window
	directoryErrorDialog          *vtui.Window
	providerOpenTarget            string
	providerOpenSourceSelect      string
	providerOpenResult            func(bool) bool
	pendingSelection              string
	providerEntryName             string // name of entry used to enter a provider VFS (e.g. NetFox connection name)
	suppressFolderHistoryPath     string // one-shot: history/menu navigation must not reorder MRU
	suppressFolderHistoryToken    uint64 // binds suppression to one specific asynchronous directory load
	fastFindMode                  bool
	fastFindStr                   string
	fastFindMatcherKey            string
	fastFindMatchers              []*vtui.FuzzyMatcher
	showInactiveCursor            bool

	sortMode    SortMode
	sortReverse bool

	lastDirMTime time.Time
	dirCache     map[dirCacheKey]dirCacheEntry

	isCheckingRefresh bool
	currentTitle      string
	// semanticTitle is the stable, presentation-neutral title exported to
	// native frontends. currentTitle may additionally carry the one-cell TUI
	// loading pulse; keeping that decoration out of the semantic model lets
	// each frontend apply its own latency policy without timer-only scenes.
	semanticTitle string

	// lastLoadedPath is the path readDirectoryEx last saw; used to
	// detect a directory switch so selectedItems can be dropped
	// (selection is per-directory, matches far/far2l).
	lastLoadedPath string

	// shiftSessionActive / shiftSessionMode implement FAR-style
	// Shift+nav selection. The mode (select vs deselect) is
	// decided on the first Shift+nav from the state of the row
	// under the cursor and held until Shift is released, so all
	// following Shift+nav keys in the same "session" apply that
	// same mode. Any event other than a Shift+nav key closes
	// the session — the next Shift+nav starts a new one.
	shiftSessionActive bool
	shiftSessionMode   bool // true = select, false = deselect

	// Semantic revisions are derived from deterministic fingerprints when a
	// scene is exported. This catches every mutation path, including async VFS
	// chunks and tests/plugins which replace entries directly.
	semanticCatalogFingerprint   uint64
	semanticMetadataFingerprint  uint64
	semanticSelectionFingerprint uint64
	semanticCatalogInitialized   bool
	semanticMetadataInitialized  bool
	semanticSelectionInitialized bool
	catalogRevision              int64
	metadataRevision             int64
	selectionRevision            int64
	// Selection changes are journaled independently from the immutable file
	// catalog. Native semantic renderers can therefore acknowledge one changed
	// row without exporting or serializing every directory entry.
	semanticSelectionBaseRevision int64
	semanticSelectionChanges      map[string]semanticSelectionChange
	semanticSelectionOverflow     bool
	semanticSelectionNeedsSync    bool
	semanticStaticCache           *semanticPanelStaticCache
	semanticMetadataSnapshot      *semanticPanelMetadataSnapshot
}

var DisableLoadingAnimationInTests = true

func NewFileSystemPanel(x, y, w, h int, vfs vfs.VFS) *FileSystemPanel {
	path := vfs.GetPath()

	fp := &FileSystemPanel{
		vfs:                   vfs,
		frame:                 vtui.NewBorderedFrame(x, y, x+w-1, y+h-1, vtui.SingleBox, path),
		table:                 vtui.NewTable(x+1, y+1, w-2, h-2, nil),
		viewMode:              ViewModeMedium,
		galleryLayoutMode:     GalleryLayoutMasonry,
		galleryColumnCount:    defaultGalleryColumnCount,
		galleryDensities:      make(map[GalleryLayoutMode]int),
		galleryLayoutRevision: 1,
		lastRightClickedIdx:   -1,
		semanticRightIndex:    -1,
		semanticPriorIndex:    -1,
		dirCache:              make(map[dirCacheKey]dirCacheEntry),
		selectedItems:         make(map[string]bool),
		selectionEpoch:        make(map[string]uint64),
		//entries:             []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}},
	}
	fp.frame.ColorBoxIdx = ColPanelBox
	fp.frame.ColorTitleIdx = ColPanelTitle
	fp.table.ColorTextIdx = ColPanelText
	fp.table.ColorSelectedTextIdx = ColPanelCursor
	fp.table.ColorItemSelectTextIdx = ColPanelSelectedText
	fp.table.ColorItemSelectCursorIdx = ColPanelSelectedCursor
	fp.table.ColorTitleIdx = ColPanelColumnTitle
	fp.table.ColorBoxIdx = ColPanelBox
	fp.table.ShowScrollBar = false
	fp.initScrollBar()
	fp.SetCanFocus(true)
	fp.SetPosition(x, y, x+w-1, y+h-1)
	fp.SetViewMode(ViewModeMedium)
	fp.ReadDirectory()
	return fp
}

func directoryCacheKey(fs vfs.VFS, path string) dirCacheKey {
	key := dirCacheKey{qualifiedPath: FileStateKey(fs, path)}
	if stable, ok := fs.(vfs.DirectoryCacheIdentity); ok {
		if cacheKey := stable.DirectoryCacheKey(); cacheKey != nil {
			if cacheType := reflect.TypeOf(cacheKey); cacheType != nil && cacheType.Comparable() {
				key.identity = cacheKey
				return key
			}
		}
	}
	if identity, ok := fs.(vfs.SessionIdentity); ok {
		if sessionKey := identity.SessionKey(); sessionKey != nil {
			if sessionType := reflect.TypeOf(sessionKey); sessionType != nil && sessionType.Comparable() {
				key.identity = sessionKey
				return key
			}
		}
	}
	// VFS is an interface and implementations are not required to be
	// comparable. Every built-in VFS is pointer-backed, but keep the fallback
	// safe for plugins that use a slice/map-bearing value implementation.
	if fs != nil && reflect.TypeOf(fs).Comparable() {
		key.identity = fs
	}
	return key
}

// sameVFSInstance is deliberately stricter than cache identity. Two pooled
// remote views may share cached directory data, but an asynchronous provider
// transition belongs to the exact parent object it was started from.
func sameVFSInstance(a, b vfs.VFS) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	if ta != tb || !ta.Comparable() {
		return false
	}
	return a == b
}

func phasedDirectoryReaderFor(filesystem vfs.VFS) vfs.PhasedDirectoryReader {
	provider, ok := filesystem.(vfs.PhasedDirectoryReadProvider)
	if !ok {
		return nil
	}
	reader := provider.PhasedDirectoryReader()
	readerVFS, ok := reader.(vfs.VFS)
	if !ok || !sameVFSInstance(readerVFS, filesystem) {
		// A promoted OSVFS capability belongs to the embedded OSVFS, not to a
		// wrapper which may override ReadDir. Only an explicit self capability
		// is safe to use.
		return nil
	}
	return reader
}

func panelSortSupportsPhasedDirectoryRead(mode SortMode) bool {
	return mode != SortSize && mode != SortTime
}

func (fp *FileSystemPanel) cacheKey(path string) dirCacheKey {
	return directoryCacheKey(fp.vfs, path)
}

func (fp *FileSystemPanel) saveToCache(path string, items []vfs.VFSItem) {
	showUpEntry := fp.vfs != nil && (!fp.vfs.IsAtRoot() || fp.vfs.ParentVFS() != nil)
	fp.saveToCacheKey(fp.cacheKey(path), items, showUpEntry)
}

func (fp *FileSystemPanel) saveToCacheKey(key dirCacheKey, items []vfs.VFSItem, showUpEntry bool) {
	fp.saveToCacheKeyWithUpItem(key, items, showUpEntry, vfs.VFSItem{})
}

func (fp *FileSystemPanel) saveToCacheKeyWithUpItem(key dirCacheKey, items []vfs.VFSItem, showUpEntry bool, upItem vfs.VFSItem) {
	if fp.dirCache == nil {
		fp.dirCache = make(map[dirCacheKey]dirCacheEntry)
	}
	if showUpEntry {
		upItem = panelUpEntryItem(upItem, true)
	} else {
		upItem = vfs.VFSItem{}
	}
	fp.dirCache[key] = dirCacheEntry{
		items:       items,
		time:        time.Now(),
		showUpEntry: showUpEntry,
		upItem:      upItem,
	}

	if len(fp.dirCache) > maxDirCache {
		var oldestKey dirCacheKey
		var oldestTime time.Time
		hasOldest := false
		for candidate, entry := range fp.dirCache {
			if !hasOldest || entry.time.Before(oldestTime) {
				oldestKey = candidate
				oldestTime = entry.time
				hasOldest = true
			}
		}
		delete(fp.dirCache, oldestKey)
	}
}

func panelUpEntryItem(stat vfs.VFSItem, hasStat bool) vfs.VFSItem {
	item := vfs.VFSItem{Name: "..", IsDir: true}
	if !hasStat {
		return item
	}
	item.MTime = stat.MTime
	item.ATime = stat.ATime
	item.CTime = stat.CTime
	item.UnixMode = stat.UnixMode
	item.Uid = stat.Uid
	item.Gid = stat.Gid
	return item
}

func (fp *FileSystemPanel) freshDirectoryEntries(items []vfs.VFSItem, showUpEntry bool,
	upItem vfs.VFSItem, filesystem vfs.VFS, path string,
) []*fileEntry {
	entries := make([]*fileEntry, 0, len(items)+1)
	if showUpEntry {
		entries = append(entries, &fileEntry{VFSItem: upItem})
	}
	for _, item := range items {
		if !AppConfig.ShowHiddenFiles && item.Name != ".." && item.IsHidden {
			continue
		}
		entry := &fileEntry{VFSItem: item}
		fp.applyPersistentSelection(entry, filesystem, path)
		entries = append(entries, entry)
	}
	fp.sortEntrySlice(entries)
	return entries
}

// refreshEquivalentCachedEntries updates the authoritative data behind live
// cached rows without replacing either fileEntry or vtui.TableRow objects.
// Selection, Ctrl+M snapshots, cursor and viewport therefore remain exactly as
// the user left them while ReadDir was running.
func (fp *FileSystemPanel) refreshEquivalentCachedEntries(fresh []*fileEntry) bool {
	if !fp.semanticDeferredEntriesEquivalent(fresh) {
		return false
	}
	for index, source := range fresh {
		live := fp.entries[index]
		live.VFSItem = source.VFSItem
		live.SizeCalculated = source.SizeCalculated
		live.IsCached = false
	}

	// The helper proved these values equal before the in-place refresh. Keep
	// initialized revision caches explicitly aligned with the live entries; no
	// revision or semantic snapshot invalidation is necessary because none of
	// the negotiated client's observable fields changed.
	catalog, metadata, selection := fp.semanticFingerprintsForEntries(fp.entries)
	if fp.semanticCatalogInitialized {
		fp.semanticCatalogFingerprint = catalog
	}
	if fp.semanticMetadataInitialized {
		fp.semanticMetadataFingerprint = metadata
	}
	if fp.semanticSelectionInitialized {
		fp.semanticSelectionFingerprint = selection
	}
	return true
}

func nativeVisualCachePath(value string) string {
	if os.PathSeparator == '\\' {
		return strings.ReplaceAll(value, "/", "\\")
	}
	return strings.ReplaceAll(value, "\\", "/")
}

// showCachedStandalonePath renders a previously visited provider directory
// before the provider reconnect/restore task has completed. The old VFS stays
// installed until that task succeeds, so these rows are presentation-only;
// the panel's provider-open guard prevents actions from being dispatched
// against the old filesystem meanwhile.
func (fp *FileSystemPanel) showCachedStandalonePath(target string) bool {
	if fp == nil || target == "" || fp.dirCache == nil || AppConfig.SyncPanelLoad {
		return false
	}
	want := nativeVisualCachePath(target)
	var cached dirCacheEntry
	found := false
	for key, candidate := range fp.dirCache {
		if nativeVisualCachePath(key.qualifiedPath) != want {
			continue
		}
		if !found || candidate.time.After(cached.time) {
			cached = candidate
			found = true
		}
	}
	if !found {
		return false
	}

	fp.entries = nil
	if cached.showUpEntry {
		fp.entries = append(fp.entries, &fileEntry{
			VFSItem:  panelUpEntryItem(cached.upItem, true),
			IsCached: true,
		})
	}
	for _, item := range cached.items {
		if !AppConfig.ShowHiddenFiles && item.Name != ".." && item.IsHidden {
			continue
		}
		fp.entries = append(fp.entries, &fileEntry{VFSItem: item, IsCached: true})
	}
	fp.sortEntries()
	fp.SetCursorIndex(0)
	return true
}
func (fp *FileSystemPanel) SetItemSelected(idx int, state bool) {
	if idx >= 0 && idx < len(fp.entries) {
		e := fp.entries[idx]
		if e.Name != ".." {
			if e.Selected == state {
				return
			}
			e.Selected = state
			if fp.selectedItems == nil {
				fp.selectedItems = make(map[string]bool)
			}
			if fp.selectionEpoch == nil {
				fp.selectionEpoch = make(map[string]uint64)
			}
			fp.selectionEpochNext++
			fp.selectionEpoch[e.Name] = fp.selectionEpochNext
			if state {
				fp.selectedItems[e.Name] = true
			} else {
				delete(fp.selectedItems, e.Name)
			}
			if fp.semanticSelectionChanges == nil && !fp.semanticSelectionOverflow {
				fp.semanticSelectionBaseRevision = fp.selectionRevision
				fp.semanticSelectionChanges = make(map[string]semanticSelectionChange)
			}
			fp.selectionRevision++
			fp.semanticSelectionNeedsSync = true
			entryID := ""
			if cache := fp.semanticStaticCache; cache != nil &&
				cache.catalogRevision == fp.catalogRevision && idx < len(cache.entries) {
				entryID = cache.entries[idx].EntryID
			}
			if entryID == "" {
				if fp.vfs != nil {
					sourceKind, _ := fp.semanticSourceInfo()
					entryID, _ = fp.semanticEntryMetadata(e, sourceKind)
				} else {
					// Lightweight panel tests and embedders may construct rows
					// before attaching a VFS. The ID is only journal-local until a
					// real catalog cache exists, but must still avoid a nil deref.
					entryID = fmt.Sprintf("entry:unbound:%d:%s", idx, e.Name)
				}
			}
			if len(fp.semanticSelectionChanges) >= maxSemanticSelectionChanges {
				fp.semanticSelectionOverflow = true
				fp.semanticSelectionChanges = nil
			} else if !fp.semanticSelectionOverflow && entryID != "" {
				fp.semanticSelectionChanges[entryID] = semanticSelectionChange{
					Index: idx, EntryID: entryID, Selected: state,
				}
			}
		}
	}
}

func (fp *FileSystemPanel) ClearSelection() {
	if fp == nil {
		return
	}
	for index := range fp.entries {
		fp.SetItemSelected(index, false)
	}
	// Preserve the old operation's cleanup guarantee even if selectedItems
	// contained a stale filename which no longer has a backing row.
	fp.selectedItems = make(map[string]bool)
}

func (fp *FileSystemPanel) previousSelectionMatches(filesystem vfs.VFS, path string) bool {
	return fp != nil && fp.previousSelectionVFS != nil && filesystem != nil &&
		sameVFSInstance(fp.previousSelectionVFS, filesystem) && fp.previousSelectionPath == path
}

func (fp *FileSystemPanel) clearPreviousSelection() {
	if fp == nil {
		return
	}
	fp.previousSelection = nil
	fp.previousSelectionVFS = nil
	fp.previousSelectionPath = ""
	for _, entry := range fp.entries {
		entry.PrevSelected = false
	}
}

func (fp *FileSystemPanel) applyPersistentSelection(entry *fileEntry, filesystem vfs.VFS, path string) {
	if fp == nil || entry == nil || entry.Name == ".." {
		return
	}
	entry.Selected = fp.selectedItems[entry.Name]
	entry.PrevSelected = fp.previousSelectionMatches(filesystem, path) && fp.previousSelection[entry.Name]
}

func (fp *FileSystemPanel) ToggleSelection(idx int) {
	if idx >= 0 && idx < len(fp.entries) {
		e := fp.entries[idx]
		if e.Name != ".." {
			fp.SetItemSelected(idx, !e.Selected)
		}
	}
}
func (fp *FileSystemPanel) SetFocus(f bool) {
	fp.ScreenObject.SetFocus(f)
	if !f && fp.fastFindMode {
		fp.fastFindMode = false
		fp.fastFindStr = ""
	}
}
func (fp *FileSystemPanel) SetSortMode(mode SortMode) {
	if fp.sortMode == mode {
		fp.sortReverse = !fp.sortReverse
	} else {
		fp.sortMode = mode
		// Every mode's base comparator is its desired first-use direction:
		// name/extension ascend, while time/size descend. Repeated activation
		// below toggles that direction uniformly for hotkeys, menus and headers.
		fp.sortReverse = false
	}
	fp.updateSortColumnTitles()
	fp.ReadDirectory()
}

func (fp *FileSystemPanel) sortEntries() {
	fp.sortEntrySlice(fp.entries)
}

// sortEntrySlice applies the panel's current ordering to a detached candidate
// catalog. Warm-cache completion uses it to prove the fresh result has exactly
// the same semantic order before touching the live table rows.
func (fp *FileSystemPanel) sortEntrySlice(entries []*fileEntry) {
	if fp.sortMode == SortUnsorted || len(entries) <= 1 {
		return
	}

	// Keep the comparison strict and deterministic. The old reverse branch
	// returned !less, which also returned true for equal values. That violates
	// sort.Interface's ordering contract and could reshuffle an otherwise
	// unchanged catalog on refresh, invalidating Gallery revisions and cursor
	// identities spuriously.
	sort.SliceStable(entries, func(i, j int) bool {
		ei, ej := entries[i], entries[j]

		// ".." всегда сверху. Keep the comparison strict even
		// for malformed/virtual catalogs containing more than one parent row.
		if ei.Name == ".." || ej.Name == ".." {
			return ei.Name == ".." && ej.Name != ".."
		}

		// Папки всегда сверху
		if ei.IsDir != ej.IsDir {
			return ei.IsDir
		}

		compareNames := func(left, right string) int {
			leftFolded := strings.ToLower(left)
			rightFolded := strings.ToLower(right)
			if leftFolded < rightFolded {
				return -1
			}
			if leftFolded > rightFolded {
				return 1
			}
			if left < right {
				return -1
			}
			if left > right {
				return 1
			}
			return 0
		}

		cmp := 0
		switch fp.sortMode {
		case SortName:
			cmp = compareNames(ei.Name, ej.Name)
		case SortExt:
			extI := strings.ToLower(filepath.Ext(ei.Name))
			extJ := strings.ToLower(filepath.Ext(ej.Name))
			if extI < extJ {
				cmp = -1
			} else if extI > extJ {
				cmp = 1
			} else {
				cmp = compareNames(ei.Name, ej.Name)
			}
		case SortTime:
			if ei.MTime.Before(ej.MTime) {
				cmp = -1
			} else if ei.MTime.After(ej.MTime) {
				cmp = 1
			} else {
				cmp = compareNames(ei.Name, ej.Name)
			}
		case SortSize:
			if ei.Size < ej.Size {
				cmp = -1
			} else if ei.Size > ej.Size {
				cmp = 1
			} else {
				cmp = compareNames(ei.Name, ej.Name)
			}
		default:
			cmp = compareNames(ei.Name, ej.Name)
		}

		if fp.sortReverse {
			cmp = -cmp
		}
		return cmp < 0
	})
}

func (fp *FileSystemPanel) SetViewMode(mode ViewMode) {
	if mode == ViewModeWide {
		fp.SetWide(true)
		return
	}
	fp.viewMode = mode
	fp.wide = false
	fp.configureCellSelection()
	fp.Resize(fp.X2-fp.X1+1, fp.Y2-fp.Y1+1)
}

// mouseEntryIndex returns the entry under the mouse. Multi-column panel modes
// are filled from top to bottom and then from left to right, so the visual
// column contributes a full table height to the entry index.
func (fp *FileSystemPanel) mouseEntryIndex(mouseX, mouseY int) int {
	if mouseX < fp.table.X1 || mouseX > fp.table.X2 {
		return -1
	}

	row := mouseY - (fp.table.Y1 + fp.table.MarginTop)
	if row < 0 || row >= fp.table.ViewHeight {
		return -1
	}

	column := 0
	if fp.gridColumnCount() > 1 {
		column = -1
		columnX := fp.table.X1
		for i, tableColumn := range fp.table.Columns {
			if mouseX >= columnX && mouseX < columnX+tableColumn.Width {
				column = i
				break
			}
			columnX += tableColumn.Width + 1 // one-character separator
		}
		if column < 0 {
			return -1
		}
	}

	idx := fp.table.TopPos + row + column*fp.table.ViewHeight
	if idx < 0 || idx >= len(fp.entries) {
		return -1
	}
	return idx
}

func (fp *FileSystemPanel) processRightDrag(idx int) {
	if !fp.rightDragActive {
		fp.rightDragActive = true
		fp.rightDragSelect = !fp.entries[idx].Selected
		fp.lastRightClickedIdx = idx
		fp.SetItemSelected(idx, fp.rightDragSelect)
		return
	}

	from := fp.lastRightClickedIdx
	step := 1
	if idx < from {
		step = -1
	}
	for current := from; ; current += step {
		fp.SetItemSelected(current, fp.rightDragSelect)
		if current == idx {
			break
		}
	}
	fp.lastRightClickedIdx = idx
}

func (fp *FileSystemPanel) semanticPointerRelease() {
	fp.lastRightClickedIdx = -1
	fp.rightDragActive = false
	fp.rowDragButton = 0
	fp.stopDragAutoScroll()
}

func (fp *FileSystemPanel) semanticRightPointerDown(idx int) bool {
	if idx < 0 || idx >= len(fp.entries) {
		return false
	}
	fp.semanticPriorIndex = fp.semanticRightIndex
	fp.semanticPriorState = fp.semanticRightState
	fp.SetCursorIndex(idx)
	if fp.entries[idx].Name == ".." {
		fp.semanticRightIndex = idx
		fp.semanticRightState = false
		return true
	}
	fp.rowDragButton = vtinput.RightmostButtonPressed
	fp.processRightDrag(idx)
	fp.semanticRightIndex = idx
	fp.semanticRightState = fp.entries[idx].Selected
	fp.Refresh()
	return true
}

func (fp *FileSystemPanel) semanticRightPointerMove(idx int) bool {
	if !fp.rightDragActive || idx < 0 || idx >= len(fp.entries) {
		return false
	}
	fp.SetCursorIndex(idx)
	if fp.entries[idx].Name != ".." {
		fp.processRightDrag(idx)
		fp.Refresh()
	}
	return true
}

func (fp *FileSystemPanel) semanticRightPointerDoubleClick(idx int) bool {
	if idx < 0 || idx >= len(fp.entries) {
		return false
	}
	fp.SetCursorIndex(idx)
	if fp.entries[idx].Name == ".." {
		return true
	}
	state := fp.semanticRightState
	if fp.semanticPriorIndex == idx {
		state = fp.semanticPriorState
	}
	fp.setAllItemsSelected(state)
	fp.rightDragActive = true
	fp.rightDragSelect = state
	fp.lastRightClickedIdx = idx
	fp.semanticRightIndex = idx
	fp.semanticRightState = state
	fp.Refresh()
	return true
}

func (fp *FileSystemPanel) stopDragAutoScroll() {
	fp.dragScrollDirection = 0
	fp.dragScrollGeneration++
	if fp.dragScrollTimer != nil {
		fp.dragScrollTimer.Stop()
		fp.dragScrollTimer = nil
	}
}

func (fp *FileSystemPanel) dragAutoScrollStep(direction int) bool {
	oldTop := fp.table.TopPos
	fp.setPanelScrollTop(oldTop + direction)
	if fp.table.TopPos == oldTop {
		return false
	}

	if fp.rightDragActive {
		fp.processRightDrag(fp.GetCursorIndex())
		fp.Refresh()
		vtui.FrameManager.Redraw()
	}
	return true
}

func (fp *FileSystemPanel) scheduleDragAutoScroll(generation uint64) {
	fp.dragScrollTimer = time.AfterFunc(panelDragScrollInterval, func() {
		vtui.FrameManager.PostTask(func() {
			if generation != fp.dragScrollGeneration || fp.dragScrollDirection == 0 {
				return
			}
			if !fp.dragAutoScrollStep(fp.dragScrollDirection) {
				fp.stopDragAutoScroll()
				return
			}
			fp.scheduleDragAutoScroll(generation)
		})
	})
}

func (fp *FileSystemPanel) updateDragAutoScroll(mouseY int) bool {
	direction := 0
	contentTop := fp.table.Y1 + fp.table.MarginTop
	contentBottom := contentTop + fp.table.ViewHeight - 1
	if mouseY < contentTop {
		direction = -1
	} else if mouseY > contentBottom {
		direction = 1
	}

	if direction == 0 {
		fp.stopDragAutoScroll()
		return false
	}
	if direction == fp.dragScrollDirection && fp.dragScrollTimer != nil {
		return true
	}

	fp.stopDragAutoScroll()
	fp.dragScrollDirection = direction
	fp.dragScrollGeneration++
	generation := fp.dragScrollGeneration
	if !fp.dragAutoScrollStep(direction) {
		fp.stopDragAutoScroll()
		return true
	}
	fp.scheduleDragAutoScroll(generation)
	return true
}

func (fp *FileSystemPanel) setAllItemsSelected(state bool) {
	for idx := range fp.entries {
		fp.SetItemSelected(idx, state)
	}
}

func (fp *FileSystemPanel) SetWide(wide bool) {
	fp.wide = wide
	fp.configureCellSelection()
	fp.Resize(fp.X2-fp.X1+1, fp.Y2-fp.Y1+1)
}

func (fp *FileSystemPanel) effectiveViewMode() ViewMode {
	if fp.wide {
		return ViewModeWide
	}
	return fp.viewMode
}

func (fp *FileSystemPanel) gridColumnCount() int {
	switch fp.effectiveViewMode() {
	case ViewModeBrief:
		return 3
	case ViewModeMedium:
		return 2
	default:
		return 1
	}
}

func (fp *FileSystemPanel) columnSortMode(column int) (SortMode, bool) {
	if column < 0 || column >= len(fp.table.Columns) {
		return SortUnsorted, false
	}
	switch fp.effectiveViewMode() {
	case ViewModeWide:
		switch column {
		case 0:
			return SortName, true
		case 1:
			return SortSize, true
		case 2:
			return SortTime, true
		}
	case ViewModeDetailed:
		if column == 0 {
			return SortName, true
		}
		if column == 1 {
			return SortSize, true
		}
	default:
		return SortName, true
	}
	return SortUnsorted, false
}

func (fp *FileSystemPanel) sortIsAscending() bool {
	switch fp.sortMode {
	case SortTime, SortSize:
		// Their base comparators are newest/largest first.
		return fp.sortReverse
	default:
		return !fp.sortReverse
	}
}

func sortModeTitle(mode SortMode) string {
	switch mode {
	case SortName:
		return Msg("Menu.SortName")
	case SortExt:
		return Msg("Menu.SortExt")
	case SortTime:
		return Msg("Menu.SortTime")
	case SortSize:
		return Msg("Menu.SortSize")
	}
	return ""
}

func composePanelColumnTitle(left, right string, width int) string {
	if right == "" || width <= 0 {
		return left
	}
	right = runewidth.Truncate(right, width, "")
	rightWidth := runewidth.StringWidth(right)
	if rightWidth >= width {
		return right
	}
	left = runewidth.Truncate(left, width-rightWidth-1, "")
	padding := width - runewidth.StringWidth(left) - rightWidth
	return left + strings.Repeat(" ", padding) + right
}

func hiddenSortColumnTitle(mode SortMode, ascending bool, width int) string {
	arrow := "↓"
	if ascending {
		arrow = "↑"
	}
	// Preserve brackets and the direction arrow on narrow Brief columns;
	// truncate only the localized sort name when the full label cannot fit.
	labelWidth := width - runewidth.StringWidth("[]"+arrow)
	if labelWidth <= 0 {
		return runewidth.Truncate(arrow, width, "")
	}
	label := runewidth.Truncate(sortModeTitle(mode), labelWidth, "")
	return "[" + label + "]" + arrow
}

func (fp *FileSystemPanel) updateSortColumnTitles() {
	visibleSortColumn := false
	for column := range fp.table.Columns {
		title := Msg("Panel.Column.Name")
		switch fp.effectiveViewMode() {
		case ViewModeWide:
			if column == 1 {
				title = Msg("Panel.Column.Size")
			} else if column == 2 {
				title = Msg("Panel.Column.Modified")
			}
		case ViewModeDetailed:
			if column == 1 {
				title = Msg("Panel.Column.Size")
			}
		}

		mode, sortable := fp.columnSortMode(column)
		if sortable && fp.sortMode != SortUnsorted && fp.sortMode == mode {
			arrow := " ↓"
			if fp.sortIsAscending() {
				arrow = " ↑"
			}
			title += arrow
			visibleSortColumn = true
		}
		fp.table.Columns[column].Title = title
	}

	if fp.sortMode != SortUnsorted && !visibleSortColumn && len(fp.table.Columns) > 0 {
		right := hiddenSortColumnTitle(
			fp.sortMode, fp.sortIsAscending(), fp.table.Columns[0].Width)
		fp.table.Columns[0].Title = composePanelColumnTitle(
			Msg("Panel.Column.Name"), right, fp.table.Columns[0].Width)
	}
}

func (fp *FileSystemPanel) headerSortModeAt(x, y int) (SortMode, bool) {
	if !fp.table.ShowHeader || y != fp.table.Y1 || x < fp.table.X1 || x > fp.table.X2 {
		return SortUnsorted, false
	}
	columnX := fp.table.X1
	for column, tableColumn := range fp.table.Columns {
		if x >= columnX && x < columnX+tableColumn.Width {
			return fp.columnSortMode(column)
		}
		columnX += tableColumn.Width
		if column < len(fp.table.Columns)-1 {
			// The separator itself is not a sortable column header.
			if x == columnX {
				return SortUnsorted, false
			}
			columnX++
		}
	}
	return SortUnsorted, false
}

// panelScrollMetrics maps the panel's item-based scrolling onto the
// row-based coordinates expected by vtui.ScrollBar. Multi-column modes show
// two or three times as many entries in the same vertical space, so using the
// table's raw ItemCount would produce an oversized scroll range and a thumb
// that is much too small.
func (fp *FileSystemPanel) panelScrollMetrics() (height, visibleItems, maxTop, virtualMax, virtualValue int) {
	height = fp.table.ViewHeight
	if height <= 0 {
		return
	}

	columns := fp.gridColumnCount()
	visibleItems = height * columns
	maxTop = len(fp.entries) - visibleItems
	if maxTop <= 0 {
		maxTop = 0
		return
	}

	virtualRows := (len(fp.entries) + columns - 1) / columns
	virtualMax = virtualRows - height
	if virtualMax <= 0 {
		virtualMax = 1
	}

	top := fp.table.TopPos
	if top < 0 {
		top = 0
	}
	if top > maxTop {
		top = maxTop
	}
	virtualValue = (top*virtualMax + maxTop/2) / maxTop
	return
}

func (fp *FileSystemPanel) initScrollBar() {
	fp.scrollBar = vtui.NewScrollBar(0, 0, 0)
	fp.scrollBar.SetOwner(fp)
	fp.scrollBar.SetVisible(true)
	fp.scrollBar.OnScroll = func(value int) {
		_, _, maxTop, virtualMax, _ := fp.panelScrollMetrics()
		if maxTop == 0 || virtualMax == 0 {
			return
		}
		fp.setPanelScrollTop((value*maxTop + virtualMax/2) / virtualMax)
	}
	fp.scrollBar.OnStep = func(step int) {
		_, visibleItems, _, _, _ := fp.panelScrollMetrics()
		delta := 1
		if step < 0 {
			delta = -1
		}
		if step == -2 || step == 2 {
			delta *= visibleItems
		}
		fp.setPanelScrollTop(fp.table.TopPos + delta)
	}
}

func (fp *FileSystemPanel) syncScrollBar() bool {
	if fp.scrollBar == nil || AppConfig.PanelScrollbarMode == PanelScrollbarOff {
		return false
	}
	height, _, maxTop, virtualMax, virtualValue := fp.panelScrollMetrics()
	if height <= 2 || maxTop == 0 {
		// Keep the previous range until button release so vtui.ScrollBar can
		// cancel an in-progress drag or auto-repeat even if a refresh made the
		// scrollbar unnecessary while the button was held.
		if !fp.scrollMouseActive {
			fp.scrollBar.SetParams(0, 0, 0)
		}
		return false
	}
	y1 := fp.table.Y1 + fp.table.MarginTop
	fp.scrollBar.SetPosition(fp.X2, y1, fp.X2, y1+height-1)
	fp.scrollBar.PgStep = height
	fp.scrollBar.SetParams(virtualValue, 0, virtualMax)
	return true
}

func (fp *FileSystemPanel) setPanelScrollTop(top int) {
	_, _, maxTop, _, _ := fp.panelScrollMetrics()
	if top < 0 {
		top = 0
	}
	if top > maxTop {
		top = maxTop
	}

	delta := top - fp.table.TopPos
	if delta == 0 {
		return
	}
	idx := fp.GetCursorIndex() + delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(fp.entries) {
		idx = len(fp.entries) - 1
	}

	fp.table.TopPos = top
	fp.SetCursorIndex(idx)
	fp.Refresh()
	vtui.FrameManager.Redraw()
}

func (fp *FileSystemPanel) drawScrollBar(scr *vtui.ScreenBuf) {
	if !fp.syncScrollBar() {
		return
	}
	height := fp.scrollBar.Y2 - fp.scrollBar.Y1 + 1
	if AppConfig.PanelScrollbarMode == PanelScrollbarMinimal {
		caretPos, caretLength := minimalPanelScrollThumb(height, fp.scrollBar.Value, fp.scrollBar.Max)
		attr := vtui.Palette[ColPanelMinimalScrollbar]
		for offset := 0; offset < caretLength; offset++ {
			scr.Write(fp.scrollBar.X1, fp.scrollBar.Y1+caretPos+offset,
				vtui.StringToCharInfo("│", attr))
		}
		return
	}
	vtui.DrawScrollBar(scr, fp.scrollBar.X1, fp.scrollBar.Y1, height,
		fp.scrollBar.Value, fp.scrollBar.Max+height, vtui.Palette[ColPanelScrollbar])
}

func minimalPanelScrollThumb(height, value, maximum int) (position, length int) {
	if height <= 0 || maximum <= 0 {
		return 0, 0
	}
	itemsCount := maximum + height
	length = (height*height + itemsCount/2) / itemsCount
	if length < 1 {
		length = 1
	}
	if length >= height {
		length = height - 1
	}
	maxPosition := height - length
	if value < 0 {
		value = 0
	}
	if value > maximum {
		value = maximum
	}
	position = (value*maxPosition + maximum/2) / maximum
	return position, length
}

// drawCursorSeparators restores the cursor background on column separators.
// vtui.Table draws all separators in one pass after drawing its rows, which
// otherwise overwrites the cursor attributes in single-entry-per-row modes.
func (fp *FileSystemPanel) drawCursorSeparators(scr *vtui.ScreenBuf) {
	if fp.gridColumnCount() != 1 || !fp.table.ShowSeparators || !fp.table.IsFocused() {
		return
	}

	y := fp.table.Y1 + fp.table.MarginTop + fp.table.SelectPos - fp.table.TopPos
	if y < fp.table.Y1+fp.table.MarginTop || y > fp.table.Y2 {
		return
	}

	x := fp.table.X1
	for column := 0; column < len(fp.table.Columns)-1; column++ {
		x += fp.table.Columns[column].Width
		// Keep the separator's own foreground and copy only the rendered
		// cursor cell's background. The separator must not inherit the file
		// name/highlighter foreground color.
		cursorAttr := scr.GetCell(x-1, y).Attributes
		attr := vtui.Palette[fp.table.ColorBoxIdx]
		if cursorAttr&vtui.IsBgRGB != 0 {
			attr = vtui.SetRGBBack(attr, vtui.GetRGBBack(cursorAttr))
		} else {
			attr = vtui.SetIndexBack(attr, vtui.GetIndexBack(cursorAttr))
		}
		attr = (attr &^ vtui.BackgroundIntensity) | (cursorAttr & vtui.BackgroundIntensity)
		scr.Write(x, y, vtui.StringToCharInfo("│", attr))
		x++
	}
}

func (fp *FileSystemPanel) processScrollBarMouse(e *vtinput.InputEvent) bool {
	if fp.scrollBar == nil || AppConfig.PanelScrollbarMode == PanelScrollbarOff {
		return false
	}
	// Releases must reach ScrollBar so it can stop dragging and auto-repeat.
	if e.ButtonState == 0 {
		if AppConfig.PanelScrollbarMode == PanelScrollbarFull {
			fp.scrollBar.ProcessMouse(e)
		}
		fp.scrollMouseActive = false
		fp.minimalScrollDragGap = 0
		fp.syncScrollBar()
		return false
	}
	if e.ButtonState&vtinput.FromLeft1stButtonPressed == 0 {
		return false
	}
	if AppConfig.PanelScrollbarMode == PanelScrollbarMinimal {
		if fp.scrollMouseActive {
			height := fp.scrollBar.Y2 - fp.scrollBar.Y1 + 1
			_, caretLength := minimalPanelScrollThumb(height, fp.scrollBar.Value, fp.scrollBar.Max)
			maxPosition := height - caretLength
			position := int(e.MouseY) - fp.scrollBar.Y1 - fp.minimalScrollDragGap
			if position < 0 {
				position = 0
			}
			if position > maxPosition {
				position = maxPosition
			}
			value := 0
			if maxPosition > 0 {
				value = (position*fp.scrollBar.Max + maxPosition/2) / maxPosition
			}
			_, _, maxTop, virtualMax, _ := fp.panelScrollMetrics()
			if virtualMax > 0 {
				fp.setPanelScrollTop((value*maxTop + virtualMax/2) / virtualMax)
			}
			return true
		}
		if !e.KeyDown || e.MouseEventFlags&vtinput.MouseMoved != 0 || !fp.syncScrollBar() || int(e.MouseX) != fp.scrollBar.X1 {
			return false
		}
		height := fp.scrollBar.Y2 - fp.scrollBar.Y1 + 1
		caretPos, caretLength := minimalPanelScrollThumb(height, fp.scrollBar.Value, fp.scrollBar.Max)
		y := int(e.MouseY) - fp.scrollBar.Y1
		if y < caretPos || y >= caretPos+caretLength {
			return false
		}
		fp.scrollMouseActive = true
		fp.minimalScrollDragGap = y - caretPos
		return true
	}
	if fp.scrollMouseActive {
		// Once a scrollbar owns the press, moving over file rows must not
		// turn the same gesture into row selection.
		fp.scrollBar.ProcessMouse(e)
		return true
	}
	// A scrollbar interaction can only start on the initial button-down,
	// never when an existing row drag merely crosses the scrollbar.
	if !e.KeyDown || e.MouseEventFlags&vtinput.MouseMoved != 0 || !fp.syncScrollBar() {
		return false
	}
	handled := fp.scrollBar.ProcessMouse(e)
	if handled {
		fp.scrollMouseActive = true
	}
	return handled
}

func (fp *FileSystemPanel) configureCellSelection() {
	if fp.gridColumnCount() > 1 {
		fp.table.CellSelection = true
	} else {
		fp.table.CellSelection = false
		fp.table.SelectCol = 0
	}
}

func (fp *FileSystemPanel) galleryDensity(mode GalleryLayoutMode) int {
	if parsed, ok := parseGalleryLayoutMode(string(mode)); ok {
		mode = parsed
	} else {
		mode = GalleryLayoutMasonry
	}
	if value, ok := fp.galleryDensities[mode]; ok {
		return value
	}
	defaultValue, _, _ := galleryDensityLimits(mode)
	return defaultValue
}

func (fp *FileSystemPanel) effectiveGalleryLayoutMode() GalleryLayoutMode {
	if mode, ok := parseGalleryLayoutMode(string(fp.galleryLayoutMode)); ok {
		return mode
	}
	return GalleryLayoutMasonry
}

func (fp *FileSystemPanel) effectiveGalleryColumnCount() int {
	if fp.galleryColumnCount >= minGalleryColumnCount &&
		fp.galleryColumnCount <= maxGalleryColumnCount {
		return fp.galleryColumnCount
	}
	return defaultGalleryColumnCount
}

func (fp *FileSystemPanel) setGalleryLayoutState(mode GalleryLayoutMode, columnCount int) bool {
	parsed, ok := parseGalleryLayoutMode(string(mode))
	if !ok {
		return false
	}
	if parsed == GalleryLayoutColumns {
		if columnCount < minGalleryColumnCount || columnCount > maxGalleryColumnCount {
			return false
		}
	} else if columnCount < minGalleryColumnCount || columnCount > maxGalleryColumnCount {
		// Non-column layouts preserve the last useful Columns setting.  Accept an
		// omitted/zero count from older native clients without overwriting it.
		columnCount = fp.effectiveGalleryColumnCount()
	}

	changed := fp.galleryLayoutMode != parsed
	if parsed == GalleryLayoutColumns && fp.effectiveGalleryColumnCount() != columnCount {
		changed = true
	}
	fp.galleryLayoutMode = parsed
	if parsed == GalleryLayoutColumns {
		fp.galleryColumnCount = columnCount
	}
	if changed {
		fp.galleryLayoutRevision++
	}
	return true
}

// SetGalleryLayout atomically selects a strategy of the reusable native panel
// renderer. The terminal frontend continues to use ViewMode independently.
func (fp *FileSystemPanel) SetGalleryLayout(mode GalleryLayoutMode, columnCount int) bool {
	return fp.setGalleryLayoutState(mode, columnCount)
}

// SetGalleryDensity changes only the saved density for one gallery mode.  It
// must not switch presentation, replace the catalog, or disturb another mode's
// preferred size. Values are clamped at the semantic boundary so malformed or
// stale native clients cannot produce unusable geometry.
func (fp *FileSystemPanel) SetGalleryDensity(mode GalleryLayoutMode, density int) bool {
	parsed, ok := parseGalleryLayoutMode(string(mode))
	if !ok {
		return false
	}
	density = clampGalleryDensity(parsed, density)
	if fp.galleryDensities == nil {
		fp.galleryDensities = make(map[GalleryLayoutMode]int)
	}
	if fp.galleryDensity(parsed) == density {
		return true
	}
	fp.galleryDensities[parsed] = density
	fp.galleryLayoutRevision++
	return true
}

// ResetGalleryDensity removes the per-mode override. Compact modes then use
// the host's exact font-derived row height, so Columns and Details share the
// same untouched density instead of persisting two nearly-equal integers.
func (fp *FileSystemPanel) ResetGalleryDensity(mode GalleryLayoutMode) bool {
	parsed, ok := parseGalleryLayoutMode(string(mode))
	if !ok {
		return false
	}
	if fp.galleryDensities == nil {
		return true
	}
	if _, exists := fp.galleryDensities[parsed]; !exists {
		return true
	}
	delete(fp.galleryDensities, parsed)
	fp.galleryLayoutRevision++
	return true
}

func cloneGalleryDensities(source map[GalleryLayoutMode]int) map[GalleryLayoutMode]int {
	clone := make(map[GalleryLayoutMode]int, len(source))
	for mode, density := range source {
		clone[mode] = density
	}
	return clone
}

type panelGallerySessionState struct {
	LayoutMode  GalleryLayoutMode
	ColumnCount int
	Densities   map[GalleryLayoutMode]int
}

func defaultPanelGallerySessionState() panelGallerySessionState {
	return panelGallerySessionState{
		LayoutMode:  GalleryLayoutMasonry,
		ColumnCount: defaultGalleryColumnCount,
		Densities:   make(map[GalleryLayoutMode]int),
	}
}

func clonePanelGallerySessionState(state panelGallerySessionState) panelGallerySessionState {
	mode, ok := parseGalleryLayoutMode(string(state.LayoutMode))
	if !ok {
		mode = GalleryLayoutMasonry
	}
	columns := state.ColumnCount
	if columns < minGalleryColumnCount || columns > maxGalleryColumnCount {
		columns = defaultGalleryColumnCount
	}
	densities := make(map[GalleryLayoutMode]int)
	for _, candidate := range galleryLayoutModes {
		if density, present := state.Densities[candidate]; present {
			densities[candidate] = clampGalleryDensity(candidate, density)
		}
	}
	return panelGallerySessionState{
		LayoutMode:  mode,
		ColumnCount: columns,
		Densities:   densities,
	}
}

func (fp *FileSystemPanel) GetCursorIndex() int {
	if fp.cursorIdx >= len(fp.entries) {
		fp.cursorIdx = len(fp.entries) - 1
	}
	if fp.cursorIdx < 0 {
		fp.cursorIdx = 0
	}
	return fp.cursorIdx
}

func (fp *FileSystemPanel) SetCursorIndex(idx int) {
	if len(fp.entries) == 0 {
		fp.cursorIdx = 0
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(fp.entries) {
		idx = len(fp.entries) - 1
	}
	fp.cursorIdx = idx

	// Sync table visual state
	if fp.gridColumnCount() == 1 {
		fp.table.SetSelectPos(fp.cursorIdx)
		fp.table.SelectCol = 0
		if fp.fastFindMode {
			H := fp.table.ViewHeight
			if H > 2 && fp.cursorIdx >= fp.table.TopPos+H-2 {
				fp.table.TopPos = fp.cursorIdx - H + 3
				if fp.table.TopPos < 0 {
					fp.table.TopPos = 0
				}
			}
		}
	} else {
		H := fp.table.ViewHeight
		if H <= 0 {
			H = 1
		}

		// 1. Ensure TopPos is sane for the current cursor
		if fp.cursorIdx < fp.table.TopPos {
			fp.table.TopPos = fp.cursorIdx
		} else if fp.cursorIdx >= fp.table.TopPos+fp.gridColumnCount()*H {
			fp.table.TopPos = fp.cursorIdx - fp.gridColumnCount()*H + 1
		}

		// Far-style 2-column scrolling: ensure cursorIdx is in [TopPos, TopPos + 2*H)
		if fp.cursorIdx < fp.table.TopPos {
			fp.table.TopPos = fp.cursorIdx
		} else if fp.cursorIdx >= fp.table.TopPos+fp.gridColumnCount()*H {
			fp.table.TopPos = fp.cursorIdx - fp.gridColumnCount()*H + 1
		}

		if fp.fastFindMode && H > 2 {
			rel := fp.cursorIdx - fp.table.TopPos
			row := rel % H
			if row >= H-2 {
				shift := row - (H - 3)
				fp.table.TopPos += shift
			}
		}

		if fp.table.TopPos < 0 {
			fp.table.TopPos = 0
		}

		rel := fp.cursorIdx - fp.table.TopPos
		fp.table.SelectCol = rel / H
		// Table internal rendering expects SelectPos to be absolute index in its row space
		// to correctly calculate vertical offset: y = Y1 + (SelectPos - TopPos)
		fp.table.SelectPos = fp.table.TopPos + (rel % H)

		// If we landed on a column that is theoretically correct but visually empty,
		// the table will handle it during Show, but we keep the absolute index.
	}
}

func (fp *FileSystemPanel) updateTitle(err error) {
	path := fp.vfs.GetPath()
	title := ""
	if fp.providerOpenTarget != "" {
		// A standalone visual path is already the complete user-facing title.
		// Do not ask the source VFS to interpret a path owned by another provider.
		path = fp.providerOpenTarget
		title = path
	} else if tp, ok := fp.vfs.(vfs.PanelTitleProvider); ok {
		title = tp.PanelTitle(path)
	}
	if title == "" {
		title = path
		if tp, ok := fp.vfs.(vfs.TitleProvider); ok {
			if prefix := tp.GetTitle(); prefix != "" {
				title = prefix + ":" + title
			}
		}
	}

	if err != nil && err != context.Canceled {
		title += " [Error]"
	}
	fp.semanticTitle = title
	if err == nil && fp.isLoading {
		title += " " + panelLoadingPulse[fp.loadingFrame%len(panelLoadingPulse)]
	}
	fp.currentTitle = title
	fp.frame.SetTitle("")
}

func (fp *FileSystemPanel) stopLoadingAnimation() {
	fp.loadingGeneration++
	if fp.loadingTimer != nil {
		fp.loadingTimer.Stop()
		fp.loadingTimer = nil
	}
}

func (fp *FileSystemPanel) startLoadingAnimation() {
	fp.stopLoadingAnimation()
	fp.loadingFrame = 0
	fp.updateTitle(nil)
	vtui.FrameManager.Redraw()

	// In tests, do not run the infinite timer loop to prevent task queue leakage.
	if DisableLoadingAnimationInTests && flag.Lookup("test.v") != nil {
		return
	}

	generation := fp.loadingGeneration
	var scheduleNext func()
	scheduleNext = func() {
		fp.loadingTimer = time.AfterFunc(panelLoadingPulseInterval, func() {
			vtui.FrameManager.PostTaskWithRedrawDecision(func() bool {
				if !fp.isLoading || fp.loadingGeneration != generation {
					return false
				}
				fp.loadingFrame = (fp.loadingFrame + 1) % len(panelLoadingPulse)
				fp.updateTitle(nil)
				vtui.FrameManager.Redraw()
				scheduleNext()
				return true
			})
		})
	}
	scheduleNext()
}

func (fp *FileSystemPanel) pathTitleHitTest(x, y int) bool {
	if y != fp.Y1 || fp.currentTitle == "" {
		return false
	}
	availW := (fp.X2 - fp.X1) - 6
	if availW < 5 {
		availW = 5
	}
	displayTitle := fp.currentTitle
	if runewidth.StringWidth(displayTitle) > availW {
		displayTitle = vtui.TruncateMiddle(displayTitle, availW)
	}
	// Include the one-cell padding drawn on both sides of the path.
	return x >= fp.X1+2 && x <= fp.X1+3+runewidth.StringWidth(displayTitle)
}

func (fp *FileSystemPanel) ReadDirectory() {
	fp.readDirectoryEx(false)
}

// enqueueDirectoryLoad keeps at most one backend read running and one newer
// read waiting. Repeated navigation replaces the pending closure instead of
// creating a FIFO of stale Stat/ReadDir goroutines. The running request is
// cancelled by readDirectoryEx; it may still need to drain one FISH+ response,
// after which only the most recent path is allowed to start.
func (fp *FileSystemPanel) enqueueDirectoryLoad(load func()) {
	fp.enqueueDirectoryLoadWithBenchmark(nil, load)
}

func (fp *FileSystemPanel) enqueueDirectoryLoadWithBenchmark(benchmark *navigationBenchmarkTrace, load func()) {
	request := &directoryLoadRequest{load: load, benchmark: benchmark}
	if benchmark != nil {
		request.enqueuedNs = navigationBenchmarkMonotonicNs()
		benchmark.eventAt("load.queued", "go.ui", request.enqueuedNs)
	}
	fp.loadQueueMu.Lock()
	if fp.loadWorkerActive {
		if replaced := fp.pendingDirectoryLoad; replaced != nil && replaced.benchmark != nil {
			replaced.benchmark.event("load.queue.superseded", "go.worker",
				"supersededBy", navigationBenchmarkTraceName(benchmark))
		}
		fp.pendingDirectoryLoad = request
		fp.loadQueueMu.Unlock()
		return
	}
	fp.loadWorkerActive = true
	fp.loadQueueMu.Unlock()

	go func() {
		next := request
		for next != nil {
			if next.benchmark != nil {
				startedNs := navigationBenchmarkMonotonicNs()
				next.benchmark.eventAt("load.worker.started", "go.worker", startedNs,
					"queueNs", startedNs-next.enqueuedNs)
			}
			next.load()

			fp.loadQueueMu.Lock()
			next = fp.pendingDirectoryLoad
			fp.pendingDirectoryLoad = nil
			if next == nil {
				fp.loadWorkerActive = false
			}
			fp.loadQueueMu.Unlock()
		}
	}()
}

// cancelProviderOpen invalidates an asynchronous VFS mount before asking its
// context to stop. A completion already queued on the UI thread will see that
// it no longer owns the transition and close any VFS it produced instead of
// replacing the panel's newer file system.
func (fp *FileSystemPanel) cancelProviderOpen() {
	fp.closeProviderOpenDialog()
	if task := fp.providerOpenTask; task != nil {
		fp.providerOpenTask = nil
		fp.providerOpenTarget = ""
		fp.providerOpenSourceSelect = ""
		fp.providerOpenResult = nil
		task.Cancel()
	}
}

func (fp *FileSystemPanel) closeProviderOpenDialog() {
	if fp == nil || fp.providerOpenDialog == nil {
		return
	}
	dlg := fp.providerOpenDialog
	fp.providerOpenDialog = nil
	dlg.OnResult = nil
	dlg.Close()
}

func (fp *FileSystemPanel) cancelProviderOpenAndRestore() {
	if fp == nil || fp.providerOpenTask == nil {
		return
	}
	sourceSelection := fp.providerOpenSourceSelect
	sourcePath := fp.vfs.GetPath()
	fp.cancelProviderOpen()
	fp.isLoading = false
	fp.stopLoadingAnimation()
	fp.updateTitle(nil)
	fp.pendingSelection = sourceSelection
	fp.suppressNextFolderHistory(sourcePath)
	fp.ReadDirectory()
	vtui.FrameManager.Redraw()
}

func (fp *FileSystemPanel) showProviderOpenDialog(status vfs.ProviderOpenStatus) {
	if fp == nil || fp.providerOpenTask == nil || status.Title == "" || status.Message == "" {
		return
	}

	const width = 68
	lines := strings.Split(status.Message, "\n")
	height := len(lines) + 7
	if height < 10 {
		height = 10
	}
	dlg := vtui.NewCenteredDialog(width, height, status.Title)
	dlg.AttentionSuppressed = true
	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	for _, line := range lines {
		label := vtui.NewLabel(0, 0, vtui.TruncateMiddle(line, width-4), nil)
		dlg.AddItem(label)
		vbox.Add(label, vtui.Margins{}, vtui.AlignCenter)
	}
	btnCancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))
	dlg.AddItem(btnCancel)
	vbox.Add(btnCancel, vtui.Margins{Top: 1}, vtui.AlignCenter)
	vbox.Apply()

	fp.providerOpenDialog = dlg
	btnCancel.OnClick = func() { dlg.Close() }
	dlg.OnResult = func(int) {
		if fp.providerOpenDialog != dlg {
			return
		}
		fp.providerOpenDialog = nil
		fp.cancelProviderOpenAndRestore()
	}
	vtui.FrameManager.AddScreenHeadless(dlg)
}

func (fp *FileSystemPanel) persistentPath() string {
	if fp != nil && fp.providerOpenTask != nil && fp.providerOpenTarget != "" {
		return fp.providerOpenTarget
	}
	if fp == nil || fp.vfs == nil {
		return ""
	}
	return fp.vfs.GetPath()
}

// openVFSAsync runs a provider or URI mount without allowing a slow or
// cancelled result to replace a panel that has since navigated elsewhere.
// The success callback decides whether the source VFS becomes ParentVFS or is
// closed as part of a complete panel switch.
func (fp *FileSystemPanel) openVFSAsync(
	persistentTarget string,
	open func(context.Context) (vfs.VFS, error),
	onSuccess func(vfs.VFS),
	onError func(error),
) bool {
	if fp == nil || fp.vfs == nil || open == nil || onSuccess == nil {
		return false
	}

	fp.cancelProviderOpen()
	if fp.cancelLoad != nil {
		fp.cancelLoad()
		fp.cancelLoad = nil
	}
	fp.stopLoadingAnimation()

	sourceVFS := fp.vfs
	sourcePath := sourceVFS.GetPath()
	sourceSelection := fp.GetSelectedName()
	fp.providerOpenTarget = persistentTarget
	fp.providerOpenSourceSelect = sourceSelection
	fp.isLoading = true
	fp.startLoadingAnimation()
	cachedPreview := fp.showCachedStandalonePath(persistentTarget)
	if !cachedPreview {
		// Keep the source rows as a stable placeholder when this destination has
		// never been visited. Input is guarded until the provider switch, so they
		// cannot dispatch operations against the wrong VFS.
		fp.Refresh()
		vtui.FrameManager.Redraw()
	} else {
		fp.Refresh()
		vtui.FrameManager.Redraw()
	}
	fp.providerOpenTask = vtui.RunAsync(func(task *vtui.TaskContext) {
		newVFS, err := open(task.Context)
		if err == nil && newVFS == nil {
			err = fmt.Errorf("provider returned no file system")
		}
		task.RunOnUI(func() {
			if fp.providerOpenTask != task {
				if newVFS != nil {
					_ = newVFS.Close()
				}
				return
			}
			fp.providerOpenTask = nil
			fp.closeProviderOpenDialog()
			resultCallback := fp.providerOpenResult
			fp.providerOpenResult = nil
			fp.providerOpenTarget = ""
			fp.providerOpenSourceSelect = ""
			if !sameVFSInstance(fp.vfs, sourceVFS) || fp.vfs.GetPath() != sourcePath {
				if newVFS != nil {
					_ = newVFS.Close()
				}
				return
			}
			if err != nil {
				if newVFS != nil {
					_ = newVFS.Close()
				}
				fp.isLoading = false
				fp.updateTitle(err)
				fp.pendingSelection = sourceSelection
				fp.suppressNextFolderHistory(sourcePath)
				// Restore the source listing before a history callback starts the
				// next asynchronous mount.  The next mount will cancel this load;
				// without it, an exhausted history walk would leave the panel in
				// the loading state of the failed provider.
				fp.ReadDirectory()
				handled := resultCallback != nil && resultCallback(false)
				if !handled && onError != nil {
					onError(err)
				}
				return
			}
			onSuccess(newVFS)
			if resultCallback != nil {
				resultCallback(true)
			}
		})
	})
	return true
}

// showCurrentVFSLoadingRows atomically stops the panel from exposing rows that
// belonged to a VFS it has just left. ReadDirectory will replace this minimal
// view immediately from cache when allowed; without a cache (or when
// SyncPanelLoad deliberately bypasses it), only a real parent row is safe to
// keep interactive while the new listing is in flight.
func (fp *FileSystemPanel) showCurrentVFSLoadingRows() {
	fp.entries = nil
	if fp.vfs != nil && (!fp.vfs.IsAtRoot() || fp.vfs.ParentVFS() != nil) {
		fp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	}
	fp.SetCursorIndex(0)
	fp.Refresh()
	vtui.FrameManager.Redraw()
}

// setDirectoryPath changes the panel VFS path while preserving the trust
// boundary between an authoritative directory row and arbitrary user input.
// Only knownDirectory may use a provider's no-I/O optimistic setter; typed,
// bookmark, and other external paths must pass through SetPath validation.
func (fp *FileSystemPanel) setDirectoryPath(target string, knownDirectory bool) error {
	benchmark := navigationBenchmarkCurrentUI()
	fromPath := ""
	direction := ""
	if benchmark != nil && fp.vfs != nil {
		fromPath = fp.vfs.GetPath()
		switch {
		case sameFolderHistoryPath(target, fromPath):
			direction = "same"
		case sameFolderHistoryPath(target, fp.vfs.Dir(fromPath)):
			direction = "parent"
		default:
			direction = "child"
		}
		benchmark.setPaths(fromPath, target, direction)
		benchmark.event("path.set.begin", "go.ui", "fromPath", fromPath,
			"toPath", target, "direction", direction)
	}
	strategy := "verified"
	var err error
	if setter, ok := fp.vfs.(vfs.OptimisticPathSetter); knownDirectory && ok {
		strategy = "optimistic"
		if fp.cancelLoad != nil {
			fp.cancelLoad()
			fp.cancelLoad = nil
		}
		err = setter.SetPathOptimistic(target)
	} else {
		err = fp.vfs.SetPath(target)
	}
	if benchmark != nil {
		fields := []any{"fromPath", fromPath, "toPath", target, "direction", direction,
			"strategy", strategy, "ok", err == nil}
		if err != nil {
			fields = append(fields, "error", err.Error())
		}
		benchmark.event("path.set.end", "go.ui", fields...)
	}
	return err
}

// setKnownDirectoryPath is reserved for a directory identity already supplied
// by the active VFS (for example Enter on a panel row) or an exact panel-cache
// hit. The following ReadDir remains authoritative if that identity went stale.
func (fp *FileSystemPanel) setKnownDirectoryPath(target string) error {
	return fp.setDirectoryPath(target, true)
}

func (fp *FileSystemPanel) setVerifiedDirectoryPath(target string) error {
	return fp.setDirectoryPath(target, false)
}

func (fp *FileSystemPanel) hasCachedDirectoryPath(target string) bool {
	if fp == nil || fp.vfs == nil || fp.dirCache == nil || AppConfig.SyncPanelLoad {
		return false
	}
	resolved, err := fp.vfs.Abs(target)
	if err != nil {
		return false
	}
	_, ok := fp.dirCache[directoryCacheKey(fp.vfs, resolved)]
	return ok
}

func (fp *FileSystemPanel) suppressNextFolderHistory(path string) {
	fp.suppressFolderHistoryToken++
	fp.suppressFolderHistoryPath = path
}

func (fp *FileSystemPanel) clearFolderHistorySuppression() {
	fp.suppressFolderHistoryToken++
	fp.suppressFolderHistoryPath = ""
}

func (fp *FileSystemPanel) folderHistorySuppression(path string) (uint64, bool) {
	if !sameFolderHistoryPath(path, fp.suppressFolderHistoryPath) {
		return 0, false
	}
	return fp.suppressFolderHistoryToken, true
}

func (fp *FileSystemPanel) consumeFolderHistorySuppression(path string, token uint64) bool {
	if token == 0 || token != fp.suppressFolderHistoryToken || !sameFolderHistoryPath(path, fp.suppressFolderHistoryPath) {
		return false
	}
	fp.suppressFolderHistoryPath = ""
	return true
}

// showDirectoryError keeps asynchronous refresh failures from stacking modal
// dialogs. A failed recovery may schedule another read before the user closes
// the first message; only the first live dialog should remain actionable.
func (fp *FileSystemPanel) showDirectoryError(title, message string) {
	if fp.directoryErrorDialog != nil && !fp.directoryErrorDialog.IsDone() {
		return
	}
	dlg := vtui.ShowMessage(title, message, []string{"&Ok"})
	fp.directoryErrorDialog = dlg
	dlg.OnResult = func(int) {
		if fp.directoryErrorDialog == dlg {
			fp.directoryErrorDialog = nil
		}
	}
}

// moveToParentAfterLoadFailure restores a panel using the VFS' canonical
// absolute parent path. Passing a bare ".." is not portable: remote VFSes such
// as AFC deliberately reject it as a possible domain-root escape.
func (fp *FileSystemPanel) moveToParentAfterLoadFailure(loadVFS vfs.VFS, failedPath string) bool {
	parentPath := loadVFS.Dir(failedPath)
	if parentPath == "" || parentPath == failedPath {
		return false
	}
	if err := fp.setKnownDirectoryPath(parentPath); err != nil {
		vtui.DebugLog("PANEL[%p]: Failed to restore parent %q after reading %q: %v", fp, parentPath, failedPath, err)
		return false
	}
	return true
}

func (fp *FileSystemPanel) readDirectoryEx(keepEntries bool) {
	benchmark := navigationBenchmarkCurrentUI()
	if previous := fp.benchmarkLoadTrace; previous != nil && previous != benchmark {
		previous.event("navigation.cancelled", "go.ui",
			"supersededBy", navigationBenchmarkTraceName(benchmark))
	}
	fp.benchmarkLoadTrace = benchmark
	if fp.cancelLoad != nil {
		fp.cancelLoad()
		fp.cancelLoad = nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	fp.loadCtx = ctx
	fp.cancelLoad = cancel
	fp.loadGeneration++
	loadGeneration := fp.loadGeneration
	fp.isLoading = true
	fp.semanticCachedCatalogReady = false
	fp.semanticCachedCatalogExported = false
	fp.startLoadingAnimation()

	loadVFS := fp.vfs
	path := loadVFS.GetPath()
	if benchmark != nil {
		_, fromPath, _, direction := benchmark.pathFields()
		if fromPath == "" {
			fromPath = path
			if direction == "" {
				direction = "refresh"
			}
		}
		benchmark.setPaths(fromPath, path, direction)
		benchmark.event("directory_read.begin", "go.ui", "path", path, "keepEntries", keepEntries)
	}
	suppressionToken, hasFolderHistorySuppression := fp.folderHistorySuppression(path)
	cacheKey := directoryCacheKey(loadVFS, path)
	loadAtRoot := loadVFS.IsAtRoot()
	showUpEntry := !loadAtRoot || loadVFS.ParentVFS() != nil
	if fp.previousSelectionVFS != nil && !fp.previousSelectionMatches(loadVFS, path) {
		fp.clearPreviousSelection()
	}

	// Drop persistent selection when we've navigated to a different
	// directory. Without this the map (keyed by bare filename)
	// silently re-applies to any incoming entry with a matching
	// name — e.g. .claude selected in ~/f4 would come back
	// pre-selected in ~/scc or ~. Same rule far/far2l use:
	// selection is per-directory.
	directoryChanged := fp.lastLoadedPath != "" && !sameFolderHistoryPath(fp.lastLoadedPath, path)
	suppressFolderHistory := hasFolderHistorySuppression && fp.consumeFolderHistorySuppression(path, suppressionToken)
	if directoryChanged {
		for k := range fp.selectedItems {
			delete(fp.selectedItems, k)
		}
		fp.directoryEpoch++
		fp.selectionEpoch = make(map[string]uint64)
	}
	fp.lastLoadedPath = path
	if directoryChanged && !suppressFolderHistory {
		// Record accepted navigation in UI order, not in backend completion
		// order. Otherwise an older slow cloud ReadDir can finish after a newer
		// visit and move its path to the front of the global MRU history.
		if benchmark != nil {
			benchmark.event("history.persist.begin", "go.ui", "path", path)
		}
		AddFolderHistory(path)
		if benchmark != nil {
			benchmark.event("history.persist.end", "go.ui", "path", path)
		}
	} else if benchmark != nil {
		reason := "unchanged"
		if suppressFolderHistory {
			reason = "suppressed"
		}
		benchmark.event("history.persist.skipped", "go.ui", "path", path, "reason", reason)
	}

	if fp.pendingSelection == "" {
		oldName := fp.getRawSelectedName()
		if oldName != "" && oldName != ".." {
			fp.pendingSelection = oldName
		}
	}

	hasCache := false
	cacheInitialCursorName := ""
	cacheInitialCursorIndex := -1
	if !keepEntries {
		if fp.dirCache == nil {
			fp.dirCache = make(map[dirCacheKey]dirCacheEntry)
		}
		if cached, ok := fp.dirCache[cacheKey]; ok && !AppConfig.SyncPanelLoad {
			hasCache = true
			fp.catalogProvisional = false
			fp.semanticCachedCatalogReady = true
			vtui.DebugLog("PANEL: Using cached entries for %s", path)
			fp.entries = nil

			if showUpEntry {
				fp.entries = append(fp.entries, &fileEntry{
					VFSItem:  panelUpEntryItem(cached.upItem, true),
					IsCached: true,
				})
			}

			for _, item := range cached.items {
				if !AppConfig.ShowHiddenFiles && item.Name != ".." && item.IsHidden {
					continue
				}
				entry := &fileEntry{VFSItem: item, IsCached: true}
				fp.applyPersistentSelection(entry, loadVFS, path)
				fp.entries = append(fp.entries, entry)
			}

			fp.sortEntries()

			target := fp.pendingSelection
			if target != "" {
				for i, entry := range fp.entries {
					if entry.Name == target {
						fp.SetCursorIndex(i)
						fp.pendingSelection = ""
						break
					}
				}
			}
			fp.Refresh()
			cacheInitialCursorName = fp.getRawSelectedName()
			cacheInitialCursorIndex = fp.GetCursorIndex()
			if benchmark != nil {
				benchmark.event("model.provisional.ready", "go.ui", "phase", "cached",
					"cacheHit", true, "entries", len(fp.entries), "cursorIndex", fp.GetCursorIndex())
				navigationBenchmarkPublishScene(benchmark, "cached")
			}
			publishPanelCatalogImmediate(fp, benchmark)
			vtui.FrameManager.Redraw()
		}
	}

	isFirstChunk := true
	if !keepEntries && !hasCache {
		fp.catalogProvisional = true
		fp.entries = nil
		if showUpEntry {
			fp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
		}
		fp.SetCursorIndex(0)
		fp.Refresh()
		if benchmark != nil {
			benchmark.event("model.provisional.ready", "go.ui", "phase", "placeholder",
				"cacheHit", false, "entries", len(fp.entries), "cursorIndex", fp.GetCursorIndex())
			navigationBenchmarkPublishScene(benchmark, "placeholder")
		}
		vtui.FrameManager.Redraw()
	}
	if keepEntries {
		fp.catalogProvisional = false
	}
	if keepEntries && benchmark != nil {
		benchmark.event("model.provisional.ready", "go.ui", "phase", "retained",
			"cacheHit", false, "entries", len(fp.entries), "cursorIndex", fp.GetCursorIndex())
		navigationBenchmarkPublishScene(benchmark, "retained")
	}

	phasedReader := phasedDirectoryReaderFor(loadVFS)
	usePhasedRead := phasedReader != nil && !keepEntries && !hasCache &&
		!AppConfig.SyncPanelLoad && panelSortSupportsPhasedDirectoryRead(fp.sortMode)
	loadIsCurrent := func() bool {
		if ctx.Err() != nil || fp.loadCtx != ctx || fp.loadGeneration != loadGeneration ||
			!sameVFSInstance(fp.vfs, loadVFS) {
			return false
		}
		return sameFolderHistoryPath(loadVFS.GetPath(), path)
	}

	fp.enqueueDirectoryLoadWithBenchmark(benchmark, func() {
		if ctx.Err() != nil {
			if benchmark != nil {
				benchmark.event("load.worker.cancelled_before_read", "go.worker", "path", path)
			}
			return
		}
		var accumulated []vfs.VFSItem
		var accumulatedByName map[string]int
		var pendingMetadata []vfs.VFSItem
		chunkCount := 0
		metadataChunkCount := 0
		if benchmark != nil {
			benchmark.event("filesystem.readdir.begin", "go.worker", "path", path,
				"phased", usePhasedRead)
		}

		publishCatalogChunk := func(chunk []vfs.VFSItem, authoritativeBase bool) {
			chunkCount++
			chunkIndex := chunkCount
			if benchmark != nil {
				benchmark.event("filesystem.readdir.chunk", "go.worker", "path", path,
					"chunk", chunkIndex, "chunkEntries", len(chunk),
					"entriesBefore", len(accumulated), "phase", "base")
			}
			if ctx.Err() != nil {
				return
			}
			accumulated = append(accumulated, chunk...)
			if authoritativeBase {
				if accumulatedByName == nil {
					accumulatedByName = make(map[string]int, len(accumulated))
				}
				for index, item := range accumulated {
					accumulatedByName[item.Name] = index
				}
			}
			if ctx.Err() != nil {
				return
			}

			// A cached directory is already a complete, interactive view. Keep it
			// on screen while all real chunks are collected and replace it once,
			// atomically, in the completion task. Rendering partial real chunks
			// would make the panel jump and could overwrite user interaction with
			// a stale pendingSelection.
			if AppConfig.SyncPanelLoad || hasCache {
				return
			}

			newEntries := make([]*fileEntry, 0, len(chunk))
			for _, item := range chunk {
				// Hide hidden files if configured, but never hide '..'
				if !AppConfig.ShowHiddenFiles && item.Name != ".." && item.IsHidden {
					continue
				}
				entry := &fileEntry{VFSItem: item}
				newEntries = append(newEntries, entry)
			}

			if ctx.Err() != nil {
				return
			}

			chunkQueuedNs := int64(0)
			if benchmark != nil {
				chunkQueuedNs = navigationBenchmarkMonotonicNs()
				benchmark.eventAt("model.chunk.queued", "go.worker", chunkQueuedNs,
					"chunk", chunkIndex, "chunkEntries", len(newEntries))
			}
			vtui.FrameManager.PostTask(func() {
				if benchmark != nil {
					startedNs := navigationBenchmarkMonotonicNs()
					benchmark.eventAt("model.chunk.started", "go.ui", startedNs,
						"chunk", chunkIndex, "queueNs", startedNs-chunkQueuedNs)
				}
				if !loadIsCurrent() {
					return
				}

				currentSelected := fp.getRawSelectedName()
				if fp.pendingSelection == "" {
					if currentSelected != "" && currentSelected != ".." {
						fp.pendingSelection = currentSelected
					}
				}

				if isFirstChunk {
					fp.entries = nil
					if showUpEntry {
						upItem := vfs.VFSItem{Name: "..", IsDir: true}
						fp.entries = []*fileEntry{{VFSItem: upItem}}
					}
					isFirstChunk = false
				}

				// Apply current and Ctrl+M snapshots to incoming items.
				for _, e := range newEntries {
					fp.applyPersistentSelection(e, loadVFS, path)
				}

				fp.entries = append(fp.entries, newEntries...)
				fp.sortEntries()
				if authoritativeBase {
					// This catalog is complete and authoritative for row identity and
					// order. Metadata is deliberately still loading in the background.
					fp.catalogProvisional = false
				}

				// Фокусировка на нужном файле
				snapped := false
				target := fp.pendingSelection
				if target == "" {
					target = currentSelected
				}

				if target != "" {
					for i, entry := range fp.entries {
						if entry.Name == target {
							fp.SetCursorIndex(i)
							if entry.Name == fp.pendingSelection {
								fp.pendingSelection = ""
							}
							snapped = true
							break
						}
					}
				}

				if !snapped && fp.pendingSelection == "" && (fp.cursorIdx >= len(fp.entries) || fp.cursorIdx < 0) {
					fp.SetCursorIndex(0)
				}

				fp.Refresh()
				if benchmark != nil {
					benchmark.event("model.chunk.ready", "go.ui", "chunk", chunkIndex,
						"entries", len(fp.entries), "cursorIndex", fp.GetCursorIndex())
					navigationBenchmarkPublishScene(benchmark, "chunk")
				}
				if authoritativeBase {
					publishPanelCatalogImmediate(fp, benchmark)
				}

				vtui.FrameManager.Redraw() // Рисуем каждый чанк!
			})
		}

		mergeMetadataChunk := func(chunk []vfs.VFSItem) {
			metadataChunkCount++
			if ctx.Err() != nil {
				return
			}
			// Keep the authoritative cache snapshot in sync without disturbing
			// the names/types order established by the base phase.
			for _, item := range chunk {
				index, ok := accumulatedByName[item.Name]
				if !ok || index < 0 || index >= len(accumulated) ||
					accumulated[index].IsDir != item.IsDir || accumulated[index].NoExtension != item.NoExtension {
					continue
				}
				accumulated[index] = item
			}
			pendingMetadata = append(pendingMetadata, chunk...)
			if benchmark != nil {
				benchmark.event("filesystem.readdir.chunk", "go.worker", "path", path,
					"chunk", metadataChunkCount, "chunkEntries", len(chunk),
					"phase", "metadata")
			}
		}

		publishMetadata := func(metadata []vfs.VFSItem) {
			if len(metadata) == 0 || ctx.Err() != nil {
				return
			}
			// The worker has finished producing metadata before this task is queued,
			// so ownership of the immutable accumulated slice can move to the UI
			// task without another full-catalog copy.
			metadataCopy := metadata
			metadataQueuedNs := int64(0)
			if benchmark != nil {
				metadataQueuedNs = navigationBenchmarkMonotonicNs()
				benchmark.eventAt("model.metadata.queued", "go.worker", metadataQueuedNs,
					"chunks", metadataChunkCount, "entries", len(metadataCopy))
			}
			vtui.FrameManager.PostTask(func() {
				if !loadIsCurrent() {
					return
				}
				byName := make(map[string]*fileEntry, len(fp.entries))
				for _, entry := range fp.entries {
					if entry != nil && entry.Name != ".." {
						byName[entry.Name] = entry
					}
				}

				// Validate the entire visible subset before mutating any row. A
				// provider that changes identity/type between phases has violated
				// the capability contract; retaining the usable base is safer than
				// partially applying a mismatched metadata chunk.
				for _, item := range metadataCopy {
					if !AppConfig.ShowHiddenFiles && item.Name != ".." && item.IsHidden {
						continue
					}
					entry, ok := byName[item.Name]
					if !ok || entry.IsDir != item.IsDir || entry.NoExtension != item.NoExtension {
						return
					}
				}
				for _, item := range metadataCopy {
					if !AppConfig.ShowHiddenFiles && item.Name != ".." && item.IsHidden {
						continue
					}
					entry := byName[item.Name]
					entry.VFSItem = item
				}
				fp.Refresh()
				if benchmark != nil {
					startedNs := navigationBenchmarkMonotonicNs()
					benchmark.eventAt("model.metadata.ready", "go.ui", startedNs,
						"chunks", metadataChunkCount, "queueNs", startedNs-metadataQueuedNs,
						"entries", len(fp.entries))
					navigationBenchmarkPublishScene(benchmark, "metadata")
				}
				vtui.FrameManager.Redraw()
			})
		}

		var err error
		if usePhasedRead {
			err = phasedReader.ReadDirPhased(ctx, path, func(phase vfs.DirectoryReadPhase, chunk []vfs.VFSItem) {
				switch phase {
				case vfs.DirectoryReadBase:
					publishCatalogChunk(chunk, true)
				case vfs.DirectoryReadMetadata:
					mergeMetadataChunk(chunk)
				}
			})
			// Publish enrichment once, after the provider has completed all of its
			// bounded worker-side chunks. The complete base task was queued first,
			// so it still gets its own fast frame; one atomic metadata commit avoids
			// reserializing the full minimal catalog and restarting the frontend pull
			// protocol for every 256 rows.
			if err == nil {
				publishMetadata(pendingMetadata)
			}
		} else {
			err = loadVFS.ReadDir(ctx, path, func(chunk []vfs.VFSItem) {
				publishCatalogChunk(chunk, false)
			})
		}
		if benchmark != nil {
			fields := []any{"path", path, "chunks", chunkCount, "entries", len(accumulated), "ok", err == nil}
			if usePhasedRead {
				fields = append(fields, "metadataChunks", metadataChunkCount, "phased", true)
			}
			if err != nil {
				fields = append(fields, "error", err.Error())
			}
			benchmark.event("filesystem.readdir.end", "go.worker", fields...)
		}

		if ctx.Err() != nil {
			if benchmark != nil {
				benchmark.event("load.worker.cancelled_after_read", "go.worker", "path", path)
			}
			return
		}

		// The listing is the only mandatory request and gets the shared remote
		// session first. Directory timestamps and metadata for ".." are useful
		// decoration/auto-refresh state, so fetch them only after ReadDir and
		// skip them entirely when the listing failed or was superseded.
		var dirStat vfs.VFSItem
		var upItemStat vfs.VFSItem
		hasUpItemStat := false
		if err == nil {
			var dirStatErr error
			if benchmark != nil {
				benchmark.event("filesystem.stat_current.begin", "go.worker", "path", path)
			}
			dirStat, dirStatErr = loadVFS.Stat(ctx, path)
			if benchmark != nil {
				fields := []any{"path", path, "ok", dirStatErr == nil}
				if dirStatErr != nil {
					fields = append(fields, "error", dirStatErr.Error())
				}
				benchmark.event("filesystem.stat_current.end", "go.worker", fields...)
			}
			if ctx.Err() != nil {
				return
			}
			if showUpEntry {
				parentPath := loadVFS.Dir(path)
				if parentPath == path && dirStatErr == nil {
					if benchmark != nil {
						benchmark.event("filesystem.stat_parent.reused", "go.worker",
							"path", parentPath, "source", "current")
					}
					upItemStat = dirStat
					hasUpItemStat = true
				} else {
					if benchmark != nil {
						benchmark.event("filesystem.stat_parent.begin", "go.worker", "path", parentPath)
					}
					pStat, statErr := loadVFS.Stat(ctx, parentPath)
					if benchmark != nil {
						fields := []any{"path", parentPath, "ok", statErr == nil}
						if statErr != nil {
							fields = append(fields, "error", statErr.Error())
						}
						benchmark.event("filesystem.stat_parent.end", "go.worker", fields...)
					}
					if statErr == nil {
						upItemStat = pStat
						hasUpItemStat = true
					}
				}
			}
		} else if benchmark != nil {
			benchmark.event("filesystem.stats.skipped", "go.worker", "reason", "readdir_error")
		}
		if ctx.Err() != nil {
			if benchmark != nil {
				benchmark.event("load.worker.cancelled_after_stats", "go.worker", "path", path)
			}
			return
		}
		upItem := panelUpEntryItem(upItemStat, hasUpItemStat)
		completionQueuedNs := int64(0)
		if benchmark != nil {
			completionQueuedNs = navigationBenchmarkMonotonicNs()
			benchmark.eventAt("model.final.queued", "go.worker", completionQueuedNs,
				"path", path, "entries", len(accumulated), "chunks", chunkCount)
		}
		vtui.FrameManager.PostTaskWithRedrawDecision(func() (needsRedraw bool) {
			// Completion tasks retain PostTask's conservative redraw by default.
			// Only the exact warm-cache equivalence proof below opts out.
			needsRedraw = true
			if benchmark != nil {
				startedNs := navigationBenchmarkMonotonicNs()
				benchmark.eventAt("model.final.started", "go.ui", startedNs,
					"path", path, "queueNs", startedNs-completionQueuedNs)
			}
			if !loadIsCurrent() {
				// This completion no longer owns the panel. Do not dereference the
				// current VFS here: another navigation may already have closed it.
				if benchmark != nil {
					benchmark.event("model.final.discarded", "go.ui", "path", path,
						"reason", "superseded")
				}
				return
			}

			if err == nil {
				fp.saveToCacheKeyWithUpItem(cacheKey, accumulated, showUpEntry, upItem)
			}

			freshCacheEquivalent := false
			if hasCache && err == nil {
				// The cached rows stayed interactive during ReadDir. Snapshot their
				// live state immediately before replacing them; this is deliberately
				// done in the UI task rather than when the load was started.
				liveCursorName := fp.getRawSelectedName()
				liveCursorIndex := fp.GetCursorIndex()
				liveCursorOffset := liveCursorIndex - fp.table.TopPos
				cursorMoved := liveCursorName != cacheInitialCursorName || liveCursorIndex != cacheInitialCursorIndex

				if fp.selectedItems == nil {
					fp.selectedItems = make(map[string]bool)
				}
				for _, entry := range fp.entries {
					if entry.Name == ".." {
						continue
					}
					if entry.Selected {
						fp.selectedItems[entry.Name] = true
					} else {
						delete(fp.selectedItems, entry.Name)
					}
				}

				freshEntries := fp.freshDirectoryEntries(
					accumulated, showUpEntry, upItem, loadVFS, path)
				freshCacheEquivalent = fp.refreshEquivalentCachedEntries(freshEntries)
				if freshCacheEquivalent {
					// The visible catalog and every negotiated metadata field are
					// unchanged. Keep the live cursor, viewport and table identities;
					// only the row backing data/cache provenance was refreshed.
					fp.pendingSelection = ""
					isFirstChunk = false
				} else {
					fp.entries = freshEntries

					// An unresolved navigation target still wins if the user did not
					// move on the cache. Once the cursor moved, the live cached row is
					// the user's latest and therefore authoritative choice.
					target := liveCursorName
					if fp.pendingSelection != "" && !cursorMoved {
						target = fp.pendingSelection
					}
					newCursorIndex := -1
					for i, entry := range fp.entries {
						if entry.Name == target {
							newCursorIndex = i
							break
						}
					}
					if newCursorIndex < 0 {
						newCursorIndex = liveCursorIndex
					}
					if newCursorIndex >= len(fp.entries) {
						newCursorIndex = len(fp.entries) - 1
					}
					if newCursorIndex < 0 {
						newCursorIndex = 0
					}

					newTop := newCursorIndex - liveCursorOffset
					if newTop < 0 {
						newTop = 0
					}
					fp.table.TopPos = newTop
					fp.SetCursorIndex(newCursorIndex)
					fp.pendingSelection = ""
					isFirstChunk = false
				}
			} else if AppConfig.SyncPanelLoad && err == nil {
				fp.entries = nil
				if showUpEntry {
					fp.entries = []*fileEntry{{VFSItem: upItem}}
				}

				newEntries := make([]*fileEntry, 0, len(accumulated))
				for _, item := range accumulated {
					if !AppConfig.ShowHiddenFiles && item.Name != ".." && item.IsHidden {
						continue
					}
					entry := &fileEntry{VFSItem: item}
					fp.applyPersistentSelection(entry, loadVFS, path)
					newEntries = append(newEntries, entry)
				}
				fp.entries = append(fp.entries, newEntries...)
				fp.sortEntries()

				if fp.pendingSelection != "" {
					fp.SelectName(fp.pendingSelection)
					fp.pendingSelection = ""
				} else if fp.cursorIdx >= len(fp.entries) || fp.cursorIdx < 0 {
					fp.SetCursorIndex(0)
				}
				isFirstChunk = false
			}

			if err == nil {
				// Clean up persistent selection only after the fresh list has been
				// built. With a cache, doing this earlier would compare the marks
				// against stale rows rather than the completed ReadDir result.
				validNames := make(map[string]bool, len(fp.entries))
				for _, e := range fp.entries {
					validNames[e.Name] = true
				}
				for name := range fp.selectedItems {
					if !validNames[name] {
						delete(fp.selectedItems, name)
					}
				}
				if fp.previousSelectionMatches(loadVFS, path) {
					for name := range fp.previousSelection {
						if !validNames[name] {
							delete(fp.previousSelection, name)
						}
					}
				}
				// The non-cached chunk path creates ".." before the deferred parent
				// Stat is available. Bring that already-visible row up to the same
				// metadata as the atomic cached/SyncPanelLoad replacement paths and
				// as the entry retained in the directory cache.
				if showUpEntry {
					for _, entry := range fp.entries {
						if entry.Name == ".." {
							entry.VFSItem = upItem
							break
						}
					}
				}
			}

			fp.catalogProvisional = false
			fp.semanticCachedCatalogReady = false
			fp.semanticCachedCatalogExported = false
			fp.stopLoadingAnimation()

			fp.lastDirMTime = dirStat.MTime
			fp.isLoading = false
			if err != nil && err != context.Canceled {
				if benchmark != nil {
					benchmark.event("model.final.error", "go.ui", "path", path,
						"entries", len(accumulated), "chunks", chunkCount, "error", err.Error())
				}
				if fp.benchmarkLoadTrace == benchmark {
					fp.benchmarkLoadTrace = nil
				}
				// A session that died is a question rather than a message: the
				// panel can often be had back, and going up a level or showing
				// an error would throw away the answer before it was asked.
				if fp.offerPanelReconnect(err, keepEntries) {
					return
				}
				if os.IsNotExist(err) && !loadAtRoot && !keepEntries {
					if !fp.moveToParentAfterLoadFailure(loadVFS, path) {
						fp.updateTitle(err)
						fp.showDirectoryError(" Error ", fmt.Sprintf("Failed to read directory:\n%v", err))
						return
					}
					vtui.DebugLog("PANEL[%p]: Directory disappeared, attempting to go up. Error: %v", fp, err)
					fp.ReadDirectory()
					return
				}

				// For permission or network errors, go back to parent and show the error.
				if !loadAtRoot && !keepEntries {
					if fp.moveToParentAfterLoadFailure(loadVFS, path) {
						fp.pendingSelection = loadVFS.Base(path)
						fp.ReadDirectory()
					} else {
						fp.updateTitle(err)
					}
					fp.showDirectoryError(" Error ", fmt.Sprintf("Cannot access folder:\n%v", err))
					return
				}

				fp.updateTitle(err)
				fp.showDirectoryError(" Error ", fmt.Sprintf("Failed to read directory:\n%v", err))
				return
			} else {
				fp.updateTitle(nil)
			}

			if isFirstChunk {
				fp.entries = nil
				if showUpEntry {
					fp.entries = []*fileEntry{{VFSItem: upItem}}
				}
				fp.SetCursorIndex(0)
			}

			if fp.pendingSelection != "" {
				fp.SelectName(fp.pendingSelection)
				fp.pendingSelection = ""
			}
			if freshCacheEquivalent {
				if benchmark != nil {
					benchmark.event("model.final.fresh_equivalent.skipped", "go.ui",
						"path", path, "entries", len(fp.entries), "chunks", chunkCount,
						"cursorIndex", fp.GetCursorIndex(), "cacheHit", true,
						"redraw", false, "semanticExport", false)
				}
			} else {
				fp.Refresh()
				if benchmark != nil {
					benchmark.event("model.final.ready", "go.ui", "phase", "fresh", "path", path,
						"entries", len(fp.entries), "chunks", chunkCount,
						"cursorIndex", fp.GetCursorIndex(), "cacheHit", hasCache)
					navigationBenchmarkPublishScene(benchmark, "fresh")
				}
			}
			if fp.benchmarkLoadTrace == benchmark {
				fp.benchmarkLoadTrace = nil
			}
			if !freshCacheEquivalent {
				vtui.FrameManager.Redraw()
			}
			needsRedraw = !freshCacheEquivalent
			return
		})
	})
}

func (fp *FileSystemPanel) Refresh() {
	idx := fp.GetCursorIndex()
	fp.updateSortColumnTitles()
	if fp.gridColumnCount() == 1 {
		rows := make([]vtui.TableRow, len(fp.entries))
		for i, e := range fp.entries {
			rows[i] = &panelEntryRow{fp: fp, entry: e}
		}
		fp.table.SetRows(rows)
	} else {
		rows := make([]vtui.TableRow, len(fp.entries))
		for i := 0; i < len(rows); i++ {
			rows[i] = &mediumRow{fp: fp, r: i}
		}
		fp.table.SetRows(rows)
	}
	fp.SetCursorIndex(idx)
	_, _, maxTop, _, _ := fp.panelScrollMetrics()
	if fp.table.TopPos > maxTop {
		fp.table.TopPos = maxTop
		fp.SetCursorIndex(idx)
	}
}

func (fp *FileSystemPanel) cursorSizeOnBottomBorder() string {
	if AppConfig.ShowPanelFileInfo || (fp.viewMode != ViewModeBrief && fp.viewMode != ViewModeMedium) {
		return ""
	}
	idx := fp.GetCursorIndex()
	if idx < 0 || idx >= len(fp.entries) {
		return ""
	}
	entry := fp.entries[idx]
	if entry == nil || entry.IsDir || entry.Name == ".." {
		return ""
	}
	return " " + formatIntWithSpaces(entry.Size) + " B "
}

func (fp *FileSystemPanel) Show(scr *vtui.ScreenBuf) {
	fp.frame.Show(scr)
	titleAttr := vtui.Palette[ColPanelTitle]
	if fp.IsFocused() || fp.showInactiveCursor {
		titleAttr = vtui.Palette[ColPanelSelectedTitle]
	}
	if fp.currentTitle != "" {
		availW := (fp.X2 - fp.X1) - 6
		if availW < 5 {
			availW = 5
		}
		displayTitle := fp.currentTitle
		if runewidth.StringWidth(displayTitle) > availW {
			displayTitle = vtui.TruncateMiddle(displayTitle, availW)
		}

		scr.Write(fp.X1+2, fp.Y1, vtui.StringToCharInfo(" ", titleAttr))
		scr.Write(fp.X1+3, fp.Y1, vtui.StringToCharInfo(displayTitle, titleAttr))
		scr.Write(fp.X1+3+runewidth.StringWidth(displayTitle), fp.Y1, vtui.StringToCharInfo(" ", titleAttr))
	}

	// Search-first keeps the active panel cursor visible while keyboard focus
	// is in the command line, but renders it with dedicated inactive colors.
	if fp.showInactiveCursor {
		fp.table.ColorSelectedTextIdx = ColPanelInactiveCursor
		fp.table.ColorItemSelectCursorIdx = ColPanelInactiveSelectedCursor
		fp.table.SetFocus(true)
	} else {
		fp.table.ColorSelectedTextIdx = ColPanelCursor
		fp.table.ColorItemSelectCursorIdx = ColPanelSelectedCursor
		fp.table.SetFocus(fp.IsFocused())
	}
	fp.table.Show(scr)
	fp.drawFastFindMatches(scr)
	fp.drawCursorSeparators(scr)
	fp.drawScrollBar(scr)

	if AppConfig.ShowPanelFileInfo && fp.Y2-fp.Y1+1 > 6 {
		p := vtui.NewPainter(scr)
		attrBox := vtui.Palette[ColPanelBox]
		// far2l paints the per-file status line with COL_PANELTEXT;
		// COL_PANELINFOTEXT (Panel.Text.Info) belongs to the info panel and
		// quick view, which use it in info_panel.go and quick_view_panel.go.
		attrInfo := vtui.Palette[ColPanelText]

		p.DrawLine(fp.X1+1, fp.Y2-2, fp.X2-1, fp.Y2-2, '─', attrBox, false, false)
		scr.Write(fp.X1, fp.Y2-2, vtui.StringToCharInfo("├", attrBox))
		scr.Write(fp.X2, fp.Y2-2, vtui.StringToCharInfo("┤", attrBox))

		p.Fill(fp.X1+1, fp.Y2-1, fp.X2-1, fp.Y2-1, ' ', attrInfo)

		idx := fp.GetCursorIndex()
		if idx >= 0 && idx < len(fp.entries) {
			e := fp.entries[idx]

			dateStr := e.MTime.Format("02.01.06 15:04")
			sizeStr := ""
			if e.IsDir {
				if e.SizeCalculated {
					sizeStr = formatIntWithSpaces(e.Size)
				} else if e.Name == ".." {
					sizeStr = "UP-DIR"
				} else {
					sizeStr = "<DIR>"
				}
			} else {
				sizeStr = formatIntWithSpaces(e.Size)
			}

			rightStr := fmt.Sprintf("%s  %s", sizeStr, dateStr)
			nameStr := e.Name

			availW := (fp.X2 - 1) - (fp.X1 + 1) + 1
			rightW := runewidth.StringWidth(rightStr)

			if availW > rightW+1 {
				nameStr = runewidth.Truncate(nameStr, availW-rightW-1, "")
			} else {
				nameStr = ""
				rightStr = runewidth.Truncate(rightStr, availW, "")
			}

			p.DrawString(fp.X1+1, fp.Y2-1, nameStr, attrInfo)
			if rightStr != "" {
				p.DrawString(fp.X2-runewidth.StringWidth(rightStr), fp.Y2-1, rightStr, attrInfo)
			}
		}
	}

	var selSize int64
	var selFiles int
	var selDirs int
	var totSize int64
	var totCount int

	for _, e := range fp.entries {
		if e.Name != ".." {
			totCount++
			if !e.IsDir {
				totSize += e.Size
			}
			if e.Selected {
				if e.IsDir {
					selDirs++
				} else {
					selFiles++
					selSize += e.Size
				}
			}
		}
	}

	totalStr := ""
	var attrTotal uint64
	if selFiles > 0 || selDirs > 0 {
		totalStr = fmt.Sprintf(" "+Msg("Panel.SelectedInfo")+" ", formatIntWithSpaces(selSize), selFiles, selDirs)
		attrTotal = vtui.Palette[ColPanelSelectedInfo]
	} else if totCount > 0 {
		totalStr = fmt.Sprintf(" %s (%d) ", formatIntWithSpaces(totSize), totCount)
		attrTotal = vtui.Palette[ColPanelTotalInfo]
	}

	totalStart := fp.X2
	if totalStr != "" {
		totalW := runewidth.StringWidth(totalStr)
		availBottom := fp.X2 - fp.X1 - 1
		if totalW < availBottom {
			totalStart = fp.X1 + 1 + (availBottom-totalW)/2
			p := vtui.NewPainter(scr)
			p.DrawString(totalStart, fp.Y2, totalStr, attrTotal)
		}
	}

	// The bottom frame now carries two numbers, so they must not read as one.
	// The panel total keeps the centre and its own colour; the entry under
	// the cursor is pinned to the left corner behind a ▸ marker. Both are in
	// exact bytes, the way far2l and the Size column spell them, and so is
	// the selected-files line. Directories say <DIR>/UP-DIR instead. When
	// the far2l status line is switched on it already states all of this
	// right above, so the marker steps aside.
	if !AppConfig.ShowPanelFileInfo && fp.gridColumnCount() > 1 {
		if idx := fp.GetCursorIndex(); idx >= 0 && idx < len(fp.entries) {
			e := fp.entries[idx]
			curStr := formatIntWithSpaces(e.Size)
			if e.IsDir && !e.SizeCalculated {
				curStr = "<DIR>"
				if e.Name == ".." {
					curStr = "UP-DIR"
				}
			}
			curStr = " ▸ " + curStr + " "
			if curW := runewidth.StringWidth(curStr); fp.X1+1+curW < totalStart {
				p := vtui.NewPainter(scr)
				p.DrawString(fp.X1+1, fp.Y2, curStr, vtui.Palette[ColPanelText])
			}
		}
	}

	if fp.fastFindMode {
		if vtui.ManageCursorStyle {
			os.Stdout.WriteString("\x1b[3 q") // Blinking underline
		}
		boxW := 24
		boxH := 3

		fx1 := fp.X1 + 9
		if fx1+boxW-1 >= scr.Width() {
			fx1 = scr.Width() - boxW
		}
		if fx1 < 0 {
			fx1 = 0
		}
		fx2 := fx1 + boxW - 1

		fy1 := fp.Y2 - 2
		if fy1 < 0 {
			fy1 = 0
		}
		fy2 := fy1 + boxH - 1

		p := vtui.NewPainter(scr)

		p.Fill(fx1, fy1, fx2, fy2, ' ', vtui.Palette[vtui.ColDialogText])
		p.DrawBox(fx1, fy1, fx2, fy2, vtui.Palette[vtui.ColDialogBox], vtui.DoubleBox)
		p.DrawTitle(fx1, fy1, fx2, Msg("Viewer.SearchTitle"), vtui.Palette[vtui.ColDialogBoxTitle])

		searchStr := fp.fastFindStr
		for runewidth.StringWidth(searchStr) > boxW-4 {
			runes := []rune(searchStr)
			searchStr = string(runes[1:])
		}

		searchColor := vtui.Palette[ColPanelFastFindNoMatch]
		if fp.fastFindHasMatches() {
			searchColor = vtui.Palette[vtui.ColMenuHighlight]
		}
		searchAttr := fastFindMatchAttr(vtui.Palette[vtui.ColDialogText], searchColor)
		p.DrawString(fx1+2, fy1+1, searchStr, searchAttr)

		scr.SetCursorPos(fx1+2+runewidth.StringWidth(searchStr), fy1+1)
		scr.SetCursorVisible(true)
	}
}

func (fp *FileSystemPanel) fastFindMatch(name string) (startRunes, matchedRunes int, ok bool) {
	if !fp.fastFindMode || fp.fastFindStr == "" {
		return 0, 0, false
	}
	queryText := fp.fastFindStr
	anywhere := strings.HasPrefix(queryText, "*")
	if anywhere {
		queryText = strings.TrimPrefix(queryText, "*")
	}
	if queryText == "" {
		return 0, 0, anywhere
	}
	// Matchers are cached per query text: the needle tables are built once
	// per keystroke, not once per visible row per redraw.
	if fp.fastFindMatcherKey != queryText {
		fp.fastFindMatcherKey = queryText
		fp.fastFindMatchers = fp.fastFindMatchers[:0]
		for _, query := range []string{
			queryText,
			vtui.GlobalXlator.TranscodeString(queryText),
		} {
			if m := vtui.NewFuzzyMatcher(query, false); m != nil {
				fp.fastFindMatchers = append(fp.fastFindMatchers, m)
			}
		}
	}
	bestScore := -1
	bestStart, bestEnd := 0, -1
	for _, m := range fp.fastFindMatchers {
		score, start, end, found := m.Match(name)
		if !found || (!anywhere && start != 0) {
			continue
		}
		if bestScore < 0 || score < bestScore || (score == bestScore && start < bestStart) {
			bestScore, bestStart, bestEnd = score, start, end
		}
	}
	if bestScore < 0 {
		return 0, 0, false
	}
	return bestStart, bestEnd - bestStart + 1, true
}

func (fp *FileSystemPanel) fastFindHasMatches() bool {
	for _, entry := range fp.entries {
		if _, _, ok := fp.fastFindMatch(entry.Name); ok {
			return true
		}
	}
	return false
}

func fastFindMatchAttr(baseAttr, matchAttr uint64) uint64 {
	if matchAttr&vtui.IsFgRGB != 0 {
		return vtui.SetRGBFore(baseAttr, vtui.GetRGBFore(matchAttr))
	}
	return vtui.SetIndexFore(baseAttr, vtui.GetIndexFore(matchAttr))
}

func (fp *FileSystemPanel) drawFastFindMatches(scr *vtui.ScreenBuf) {
	if !fp.fastFindMode || fp.fastFindStr == "" || !fp.table.IsVisible() {
		return
	}
	height := fp.table.ViewHeight
	if height <= 0 {
		return
	}
	columns := fp.gridColumnCount()
	matchAttr := vtui.Palette[ColPanelHighlightText]

	for rowOffset := 0; rowOffset < height; rowOffset++ {
		row := fp.table.TopPos + rowOffset
		y := fp.table.Y1 + fp.table.MarginTop + rowOffset
		x := fp.table.X1
		for column := 0; column < columns && column < len(fp.table.Columns); column++ {
			entryIndex := row
			if columns > 1 {
				entryIndex += column * height
			}
			cellWidth := fp.table.Columns[column].Width
			if entryIndex >= 0 && entryIndex < len(fp.entries) {
				entry := fp.entries[entryIndex]
				matchStart, matchedRunes, _ := fp.fastFindMatch(entry.Name)
				for _, span := range panelFileNameMatchSpans(entry, cellWidth, matchStart, matchedRunes) {
					for cellOffset := 0; cellOffset < span.width; cellOffset++ {
						cell := scr.GetCell(x+span.start+cellOffset, y)
						cell.Attributes = fastFindMatchAttr(cell.Attributes, matchAttr)
						scr.Write(x+span.start+cellOffset, y, []vtui.CharInfo{cell})
					}
				}
			}
			x += cellWidth + 1
		}
	}
}

func (fp *FileSystemPanel) SetPosition(x1, y1, x2, y2 int) {
	fp.ScreenObject.SetPosition(x1, y1, x2, y2)
	fp.frame.SetPosition(x1, y1, x2, y2)
	// Table stays inside the frame, reserving space for status info only when enabled.
	if AppConfig.ShowPanelFileInfo && y2-y1+1 > 6 {
		fp.table.SetPosition(x1+1, y1+1, x2-1, y2-3)
	} else {
		fp.table.SetPosition(x1+1, y1+1, x2-1, y2-1)
	}
}

func (fp *FileSystemPanel) Resize(w, h int) {
	fp.SetPosition(fp.X1, fp.Y1, fp.X1+w-1, fp.Y1+h-1)

	switch fp.effectiveViewMode() {
	case ViewModeWide:
		nameW := w - 2 - 2 - panelSizeColumnWidth - panelModifiedColumnWidth
		if nameW < 1 {
			nameW = 1
		}
		fp.table.Columns = []vtui.TableColumn{
			{Title: Msg("Panel.Column.Name"), Width: nameW},
			{Title: Msg("Panel.Column.Size"), Width: panelSizeColumnWidth, Alignment: vtui.AlignRight},
			{Title: Msg("Panel.Column.Modified"), Width: panelModifiedColumnWidth},
		}
	case ViewModeDetailed:
		// The panel's inner table is w-2 characters wide. The size column
		// consumes 11 and its separator consumes 1, leaving w-14 for Name.
		nameW := w - 14
		if nameW < 5 {
			nameW = 5
		}
		fp.table.Columns = []vtui.TableColumn{
			{Title: Msg("Panel.Column.Name"), Width: nameW},
			{Title: Msg("Panel.Column.Size"), Width: panelSizeColumnWidth, Alignment: vtui.AlignRight},
		}
	default:
		columnCount := fp.gridColumnCount()
		available := w - 2 - (columnCount - 1)
		if available < columnCount {
			available = columnCount
		}
		columns := make([]vtui.TableColumn, columnCount)
		remaining := available
		for i := range columns {
			width := remaining / (columnCount - i)
			if width < 1 {
				width = 1
			}
			columns[i] = vtui.TableColumn{Title: Msg("Panel.Column.Name"), Width: width}
			remaining -= width
		}
		fp.table.Columns = columns
	}
	fp.updateSortColumnTitles()
	fp.Refresh()
}

// isShiftSelectNavKey reports whether a virtual key is a navigation
// key that participates in the Shift+nav selection session. The set
// mirrors the nav switch below so any change stays in sync.
func isShiftSelectNavKey(vk uint16) bool {
	switch vk {
	case vtinput.VK_UP, vtinput.VK_DOWN, vtinput.VK_LEFT, vtinput.VK_RIGHT,
		vtinput.VK_PRIOR, vtinput.VK_NEXT, vtinput.VK_HOME, vtinput.VK_END:
		return true
	}
	return false
}

// shiftSelectDirection returns +1 for keys that move the cursor
// forward (Down/Right/PgDn/End) and -1 for backward (Up/Left/
// PgUp/Home). Used to look past a ".." starting row so
// session-mode detection can still see a real, selectable row.
func shiftSelectDirection(vk uint16) int {
	switch vk {
	case vtinput.VK_DOWN, vtinput.VK_RIGHT, vtinput.VK_NEXT, vtinput.VK_END:
		return 1
	case vtinput.VK_UP, vtinput.VK_LEFT, vtinput.VK_PRIOR, vtinput.VK_HOME:
		return -1
	}
	return 0
}

func fastFindRune(e *vtinput.InputEvent, _ bool) rune {
	if e == nil {
		return 0
	}
	return e.Char
}

func (fp *FileSystemPanel) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown {
		return false
	}
	if fp.providerOpenTask != nil {
		// A provider row has started an asynchronous mount, but there is no
		// child VFS to navigate yet. Autorepeat Enter must be idempotent here;
		// once the real child is installed, later repeats may navigate it.
		if e.VirtualKeyCode == vtinput.VK_RETURN {
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_ESCAPE {
			fp.cancelProviderOpenAndRestore()
			return true
		}
	}

	shift := (e.ControlKeyState & vtinput.ShiftPressed) != 0

	alt := (e.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0
	ctrl := (e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0

	// Detailed view has no horizontal cell navigation. Outside Vim mode,
	// reuse plain Left/Right as Page Up/Page Down while preserving the rest
	// of the event (notably Shift selection).
	if fp.viewMode == ViewModeDetailed && AppConfig.NavigationMode != NavigationVim && !ctrl && !alt &&
		(e.VirtualKeyCode == vtinput.VK_LEFT || e.VirtualKeyCode == vtinput.VK_RIGHT) {
		mapped := *e
		if e.VirtualKeyCode == vtinput.VK_LEFT {
			mapped.VirtualKeyCode = vtinput.VK_PRIOR
		} else {
			mapped.VirtualKeyCode = vtinput.VK_NEXT
		}
		e = &mapped
	}

	// Close the shift-selection session on anything other than a
	// Shift+nav key so the next Shift+nav re-decides its mode
	// from the row under the cursor.
	if !(shift && isShiftSelectNavKey(e.VirtualKeyCode)) {
		fp.shiftSessionActive = false
	}

	if fp.fastFindMode {
		if e.VirtualKeyCode == vtinput.VK_UP || e.VirtualKeyCode == vtinput.VK_DOWN {
			fp.fastFindMode = false
			fp.fastFindStr = ""
			vtui.FrameManager.Redraw()
			// Reprocess the key as ordinary panel navigation now that Fast Find
			// no longer owns it.
			return fp.ProcessKey(e)
		}
		switch e.VirtualKeyCode {
		case vtinput.VK_LEFT, vtinput.VK_RIGHT, vtinput.VK_PRIOR, vtinput.VK_NEXT, vtinput.VK_HOME, vtinput.VK_END:
			fp.fastFindMode = false
			fp.fastFindStr = ""
			vtui.FrameManager.Redraw()
			// Проваливаемся дальше, чтобы обработать саму навигацию
		case vtinput.VK_ESCAPE:
			fp.fastFindMode = false
			fp.fastFindStr = ""
			vtui.FrameManager.Redraw()
			return true
		case vtinput.VK_DELETE:
			// Fast Find keeps its insertion point at the end, so forward Delete
			// has nothing to remove. Consume it here rather than allowing the
			// panel-level Del binding to hide the panels.
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_F2 && !shift && !ctrl && !alt {
			if strings.HasPrefix(fp.fastFindStr, "*") {
				fp.fastFindStr = strings.TrimPrefix(fp.fastFindStr, "*")
			} else {
				fp.fastFindStr = "*" + fp.fastFindStr
			}
			if fp.fastFindStr == "" {
				fp.fastFindMode = false
			} else {
				fp.doFastFind(0)
			}
			vtui.FrameManager.Redraw()
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_BACK {
			if len(fp.fastFindStr) > 0 {
				runes := []rune(fp.fastFindStr)
				fp.fastFindStr = string(runes[:len(runes)-1])
				if len(fp.fastFindStr) == 0 {
					fp.fastFindMode = false
				} else {
					fp.doFastFind(0)
				}
			}
			vtui.FrameManager.Redraw()
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_UP {
			fp.doFastFind(-1)
			vtui.FrameManager.Redraw()
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_DOWN {
			fp.doFastFind(1)
			vtui.FrameManager.Redraw()
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_RETURN && ctrl && !alt {
			dir := 1
			if shift {
				dir = -1
			}
			fp.doFastFind(dir)
			vtui.FrameManager.Redraw()
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_RETURN {
			fp.fastFindMode = false
			fp.fastFindStr = ""
			vtui.FrameManager.Redraw()
			// Проваливаемся ниже, чтобы обработать Enter как вход в файл/директорию
		} else if r := fastFindRune(e, alt); r != 0 && !ctrl && unicode.IsPrint(r) {
			fp.fastFindStr += string(unicode.ToLower(r))
			fp.doFastFind(0)
			vtui.FrameManager.Redraw()
			return true
		}
	} else {
		searchFirstInput := AppConfig.NavigationMode == NavigationSearchFirst && fp.IsFocused() && !alt
		if r := fastFindRune(e, alt); r != 0 && (alt || searchFirstInput) && !ctrl && unicode.IsPrint(r) {
			fp.fastFindMode = true
			fp.fastFindStr = string(unicode.ToLower(r))
			fp.doFastFind(0)
			vtui.FrameManager.Redraw()
			return true
		}
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_INSERT:
		if shift || ctrl || alt {
			return false
		}
		idx := fp.GetCursorIndex()
		fp.ToggleSelection(idx)
		fp.SetCursorIndex(idx + 1)
		return true

	case vtinput.VK_UP, vtinput.VK_DOWN, vtinput.VK_LEFT, vtinput.VK_RIGHT, vtinput.VK_PRIOR, vtinput.VK_NEXT, vtinput.VK_HOME, vtinput.VK_END:
		// FAR-style Shift+nav selection.
		//
		// The session concept unifies "select" and "deselect"
		// across every navigation key: the first Shift+nav decides
		// the mode from the state of the row under the cursor
		// (unselected → we're selecting; selected → we're
		// deselecting), and every subsequent Shift+nav in the same
		// session applies that same mode. Releasing Shift (or any
		// non-nav key) closes the session; the next Shift+nav
		// starts a new one, potentially with the opposite mode.
		//
		// Range width is per-key: Up/Down affect just the starting
		// row (session grows by one row per tap); Left/Right in
		// grid, PgUp/PgDn, Home/End paint the whole [start..new]
		// sweep. ".." is skipped inside SetItemSelected.
		startIdx := fp.GetCursorIndex()

		if shift {
			if !fp.shiftSessionActive {
				fp.shiftSessionActive = true
				// Decide session mode from the state of the row
				// under the cursor. If it's ".." (never selectable),
				// look past it in the direction of movement to
				// find the first real row — otherwise the ".."
				// start would always resolve to "select" and users
				// couldn't clear a selection with Shift+End/etc
				// from the top of the list.
				modeIdx := startIdx
				if startIdx >= 0 && startIdx < len(fp.entries) &&
					fp.entries[startIdx].Name == ".." {
					if dir := shiftSelectDirection(e.VirtualKeyCode); dir != 0 {
						for i := startIdx + dir; i >= 0 && i < len(fp.entries); i += dir {
							if fp.entries[i].Name != ".." {
								modeIdx = i
								break
							}
						}
					}
				}
				if modeIdx >= 0 && modeIdx < len(fp.entries) &&
					fp.entries[modeIdx].Name != ".." &&
					fp.entries[modeIdx].Selected {
					fp.shiftSessionMode = false
				} else {
					fp.shiftSessionMode = true
				}
			}
			fp.SetItemSelected(startIdx, fp.shiftSessionMode)
		}

		isMultiStep := false
		switch e.VirtualKeyCode {
		case vtinput.VK_LEFT, vtinput.VK_RIGHT, vtinput.VK_PRIOR, vtinput.VK_NEXT, vtinput.VK_HOME, vtinput.VK_END:
			isMultiStep = true
		}

		idx := startIdx
		H := fp.table.ViewHeight
		if H <= 0 {
			H = 1
		}

		handled := false
		if columns := fp.gridColumnCount(); columns > 1 {
			switch e.VirtualKeyCode {
			case vtinput.VK_UP:
				idx--
			case vtinput.VK_DOWN:
				idx++
			case vtinput.VK_LEFT:
				idx -= H
			case vtinput.VK_RIGHT:
				idx += H
			case vtinput.VK_PRIOR:
				idx -= H * columns
			case vtinput.VK_NEXT:
				idx += H * columns
			case vtinput.VK_HOME:
				idx = 0
			case vtinput.VK_END:
				idx = len(fp.entries) - 1
			default:
				return false
			}
			fp.SetCursorIndex(idx)
			handled = true
		} else {
			// In Detailed mode, we let the table handle navigation but sync our index back
			if fp.table.ProcessKey(e) {
				fp.cursorIdx = fp.table.SelectPos
				handled = true
			}
		}

		if shift && handled && isMultiStep {
			newIdx := fp.GetCursorIndex()
			lo, hi := startIdx, newIdx
			if lo > hi {
				lo, hi = hi, lo
			}
			for i := lo; i <= hi; i++ {
				fp.SetItemSelected(i, fp.shiftSessionMode)
			}
		}
		return handled

	case vtinput.VK_RETURN:
		idx := fp.GetCursorIndex()
		if idx >= 0 && idx < len(fp.entries) {
			selected := fp.entries[idx]

			// Logic for leaving a virtual VFS (like an archive)
			if selected.Name == ".." {

				parent := fp.vfs.ParentVFS()
				isRoot := fp.vfs.IsAtRoot()

				if isRoot {
					if parent != nil {
						oldPath := fp.vfs.GetPath()

						fp.cancelProviderOpen()
						// Закрываем текущую систему (удаляем временные файлы)
						fp.vfs.Close()

						fp.vfs = parent
						if fp.providerEntryName != "" {
							fp.pendingSelection = fp.providerEntryName
							fp.providerEntryName = ""
						} else {
							fp.pendingSelection = fp.vfs.Base(oldPath)
						}
						fp.showCurrentVFSLoadingRows()
						fp.ReadDirectory()
					}
					// A root without a parent has nowhere to go. This is also a
					// final safety net for a stale ".." row left by an asynchronous
					// VFS transition: never turn it into manager.Join(root, "..").
					return true
				}
			}

			// A provider transition can represent a virtual directory just as well
			// as an archive-like file. Try it before ordinary SetPath navigation so
			// those rows can truthfully use IsDir and receive directory rendering.
			fullPath := fp.vfs.Join(fp.vfs.GetPath(), selected.Name)
			provider := vfs.FindProvider(context.Background(), fp.vfs, fullPath)
			if selected.IsDir && provider != nil {
				directoryProvider, ok := provider.(vfs.VirtualDirectoryProvider)
				if !ok || !directoryProvider.OpensVirtualDirectories() {
					provider = nil
				}
			}
			if provider != nil {
				sourceVFS := fp.vfs
				selectedName := selected.Name
				status, showStatus := vfs.ProviderOpenStatus{}, false
				if statusProvider, ok := provider.(vfs.ProviderOpenStatusProvider); ok {
					status, showStatus = statusProvider.ProviderOpenStatus(sourceVFS, fullPath)
				}
				started := fp.openVFSAsync(
					"",
					func(ctx context.Context) (vfs.VFS, error) {
						newVFS, err := provider.Open(ctx, sourceVFS, fullPath)
						if err == nil && newVFS == nil {
							err = fmt.Errorf("provider %s returned no file system", provider.Name())
						}
						return newVFS, err
					},
					func(newVFS vfs.VFS) {
						fp.providerEntryName = selectedName
						fp.vfs = newVFS
						fp.pendingSelection = ".."
						fp.showCurrentVFSLoadingRows()
						fp.ReadDirectory()
					},
					func(err error) {
						fp.pendingSelection = selectedName
						vtui.ShowMessage(" Connection Error ", fmt.Sprintf("Failed to connect to %s:\n%v", selectedName, err), []string{"&Ok"})
					},
				)
				if started && showStatus {
					fp.showProviderOpenDialog(status)
				}
				return started
			}

			if selected.IsDir {
				oldPath := fp.vfs.GetPath()
				newPath := fp.vfs.Join(oldPath, selected.Name)
				vtui.DebugLog("PANEL: Navigating %q -> %q", oldPath, newPath)
				if err := fp.setKnownDirectoryPath(newPath); err == nil {
					if selected.Name == ".." {
						fp.pendingSelection = fp.vfs.Base(oldPath)
					} else {
						fp.pendingSelection = ".."
					}
					fp.ReadDirectory()
					return true
				} else {
					vtui.FrameManager.PostTask(func() {
						vtui.ShowMessage(" Error ", fmt.Sprintf("Cannot access folder:\n%v", err), []string{"&Ok"})
					})
					return true
				}
			}
		}
	}

	return false
}

func (fp *FileSystemPanel) ProcessMouse(e *vtinput.InputEvent) bool {
	if e.Type != vtinput.MouseEventType {
		return false
	}
	if fp.providerOpenTask != nil {
		// The visible rows belong to the destination cache while fp.vfs still
		// points at the source. Consume panel mouse input until the switch so a
		// double-click/context action cannot run against the wrong filesystem.
		return true
	}

	isMove := e.MouseEventFlags&vtinput.MouseMoved != 0
	isRelease := !isMove && (e.ButtonState == 0 || !e.KeyDown)
	if isRelease {
		fp.lastRightClickedIdx = -1
		fp.rightDragActive = false
		fp.headerMouseActive = false
		fp.rowDragButton = 0
		fp.stopDragAutoScroll()
	}

	if e.WheelDirection == 0 && e.ButtonState&vtinput.FromLeft1stButtonPressed != 0 &&
		e.KeyDown && e.MouseEventFlags&vtinput.MouseMoved == 0 {
		if mode, ok := fp.headerSortModeAt(int(e.MouseX), int(e.MouseY)); ok {
			fp.headerMouseActive = true
			fp.SetSortMode(mode)
			return true
		}
	}

	if fp.processScrollBarMouse(e) {
		return true
	}

	if isMove && fp.rowDragButton != 0 {
		if fp.updateDragAutoScroll(int(e.MouseY)) {
			return true
		}
	}

	if fp.fastFindMode && e.ButtonState != 0 {
		fp.fastFindMode = false
		fp.fastFindStr = ""
		vtui.FrameManager.Redraw()
	}

	if e.WheelDirection != 0 {
		// Determine direction: up is -1, down is 1
		direction := 1
		speed := AppConfig.WheelPanelDown
		if e.WheelDirection > 0 {
			direction = -1
			speed = AppConfig.WheelPanelUp
		}
		step := direction * wheelScrollLines(speed)

		H := fp.table.ViewHeight
		if H <= 0 {
			H = 1
		}

		if fp.gridColumnCount() == 1 {
			// Detailed view (1-column)
			idx := fp.GetCursorIndex()
			newIdx := idx + step
			if newIdx < 0 {
				newIdx = 0
			}
			if newIdx >= len(fp.entries) {
				newIdx = len(fp.entries) - 1
			}

			// Scroll the list if possible, keeping the cursor visually stable
			newTop := fp.table.TopPos + step
			maxTop := len(fp.entries) - H
			if maxTop < 0 {
				maxTop = 0
			}
			if newTop < 0 {
				newTop = 0
			}
			if newTop > maxTop {
				newTop = maxTop
			}

			fp.table.TopPos = newTop
			fp.SetCursorIndex(newIdx)
			fp.Refresh()
			return true
		} else {
			// Medium/Brief grid view.
			idx := fp.GetCursorIndex()
			newIdx := idx + step
			if newIdx < 0 {
				newIdx = 0
			}
			if newIdx >= len(fp.entries) {
				newIdx = len(fp.entries) - 1
			}

			// Scroll the list if possible, keeping the cursor visually stable
			newTop := fp.table.TopPos + step
			maxTop := len(fp.entries) - fp.gridColumnCount()*H
			if maxTop < 0 {
				maxTop = 0
			}
			if newTop < 0 {
				newTop = 0
			}
			if newTop > maxTop {
				newTop = maxTop
			}

			fp.table.TopPos = newTop
			fp.SetCursorIndex(newIdx)
			fp.Refresh()
			return true
		}
	}

	isRightDragMove := isMove && fp.rightDragActive &&
		(e.ButtonState&vtinput.RightmostButtonPressed != 0 || e.ButtonState == 0)
	if e.ButtonState == vtinput.RightmostButtonPressed && e.KeyDown || isRightDragMove {
		idx := fp.mouseEntryIndex(int(e.MouseX), int(e.MouseY))
		if idx >= 0 {
			fp.rowDragButton = vtinput.RightmostButtonPressed
			fp.SetCursorIndex(idx)
			if fp.entries[idx].Name == ".." {
				return true
			}
			if e.MouseEventFlags&vtinput.DoubleClick != 0 {
				// Windows reports the second press as a double-click. The first
				// press has already established whether this is a select or
				// deselect operation, so propagate its result to the whole panel.
				state := fp.entries[idx].Selected
				fp.setAllItemsSelected(state)
				fp.rightDragActive = true
				fp.rightDragSelect = state
				fp.lastRightClickedIdx = idx
				fp.Refresh()
				return true
			}
			fp.processRightDrag(idx)
			fp.Refresh()
			return true
		}
		// Keep the drag captured while the pointer temporarily leaves a file row.
		return fp.rightDragActive
	}

	handled := fp.table.ProcessMouse(e)
	if handled {
		if e.KeyDown && !isMove && e.ButtonState&vtinput.FromLeft1stButtonPressed != 0 {
			fp.rowDragButton = vtinput.FromLeft1stButtonPressed
		}
		// Sync absolute index from table's visual selection
		if fp.gridColumnCount() == 1 {
			fp.cursorIdx = fp.table.SelectPos
		} else {
			H := fp.table.ViewHeight
			if H <= 0 {
				H = 1
			}
			// SelectPos is already absolute (TopPos + row) in Medium mode,
			// so we just add the column offset.
			newIdx := fp.table.SelectPos + fp.table.SelectCol*H

			// Fix for "click in empty space": if we selected an empty slot,
			// snap to the last valid entry.
			if newIdx >= len(fp.entries) {
				fp.SetCursorIndex(len(fp.entries) - 1)
			} else {
				fp.cursorIdx = newIdx
			}
		}
	}

	return handled
}

func (fp *FileSystemPanel) GetSelectedName() string {
	idx := fp.GetCursorIndex()
	if len(fp.entries) == 0 || idx < 0 || idx >= len(fp.entries) {
		return ""
	}
	entry := fp.entries[idx]
	if entry.Name == ".." {
		return fp.vfs.Dir(fp.vfs.GetPath())
	}
	return entry.Name
}

func (fp *FileSystemPanel) getRawSelectedName() string {
	idx := fp.GetCursorIndex()
	if len(fp.entries) == 0 || idx < 0 || idx >= len(fp.entries) {
		return ""
	}
	return fp.entries[idx].Name
}

// SetSelectedByName picks or unpicks an entry by name and reports whether the
// panel shows such an entry at all. It is how the picture gallery keeps the
// panel underneath in step with what the reader has picked; a panel that has
// walked away to another directory simply answers no.
func (fp *FileSystemPanel) SetSelectedByName(name string, state bool) bool {
	for i, e := range fp.entries {
		if e.Name == name {
			fp.SetItemSelected(i, state)
			return true
		}
	}
	return false
}

// IsNameSelected reports whether an entry has been picked explicitly.
func (fp *FileSystemPanel) IsNameSelected(name string) bool {
	return fp.selectedItems[name]
}

// panelSelectionToken identifies one explicit mark without giving an async
// Apply batch permission to undo a later user action on the same filename.
type panelSelectionToken struct {
	panel          *FileSystemPanel
	vfs            vfs.VFS
	path           string
	name           string
	directoryEpoch uint64
	selectionEpoch uint64
}

func (fp *FileSystemPanel) captureSelectionToken(name string) (panelSelectionToken, bool) {
	if fp == nil || !fp.IsNameSelected(name) || fp.vfs == nil {
		return panelSelectionToken{}, false
	}
	return panelSelectionToken{
		panel:          fp,
		vfs:            fp.vfs,
		path:           fp.vfs.GetPath(),
		name:           name,
		directoryEpoch: fp.directoryEpoch,
		selectionEpoch: fp.selectionEpoch[name],
	}, true
}

func (fp *FileSystemPanel) clearSelectionIfUnchanged(token panelSelectionToken) bool {
	if fp == nil || token.panel != fp || fp.vfs == nil || !sameVFSInstance(fp.vfs, token.vfs) || fp.vfs.GetPath() != token.path ||
		fp.directoryEpoch != token.directoryEpoch || fp.selectionEpoch[token.name] != token.selectionEpoch ||
		!fp.IsNameSelected(token.name) {
		return false
	}
	return fp.SetSelectedByName(token.name, false)
}

// ImageSiblings lists the pictures of this panel in the order it shows them,
// together with the position of the one under the cursor, or minus one when
// the cursor is not on a picture.
func (fp *FileSystemPanel) ImageSiblings() ([]string, int) {
	current := fp.getRawSelectedName()
	names := make([]string, 0, len(fp.entries))
	index := -1
	for _, e := range fp.entries {
		if e.IsDir || e.Name == ".." || !IsImageFile(e.Name) {
			continue
		}
		if e.Name == current {
			index = len(names)
		}
		names = append(names, e.Name)
	}
	return names, index
}

// SelectName searches for an entry by name and moves the cursor to it.
func (fp *FileSystemPanel) SelectName(name string) {
	for i, entry := range fp.entries {
		if entry.Name == name {
			fp.SetCursorIndex(i)
			fp.Refresh()
			break
		}
	}
}

// GetSelectedNames returns a list of selected files. If none are selected, returns the focused one.
func (fp *FileSystemPanel) GetSelectedNames() []string {
	var names []string
	// 1. Collect explicitly selected items (ins/shift+arrows)
	for _, e := range fp.entries {
		if e.Selected && e.Name != ".." {
			names = append(names, e.Name)
		}
	}
	// 2. If nothing is selected, fallback to the item under cursor
	if len(names) == 0 {
		idx := fp.GetCursorIndex()
		if idx >= 0 && idx < len(fp.entries) {
			entry := fp.entries[idx]
			// CRITICAL: Prevent actions on parent directory ".."
			if entry.Name != ".." {
				names = append(names, entry.Name)
			}
		}
	}
	return names
}

// GetMarkedNames returns only explicitly marked panel items, in panel order.
// Unlike GetSelectedNames it deliberately does not fall back to the cursor.
func (fp *FileSystemPanel) GetMarkedNames() []string {
	names := make([]string, 0)
	for _, entry := range fp.entries {
		if entry.Selected && entry.Name != ".." {
			names = append(names, entry.Name)
		}
	}
	return names
}

// ReplaceMarkedNames atomically replaces the explicit panel selection.
func (fp *FileSystemPanel) ReplaceMarkedNames(names []string) {
	selected := make(map[string]struct{}, len(names))
	for _, name := range names {
		selected[name] = struct{}{}
	}
	for i, entry := range fp.entries {
		_, state := selected[entry.Name]
		fp.SetItemSelected(i, state)
	}
	fp.Refresh()
}

// GetSuccessorName determines which file should receive focus after the current
// selection (or focused item) is deleted or moved.
func (fp *FileSystemPanel) doFastFind(dir int) {
	if fp.fastFindStr == "" {
		return
	}
	startIdx := fp.GetCursorIndex()

	checkMatch := func(i int) bool {
		_, _, ok := fp.fastFindMatch(fp.entries[i].Name)
		return ok
	}

	if dir == 0 {
		for i := startIdx; i < len(fp.entries); i++ {
			if checkMatch(i) {
				fp.SetCursorIndex(i)
				fp.Refresh()
				return
			}
		}
		for i := 0; i < startIdx; i++ {
			if checkMatch(i) {
				fp.SetCursorIndex(i)
				fp.Refresh()
				return
			}
		}
	} else if dir == 1 {
		for i := startIdx + 1; i < len(fp.entries); i++ {
			if checkMatch(i) {
				fp.SetCursorIndex(i)
				fp.Refresh()
				return
			}
		}
		for i := 0; i <= startIdx; i++ {
			if checkMatch(i) {
				fp.SetCursorIndex(i)
				fp.Refresh()
				return
			}
		}
	} else if dir == -1 {
		for i := startIdx - 1; i >= 0; i-- {
			if checkMatch(i) {
				fp.SetCursorIndex(i)
				fp.Refresh()
				return
			}
		}
		for i := len(fp.entries) - 1; i >= startIdx; i-- {
			if checkMatch(i) {
				fp.SetCursorIndex(i)
				fp.Refresh()
				return
			}
		}
	}
}

// SaveSelection snapshots the current selection. The name map is the durable
// source of truth: fileEntry rows are rebuilt by asynchronous refreshes, so a
// row-only snapshot would make Ctrl+M forget Apply's original marks.
func (fp *FileSystemPanel) SaveSelection() {
	previous := make(map[string]bool)
	for _, e := range fp.entries {
		e.PrevSelected = e.Selected
		if e.Name != ".." && e.Selected {
			previous[e.Name] = true
		}
	}
	fp.previousSelection = previous
	fp.previousSelectionVFS = fp.vfs
	if fp.vfs != nil {
		fp.previousSelectionPath = fp.vfs.GetPath()
	} else {
		fp.previousSelectionPath = ""
	}
}

// RestoreSelection swaps the current selection with the last snapshot taken
// by SaveSelection. Because the swap goes both ways, pressing Ctrl+M twice
// returns to the state you started from. Mirrors far2l's
// FileList::RestoreSelection().
func (fp *FileSystemPanel) RestoreSelection() {
	saved := fp.previousSelection
	if fp.vfs == nil || !fp.previousSelectionMatches(fp.vfs, fp.vfs.GetPath()) {
		saved = nil
	}
	current := make(map[string]bool)
	for i, e := range fp.entries {
		if e.Name == ".." {
			continue
		}
		if e.Selected {
			current[e.Name] = true
		}
		e.PrevSelected = e.Selected
		fp.SetItemSelected(i, saved[e.Name])
	}
	fp.previousSelection = current
	fp.previousSelectionVFS = fp.vfs
	if fp.vfs != nil {
		fp.previousSelectionPath = fp.vfs.GetPath()
	} else {
		fp.previousSelectionPath = ""
	}
	vtui.FrameManager.Redraw()
}

func (fp *FileSystemPanel) InvertSelection() {
	fp.SaveSelection()
	for i, e := range fp.entries {
		if e.Name != ".." {
			fp.SetItemSelected(i, !e.Selected)
		}
	}
	vtui.FrameManager.Redraw()
}

func (fp *FileSystemPanel) ApplyMaskSelection(mask string, state bool) {
	if mask == "" {
		return
	}
	// Far style: *.* matches everything
	if mask == "*.*" {
		mask = "*"
	}
	maskLower := strings.ToLower(mask)

	fp.SaveSelection()
	for i, e := range fp.entries {
		if e.Name == ".." {
			continue
		}
		nameLower := strings.ToLower(e.Name)
		matched, _ := filepath.Match(maskLower, nameLower)
		if matched {
			fp.SetItemSelected(i, state)
		}
	}
	vtui.FrameManager.Redraw()
}

func (fp *FileSystemPanel) GetSuccessorName() string {
	if len(fp.entries) <= 1 {
		return ".."
	}

	anySelected := false
	for _, e := range fp.entries {
		if e.Selected && e.Name != ".." {
			anySelected = true
			break
		}
	}

	var firstIdx, lastIdx int

	if anySelected {
		// If something is selected, we only care about the selection range
		firstIdx = len(fp.entries)
		lastIdx = -1
		for i, e := range fp.entries {
			if e.Selected && e.Name != ".." {
				if i < firstIdx {
					firstIdx = i
				}
				if i > lastIdx {
					lastIdx = i
				}
			}
		}
	} else {
		// If nothing selected, the "range" is just the current cursor
		firstIdx = fp.cursorIdx
		lastIdx = fp.cursorIdx
	}

	// Helper to check if an item at index i is about to be removed
	isToBeRemoved := func(i int) bool {
		if anySelected {
			return fp.entries[i].Selected && fp.entries[i].Name != ".."
		}
		return i == fp.cursorIdx
	}

	// 1. Try to find the first valid item AFTER the removed block
	for i := lastIdx + 1; i < len(fp.entries); i++ {
		if !isToBeRemoved(i) {
			return fp.entries[i].Name
		}
	}

	// 2. If no "next" item, try to find the first valid item BEFORE the removed block
	for i := firstIdx - 1; i >= 0; i-- {
		if !isToBeRemoved(i) && fp.entries[i].Name != ".." {
			return fp.entries[i].Name
		}
	}

	// 3. Fallback to parent directory entry
	return ".."
}
