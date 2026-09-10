package panel

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
	"github.com/unxed/f4/internal/history"
	"github.com/unxed/f4/internal/sysinfo"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/media"
	"github.com/unxed/f4/internal/navtrace"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"runtime"
	"slices"
	"unicode/utf8"
)

// FileEntry implements vtui.TableRow for display in a table.
type FileEntry struct {
	vfs.VFSItem
	Selected       bool
	PrevSelected   bool // snapshot of Selected taken by SaveSelection; swapped in by RestoreSelection (Ctrl+M)
	SizeCalculated bool
}

// directoryFileEntryWriter is the one authoritative source allocation used by
// the windowed local-directory path. VFS fills it after publishing the exact
// bounded window, avoiding a second full []VFSItem copy of the same rows.
type directoryFileEntryWriter struct {
	backing   []FileEntry
	entries   []*FileEntry
	completed bool
}

func (writer *directoryFileEntryWriter) BeginDirectoryItems(total int) {
	writer.backing = make([]FileEntry, max(0, total))
	writer.entries = make([]*FileEntry, max(0, total))
	writer.completed = false
}

func (writer *directoryFileEntryWriter) WriteDirectoryItem(
	index int,
	item vfs.VFSItem,
) {
	if index < 0 || index >= len(writer.backing) {
		return
	}
	writer.backing[index].VFSItem = item
	writer.entries[index] = &writer.backing[index]
}

func (writer *directoryFileEntryWriter) EndDirectoryItems() {
	writer.completed = true
}

type mediumRow struct {
	fp *FileSystemPanel
	r  int
}

type panelEntryRow struct {
	fp    *FileSystemPanel
	entry *FileEntry
}

type panelMatchSpan struct {
	start int
	width int
}

func (f *FileEntry) visibleName() string {
	if f == nil {
		return ""
	}
	if f.DisplayName != "" && f.Name != ".." {
		return f.DisplayName
	}
	return f.Name
}

func (r *panelEntryRow) GetCellText(col int) string {
	if col == 0 && len(r.fp.Table.Columns) > 0 {
		return formatPanelFileName(r.entry, r.fp.Table.Columns[0].Width)
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
	h := m.fp.Table.ViewHeight
	if h <= 0 {
		h = 1
	}
	idx := m.r + col*h
	if idx >= len(m.fp.Entries) {
		return ""
	}
	e := m.fp.Entries[idx]
	width := 0
	if col >= 0 && col < len(m.fp.Table.Columns) {
		width = m.fp.Table.Columns[col].Width
	}
	return formatPanelFileNameAt(e, width, m.fp.nameLeftPos)
}

func (fp *FileSystemPanel) RowCount() int {
	return len(fp.Entries)
}

func (fp *FileSystemPanel) GetCellText(row, col int) string {
	if fp.gridColumnCount() == 1 {
		if row < 0 || row >= len(fp.Entries) {
			return ""
		}
		e := fp.Entries[row]
		if col == 0 && len(fp.Table.Columns) > 0 {
			return formatPanelFileNameAt(e, fp.Table.Columns[0].Width, fp.nameLeftPos)
		}
		return e.GetCellText(col)
	}

	h := fp.Table.ViewHeight
	if h <= 0 {
		h = 1
	}
	idx := row + col*h
	if idx < 0 || idx >= len(fp.Entries) {
		return ""
	}
	e := fp.Entries[idx]
	width := 0
	if col >= 0 && col < len(fp.Table.Columns) {
		width = fp.Table.Columns[col].Width
	}
	return formatPanelFileNameAt(e, width, fp.nameLeftPos)
}

// IsCellSelected implements vtui.TableCellColSelectProvider. Unlike a plain
// row number, (row, col) is enough to resolve exactly which entry sits here
// even in multi file-column view modes (Medium, Brief), where one row is
// shared by several columns via the same row+col*H math GetCellAttr uses.
func (fp *FileSystemPanel) IsCellSelected(row, col int) bool {
	idx := fp.entryIndex(row, col)
	if idx < 0 || idx >= len(fp.Entries) {
		return false
	}
	return fp.Entries[idx].IsSelected()
}

// entryIndex resolves the fp.entries index shown at (row, col) for the
// current view mode: a single file-column in Wide/Detailed, or several
// file-columns of height ViewHeight in Medium/Brief.
func (fp *FileSystemPanel) entryIndex(row, col int) int {
	if fp.gridColumnCount() == 1 {
		return row
	}
	h := fp.Table.ViewHeight
	if h <= 0 {
		h = 1
	}
	return row + col*h
}

func (fp *FileSystemPanel) GetCellAttr(row, col int, defaultAttr uint64) uint64 {
	idx := fp.entryIndex(row, col)
	if idx < 0 || idx >= len(fp.Entries) {
		return defaultAttr
	}
	if fp.gridColumnCount() == 1 {
		return fp.Entries[idx].GetCellAttr(col, defaultAttr)
	}
	return fp.Entries[idx].GetCellAttr(0, defaultAttr)
}

func (f *FileEntry) displayName(name string) string {
	if f.Name == ".." {
		return ".."
	}
	marker := ""
	if config.App.ShowHighlightMarks {
		marker = theme.GlobalFileHighlighter.GetMarker(&f.VFSItem)
	}
	if marker == "" && f.IsSymlink {
		marker = "→"
	}
	prefix := ""
	if f.IsDir {
		if config.App.ShowDirPrefix {
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

func shouldSeparatePanelExtension(entry *FileEntry) bool {
	return config.App.SeparateFileExtensions && !entry.IsDir && !entry.NoExtension && entry.Name != ".."
}

func formatPanelFileName(entry *FileEntry, width int) string {
	return formatPanelFileNameAt(entry, width, 0)
}

// panelNameOverflow reports how many display cells of entry's name do not
// fit into a name column of the given width, i.e. how far the name can be
// scrolled to the right before its end comes into view. It is 0 for names
// that fit. In separate-extensions mode the extension keeps its right-aligned
// field, so only the base name counts.
func panelNameOverflow(entry *FileEntry, width int) int {
	if width <= 0 {
		return 0
	}
	if shouldSeparatePanelExtension(entry) {
		if base, extension := splitFileExtension(entry.Name); extension != "" {
			baseWidth := width - panelExtensionFieldWidth(extension) - 1
			if baseWidth <= 0 {
				return 0
			}
			return max(runewidth.StringWidth(entry.displayName(base))-baseWidth, 0)
		}
	}
	return max(runewidth.StringWidth(entry.displayName(entry.Name))-width, 0)
}

// panelNameShift clamps a panel-wide scroll position to what this particular
// name can absorb: a name that fits its column never moves, a longer one
// stops once its last cell is visible (far2l's MakeCurLeftPos).
func panelNameShift(entry *FileEntry, width, leftPos int) int {
	if leftPos <= 0 {
		return 0
	}
	return min(leftPos, panelNameOverflow(entry, width))
}

// scrollPanelName drops shift leading display cells from a name. A wide
// character straddling the cut is replaced by a space so the visible
// columns stay aligned.
func scrollPanelName(name string, shift int) string {
	if shift <= 0 {
		return name
	}
	if runewidth.StringWidth(name) <= shift {
		return ""
	}
	return runewidth.TruncateLeft(name, shift, "")
}

func panelExtensionFieldWidth(extension string) int {
	return max(runewidth.StringWidth(extension), 3)
}

// formatPanelFileNameAt renders the name column cell for entry with the
// panel's name scroll position applied (see nameLeftPos). Only the part of
// the name that overflows the column can scroll out of view on the left.
func formatPanelFileNameAt(entry *FileEntry, width, leftPos int) string {
	visibleName := entry.visibleName()
	if !shouldSeparatePanelExtension(entry) || width <= 0 {
		return scrollPanelName(entry.displayName(visibleName), panelNameShift(entry, width, leftPos))
	}
	base, extension := splitFileExtension(visibleName)
	if extension == "" {
		return scrollPanelName(entry.displayName(visibleName), panelNameShift(entry, width, leftPos))
	}

	extensionWidth := runewidth.StringWidth(extension)
	extensionFieldWidth := extensionWidth
	if extensionFieldWidth < 3 {
		extensionFieldWidth = 3
	}
	if extensionFieldWidth >= width {
		if extensionFieldWidth > extensionWidth {
			return runewidth.Truncate(extension+strings.Repeat(" ", extensionFieldWidth-extensionWidth), width, "")
		}
		return runewidth.Truncate(extension, width, "")
	}
	left := scrollPanelName(entry.displayName(base), panelNameShift(entry, width, leftPos))
	left = runewidth.Truncate(left, width-extensionFieldWidth-1, "")
	leftWidth := runewidth.StringWidth(left)
	padding := width - leftWidth - extensionFieldWidth
	if padding < 0 {
		padding = 0
	}
	extPadding := extensionFieldWidth - extensionWidth
	if extPadding < 0 {
		extPadding = 0
	}

	var sb strings.Builder
	sb.Grow(len(left) + padding + len(extension) + extPadding)
	sb.WriteString(left)
	for i := 0; i < padding; i++ {
		sb.WriteByte(' ')
	}
	sb.WriteString(extension)
	for i := 0; i < extPadding; i++ {
		sb.WriteByte(' ')
	}
	return sb.String()
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

func panelFileNameMatchSpans(entry *FileEntry, width, matchStartRunes, matchedRunes int) []panelMatchSpan {
	return panelFileNameMatchSpansAt(entry, width, 0, matchStartRunes, matchedRunes)
}

// panelFileNameMatchSpansAt is panelFileNameMatchSpans for a name column
// scrolled by leftPos cells: the highlighted cells move left together with
// the name (the right-aligned extension field does not scroll), and spans
// that scrolled out of the column are clipped away.
func panelFileNameMatchSpansAt(entry *FileEntry, width, leftPos, matchStartRunes, matchedRunes int) []panelMatchSpan {
	if matchStartRunes < 0 || matchedRunes <= 0 || width <= 0 {
		return nil
	}
	shift := panelNameShift(entry, width, leftPos)
	visibleName := entry.visibleName()
	nameRunes := []rune(visibleName)
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
			prefixWidth+runewidth.StringWidth(string(nameRunes[:matchStartRunes]))-shift,
			runewidth.StringWidth(string(nameRunes[matchStartRunes:matchEndRunes])), width,
		); ok {
			return []panelMatchSpan{span}
		}
		return nil
	}

	base, extension := splitFileExtension(visibleName)
	if extension == "" {
		if span, ok := clippedPanelMatchSpan(
			prefixWidth+runewidth.StringWidth(string(nameRunes[:matchStartRunes]))-shift,
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
			prefixWidth+runewidth.StringWidth(string(baseRunes[:baseMatchStart]))-shift,
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
	h := m.fp.Table.ViewHeight
	if h <= 0 {
		h = 1
	}
	idx := m.r + col*h
	if idx >= len(m.fp.Entries) {
		return false
	}
	return m.fp.Entries[idx].Selected
}
func (m *mediumRow) GetCellAttr(col int, defaultAttr uint64) uint64 {
	h := m.fp.Table.ViewHeight
	if h <= 0 {
		h = 1
	}
	idx := m.r + col*h
	if idx >= len(m.fp.Entries) {
		return defaultAttr
	}
	e := m.fp.Entries[idx]
	attr := defaultAttr
	isCursor := (defaultAttr == vtui.Palette[theme.ColPanelCursor] || defaultAttr == vtui.Palette[theme.ColPanelSelectedCursor] || defaultAttr == vtui.Palette[theme.ColPanelInactiveCursor] || defaultAttr == vtui.Palette[theme.ColPanelInactiveSelectedCursor])

	attr = theme.GlobalFileHighlighter.GetColor(&e.VFSItem, attr, e.Selected, isCursor)

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
	// panelLoadingShowDelay hides the loading pulse until an operation has run
	// at least this long. Quick loads (fast directory reads, snappy VFS mounts)
	// then finish without ever showing the spinner, avoiding a brief flash.
	panelLoadingShowDelay = 200 * time.Millisecond
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
	DefaultGalleryColumnCount = 2
	MinGalleryColumnCount     = 2
	MaxGalleryColumnCount     = 3
)

var GalleryLayoutModes = [...]GalleryLayoutMode{
	GalleryLayoutMasonry,
	GalleryLayoutColumns,
	GalleryLayoutDetails,
	GalleryLayoutGrid,
	GalleryLayoutIcons,
}

func ParseGalleryLayoutMode(value string) (GalleryLayoutMode, bool) {
	mode := GalleryLayoutMode(strings.ToLower(strings.TrimSpace(value)))
	for _, candidate := range GalleryLayoutModes {
		if mode == candidate {
			return mode, true
		}
	}
	return GalleryLayoutMasonry, false
}

func GalleryDensityLimits(mode GalleryLayoutMode) (defaultValue, minimum, maximum int) {
	switch mode {
	case GalleryLayoutColumns, GalleryLayoutDetails:
		// Zero asks the QML host to derive the untouched default from its font.
		// Explicit compact zoom values use the same bounded row-pitch contract.
		return 0, 22, 72
	case GalleryLayoutGrid:
		return 160, 96, 320
	case GalleryLayoutIcons:
		return 64, 18, 256
	default:
		return 150, 30, 500
	}
}

func ClampGalleryDensity(mode GalleryLayoutMode, density int) int {
	_, minimum, maximum := GalleryDensityLimits(mode)
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

func (f *FileEntry) IsSelected() bool {
	return f.Selected
}

func (f *FileEntry) GetCellText(col int) string {
	switch col {
	case 0:
		return f.displayName(f.visibleName())
	case 1:
		if f.IsDir {
			if f.SizeCalculated {
				return fileops.FormatIntWithSpaces(f.Size)
			}
			if f.Name == ".." {
				return i18n.Msg("Panel.UpDir")
			}
			return ""
		}
		return fileops.FormatIntWithSpaces(f.Size)
	case 2:
		if f.MTime.IsZero() {
			return ""
		}
		return f.MTime.Format("02.01.06 15:04")
	}
	return ""
}
func (f *FileEntry) GetCellAttr(col int, defaultAttr uint64) uint64 {
	attr := defaultAttr
	isCursor := (defaultAttr == vtui.Palette[theme.ColPanelCursor] || defaultAttr == vtui.Palette[theme.ColPanelSelectedCursor] || defaultAttr == vtui.Palette[theme.ColPanelInactiveCursor] || defaultAttr == vtui.Palette[theme.ColPanelInactiveSelectedCursor])

	attr = theme.GlobalFileHighlighter.GetColor(&f.VFSItem, attr, f.Selected, isCursor)

	return attr
}

// FileSystemPanel is a panel displaying files on disk.
type directoryLoadRequest struct {
	load       func()
	benchmark  *navtrace.NavigationBenchmarkTrace
	enqueuedNs int64
}

type FileSystemPanel struct {
	nativeStatus nativePanelStatusCache
	vtui.ScreenObject
	Table                 *vtui.Table
	scrollBar             *vtui.ScrollBar
	scrollMouseActive     bool
	minimalScrollDragGap  int
	headerMouseActive     bool
	Frame                 *vtui.BorderedFrame
	Vfs                   vfs.VFS
	Entries               []*FileEntry
	SelectedItems         map[string]bool
	previousSelection     map[string]bool
	previousSelectionVFS  vfs.VFS
	previousSelectionPath string
	selectionEpoch        map[string]uint64
	selectionEpochNext    uint64
	DirectoryEpoch        uint64
	ViewMode              ViewMode
	Wide                  bool
	CursorIdx             int
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
	GalleryLayoutMode     GalleryLayoutMode
	GalleryColumnCount    int
	GalleryDensities      map[GalleryLayoutMode]int
	GalleryLayoutRevision int64

	loadCtx    context.Context
	CancelLoad context.CancelFunc
	// loadGeneration binds every asynchronous base/metadata callback to the
	// exact ReadDirectory request that created it. Context cancellation usually
	// rejects stale work first; the generation also covers providers which
	// finish a callback concurrently with cancellation.
	loadGeneration uint64
	IsLoading      bool
	// catalogInteractive becomes true as soon as the panel owns a useful,
	// destination-bound row window. A local preview can therefore be visible and
	// actionable while the same uncached enumeration continues building the
	// complete authoritative catalog. isLoading remains true until that worker
	// finishes, but semantic frontends no longer gate interaction or animation.
	catalogInteractive bool
	// catalogProvisional marks the synthetic cold-load placeholder (normally
	// just ".."). Native frontends keep the destination hidden until the first
	// authoritative catalog arrives instead of briefly painting an empty list.
	catalogProvisional bool
	// catalogLogicalCount is non-zero only while a paged frontend has received
	// an exact sparse catalog before the complete source rows have finished
	// materializing. It is a scalar protocol fact, never a rendered-row cache.
	catalogLogicalCount int
	// semanticPendingMu protects the immutable full source which a directory
	// worker has finished converting but whose UI append task has not run yet.
	// Native row requests can arrive in that interval, so the semantic pager
	// must not mistake the bounded presentation window for the authoritative
	// source.
	semanticPendingMu             sync.RWMutex
	semanticPendingEntries        []*FileEntry
	semanticPendingUpEntry        *FileEntry
	semanticPendingLoadGeneration uint64
	LoadingTimer                  *time.Timer
	loadingFrame                  int
	loadingVisible                bool
	LoadingGeneration             uint64
	loadQueueMu                   sync.Mutex
	LoadWorkerWG                  sync.WaitGroup // joins the single directory-load queue worker
	loadWorkerActive              bool
	pendingDirectoryLoad          *directoryLoadRequest
	benchmarkLoadTrace            *navtrace.NavigationBenchmarkTrace
	ProviderOpenTask              *vtui.TaskContext
	providerOpenDialog            *vtui.Window
	directoryErrorDialog          *vtui.Window
	ProviderOpenTarget            string
	ProviderOpenSourceSelect      string
	providerOpenResult            func(bool) bool
	PendingSelection              string
	ProviderEntryName             string // name of entry used to enter a provider VFS (e.g. NetFox connection name)
	suppressFolderHistoryPath     string // one-shot: history/menu navigation must not reorder MRU
	suppressFolderHistoryToken    uint64 // binds suppression to one specific asynchronous directory load
	FastFindMode                  bool
	FastFindStr                   string
	fastFindMatcherKey            string
	fastFindMatchers              []*vtui.FuzzyMatcher
	fastFindMatcherQueries        []string
	// Fast Find is evaluated lazily by row. A 30k-entry directory must not be
	// re-matched in full merely because one character or the cursor changed;
	// only rows touched by navigation or the bounded semantic viewport are
	// retained for the current query/catalog generation.
	fastFindMatchCacheKey               string
	fastFindMatchCacheCatalogGeneration uint64
	fastFindMatchCacheEntryCount        int
	fastFindMatchCache                  map[int]fastFindCachedMatch
	fastFindMatchEvaluations            uint64
	fastFindAnyMatchKnown               bool
	fastFindAnyMatch                    bool
	showInactiveCursor                  bool
	nameLeftPos                         int

	SortMode    SortMode
	SortReverse bool
	// UseSortGroups clusters the panel by the Group-bearing highlight.ini
	// rules before the sort mode is applied (far's Shift+F11).
	UseSortGroups bool

	lastDirMTime time.Time

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
	lastLoadedPath       string
	committedListingVFS  vfs.VFS
	committedListingPath string
	catalogRefreshDelta  map[string]any

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
	mediaSourceEpoch             int64
	// Selection changes are journaled independently from the immutable file
	// catalog. Native semantic renderers can therefore acknowledge one changed
	// row without exporting or serializing every directory entry.
	semanticSelectionBaseRevision int64
	semanticSelectionChanges      map[string]semanticSelectionChange
	semanticSelectionOverflow     bool
	semanticSelectionNeedsSync    bool
	semanticStaticCache           *semanticPanelStaticCache
	semanticMetadataSnapshot      *semanticPanelMetadataSnapshot
	semanticCatalogGeneration     uint64
	semanticPublishedGeneration   uint64
	semanticPagedSignature        string
	semanticPagedResourceRevision int64
	semanticPagedResourceIDs      map[string]struct{}
}

var DisableLoadingAnimationInTests = true

// DirectoryLoadWorkers counts every directory-load worker alive in the
// process, across all panels. See enqueueDirectoryLoad for why a per-panel
// WaitGroup is not enough on its own.
var DirectoryLoadWorkers sync.WaitGroup

func NewFileSystemPanel(x, y, w, h int, vfs vfs.VFS) *FileSystemPanel {
	path := vfs.GetPath()

	fp := &FileSystemPanel{
		Vfs:                   vfs,
		Frame:                 vtui.NewBorderedFrame(x, y, x+w-1, y+h-1, vtui.SingleBox, path),
		Table:                 vtui.NewTable(x+1, y+1, w-2, h-2, nil),
		ViewMode:              ViewModeMedium,
		GalleryLayoutMode:     GalleryLayoutMasonry,
		GalleryColumnCount:    DefaultGalleryColumnCount,
		GalleryDensities:      make(map[GalleryLayoutMode]int),
		GalleryLayoutRevision: 1,
		lastRightClickedIdx:   -1,
		semanticRightIndex:    -1,
		semanticPriorIndex:    -1,
		SelectedItems:         make(map[string]bool),
		selectionEpoch:        make(map[string]uint64),
		//entries:             []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}},
	}
	fp.Frame.ColorBoxIdx = theme.ColPanelBox
	fp.Frame.ColorTitleIdx = theme.ColPanelTitle
	fp.Table.ColorTextIdx = theme.ColPanelText
	fp.Table.ColorSelectedTextIdx = theme.ColPanelCursor
	fp.Table.ColorItemSelectTextIdx = theme.ColPanelSelectedText
	fp.Table.ColorItemSelectCursorIdx = theme.ColPanelSelectedCursor
	fp.Table.ColorTitleIdx = theme.ColPanelColumnTitle
	fp.Table.ColorBoxIdx = theme.ColPanelBox
	fp.Table.ShowScrollBar = false
	fp.initScrollBar()
	fp.SetCanFocus(true)
	fp.SetPosition(x, y, x+w-1, y+h-1)
	fp.SetViewMode(ViewModeMedium)
	fp.ReadDirectory()
	return fp
}

// sameVFSInstance binds asynchronous provider work to the exact VFS object
// that started it. Two views may share a remote session, but one view's
// navigation must never install a result into the other.
func SameVFSInstance(a, b vfs.VFS) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	if ta != tb || !ta.Comparable() {
		return false
	}
	return a == b
}

// isNilVFS reports whether v is nil, including a typed nil wrapped inside a
// non-nil interface. Calling a method on such a provider result would panic.
func isNilVFS(v vfs.VFS) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return rv.IsNil()
	}
	return false
}

// isArchiveProvider identifies the archive plugin so its open failures use
// file-oriented wording instead of a network connection error.
func isArchiveProvider(p vfs.VFSProvider) bool {
	return p != nil && p.Name() == "zipper/archive"
}

func phasedDirectoryReaderFor(filesystem vfs.VFS) vfs.PhasedDirectoryReader {
	provider, ok := filesystem.(vfs.PhasedDirectoryReadProvider)
	if !ok {
		return nil
	}
	reader := provider.PhasedDirectoryReader()
	readerVFS, ok := reader.(vfs.VFS)
	if !ok || !fileops.SameVFSInstance(readerVFS, filesystem) {
		// A promoted OSVFS capability belongs to the embedded OSVFS, not to a
		// wrapper which may override ReadDir. Only an explicit self capability
		// is safe to use.
		return nil
	}
	return reader
}

func windowedDirectoryReaderFor(filesystem vfs.VFS) vfs.WindowedDirectoryReader {
	provider, ok := filesystem.(vfs.WindowedDirectoryReadProvider)
	if !ok {
		return nil
	}
	reader := provider.WindowedDirectoryReader()
	readerVFS, ok := reader.(vfs.VFS)
	if !ok || !fileops.SameVFSInstance(readerVFS, filesystem) {
		return nil
	}
	return reader
}

func panelSortSupportsPhasedDirectoryRead(mode SortMode) bool {
	return mode != SortSize && mode != SortTime
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
) []*FileEntry {
	visibleCount := len(items)
	if !config.App.ShowHiddenFiles {
		visibleCount = 0
		for _, item := range items {
			if item.Name == ".." || !item.IsHidden {
				visibleCount++
			}
		}
	}
	entryCount := visibleCount
	if showUpEntry {
		entryCount++
	}
	entries := make([]*FileEntry, 0, entryCount)
	// One backing allocation replaces one heap object per file. Interior
	// pointers keep the backing array alive for exactly as long as the panel
	// rows; no directory data is cached or duplicated by this layout.
	backing := make([]FileEntry, entryCount)
	next := 0
	if showUpEntry {
		backing[next].VFSItem = upItem
		entries = append(entries, &backing[next])
		next++
	}
	for _, item := range items {
		if !config.App.ShowHiddenFiles && item.Name != ".." && item.IsHidden {
			continue
		}
		backing[next].VFSItem = item
		entry := &backing[next]
		next++
		fp.applyPersistentSelection(entry, filesystem, path)
		entries = append(entries, entry)
	}
	fp.sortEntrySlice(entries)
	return entries
}

func fileEntriesFromItems(items []vfs.VFSItem) []*FileEntry {
	if config.App.ShowHiddenFiles && len(items) >= 4096 {
		entries := make([]*FileEntry, len(items))
		backing := make([]FileEntry, len(items))
		workerCount := min(runtime.GOMAXPROCS(0), 8)
		workerCount = min(workerCount, (len(items)+2047)/2048)
		var workers sync.WaitGroup
		workers.Add(workerCount)
		for worker := 0; worker < workerCount; worker++ {
			start := len(items) * worker / workerCount
			end := len(items) * (worker + 1) / workerCount
			go func() {
				defer workers.Done()
				for index := start; index < end; index++ {
					backing[index].VFSItem = items[index]
					entries[index] = &backing[index]
				}
			}()
		}
		workers.Wait()
		return entries
	}
	visibleCount := len(items)
	if !config.App.ShowHiddenFiles {
		visibleCount = 0
		for _, item := range items {
			if item.Name == ".." || !item.IsHidden {
				visibleCount++
			}
		}
	}
	entries := make([]*FileEntry, 0, visibleCount)
	backing := make([]FileEntry, visibleCount)
	next := 0
	for _, item := range items {
		if !config.App.ShowHiddenFiles && item.Name != ".." && item.IsHidden {
			continue
		}
		backing[next].VFSItem = item
		entries = append(entries, &backing[next])
		next++
	}
	return entries
}

func (fp *FileSystemPanel) SetItemSelected(idx int, state bool) {
	if idx >= 0 && idx < len(fp.Entries) {
		e := fp.Entries[idx]
		if e.Name != ".." {
			if e.Selected == state {
				return
			}
			e.Selected = state
			if fp.SelectedItems == nil {
				fp.SelectedItems = make(map[string]bool)
			}
			if fp.selectionEpoch == nil {
				fp.selectionEpoch = make(map[string]uint64)
			}
			fp.selectionEpochNext++
			fp.selectionEpoch[e.Name] = fp.selectionEpochNext
			if state {
				fp.SelectedItems[e.Name] = true
			} else {
				delete(fp.SelectedItems, e.Name)
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
				if fp.Vfs != nil {
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
	for index := range fp.Entries {
		fp.SetItemSelected(index, false)
	}
	// Preserve the old operation's cleanup guarantee even if selectedItems
	// contained a stale filename which no longer has a backing row.
	fp.SelectedItems = make(map[string]bool)
}

func (fp *FileSystemPanel) previousSelectionMatches(filesystem vfs.VFS, path string) bool {
	return fp != nil && fp.previousSelectionVFS != nil && filesystem != nil &&
		fileops.SameVFSInstance(fp.previousSelectionVFS, filesystem) && fp.previousSelectionPath == path
}

func (fp *FileSystemPanel) clearPreviousSelection() {
	if fp == nil {
		return
	}
	fp.previousSelection = nil
	fp.previousSelectionVFS = nil
	fp.previousSelectionPath = ""
	for _, entry := range fp.Entries {
		entry.PrevSelected = false
	}
}

func (fp *FileSystemPanel) applyPersistentSelection(entry *FileEntry, filesystem vfs.VFS, path string) {
	if fp == nil || entry == nil || entry.Name == ".." {
		return
	}
	entry.Selected = fp.SelectedItems[entry.Name]
	entry.PrevSelected = fp.previousSelectionMatches(filesystem, path) && fp.previousSelection[entry.Name]
}

func (fp *FileSystemPanel) ToggleSelection(idx int) {
	if idx >= 0 && idx < len(fp.Entries) {
		e := fp.Entries[idx]
		if e.Name != ".." {
			fp.SetItemSelected(idx, !e.Selected)
		}
	}
}
func (fp *FileSystemPanel) SetFocus(f bool) {
	fp.ScreenObject.SetFocus(f)
	if !f && fp.FastFindMode {
		fp.FastFindMode = false
		fp.FastFindStr = ""
	}
}
func (fp *FileSystemPanel) SetSortMode(mode SortMode) {
	if fp.SortMode == mode {
		fp.SortReverse = !fp.SortReverse
	} else {
		fp.SortMode = mode
		// Every mode's base comparator is its desired first-use direction:
		// name/extension ascend, while time/size descend. Repeated activation
		// below toggles that direction uniformly for hotkeys, menus and headers.
		fp.SortReverse = false
	}
	fp.updateSortColumnTitles()
	fp.ReadDirectory()
}

// SetUseSortGroups switches the sort-group clustering of this  Like a
// sort mode change it goes through ReadDirectory, so the cursor is kept on the
// same file while the rows move under it.
func (fp *FileSystemPanel) SetUseSortGroups(use bool) {
	if fp == nil || fp.UseSortGroups == use {
		return
	}
	fp.UseSortGroups = use
	fp.ReadDirectory()
}

func (fp *FileSystemPanel) ToggleSortGroups() {
	if fp == nil {
		return
	}
	fp.SetUseSortGroups(!fp.UseSortGroups)
}

// sortGroupsActive reports whether this panel's entries have to be clustered:
// the panel asked for it and there is at least one configured group.
func (fp *FileSystemPanel) sortGroupsActive() bool {
	return fp != nil && fp.UseSortGroups && GlobalSortGroups.Configured()
}

func (fp *FileSystemPanel) SortEntries() {
	fp.sortEntrySlice(fp.Entries)
	fp.markSemanticCatalogMutation()
}

func comparePanelFolded(left, right string) int {
	collator := panelNameCollators.Get().(*collate.Collator)
	cmp := collator.CompareString(left, right)
	panelNameCollators.Put(collator)
	return cmp
}

func comparePanelNames(left, right string) int {
	if folded := comparePanelFolded(left, right); folded != 0 {
		return folded
	}
	return strings.Compare(left, right)
}

var panelNameCollators = sync.Pool{New: func() any {
	return collate.New(language.Und, collate.IgnoreCase, collate.Force)
}}

func (fp *FileSystemPanel) compareEntryOrder(ei, ej *FileEntry) int {
	if ei.Name == ".." || ej.Name == ".." {
		if ei.Name == ej.Name {
			return 0
		}
		if ei.Name == ".." {
			return -1
		}
		return 1
	}
	if ei.IsDir != ej.IsDir {
		if ei.IsDir {
			return -1
		}
		return 1
	}

	cmp := 0
	leftName := ei.visibleName()
	rightName := ej.visibleName()
	switch fp.SortMode {
	case SortName:
		cmp = comparePanelNames(leftName, rightName)
	case SortExt:
		cmp = comparePanelFolded(filepath.Ext(leftName), filepath.Ext(rightName))
		if cmp == 0 {
			cmp = comparePanelNames(leftName, rightName)
		}
	case SortTime:
		if ei.MTime.Before(ej.MTime) {
			cmp = -1
		} else if ei.MTime.After(ej.MTime) {
			cmp = 1
		} else {
			cmp = comparePanelNames(leftName, rightName)
		}
	case SortSize:
		if ei.Size < ej.Size {
			cmp = -1
		} else if ei.Size > ej.Size {
			cmp = 1
		} else {
			cmp = comparePanelNames(leftName, rightName)
		}
	default:
		cmp = comparePanelNames(leftName, rightName)
	}
	if fp.SortReverse {
		return -cmp
	}
	return cmp
}

// Windows directory indexes already enumerate names in case-insensitive name
// order. The panel only additionally groups folders before files. Prove both
// subsequences are ordered, then perform that grouping in one linear pass.
// Providers with any other order fall through to the general comparison sort.
func (fp *FileSystemPanel) tryLinearNameSort(entries []*FileEntry) bool {
	if fp.SortMode != SortName || len(entries) <= 1 {
		return false
	}
	start := 0
	if entries[0].Name == ".." {
		start = 1
	}
	var lastDir, lastFile *FileEntry
	directoryCount := 0
	filesSeen := false
	grouped := true
	orderedNames := func(previous, current *FileEntry) bool {
		if previous == nil {
			return true
		}
		cmp := comparePanelNames(previous.visibleName(), current.visibleName())
		if fp.SortReverse {
			cmp = -cmp
		}
		return cmp <= 0
	}
	for _, entry := range entries[start:] {
		if entry.Name == ".." {
			return false
		}
		if entry.IsDir {
			if !orderedNames(lastDir, entry) {
				return false
			}
			lastDir = entry
			directoryCount++
			if filesSeen {
				grouped = false
			}
		} else {
			if !orderedNames(lastFile, entry) {
				return false
			}
			lastFile = entry
			filesSeen = true
		}
	}
	if grouped {
		return true
	}
	ordered := make([]*FileEntry, len(entries)-start)
	directoryIndex := 0
	fileIndex := directoryCount
	for _, entry := range entries[start:] {
		if entry.IsDir {
			ordered[directoryIndex] = entry
			directoryIndex++
		} else {
			ordered[fileIndex] = entry
			fileIndex++
		}
	}
	copy(entries[start:], ordered)
	return true
}

func panelFoldKey(collator *collate.Collator, buffer *collate.Buffer, value string) string {
	key := string(collator.KeyFromString(buffer, value))
	buffer.Reset()
	return key
}

// Shared-prefix Windows component names make byte-at-a-time comparator work
// disproportionately expensive. Build each folded key once, then let Go's
// optimized native string comparison handle the N log N ordering.
type panelNameSortEntry struct {
	entry  *FileEntry
	folded string
}

func comparePreparedPanelName(left, right panelNameSortEntry, reverse bool) int {
	if left.entry.Name == ".." || right.entry.Name == ".." {
		if left.entry.Name == right.entry.Name {
			return 0
		}
		if left.entry.Name == ".." {
			return -1
		}
		return 1
	}
	if left.entry.IsDir != right.entry.IsDir {
		if left.entry.IsDir {
			return -1
		}
		return 1
	}
	cmp := strings.Compare(left.folded, right.folded)
	if cmp == 0 {
		cmp = strings.Compare(left.entry.visibleName(), right.entry.visibleName())
	}
	if reverse {
		cmp = -cmp
	}
	return cmp
}

func mergePreparedPanelNames(destination, source []panelNameSortEntry,
	leftStart, middle, rightEnd int, reverse bool,
) {
	left, right, output := leftStart, middle, leftStart
	for left < middle && right < rightEnd {
		if comparePreparedPanelName(source[left], source[right], reverse) <= 0 {
			destination[output] = source[left]
			left++
		} else {
			destination[output] = source[right]
			right++
		}
		output++
	}
	output += copy(destination[output:rightEnd], source[left:middle])
	copy(destination[output:rightEnd], source[right:rightEnd])
}

// sortEntriesByPreparedName folds every name once. Large local catalogs are
// split into independent runs and merged in parallel; this keeps exact global
// ordering while avoiding a serial 30k-row comparison pass before the first
// bounded Qt catalog window can be published.
func (fp *FileSystemPanel) sortEntriesByPreparedName(entries []*FileEntry) {
	keyed := make([]panelNameSortEntry, len(entries))
	workerCount := 1
	if len(entries) >= 8192 {
		workerCount = min(runtime.GOMAXPROCS(0), 8)
		workerCount = min(workerCount, (len(entries)+4095)/4096)
	}
	runSize := (len(entries) + workerCount - 1) / workerCount
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for worker := 0; worker < workerCount; worker++ {
		start := worker * runSize
		end := min(start+runSize, len(entries))
		go func() {
			defer workers.Done()
			collator := collate.New(language.Und, collate.IgnoreCase, collate.Force)
			var buffer collate.Buffer
			for index := start; index < end; index++ {
				entry := entries[index]
				keyed[index] = panelNameSortEntry{
					entry: entry, folded: panelFoldKey(collator, &buffer, entry.visibleName()),
				}
			}
			slices.SortFunc(keyed[start:end], func(left, right panelNameSortEntry) int {
				return comparePreparedPanelName(left, right, fp.SortReverse)
			})
		}()
	}
	workers.Wait()

	if workerCount > 1 {
		scratch := make([]panelNameSortEntry, len(keyed))
		source, destination := keyed, scratch
		for runSize < len(keyed) {
			pairCount := (len(keyed) + 2*runSize - 1) / (2 * runSize)
			workers.Add(pairCount)
			for pair := 0; pair < pairCount; pair++ {
				leftStart := pair * 2 * runSize
				middle := min(leftStart+runSize, len(keyed))
				rightEnd := min(middle+runSize, len(keyed))
				go func() {
					defer workers.Done()
					mergePreparedPanelNames(destination, source,
						leftStart, middle, rightEnd, fp.SortReverse)
				}()
			}
			workers.Wait()
			source, destination = destination, source
			runSize *= 2
		}
		keyed = source
	}
	for index := range keyed {
		entries[index] = keyed[index].entry
	}
}

// sortEntrySlice applies the panel's current ordering to a detached catalog.
func (fp *FileSystemPanel) sortEntrySlice(entries []*FileEntry) {
	if (fp.SortMode == SortUnsorted && !fp.sortGroupsActive()) || len(entries) <= 1 {
		return
	}
	if fp.sortGroupsActive() {
		groups := make(map[*FileEntry]int, len(entries))
		for _, entry := range entries {
			groups[entry] = GlobalSortGroups.GroupOf(&entry.VFSItem)
		}
		slices.SortStableFunc(entries, func(left, right *FileEntry) int {
			if left.Name == ".." || right.Name == ".." || (fp.SortMode != SortUnsorted && left.IsDir != right.IsDir) {
				return fp.compareEntryOrder(left, right)
			}
			if groups[left] < groups[right] {
				return -1
			}
			if groups[left] > groups[right] {
				return 1
			}
			if fp.SortMode == SortUnsorted {
				return 0
			}
			return fp.compareEntryOrder(left, right)
		})
		return
	}
	if fp.tryLinearNameSort(entries) {
		return
	}
	if fp.SortMode == SortName {
		fp.sortEntriesByPreparedName(entries)
		return
	}

	// Keep the comparison strict and deterministic. The old reverse branch
	// returned !less, which also returned true for equal values. That violates
	// sort.Interface's ordering contract and could reshuffle an otherwise
	// unchanged catalog on refresh, invalidating Gallery revisions and cursor
	// identities spuriously.
	slices.SortFunc(entries, func(left, right *FileEntry) int {
		return fp.compareEntryOrder(left, right)
	})
}

func (fp *FileSystemPanel) SetViewMode(mode ViewMode) {
	if mode == ViewModeWide {
		fp.SetWide(true)
		return
	}
	fp.ViewMode = mode
	fp.Wide = false
	fp.configureCellSelection()
	fp.Resize(fp.X2-fp.X1+1, fp.Y2-fp.Y1+1)
}

// mouseEntryIndex returns the entry under the mouse. Multi-column panel modes
// are filled from top to bottom and then from left to right, so the visual
// column contributes a full table height to the entry index.
func (fp *FileSystemPanel) mouseEntryIndex(mouseX, mouseY int) int {
	if mouseX < fp.Table.X1 || mouseX > fp.Table.X2 {
		return -1
	}

	row := mouseY - (fp.Table.Y1 + fp.Table.MarginTop)
	if row < 0 || row >= fp.Table.ViewHeight {
		return -1
	}

	column := 0
	if fp.gridColumnCount() > 1 {
		column = -1
		columnX := fp.Table.X1
		for i, tableColumn := range fp.Table.Columns {
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

	idx := fp.Table.TopPos + row + column*fp.Table.ViewHeight
	if idx < 0 || idx >= len(fp.Entries) {
		return -1
	}
	return idx
}

func (fp *FileSystemPanel) processRightDrag(idx int) {
	if !fp.rightDragActive {
		fp.rightDragActive = true
		fp.rightDragSelect = !fp.Entries[idx].Selected
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
	if idx < 0 || idx >= len(fp.Entries) {
		return false
	}
	fp.semanticPriorIndex = fp.semanticRightIndex
	fp.semanticPriorState = fp.semanticRightState
	fp.SetCursorIndex(idx)
	if fp.Entries[idx].Name == ".." {
		fp.semanticRightIndex = idx
		fp.semanticRightState = false
		return true
	}
	fp.rowDragButton = vtinput.RightmostButtonPressed
	fp.processRightDrag(idx)
	fp.semanticRightIndex = idx
	fp.semanticRightState = fp.Entries[idx].Selected
	fp.Refresh()
	return true
}

func (fp *FileSystemPanel) semanticRightPointerMove(idx int) bool {
	if !fp.rightDragActive || idx < 0 || idx >= len(fp.Entries) {
		return false
	}
	fp.SetCursorIndex(idx)
	if fp.Entries[idx].Name != ".." {
		fp.processRightDrag(idx)
		fp.Refresh()
	}
	return true
}

func (fp *FileSystemPanel) semanticRightPointerDoubleClick(idx int) bool {
	if idx < 0 || idx >= len(fp.Entries) {
		return false
	}
	fp.SetCursorIndex(idx)
	if fp.Entries[idx].Name == ".." {
		return true
	}
	state := fp.semanticRightState
	if fp.semanticPriorIndex == idx {
		state = fp.semanticPriorState
	}
	fp.SetAllItemsSelected(state)
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
	oldTop := fp.Table.TopPos
	fp.setPanelScrollTop(oldTop + direction)
	if fp.Table.TopPos == oldTop {
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
	// Read on the goroutine that starts this work, not inside it: the
	// work outlives the call, and reading the global from it races
	// anything that reassigns vtui.FrameManager meanwhile.
	frames := vtui.FrameManager
	fp.dragScrollTimer = time.AfterFunc(panelDragScrollInterval, func() {
		frames.PostTask(func() {
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
	contentTop := fp.Table.Y1 + fp.Table.MarginTop
	contentBottom := contentTop + fp.Table.ViewHeight - 1
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

func (fp *FileSystemPanel) SetAllItemsSelected(state bool) {
	for idx := range fp.Entries {
		fp.SetItemSelected(idx, state)
	}
}

func (fp *FileSystemPanel) SetWide(wide bool) {
	fp.Wide = wide
	fp.configureCellSelection()
	fp.Resize(fp.X2-fp.X1+1, fp.Y2-fp.Y1+1)
}

func (fp *FileSystemPanel) EffectiveViewMode() ViewMode {
	if fp.Wide {
		return ViewModeWide
	}
	return fp.ViewMode
}

func (fp *FileSystemPanel) gridColumnCount() int {
	switch fp.EffectiveViewMode() {
	case ViewModeBrief:
		return 3
	case ViewModeMedium:
		return 2
	default:
		return 1
	}
}

func (fp *FileSystemPanel) columnSortMode(column int) (SortMode, bool) {
	if column < 0 || column >= len(fp.Table.Columns) {
		return SortUnsorted, false
	}
	switch fp.EffectiveViewMode() {
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

func (fp *FileSystemPanel) SortIsAscending() bool {
	switch fp.SortMode {
	case SortTime, SortSize:
		// Their base comparators are newest/largest first.
		return fp.SortReverse
	default:
		return !fp.SortReverse
	}
}

func sortModeTitle(mode SortMode) string {
	switch mode {
	case SortName:
		return i18n.Msg("Menu.SortName")
	case SortExt:
		return i18n.Msg("Menu.SortExt")
	case SortTime:
		return i18n.Msg("Menu.SortTime")
	case SortSize:
		return i18n.Msg("Menu.SortSize")
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
	for column := range fp.Table.Columns {
		title := i18n.Msg("Panel.Column.Name")
		switch fp.EffectiveViewMode() {
		case ViewModeWide:
			switch column {
			case 1:
				title = i18n.Msg("Panel.Column.Size")
			case 2:
				title = i18n.Msg("Panel.Column.Modified")
			}
		case ViewModeDetailed:
			if column == 1 {
				title = i18n.Msg("Panel.Column.Size")
			}
		}

		mode, sortable := fp.columnSortMode(column)
		if sortable && fp.SortMode != SortUnsorted && fp.SortMode == mode {
			arrow := " ↓"
			if fp.SortIsAscending() {
				arrow = " ↑"
			}
			title += arrow
			visibleSortColumn = true
		}
		fp.Table.Columns[column].Title = title
	}

	if fp.SortMode != SortUnsorted && !visibleSortColumn && len(fp.Table.Columns) > 0 {
		right := hiddenSortColumnTitle(
			fp.SortMode, fp.SortIsAscending(), fp.Table.Columns[0].Width)
		fp.Table.Columns[0].Title = composePanelColumnTitle(
			i18n.Msg("Panel.Column.Name"), right, fp.Table.Columns[0].Width)
	}
}

func (fp *FileSystemPanel) headerSortModeAt(x, y int) (SortMode, bool) {
	if !fp.Table.ShowHeader || y != fp.Table.Y1 || x < fp.Table.X1 || x > fp.Table.X2 {
		return SortUnsorted, false
	}
	if mode, ok := fp.hiddenSortModeHeaderAt(x); ok {
		return mode, true
	}
	columnX := fp.Table.X1
	for column, tableColumn := range fp.Table.Columns {
		if x >= columnX && x < columnX+tableColumn.Width {
			return fp.columnSortMode(column)
		}
		columnX += tableColumn.Width
		if column < len(fp.Table.Columns)-1 {
			// The separator itself is not a sortable column header.
			if x == columnX {
				return SortUnsorted, false
			}
			columnX++
		}
	}
	return SortUnsorted, false
}

func (fp *FileSystemPanel) hiddenSortModeHeaderAt(x int) (SortMode, bool) {
	if fp.SortMode == SortUnsorted || len(fp.Table.Columns) == 0 {
		return SortUnsorted, false
	}

	for column := range fp.Table.Columns {
		mode, sortable := fp.columnSortMode(column)
		if sortable && mode == fp.SortMode {
			return SortUnsorted, false
		}
	}

	width := fp.Table.Columns[0].Width
	right := hiddenSortColumnTitle(fp.SortMode, fp.SortIsAscending(), width)
	rightWidth := runewidth.StringWidth(right)
	columnEnd := fp.Table.X1 + width
	if rightWidth >= width {
		return fp.SortMode, x >= fp.Table.X1 && x < columnEnd
	}
	return fp.SortMode, x >= columnEnd-rightWidth && x < columnEnd
}

// panelScrollMetrics maps the panel's item-based scrolling onto the
// row-based coordinates expected by vtui.ScrollBar. Multi-column modes show
// two or three times as many entries in the same vertical space, so using the
// table's raw ItemCount would produce an oversized scroll range and a thumb
// that is much too small.
func (fp *FileSystemPanel) panelScrollMetrics() (height, visibleItems, maxTop, virtualMax, virtualValue int) {
	height = fp.Table.ViewHeight
	if height <= 0 {
		return
	}

	columns := fp.gridColumnCount()
	visibleItems = height * columns
	maxTop = len(fp.Entries) - visibleItems
	if maxTop <= 0 {
		maxTop = 0
		return
	}

	virtualRows := (len(fp.Entries) + columns - 1) / columns
	virtualMax = virtualRows - height
	if virtualMax <= 0 {
		virtualMax = 1
	}

	top := fp.Table.TopPos
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
		fp.setPanelScrollTop(fp.Table.TopPos + delta)
	}
}

func (fp *FileSystemPanel) syncScrollBar() bool {
	if fp.scrollBar == nil || config.App.PanelScrollbarMode == config.PanelScrollbarOff {
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
	y1 := fp.Table.Y1 + fp.Table.MarginTop
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

	delta := top - fp.Table.TopPos
	if delta == 0 {
		return
	}
	idx := fp.GetCursorIndex() + delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(fp.Entries) {
		idx = len(fp.Entries) - 1
	}

	fp.Table.TopPos = top
	fp.SetCursorIndex(idx)
	fp.Refresh()
	vtui.FrameManager.Redraw()
}

func (fp *FileSystemPanel) drawScrollBar(scr *vtui.ScreenBuf) {
	if !fp.syncScrollBar() {
		return
	}
	height := fp.scrollBar.Y2 - fp.scrollBar.Y1 + 1
	if config.App.PanelScrollbarMode == config.PanelScrollbarMinimal {
		caretPos, caretLength := minimalPanelScrollThumb(height, fp.scrollBar.Value, fp.scrollBar.Max)
		attr := vtui.Palette[theme.ColPanelMinimalScrollbar]
		for offset := 0; offset < caretLength; offset++ {
			scr.Write(fp.scrollBar.X1, fp.scrollBar.Y1+caretPos+offset,
				vtui.StringToCharInfo("│", attr))
		}
		return
	}
	vtui.DrawScrollBar(scr, fp.scrollBar.X1, fp.scrollBar.Y1, height,
		fp.scrollBar.Value, fp.scrollBar.Max+height, vtui.Palette[theme.ColPanelScrollbar])
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
	if fp.gridColumnCount() != 1 || !fp.Table.ShowSeparators || !fp.Table.IsFocused() {
		return
	}

	y := fp.Table.Y1 + fp.Table.MarginTop + fp.Table.SelectPos - fp.Table.TopPos
	if y < fp.Table.Y1+fp.Table.MarginTop || y > fp.Table.Y2 {
		return
	}

	x := fp.Table.X1
	for column := 0; column < len(fp.Table.Columns)-1; column++ {
		x += fp.Table.Columns[column].Width
		// Keep the separator's own foreground and copy only the rendered
		// cursor cell's background. The separator must not inherit the file
		// name/highlighter foreground color.
		cursorAttr := scr.GetCell(x-1, y).Attributes
		attr := vtui.Palette[fp.Table.ColorBoxIdx]
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

// truncateNameKeepingEnd fits a name into width cells by dropping its
// beginning, so a long file name still shows its extension.
func truncateNameKeepingEnd(name string, width int) string {
	if width <= 0 {
		return ""
	}
	overflow := runewidth.StringWidth(name) - width
	if overflow <= 0 {
		return name
	}
	return runewidth.TruncateLeft(name, overflow, "")
}

// visibleNameCells calls fn for every name cell currently on screen with the
// entry it shows, the cell's left screen column, its row and its width.
func (fp *FileSystemPanel) visibleNameCells(fn func(entry *FileEntry, x, y, width int)) {
	height := fp.Table.ViewHeight
	if height <= 0 {
		return
	}
	columns := fp.gridColumnCount()
	for rowOffset := 0; rowOffset < height; rowOffset++ {
		row := fp.Table.TopPos + rowOffset
		y := fp.Table.Y1 + fp.Table.MarginTop + rowOffset
		x := fp.Table.X1
		for column := 0; column < columns && column < len(fp.Table.Columns); column++ {
			width := fp.Table.Columns[column].Width
			entryIndex := row
			if columns > 1 {
				entryIndex += column * height
			}
			if entryIndex >= 0 && entryIndex < len(fp.Entries) {
				fn(fp.Entries[entryIndex], x, y, width)
			}
			x += width + 1
		}
	}
}

// maxVisibleNameOverflow is the overflow of the longest name on screen: the
// farthest the name columns can usefully be scrolled right now.
func (fp *FileSystemPanel) maxVisibleNameOverflow() int {
	maxOverflow := 0
	fp.visibleNameCells(func(entry *FileEntry, _, _, width int) {
		maxOverflow = max(maxOverflow, panelNameOverflow(entry, width))
	})
	return maxOverflow
}

// namesOverflow reports whether at least one name on screen is cut off, so
// Alt+Left/Alt+Right have something to scroll.
func (fp *FileSystemPanel) NamesOverflow() bool {
	return fp.maxVisibleNameOverflow() > 0
}

func (fp *FileSystemPanel) clampNameLeftPos() {
	if fp.nameLeftPos <= 0 {
		fp.nameLeftPos = 0
		return
	}
	fp.nameLeftPos = min(fp.nameLeftPos, fp.maxVisibleNameOverflow())
}

// ScrollNames shifts the name columns by delta cells: positive brings the
// end of long names into view, negative scrolls back toward their
// beginning. It reports whether the position changed.
func (fp *FileSystemPanel) ScrollNames(delta int) bool {
	return fp.SetNameLeftPos(fp.nameLeftPos + delta)
}

// SetNameLeftPos scrolls the name columns to pos cells, clamped to the
// longest visible name (so any large value means "to the end"). It reports
// whether the position changed.
func (fp *FileSystemPanel) SetNameLeftPos(pos int) bool {
	pos = max(0, min(pos, fp.maxVisibleNameOverflow()))
	if pos == fp.nameLeftPos {
		return false
	}
	fp.nameLeftPos = pos
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
	return true
}

// drawNameScrollBrackets marks cut-off names the way far2l does: '{' in the
// cell left of a name whose beginning is scrolled out of view, '}' in the
// cell right of a name whose end is still hidden. Those cells are the panel
// border or the column separator; on the cursor row they take the cursor's
// background like drawCursorSeparators does.
func (fp *FileSystemPanel) drawNameScrollBrackets(scr *vtui.ScreenBuf) {
	if !fp.Table.IsVisible() {
		return
	}
	bracketAttr := func(neighborX, y int) uint64 {
		attr := vtui.Palette[theme.ColPanelBox]
		cellAttr := scr.GetCell(neighborX, y).Attributes
		if cellAttr&vtui.IsBgRGB != 0 {
			attr = vtui.SetRGBBack(attr, vtui.GetRGBBack(cellAttr))
		} else {
			attr = vtui.SetIndexBack(attr, vtui.GetIndexBack(cellAttr))
		}
		return (attr &^ vtui.BackgroundIntensity) | (cellAttr & vtui.BackgroundIntensity)
	}
	fp.visibleNameCells(func(entry *FileEntry, x, y, width int) {
		overflow := panelNameOverflow(entry, width)
		if overflow <= 0 {
			return
		}
		shift := panelNameShift(entry, width, fp.nameLeftPos)
		if shift > 0 && x-1 >= fp.X1 {
			attr := vtui.Palette[theme.ColPanelBox]
			if x-1 > fp.X1 {
				attr = bracketAttr(x, y)
			}
			scr.Write(x-1, y, vtui.StringToCharInfo("{", attr))
		}
		if overflow-shift > 0 && x+width <= fp.X2 {
			attr := vtui.Palette[theme.ColPanelBox]
			if x+width < fp.X2 {
				attr = bracketAttr(x+width-1, y)
			}
			scr.Write(x+width, y, vtui.StringToCharInfo("}", attr))
		}
	})
}

func (fp *FileSystemPanel) processScrollBarMouse(e *vtinput.InputEvent) bool {
	if fp.scrollBar == nil || config.App.PanelScrollbarMode == config.PanelScrollbarOff {
		return false
	}
	// Releases must reach ScrollBar so it can stop dragging and auto-repeat.
	if e.ButtonState == 0 {
		if config.App.PanelScrollbarMode == config.PanelScrollbarFull {
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
	if config.App.PanelScrollbarMode == config.PanelScrollbarMinimal {
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
		fp.Table.CellSelection = true
	} else {
		fp.Table.CellSelection = false
		fp.Table.SelectCol = 0
	}
}

func (fp *FileSystemPanel) galleryDensity(mode GalleryLayoutMode) int {
	if parsed, ok := ParseGalleryLayoutMode(string(mode)); ok {
		mode = parsed
	} else {
		mode = GalleryLayoutMasonry
	}
	if value, ok := fp.GalleryDensities[mode]; ok {
		return ClampGalleryDensity(mode, value)
	}
	defaultValue, _, _ := GalleryDensityLimits(mode)
	return defaultValue
}

// galleryDensitiesSnapshot is the bounded zoom contract needed by native
// renderers to prepare another presentation before it becomes active. Compact
// defaults stay omitted because their exact row pitch belongs to frontend font
// metrics; an explicit user override is included and persisted like any other
// mode.
func (fp *FileSystemPanel) galleryDensitiesSnapshot() map[string]int {
	densities := make(map[string]int, len(GalleryLayoutModes))
	for _, mode := range GalleryLayoutModes {
		if density, overridden := fp.GalleryDensities[mode]; overridden {
			densities[string(mode)] = ClampGalleryDensity(mode, density)
			continue
		}
		if defaultValue, _, _ := GalleryDensityLimits(mode); defaultValue > 0 {
			densities[string(mode)] = defaultValue
		}
	}
	return densities
}

func (fp *FileSystemPanel) effectiveGalleryLayoutMode() GalleryLayoutMode {
	if mode, ok := ParseGalleryLayoutMode(string(fp.GalleryLayoutMode)); ok {
		return mode
	}
	return GalleryLayoutMasonry
}

func (fp *FileSystemPanel) effectiveGalleryColumnCount() int {
	if fp.GalleryColumnCount >= MinGalleryColumnCount &&
		fp.GalleryColumnCount <= MaxGalleryColumnCount {
		return fp.GalleryColumnCount
	}
	return DefaultGalleryColumnCount
}

func (fp *FileSystemPanel) setGalleryLayoutState(mode GalleryLayoutMode, columnCount int) bool {
	parsed, ok := ParseGalleryLayoutMode(string(mode))
	if !ok {
		return false
	}
	if parsed == GalleryLayoutColumns {
		if columnCount < MinGalleryColumnCount || columnCount > MaxGalleryColumnCount {
			return false
		}
	} else if columnCount < MinGalleryColumnCount || columnCount > MaxGalleryColumnCount {
		// Non-column layouts preserve the last useful Columns setting.  Accept an
		// omitted/zero count from older native clients without overwriting it.
		columnCount = fp.effectiveGalleryColumnCount()
	}

	changed := fp.GalleryLayoutMode != parsed
	if parsed == GalleryLayoutColumns && fp.effectiveGalleryColumnCount() != columnCount {
		changed = true
	}
	fp.GalleryLayoutMode = parsed
	if parsed == GalleryLayoutColumns {
		fp.GalleryColumnCount = columnCount
	}
	if changed {
		fp.GalleryLayoutRevision++
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
	parsed, ok := ParseGalleryLayoutMode(string(mode))
	if !ok {
		return false
	}
	density = ClampGalleryDensity(parsed, density)
	if fp.GalleryDensities == nil {
		fp.GalleryDensities = make(map[GalleryLayoutMode]int)
	}
	if fp.galleryDensity(parsed) == density {
		return true
	}
	fp.GalleryDensities[parsed] = density
	fp.GalleryLayoutRevision++
	return true
}

// ResetGalleryDensity removes the per-mode override. Compact modes then return
// to the host's exact font-derived row height instead of persisting a rounded
// approximation of that default.
func (fp *FileSystemPanel) ResetGalleryDensity(mode GalleryLayoutMode) bool {
	parsed, ok := ParseGalleryLayoutMode(string(mode))
	if !ok {
		return false
	}
	if fp.GalleryDensities == nil {
		return true
	}
	if _, exists := fp.GalleryDensities[parsed]; !exists {
		return true
	}
	delete(fp.GalleryDensities, parsed)
	fp.GalleryLayoutRevision++
	return true
}

func CloneGalleryDensities(source map[GalleryLayoutMode]int) map[GalleryLayoutMode]int {
	clone := make(map[GalleryLayoutMode]int, len(source))
	for _, mode := range GalleryLayoutModes {
		if density, present := source[mode]; present {
			clone[mode] = ClampGalleryDensity(mode, density)
		}
	}
	return clone
}

type PanelGallerySessionState struct {
	LayoutMode  GalleryLayoutMode
	ColumnCount int
	Densities   map[GalleryLayoutMode]int
}

func DefaultPanelGallerySessionState() PanelGallerySessionState {
	return PanelGallerySessionState{
		LayoutMode:  GalleryLayoutMasonry,
		ColumnCount: DefaultGalleryColumnCount,
		Densities:   make(map[GalleryLayoutMode]int),
	}
}

func ClonePanelGallerySessionState(state PanelGallerySessionState) PanelGallerySessionState {
	mode, ok := ParseGalleryLayoutMode(string(state.LayoutMode))
	if !ok {
		mode = GalleryLayoutMasonry
	}
	columns := state.ColumnCount
	if columns < MinGalleryColumnCount || columns > MaxGalleryColumnCount {
		columns = DefaultGalleryColumnCount
	}
	densities := CloneGalleryDensities(state.Densities)
	return PanelGallerySessionState{
		LayoutMode:  mode,
		ColumnCount: columns,
		Densities:   densities,
	}
}

func (fp *FileSystemPanel) GetCursorIndex() int {
	if fp.CursorIdx >= len(fp.Entries) {
		fp.CursorIdx = len(fp.Entries) - 1
	}
	if fp.CursorIdx < 0 {
		fp.CursorIdx = 0
	}
	return fp.CursorIdx
}

func (fp *FileSystemPanel) SetCursorIndex(idx int) {
	if len(fp.Entries) == 0 {
		fp.CursorIdx = 0
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(fp.Entries) {
		idx = len(fp.Entries) - 1
	}
	fp.CursorIdx = idx

	// Sync table visual state
	if fp.gridColumnCount() == 1 {
		fp.Table.SetSelectPos(fp.CursorIdx)
		fp.Table.SelectCol = 0
		if fp.FastFindMode {
			h := fp.Table.ViewHeight
			if h > 2 && fp.CursorIdx >= fp.Table.TopPos+h-2 {
				fp.Table.TopPos = fp.CursorIdx - h + 3
				if fp.Table.TopPos < 0 {
					fp.Table.TopPos = 0
				}
			}
		}
	} else {
		h := fp.Table.ViewHeight
		if h <= 0 {
			h = 1
		}

		// 1. Ensure TopPos is sane for the current cursor
		if fp.CursorIdx < fp.Table.TopPos {
			fp.Table.TopPos = fp.CursorIdx
		} else if fp.CursorIdx >= fp.Table.TopPos+fp.gridColumnCount()*h {
			fp.Table.TopPos = fp.CursorIdx - fp.gridColumnCount()*h + 1
		}

		// Far-style 2-column scrolling: ensure cursorIdx is in [TopPos, TopPos + 2*H)
		if fp.CursorIdx < fp.Table.TopPos {
			fp.Table.TopPos = fp.CursorIdx
		} else if fp.CursorIdx >= fp.Table.TopPos+fp.gridColumnCount()*h {
			fp.Table.TopPos = fp.CursorIdx - fp.gridColumnCount()*h + 1
		}

		if fp.FastFindMode && h > 2 {
			rel := fp.CursorIdx - fp.Table.TopPos
			row := rel % h
			if row >= h-2 {
				shift := row - (h - 3)
				fp.Table.TopPos += shift
			}
		}

		if fp.Table.TopPos < 0 {
			fp.Table.TopPos = 0
		}

		rel := fp.CursorIdx - fp.Table.TopPos
		fp.Table.SelectCol = rel / h
		// Table internal rendering expects SelectPos to be absolute index in its row space
		// to correctly calculate vertical offset: y = Y1 + (SelectPos - TopPos)
		fp.Table.SelectPos = fp.Table.TopPos + (rel % h)

		// If we landed on a column that is theoretically correct but visually empty,
		// the table will handle it during Show, but we keep the absolute index.
	}
}

func (fp *FileSystemPanel) updateTitle(err error) {
	path := fp.Vfs.GetPath()
	title := ""
	if fp.ProviderOpenTarget != "" {
		// A standalone visual path is already the complete user-facing title.
		// Do not ask the source VFS to interpret a path owned by another provider.
		path = fp.ProviderOpenTarget
		title = path
	} else if tp, ok := fp.Vfs.(vfs.PanelTitleProvider); ok {
		title = tp.PanelTitle(path)
	}
	if title == "" {
		title = path
		if tp, ok := fp.Vfs.(vfs.TitleProvider); ok {
			if prefix := tp.GetTitle(); prefix != "" {
				title = prefix + ":" + title
			}
		}
	}

	if err != nil && err != context.Canceled {
		title += " [Error]"
	}
	fp.semanticTitle = title
	if err == nil && fp.IsLoading && !fp.catalogInteractive {
		title += " " + panelLoadingPulse[fp.loadingFrame%len(panelLoadingPulse)]
	}
	fp.currentTitle = title
	fp.Frame.SetTitle("")
}

func (fp *FileSystemPanel) StopLoadingAnimation() {
	fp.LoadingGeneration++
	if fp.LoadingTimer != nil {
		fp.LoadingTimer.Stop()
		fp.LoadingTimer = nil
	}
	fp.loadingVisible = false
}

func (fp *FileSystemPanel) startLoadingAnimation() {
	fp.StopLoadingAnimation()
	fp.loadingFrame = 0

	// Reflect the loading state in the title immediately (path/error text), but
	// the spinner glyph stays hidden until panelLoadingShowDelay elapses.
	fp.updateTitle(nil)

	// In tests, do not run the infinite timer loop to prevent task queue leakage.
	if DisableLoadingAnimationInTests && flag.Lookup("test.v") != nil {
		return
	}

	// Defer the visible pulse: only show it once the operation has been loading
	// for panelLoadingShowDelay. Fast operations finish before this fires and
	// never flash the spinner.
	generation := fp.LoadingGeneration
	frames := vtui.FrameManager
	fp.LoadingTimer = time.AfterFunc(panelLoadingShowDelay, func() {
		// Read on the goroutine that starts this work, not inside it: the
		// work outlives the call, and reading the global from it races
		// anything that reassigns vtui.FrameManager meanwhile.
		f := frames
		f.PostTask(func() {
			if !fp.IsLoading || fp.LoadingGeneration != generation {
				return
			}
			fp.loadingVisible = true
			fp.updateTitle(nil)
			f.Redraw()
			fp.scheduleLoadingPulse(generation)
		})
	})
}

// scheduleLoadingPulse advances the loading spinner while the panel stays in a
// loading state. It reuses fp.loadingTimer, so stopLoadingAnimation cancels it.
func (fp *FileSystemPanel) scheduleLoadingPulse(generation uint64) {
	var scheduleNext func()
	scheduleNext = func() {
		// Read the manager on this goroutine, not inside the posted task: the
		// work outlives the call and reading the global from it races anything
		// that reassigns vtui.FrameManager meanwhile.
		frames := vtui.FrameManager
		if frames == nil {
			return
		}
		fp.LoadingTimer = time.AfterFunc(panelLoadingPulseInterval, func() {
			frames.PostTaskWithRedrawDecision(func() bool {
				if !fp.IsLoading || fp.LoadingGeneration != generation {
					return false
				}
				fp.loadingFrame = (fp.loadingFrame + 1) % len(panelLoadingPulse)
				fp.updateTitle(nil)
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
	fp.readDirectoryEx(fp.Vfs != nil && fileops.SameVFSInstance(fp.committedListingVFS, fp.Vfs) &&
		fp.committedListingPath == fp.Vfs.GetPath())
}

// enqueueDirectoryLoad keeps at most one backend read running and one newer
// read waiting. Repeated navigation replaces the pending closure instead of
// creating a FIFO of stale Stat/ReadDir goroutines. The running request is
// cancelled by readDirectoryEx; it may still need to drain one FISH+ response,
// after which only the most recent path is allowed to start.
func (fp *FileSystemPanel) EnqueueDirectoryLoad(load func()) {
	fp.enqueueDirectoryLoadWithBenchmark(nil, load)
}

func (fp *FileSystemPanel) enqueueDirectoryLoadWithBenchmark(benchmark *navtrace.NavigationBenchmarkTrace, load func()) {
	request := &directoryLoadRequest{load: load, benchmark: benchmark}
	if benchmark != nil {
		request.enqueuedNs = navtrace.NavigationBenchmarkMonotonicNs()
		benchmark.EventAt("load.queued", "go.ui", request.enqueuedNs)
	}
	fp.loadQueueMu.Lock()
	if fp.loadWorkerActive {
		if replaced := fp.pendingDirectoryLoad; replaced != nil && replaced.benchmark != nil {
			replaced.benchmark.Event("load.queue.superseded", "go.worker",
				"supersededBy", navtrace.NavigationBenchmarkTraceName(benchmark))
		}
		fp.pendingDirectoryLoad = request
		fp.loadQueueMu.Unlock()
		return
	}
	fp.loadWorkerActive = true
	fp.LoadWorkerWG.Add(1)
	// Every worker is also counted process-wide. A worker reads globals while
	// it runs -- config.App and vtui.FrameManager, and the frame manager's task
	// queue when it posts back -- so anything that replaces one of those has to
	// know whether a worker is still out there. Panels are created deep inside
	// PanelsFrame.ResizeConsole as well as directly, so a per-panel WaitGroup
	// alone leaves no way to ask that question about the panels a caller never
	// sees. Production only counts; the tests are what wait.
	DirectoryLoadWorkers.Add(1)
	fp.loadQueueMu.Unlock()

	go func() {
		defer DirectoryLoadWorkers.Done()
		defer fp.LoadWorkerWG.Done()
		next := request
		for next != nil {
			if next.benchmark != nil {
				startedNs := navtrace.NavigationBenchmarkMonotonicNs()
				next.benchmark.EventAt("load.worker.started", "go.worker", startedNs,
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
	if task := fp.ProviderOpenTask; task != nil {
		fp.ProviderOpenTask = nil
		fp.ProviderOpenTarget = ""
		fp.ProviderOpenSourceSelect = ""
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
	if fp == nil || fp.ProviderOpenTask == nil {
		return
	}
	sourceSelection := fp.ProviderOpenSourceSelect
	sourcePath := fp.Vfs.GetPath()
	fp.cancelProviderOpen()
	fp.IsLoading = false
	fp.StopLoadingAnimation()
	fp.updateTitle(nil)
	fp.PendingSelection = sourceSelection
	fp.SuppressNextFolderHistory(sourcePath)
	fp.ReadDirectory()
	vtui.FrameManager.Redraw()
}

func (fp *FileSystemPanel) showProviderOpenDialog(status vfs.ProviderOpenStatus) {
	if fp == nil || fp.ProviderOpenTask == nil || status.Title == "" || status.Message == "" {
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
	btnCancel := vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel"))
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

func (fp *FileSystemPanel) PersistentPath() string {
	if fp != nil && fp.ProviderOpenTask != nil && fp.ProviderOpenTarget != "" {
		return fp.ProviderOpenTarget
	}
	if fp == nil || fp.Vfs == nil {
		return ""
	}
	path := fp.Vfs.GetPath()
	if !shouldPersistPanelPath(fp, path) {
		return ""
	}
	return path
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
	if fp == nil || fp.Vfs == nil || open == nil || onSuccess == nil {
		return false
	}

	fp.cancelProviderOpen()
	if fp.CancelLoad != nil {
		fp.CancelLoad()
		fp.CancelLoad = nil
	}
	fp.StopLoadingAnimation()

	sourceVFS := fp.Vfs
	sourcePath := sourceVFS.GetPath()
	sourceSelection := fp.GetSelectedName()
	fp.ProviderOpenTarget = persistentTarget
	fp.ProviderOpenSourceSelect = sourceSelection
	fp.IsLoading = true
	fp.catalogInteractive = false
	fp.startLoadingAnimation()
	// Keep the source rows as a stable placeholder while the provider opens.
	// Input is guarded until the provider switch, so these rows cannot dispatch
	// operations against the wrong VFS.
	fp.Refresh()
	vtui.FrameManager.Redraw()
	fp.ProviderOpenTask = vtui.RunAsync(func(task *vtui.TaskContext) {
		newVFS, err := open(task.Context)
		if err == nil && newVFS == nil {
			err = fmt.Errorf("provider returned no file system")
		}
		task.RunOnUI(func() {
			// A provider may hand back a typed nil wrapped in the interface
			// (e.g. (*ArchiveVFS)(nil), err): it compares != nil, so Close()
			// on it would panic. Normalize it to a plain error instead.
			if err == nil && isNilVFS(newVFS) {
				err = fmt.Errorf("provider returned no file system")
			}
			if fp.ProviderOpenTask != task {
				if !isNilVFS(newVFS) {
					_ = newVFS.Close()
				}
				return
			}
			fp.ProviderOpenTask = nil
			fp.closeProviderOpenDialog()
			resultCallback := fp.providerOpenResult
			fp.providerOpenResult = nil
			fp.ProviderOpenTarget = ""
			fp.ProviderOpenSourceSelect = ""
			if !fileops.SameVFSInstance(fp.Vfs, sourceVFS) || fp.Vfs.GetPath() != sourcePath {
				if !isNilVFS(newVFS) {
					_ = newVFS.Close()
				}
				return
			}
			if err != nil {
				if !isNilVFS(newVFS) {
					_ = newVFS.Close()
				}
				fp.IsLoading = false
				fp.updateTitle(err)
				fp.PendingSelection = sourceSelection
				fp.SuppressNextFolderHistory(sourcePath)
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
// belonged to a VFS it has just left. Only a real parent row is safe to keep
// interactive while the new listing is in flight.
func (fp *FileSystemPanel) showCurrentVFSLoadingRows() {
	fp.Entries = nil
	if fp.Vfs != nil && (!fp.Vfs.IsAtRoot() || fp.Vfs.ParentVFS() != nil) {
		fp.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
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
	benchmark := navtrace.NavigationBenchmarkCurrentUI()
	fromPath := ""
	direction := ""
	if benchmark != nil && fp.Vfs != nil {
		fromPath = fp.Vfs.GetPath()
		switch {
		case fileops.SameFolderHistoryPath(target, fromPath):
			direction = "same"
		case fileops.SameFolderHistoryPath(target, fp.Vfs.Dir(fromPath)):
			direction = "parent"
		default:
			direction = "child"
		}
		benchmark.SetPaths(fromPath, target, direction)
		benchmark.Event("path.set.begin", "go.ui", "fromPath", fromPath,
			"toPath", target, "direction", direction)
	}
	strategy := "verified"
	var err error
	if setter, ok := fp.Vfs.(vfs.OptimisticPathSetter); knownDirectory && ok {
		strategy = "optimistic"
		if fp.CancelLoad != nil {
			fp.CancelLoad()
			fp.CancelLoad = nil
		}
		err = setter.SetPathOptimistic(target)
	} else {
		err = fp.Vfs.SetPath(target)
	}
	if benchmark != nil {
		fields := []any{"fromPath", fromPath, "toPath", target, "direction", direction,
			"strategy", strategy, "ok", err == nil}
		if err != nil {
			fields = append(fields, "error", err.Error())
		}
		benchmark.Event("path.set.end", "go.ui", fields...)
	}
	return err
}

// setKnownDirectoryPath is reserved for a directory identity already supplied
// by the active VFS (for example Enter on a panel row). The following ReadDir
// remains authoritative if that identity went stale.
func (fp *FileSystemPanel) SetKnownDirectoryPath(target string) error {
	return fp.setDirectoryPath(target, true)
}

func (fp *FileSystemPanel) setVerifiedDirectoryPath(target string) error {
	return fp.setDirectoryPath(target, false)
}

func (fp *FileSystemPanel) SuppressNextFolderHistory(path string) {
	fp.suppressFolderHistoryToken++
	fp.suppressFolderHistoryPath = path
}

func (fp *FileSystemPanel) clearFolderHistorySuppression() {
	fp.suppressFolderHistoryToken++
	fp.suppressFolderHistoryPath = ""
}

func (fp *FileSystemPanel) FolderHistorySuppression(path string) (uint64, bool) {
	if !fileops.SameFolderHistoryPath(path, fp.suppressFolderHistoryPath) {
		return 0, false
	}
	return fp.suppressFolderHistoryToken, true
}

func (fp *FileSystemPanel) ConsumeFolderHistorySuppression(path string, token uint64) bool {
	if token == 0 || token != fp.suppressFolderHistoryToken || !fileops.SameFolderHistoryPath(path, fp.suppressFolderHistoryPath) {
		return false
	}
	fp.suppressFolderHistoryPath = ""
	return true
}

// shouldPersistPanelPath keeps paths that can be restored without the VFS
// instance that produced them. A nested provider may expose an absolute
// remote path such as /home/user, but saving it in session.ini would make the
// next startup interpret that path as a local OS directory.
func shouldPersistPanelPath(fp *FileSystemPanel, path string) bool {
	if fp == nil || fp.Vfs == nil || path == "" {
		return false
	}
	if fp.Vfs.ParentVFS() == nil {
		return true
	}
	return fileops.IsPersistentURIPath(path) || vfs.FindStandaloneProvider(context.Background(), nil, path) != nil
}

// shouldRecordFolderHistory prevents an internal path of a nested VFS from
// leaking into the global OS-folder history. Some providers (notably NetFox)
// expose an absolute remote path such as /home/user, but that path cannot be
// reopened after the panel leaves the provider and would otherwise be treated
// as a local directory. Keep URI and standalone-provider paths, which have a
// host-level restore route (archives and other persistent virtual paths).
func ShouldRecordFolderHistory(fp *FileSystemPanel, path string) bool {
	if fp == nil || fp.Vfs == nil || path == "" {
		return false
	}
	if fp.Vfs.ParentVFS() == nil {
		return true
	}
	if fileops.IsPersistentURIPath(path) || vfs.FindStandaloneProvider(context.Background(), nil, path) != nil {
		return true
	}
	// filepath.IsAbs does not treat a slash-rooted POSIX path as absolute on
	// Windows, although a remote Unix VFS can legitimately return one there.
	return !filepath.IsAbs(path) && !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "\\")
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
	if err := fp.SetKnownDirectoryPath(parentPath); err != nil {
		vtui.DebugLog("PANEL[%p]: Failed to restore parent %q after reading %q: %v", fp, parentPath, failedPath, err)
		return false
	}
	return true
}

func (fp *FileSystemPanel) readDirectoryEx(keepEntries bool) {
	if !keepEntries {
		fp.catalogRefreshDelta = nil
	}
	benchmark := navtrace.NavigationBenchmarkCurrentUI()
	if previous := fp.benchmarkLoadTrace; previous != nil && previous != benchmark {
		previous.Event("navigation.cancelled", "go.ui",
			"supersededBy", navtrace.NavigationBenchmarkTraceName(benchmark))
	}
	fp.benchmarkLoadTrace = benchmark
	if fp.CancelLoad != nil {
		fp.CancelLoad()
		fp.CancelLoad = nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	fp.loadCtx = ctx
	fp.CancelLoad = cancel
	fp.loadGeneration++
	loadGeneration := fp.loadGeneration
	fp.resetSemanticPendingSource(loadGeneration)
	fp.IsLoading = true
	fp.catalogInteractive = false
	fp.catalogLogicalCount = 0
	fp.startLoadingAnimation()

	loadVFS := fp.Vfs
	path := loadVFS.GetPath()
	// Background directory work must use one immutable settings/renderer
	// snapshot; tests and runtime settings can replace the globals meanwhile.
	loadSyncPanel := config.App.SyncPanelLoad || keepEntries
	loadShowHidden := config.App.ShowHiddenFiles
	loadFrames := vtui.FrameManager
	if benchmark != nil {
		_, fromPath, _, direction := benchmark.PathFields()
		if fromPath == "" {
			fromPath = path
			if direction == "" {
				direction = "refresh"
			}
		}
		benchmark.SetPaths(fromPath, path, direction)
		benchmark.Event("directory_read.begin", "go.ui", "path", path, "keepEntries", keepEntries)
	}
	suppressionToken, hasFolderHistorySuppression := fp.FolderHistorySuppression(path)
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
	directoryChanged := fp.lastLoadedPath != "" && !fileops.SameFolderHistoryPath(fp.lastLoadedPath, path)
	suppressFolderHistory := hasFolderHistorySuppression && fp.ConsumeFolderHistorySuppression(path, suppressionToken)
	if directoryChanged {
		for k := range fp.SelectedItems {
			delete(fp.SelectedItems, k)
		}
		fp.DirectoryEpoch++
		fp.selectionEpoch = make(map[string]uint64)
	}
	fp.lastLoadedPath = path
	if directoryChanged && !suppressFolderHistory && ShouldRecordFolderHistory(fp, path) {
		// Record accepted navigation in UI order, not in backend completion
		// order. Otherwise an older slow cloud ReadDir can finish after a newer
		// visit and move its path to the front of the global MRU history.
		if benchmark != nil {
			benchmark.Event("history.persist.begin", "go.ui", "path", path)
		}
		history.AddFolderHistory(path)
		if benchmark != nil {
			benchmark.Event("history.persist.end", "go.ui", "path", path)
		}
	} else if benchmark != nil {
		reason := "unchanged"
		if suppressFolderHistory {
			reason = "suppressed"
		}
		benchmark.Event("history.persist.skipped", "go.ui", "path", path, "reason", reason)
	}

	if !keepEntries && fp.PendingSelection == "" {
		oldName := fp.GetRawSelectedName()
		if oldName != "" && oldName != ".." {
			fp.PendingSelection = oldName
		}
	}

	// Advance the observation epoch only when a real destination listing is
	// committed. The placeholder does not identify any destination file.
	sourceEpochAdvanced := false
	advanceSourceEpoch := func() {
		if sourceEpochAdvanced {
			return
		}
		fp.mediaSourceEpoch++
		sourceEpochAdvanced = true
	}

	isFirstChunk := true
	if !keepEntries {
		fp.catalogProvisional = true
		fp.Entries = nil
		if showUpEntry {
			fp.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
		}
		fp.SetCursorIndex(0)
		fp.Refresh()
		deferredPlaceholder := false
		if vtui.FrameManager != nil {
			screen := vtui.FrameManager.Screen()
			if screen != nil {
				if _, ok := screen.Renderer.(vtui.SemanticInputUnchangedRenderer); ok {
					deferredPlaceholder = vtui.FrameManager.DeclareCurrentInputUnchanged()
				}
			}
		}
		if benchmark != nil {
			benchmark.Event("model.provisional.ready", "go.ui", "phase", "placeholder",
				"entries", len(fp.Entries), "cursorIndex", fp.GetCursorIndex(),
				"semanticDeferred", deferredPlaceholder)
			if !deferredPlaceholder {
				navtrace.NavigationBenchmarkPublishScene(benchmark, "placeholder")
			}
		}
		if !deferredPlaceholder {
			vtui.FrameManager.Redraw()
		}
	}
	if keepEntries {
		fp.catalogProvisional = false
	}
	if keepEntries && benchmark != nil {
		benchmark.Event("model.provisional.ready", "go.ui", "phase", "retained",
			"entries", len(fp.Entries), "cursorIndex", fp.GetCursorIndex())
		navtrace.NavigationBenchmarkPublishScene(benchmark, "retained")
	}

	phasedReader := phasedDirectoryReaderFor(loadVFS)
	usePhasedRead := phasedReader != nil && (keepEntries ||
		(!loadSyncPanel && panelSortSupportsPhasedDirectoryRead(fp.SortMode)))
	loadSortMode, loadSortReverse := fp.SortMode, fp.SortReverse
	loadSortGroups := fp.UseSortGroups
	previewEligible := !fp.sortGroupsActive() && loadSortMode == SortName && !loadSortReverse &&
		!loadSyncPanel
	windowedReader := windowedDirectoryReaderFor(loadVFS)
	useWindowedRead := windowedReader != nil && !keepEntries && usePhasedRead && previewEligible &&
		semantic.PanelCatalogRowsIsEnabled()
	loadPendingSelection := fp.PendingSelection
	loadIsCurrent := func() bool {
		if ctx.Err() != nil || fp.loadCtx != ctx || fp.loadGeneration != loadGeneration ||
			!fileops.SameVFSInstance(fp.Vfs, loadVFS) {
			return false
		}
		return fileops.SameFolderHistoryPath(loadVFS.GetPath(), path)
	}

	fp.enqueueDirectoryLoadWithBenchmark(benchmark, func() {
		if ctx.Err() != nil {
			if benchmark != nil {
				benchmark.Event("load.worker.cancelled_before_read", "go.worker", "path", path)
			}
			return
		}
		var accumulated []vfs.VFSItem
		var accumulatedByName map[string]int
		var pendingMetadata []vfs.VFSItem
		chunkCount := 0
		directoryEntryCount := 0
		metadataChunkCount := 0
		authoritativeCatalogQueued := false
		authoritativeWindowQueued := false
		authoritativePresentationComplete := false
		if benchmark != nil {
			benchmark.Event("filesystem.readdir.begin", "go.worker", "path", path,
				"phased", usePhasedRead)
		}

		publishCatalogPreview := func(chunk []vfs.VFSItem) {
			// A preview is useful only when the panel's requested order matches the
			// reader's stable leading directory window. The authoritative base below
			// remains the sole source for every other sort order.
			if len(chunk) == 0 || !previewEligible || ctx.Err() != nil {
				return
			}
			previewEntries := fileEntriesFromItems(chunk)
			if len(previewEntries) == 0 || ctx.Err() != nil {
				return
			}
			previewQueuedNs := int64(0)
			if benchmark != nil {
				previewQueuedNs = navtrace.NavigationBenchmarkMonotonicNs()
				benchmark.EventAt("model.preview.queued", "go.worker", previewQueuedNs,
					"entries", len(previewEntries))
			}
			loadFrames.PostPriorityTaskWithRedrawDecision(func() bool {
				if !loadIsCurrent() || !isFirstChunk {
					return false
				}

				target := fp.PendingSelection
				if target != "" && target != ".." {
					found := false
					for _, entry := range previewEntries {
						if entry.Name == target {
							found = true
							break
						}
					}
					// Showing an unrelated first row before snapping to a remembered
					// child is worse than retaining the previous panel for a few ms.
					if !found {
						return false
					}
				}

				advanceSourceEpoch()
				fp.Entries = nil
				if showUpEntry {
					fp.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
				}
				if len(fp.SelectedItems) != 0 ||
					(fp.previousSelectionMatches(loadVFS, path) && len(fp.previousSelection) != 0) {
					for _, entry := range previewEntries {
						fp.applyPersistentSelection(entry, loadVFS, path)
					}
				}
				fp.Entries = append(fp.Entries, previewEntries...)
				fp.SortEntries()
				fp.catalogProvisional = true
				fp.catalogInteractive = true
				fp.StopLoadingAnimation()
				fp.updateTitle(nil)

				cursorIndex := 0
				if target != "" {
					for index, entry := range fp.Entries {
						if entry.Name == target {
							cursorIndex = index
							break
						}
					}
				}
				fp.SetCursorIndex(cursorIndex)
				if benchmark != nil {
					startedNs := navtrace.NavigationBenchmarkMonotonicNs()
					benchmark.EventAt("model.preview.ready", "go.ui", startedNs,
						"entries", len(fp.Entries), "queueNs", startedNs-previewQueuedNs,
						"cursorIndex", fp.GetCursorIndex())
					navtrace.NavigationBenchmarkPublishScene(benchmark, "preview")
				}
				publishPanelCatalogImmediate(fp, benchmark)
				fp.Refresh()
				loadFrames.Redraw()
				return true
			})
		}

		publishCatalogWindow := func(window vfs.DirectoryWindow) {
			if !useWindowedRead || window.TotalCount < 0 ||
				len(window.Entries) > window.TotalCount || ctx.Err() != nil {
				return
			}
			windowEntries := fileEntriesFromItems(window.Entries)
			target := loadPendingSelection
			targetFound := target == "" || target == ".." && showUpEntry
			if !targetFound {
				for _, entry := range windowEntries {
					if entry.Name == target {
						targetFound = true
						break
					}
				}
			}
			// A remembered row outside the bounded prefix needs the complete
			// catalog to calculate its authoritative index and viewport.
			if !targetFound {
				return
			}
			authoritativeWindowQueued = true
			windowQueuedNs := int64(0)
			if benchmark != nil {
				windowQueuedNs = navtrace.NavigationBenchmarkMonotonicNs()
				benchmark.EventAt("model.window.queued", "go.worker", windowQueuedNs,
					"entries", len(windowEntries), "totalCount", window.TotalCount)
			}
			loadFrames.PostPriorityTaskWithRedrawDecision(func() bool {
				if !loadIsCurrent() || !isFirstChunk {
					return false
				}
				advanceSourceEpoch()
				fp.Entries = nil
				if showUpEntry {
					fp.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
				}
				if len(fp.SelectedItems) != 0 ||
					(fp.previousSelectionMatches(loadVFS, path) && len(fp.previousSelection) != 0) {
					for _, entry := range windowEntries {
						fp.applyPersistentSelection(entry, loadVFS, path)
					}
				}
				fp.Entries = append(fp.Entries, windowEntries...)
				fp.sortEntrySlice(fp.Entries)
				fp.markSemanticCatalogMutation()
				fp.catalogLogicalCount = window.TotalCount
				if showUpEntry {
					fp.catalogLogicalCount++
				}
				fp.catalogProvisional = false
				fp.catalogInteractive = true
				fp.StopLoadingAnimation()
				fp.updateTitle(nil)

				cursorIndex := 0
				if target != "" {
					for index, entry := range fp.Entries {
						if entry.Name == target {
							cursorIndex = index
							if fp.PendingSelection == target {
								fp.PendingSelection = ""
							}
							break
						}
					}
				}
				fp.SetCursorIndex(cursorIndex)
				if benchmark != nil {
					startedNs := navtrace.NavigationBenchmarkMonotonicNs()
					benchmark.EventAt("model.window.ready", "go.ui", startedNs,
						"entries", len(fp.Entries), "totalCount", fp.catalogLogicalCount,
						"queueNs", startedNs-windowQueuedNs, "cursorIndex", fp.GetCursorIndex())
					navtrace.NavigationBenchmarkPublishScene(benchmark, "window")
				}
				authoritativePresentationComplete =
					publishPanelCatalogImmediate(fp, benchmark)
				fp.Refresh()
				loadFrames.Redraw()
				return true
			})
		}

		queueCatalogEntries := func(
			newEntries []*FileEntry,
			authoritativeBase bool,
			chunkIndex int,
		) {
			if ctx.Err() != nil {
				return
			}
			preSorted := false
			if authoritativeBase && len(newEntries) > 1 {
				sortStartedNs := navtrace.NavigationBenchmarkMonotonicNs()
				sorter := FileSystemPanel{
					SortMode:      loadSortMode,
					SortReverse:   loadSortReverse,
					UseSortGroups: loadSortGroups,
				}
				sorter.sortEntrySlice(newEntries)
				sortFinishedNs := navtrace.NavigationBenchmarkMonotonicNs()
				preSorted = true
				if benchmark != nil {
					benchmark.EventAt("model.chunk.presorted", "go.worker",
						sortFinishedNs, "chunk", chunkIndex,
						"entries", len(newEntries), "durationNs",
						sortFinishedNs-sortStartedNs)
				}
			}

			chunkQueuedNs := int64(0)
			if benchmark != nil {
				chunkQueuedNs = navtrace.NavigationBenchmarkMonotonicNs()
				benchmark.EventAt("model.chunk.queued", "go.worker", chunkQueuedNs,
					"chunk", chunkIndex, "chunkEntries", len(newEntries))
			}
			if authoritativeBase {
				authoritativeCatalogQueued = true
				// The worker owns an immutable full source before the UI append
				// task runs. Publish that source to the live pager so a native
				// viewport request which follows the bounded window can be served
				// immediately instead of being rejected against fp.entries.
				fp.setSemanticPendingSource(
					loadGeneration, newEntries, showUpEntry)
			}
			loadFrames.PostTaskWithRedrawDecision(func() bool {
				if benchmark != nil {
					startedNs := navtrace.NavigationBenchmarkMonotonicNs()
					benchmark.EventAt("model.chunk.started", "go.ui", startedNs,
						"chunk", chunkIndex, "queueNs", startedNs-chunkQueuedNs)
				}
				if !loadIsCurrent() {
					return false
				}

				currentSelected := fp.GetRawSelectedName()
				if fp.PendingSelection == "" {
					if currentSelected != "" && currentSelected != ".." {
						fp.PendingSelection = currentSelected
					}
				}

				if isFirstChunk {
					advanceSourceEpoch()
					fp.Entries = nil
					if showUpEntry {
						upItem := vfs.VFSItem{Name: "..", IsDir: true}
						fp.Entries = []*FileEntry{{VFSItem: upItem}}
					}
					isFirstChunk = false
				}

				selectionStartedNs := navtrace.NavigationBenchmarkMonotonicNs()
				// Empty selection maps are overwhelmingly common during folder
				// navigation; avoid two hash lookups for every catalog row.
				if len(fp.SelectedItems) != 0 ||
					(fp.previousSelectionMatches(loadVFS, path) && len(fp.previousSelection) != 0) {
					for _, e := range newEntries {
						fp.applyPersistentSelection(e, loadVFS, path)
					}
				}

				selectionFinishedNs := navtrace.NavigationBenchmarkMonotonicNs()
				fp.Entries = append(fp.Entries, newEntries...)
				if authoritativeBase {
					fp.clearSemanticPendingSource(loadGeneration)
				}
				appendFinishedNs := navtrace.NavigationBenchmarkMonotonicNs()
				if preSorted && fp.SortMode == loadSortMode &&
					fp.SortReverse == loadSortReverse && fp.UseSortGroups == loadSortGroups {
					if !authoritativeWindowQueued {
						fp.markSemanticCatalogMutation()
					}
				} else {
					fp.sortEntrySlice(fp.Entries)
					if !authoritativeWindowQueued {
						fp.markSemanticCatalogMutation()
					}
				}
				sortFinishedNs := navtrace.NavigationBenchmarkMonotonicNs()
				if authoritativeBase {
					// This catalog is complete and authoritative for row identity and
					// order. Metadata is deliberately still loading in the background.
					fp.catalogProvisional = false
					fp.catalogInteractive = true
					fp.StopLoadingAnimation()
					fp.updateTitle(nil)
				}

				// Фокусировка на нужном файле
				snapped := false
				target := fp.PendingSelection
				if target == "" {
					target = currentSelected
				}

				if target != "" {
					for i, entry := range fp.Entries {
						if entry.Name == target {
							fp.SetCursorIndex(i)
							if entry.Name == fp.PendingSelection {
								fp.PendingSelection = ""
							}
							snapped = true
							break
						}
					}
				}

				if !snapped && fp.PendingSelection == "" && (fp.CursorIdx >= len(fp.Entries) || fp.CursorIdx < 0) {
					fp.SetCursorIndex(0)
				}

				if benchmark != nil {
					benchmark.Event("model.chunk.ready", "go.ui", "chunk", chunkIndex,
						"entries", len(fp.Entries), "cursorIndex", fp.GetCursorIndex(),
						"selectionNs", selectionFinishedNs-selectionStartedNs,
						"appendNs", appendFinishedNs-selectionFinishedNs,
						"sortNs", sortFinishedNs-appendFinishedNs,
						"windowAlreadyPublished", authoritativeWindowQueued)
					if !authoritativeWindowQueued {
						navtrace.NavigationBenchmarkPublishScene(benchmark, "chunk")
					}
				}
				if authoritativeBase && !authoritativeWindowQueued {
					authoritativePresentationComplete =
						publishPanelCatalogImmediate(fp, benchmark)
				}
				// The native catalog does not depend on the hidden terminal table.
				// Publish its bounded page first, then rebuild the fallback rows.
				fp.Refresh()

				if !authoritativeWindowQueued {
					loadFrames.Redraw() // Рисуем каждый чанк!
					return true
				}
				return false
			})
		}

		publishCatalogChunk := func(chunk []vfs.VFSItem, authoritativeBase bool) {
			chunkCount++
			chunkIndex := chunkCount
			if benchmark != nil {
				benchmark.Event("filesystem.readdir.chunk", "go.worker", "path", path,
					"chunk", chunkIndex, "chunkEntries", len(chunk),
					"entriesBefore", len(accumulated), "phase", "base")
			}
			if ctx.Err() != nil {
				return
			}
			// Directory-reader chunks are immutable after the callback. Take
			// ownership of the first complete base instead of copying tens of
			// thousands of VFSItem values into an identical accumulator.
			if len(accumulated) == 0 && authoritativeBase {
				accumulated = chunk
			} else {
				accumulated = append(accumulated, chunk...)
			}
			directoryEntryCount = len(accumulated)
			if ctx.Err() != nil || loadSyncPanel {
				return
			}

			conversionStartedNs := int64(0)
			if benchmark != nil {
				conversionStartedNs = navtrace.NavigationBenchmarkMonotonicNs()
			}
			newEntries := fileEntriesFromItems(chunk)
			if benchmark != nil {
				conversionFinishedNs := navtrace.NavigationBenchmarkMonotonicNs()
				benchmark.EventAt("model.chunk.converted", "go.worker", conversionFinishedNs,
					"chunk", chunkIndex, "entries", len(newEntries),
					"durationNs", conversionFinishedNs-conversionStartedNs)
			}
			queueCatalogEntries(newEntries, authoritativeBase, chunkIndex)
		}

		mergeMetadataChunk := func(chunk []vfs.VFSItem) {
			metadataChunkCount++
			if ctx.Err() != nil {
				return
			}
			// Enrich the authoritative names/types base without disturbing its
			// identity or order.
			if accumulatedByName == nil {
				accumulatedByName = make(map[string]int, len(accumulated))
				for index, item := range accumulated {
					accumulatedByName[item.Name] = index
				}
			}
			metadataToPublish := make([]vfs.VFSItem, 0, len(chunk))
			for _, item := range chunk {
				index, ok := accumulatedByName[item.Name]
				if !ok || index < 0 || index >= len(accumulated) ||
					accumulated[index].IsDir != item.IsDir || accumulated[index].NoExtension != item.NoExtension {
					metadataToPublish = append(metadataToPublish, item)
					continue
				}
				// On Windows the base phase is built from the same WIN32_FIND_DATA
				// record as the metadata phase. Keep the base row and, most
				// importantly, do not queue a second full-catalog UI mutation for
				// metadata that is already complete. If the record changed between
				// phases, retain the normal enrichment path.
				baseItem := accumulated[index]
				if baseItem.SizeKnown && baseItem.Size == item.Size &&
					baseItem.MTime.Equal(item.MTime) &&
					baseItem.IsExecutable == item.IsExecutable &&
					baseItem.PhysicalSize == item.PhysicalSize {
					continue
				}
				accumulated[index] = item
				metadataToPublish = append(metadataToPublish, item)
			}
			pendingMetadata = append(pendingMetadata, metadataToPublish...)
			if benchmark != nil {
				benchmark.Event("filesystem.readdir.chunk", "go.worker", "path", path,
					"chunk", metadataChunkCount, "chunkEntries", len(metadataToPublish),
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
				metadataQueuedNs = navtrace.NavigationBenchmarkMonotonicNs()
				benchmark.EventAt("model.metadata.queued", "go.worker", metadataQueuedNs,
					"chunks", metadataChunkCount, "entries", len(metadataCopy))
			}
			loadFrames.PostTaskWithRedrawDecision(func() bool {
				if !loadIsCurrent() {
					return false
				}
				byName := make(map[string]*FileEntry, len(fp.Entries))
				for _, entry := range fp.Entries {
					if entry != nil && entry.Name != ".." {
						byName[entry.Name] = entry
					}
				}

				// Validate the entire visible subset before mutating any row. A
				// provider that changes identity/type between phases has violated
				// the capability contract; retaining the usable base is safer than
				// partially applying a mismatched metadata chunk.
				for _, item := range metadataCopy {
					if !loadShowHidden && item.Name != ".." && item.IsHidden {
						continue
					}
					entry, ok := byName[item.Name]
					if !ok || entry.IsDir != item.IsDir || entry.NoExtension != item.NoExtension {
						return false
					}
				}
				for _, item := range metadataCopy {
					if !loadShowHidden && item.Name != ".." && item.IsHidden {
						continue
					}
					entry := byName[item.Name]
					entry.VFSItem = item
				}
				fp.CommitSemanticMetadataMutation()
				fp.Refresh()
				if benchmark != nil {
					startedNs := navtrace.NavigationBenchmarkMonotonicNs()
					benchmark.EventAt("model.metadata.ready", "go.ui", startedNs,
						"chunks", metadataChunkCount, "queueNs", startedNs-metadataQueuedNs,
						"entries", len(fp.Entries))
					navtrace.NavigationBenchmarkPublishScene(benchmark, "metadata")
				}
				loadFrames.Redraw()
				return true
			})
		}

		var err error
		if usePhasedRead {
			onPhasedChunk := func(phase vfs.DirectoryReadPhase, chunk []vfs.VFSItem) {
				switch phase {
				case vfs.DirectoryReadPreview:
					if !keepEntries {
						publishCatalogPreview(chunk)
					}
				case vfs.DirectoryReadBase:
					publishCatalogChunk(chunk, true)
				case vfs.DirectoryReadMetadata:
					mergeMetadataChunk(chunk)
				}
			}
			if useWindowedRead {
				sourceWriter := &directoryFileEntryWriter{}
				err = windowedReader.ReadDirWindowed(
					ctx, path,
					vfs.DirectoryWindowRequest{
						Limit:         initialPanelCatalogRowsLimit,
						IncludeHidden: loadShowHidden,
						Writer:        sourceWriter,
					},
					publishCatalogWindow,
					onPhasedChunk,
				)
				if err == nil && sourceWriter.completed && ctx.Err() == nil {
					chunkCount++
					chunkIndex := chunkCount
					directoryEntryCount = len(sourceWriter.entries)
					if benchmark != nil {
						benchmark.Event("filesystem.readdir.chunk", "go.worker", "path", path,
							"chunk", chunkIndex, "chunkEntries", directoryEntryCount,
							"entriesBefore", 0, "phase", "direct-source")
						benchmark.Event("model.chunk.converted", "go.worker",
							"chunk", chunkIndex, "entries", directoryEntryCount,
							"durationNs", 0, "directSource", true)
					}
					queueCatalogEntries(sourceWriter.entries, true, chunkIndex)
				}
			} else {
				err = phasedReader.ReadDirPhased(ctx, path, onPhasedChunk)
			}
		} else {
			err = loadVFS.ReadDir(ctx, path, func(chunk []vfs.VFSItem) {
				publishCatalogChunk(chunk, false)
			})
		}
		if benchmark != nil {
			fields := []any{"path", path, "chunks", chunkCount, "entries", directoryEntryCount, "ok", err == nil}
			if usePhasedRead {
				fields = append(fields, "metadataChunks", metadataChunkCount, "phased", true)
			}
			if err != nil {
				fields = append(fields, "error", err.Error())
			}
			benchmark.Event("filesystem.readdir.end", "go.worker", fields...)
		}

		if ctx.Err() != nil {
			if benchmark != nil {
				benchmark.Event("load.worker.cancelled_after_read", "go.worker", "path", path)
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
				benchmark.Event("filesystem.stat_current.begin", "go.worker", "path", path)
			}
			dirStat, dirStatErr = loadVFS.Stat(ctx, path)
			if benchmark != nil {
				fields := []any{"path", path, "ok", dirStatErr == nil}
				if dirStatErr != nil {
					fields = append(fields, "error", dirStatErr.Error())
				}
				benchmark.Event("filesystem.stat_current.end", "go.worker", fields...)
			}
			if ctx.Err() != nil {
				return
			}
			if showUpEntry {
				parentPath := loadVFS.Dir(path)
				if parentPath == path && dirStatErr == nil {
					if benchmark != nil {
						benchmark.Event("filesystem.stat_parent.reused", "go.worker",
							"path", parentPath, "source", "current")
					}
					upItemStat = dirStat
					hasUpItemStat = true
				} else {
					if benchmark != nil {
						benchmark.Event("filesystem.stat_parent.begin", "go.worker", "path", parentPath)
					}
					pStat, statErr := loadVFS.Stat(ctx, parentPath)
					if benchmark != nil {
						fields := []any{"path", parentPath, "ok", statErr == nil}
						if statErr != nil {
							fields = append(fields, "error", statErr.Error())
						}
						benchmark.Event("filesystem.stat_parent.end", "go.worker", fields...)
					}
					if statErr == nil {
						upItemStat = pStat
						hasUpItemStat = true
					}
				}
			}
		} else if benchmark != nil {
			benchmark.Event("filesystem.stats.skipped", "go.worker", "reason", "readdir_error")
		}
		if ctx.Err() != nil {
			if benchmark != nil {
				benchmark.Event("load.worker.cancelled_after_stats", "go.worker", "path", path)
			}
			return
		}
		upItem := panelUpEntryItem(upItemStat, hasUpItemStat)
		completionQueuedNs := int64(0)
		if benchmark != nil {
			completionQueuedNs = navtrace.NavigationBenchmarkMonotonicNs()
			benchmark.EventAt("model.final.queued", "go.worker", completionQueuedNs,
				"path", path, "entries", directoryEntryCount, "chunks", chunkCount)
		}
		loadFrames.PostTaskWithRedrawDecision(func() (needsRedraw bool) {
			completionPresentationChanged := false
			if benchmark != nil {
				startedNs := navtrace.NavigationBenchmarkMonotonicNs()
				benchmark.EventAt("model.final.started", "go.ui", startedNs,
					"path", path, "queueNs", startedNs-completionQueuedNs)
			}
			if !loadIsCurrent() {
				// This completion no longer owns the panel. Do not dereference the
				// current VFS here: another navigation may already have closed it.
				if benchmark != nil {
					benchmark.Event("model.final.discarded", "go.ui", "path", path,
						"reason", "superseded")
				}
				return
			}
			needsRedraw = true

			refreshChanged := !keepEntries
			if loadSyncPanel && err == nil {
				newEntries := make([]*FileEntry, 0, len(accumulated)+1)
				if showUpEntry {
					newEntries = append(newEntries, &FileEntry{VFSItem: upItem})
				}
				for _, item := range accumulated {
					if !loadShowHidden && item.Name != ".." && item.IsHidden {
						continue
					}
					entry := &FileEntry{VFSItem: item}
					fp.applyPersistentSelection(entry, loadVFS, path)
					newEntries = append(newEntries, entry)
				}
				fp.sortEntrySlice(newEntries)
				if keepEntries {
					refreshChanged = fp.reconcileDirectoryEntries(newEntries)
				} else {
					fp.Entries = newEntries
				}
				if refreshChanged {
					completionPresentationChanged = true
					advanceSourceEpoch()
					fp.markSemanticCatalogMutation()
				}

				if fp.PendingSelection != "" {
					fp.SelectName(fp.PendingSelection)
					fp.PendingSelection = ""
				} else if fp.CursorIdx >= len(fp.Entries) || fp.CursorIdx < 0 {
					fp.SetCursorIndex(0)
				}
				isFirstChunk = false
			}

			if err == nil {
				// Clean up persistent selection only after the fresh list has been
				// built, so the completed ReadDir result is authoritative.
				previousSelectionNeedsCleanup :=
					fp.previousSelectionMatches(loadVFS, path) &&
						len(fp.previousSelection) != 0
				if len(fp.SelectedItems) != 0 || previousSelectionNeedsCleanup {
					validNames := make(map[string]bool, len(fp.Entries))
					for _, e := range fp.Entries {
						validNames[e.Name] = true
					}
					for name := range fp.SelectedItems {
						if !validNames[name] {
							delete(fp.SelectedItems, name)
							completionPresentationChanged = true
						}
					}
					if previousSelectionNeedsCleanup {
						for name := range fp.previousSelection {
							if !validNames[name] {
								delete(fp.previousSelection, name)
							}
						}
					}
				}
				// The chunk path creates ".." before the deferred parent Stat is
				// available. Enrich that already-visible row at completion.
				if showUpEntry {
					for _, entry := range fp.Entries {
						if entry.Name == ".." {
							entry.VFSItem = upItem
							break
						}
					}
				}
			}

			if err == nil && (!authoritativeWindowQueued ||
				fp.catalogLogicalCount == len(fp.Entries)) {
				fp.catalogLogicalCount = 0
			}
			fp.catalogProvisional = false
			fp.StopLoadingAnimation()

			fp.lastDirMTime = dirStat.MTime
			fp.IsLoading = false
			fp.catalogInteractive = false
			if err != nil && err != context.Canceled {
				if benchmark != nil {
					benchmark.Event("model.final.error", "go.ui", "path", path,
						"entries", directoryEntryCount, "chunks", chunkCount, "error", err.Error())
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
						fp.PendingSelection = loadVFS.Base(path)
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
			if err == nil {
				fp.committedListingVFS = loadVFS
				fp.committedListingPath = path
				fp.watchDirectoryChanges(ctx, loadVFS, path, loadIsCurrent)
			}

			if isFirstChunk {
				completionPresentationChanged = true
				advanceSourceEpoch()
				fp.Entries = nil
				if showUpEntry {
					fp.Entries = []*FileEntry{{VFSItem: upItem}}
				}
				fp.SetCursorIndex(0)
			}

			if fp.PendingSelection != "" {
				completionPresentationChanged = true
				fp.SelectName(fp.PendingSelection)
				fp.PendingSelection = ""
			}
			if keepEntries {
				fp.Refresh()
				if refreshChanged {
					authoritativePresentationComplete = publishPanelCatalogImmediate(fp, benchmark)
				}
			} else if !authoritativeCatalogQueued {
				// ReadDir is the compatibility path used by size/time sorts. Its
				// callback is intentionally non-authoritative while chunks are
				// arriving, so the completed full catalog must replace the bounded
				// sparse page. The revision bump is required because the row shape
				// changes from a sparse window to a dense catalog even though the
				// underlying directory identity is unchanged.
				if semantic.PanelCatalogRowsIsEnabled() && fp.semanticCatalogCanBeDense() {
					fp.markSemanticCatalogMutation()
					authoritativePresentationComplete =
						publishPanelCatalogImmediate(fp, benchmark)
					fp.Refresh()
				} else {
					fp.Refresh()
					authoritativePresentationComplete =
						publishPanelCatalogImmediate(fp, benchmark)
				}
			} else if semantic.PanelCatalogRowsIsEnabled() && fp.semanticCatalogCanBeDense() {
				// A windowed name-sort load already sent its bounded prefix. Once
				// the complete source is available, promote small folders to the
				// dense model so Masonry receives every image's eventual natural
				// dimensions. Large folders intentionally remain sparse.
				fp.markSemanticCatalogMutation()
				authoritativePresentationComplete =
					publishPanelCatalogImmediate(fp, benchmark)
				fp.Refresh()
			}
			if benchmark != nil {
				benchmark.Event("model.final.ready", "go.ui", "phase", "fresh", "path", path,
					"entries", len(fp.Entries), "chunks", chunkCount,
					"cursorIndex", fp.GetCursorIndex(),
					"catalogAlreadyPublished", authoritativeCatalogQueued)
				if !authoritativeCatalogQueued {
					navtrace.NavigationBenchmarkPublishScene(benchmark, "fresh")
				}
			}
			if fp.benchmarkLoadTrace == benchmark {
				fp.benchmarkLoadTrace = nil
			}
			if !authoritativeCatalogQueued {
				vtui.FrameManager.Redraw()
			} else if authoritativePresentationComplete &&
				!completionPresentationChanged {
				needsRedraw = false
			}
			return
		})
		// Publish enrichment only after the complete listing task has been queued.
		// The base catalog is the interactive result; metadata is decoration and
		// must never sit in front of the navigation commit in the UI queue.
		if usePhasedRead && !keepEntries && err == nil {
			publishMetadata(pendingMetadata)
		}
	})
}

func (fp *FileSystemPanel) watchDirectoryChanges(
	ctx context.Context, fs vfs.VFS, path string, loadIsCurrent func() bool,
) {
	watcher, ok := fs.(vfs.DirectoryWatcher)
	if !ok {
		return
	}
	frames := vtui.FrameManager
	go func() {
		var refreshOnce sync.Once
		err := watcher.WatchDirectory(ctx, path, func() {
			refreshOnce.Do(func() {
				frames.PostTask(func() {
					if !loadIsCurrent() || fp.IsLoading {
						return
					}
					fp.ReadDirectory()
				})
			})
		})
		if err != nil && ctx.Err() == nil {
			vtui.DebugLog("PANEL: directory watcher for %q stopped: %v", path, err)
		}
	}()
}

func (fp *FileSystemPanel) Refresh() {
	idx := fp.GetCursorIndex()
	fp.updateSortColumnTitles()
	virtualRows := false
	if vtui.FrameManager != nil {
		if screen := vtui.FrameManager.Screen(); screen != nil {
			if renderer, ok := screen.Renderer.(interface {
				VirtualizePanelTableRows() bool
			}); ok {
				virtualRows = renderer.VirtualizePanelTableRows()
			}
		}
	}
	if virtualRows {
		if fp.gridColumnCount() == 1 {
			fp.Table.SetTableRowProvider(len(fp.Entries), func(index int) vtui.TableRow {
				if index < 0 || index >= len(fp.Entries) {
					return nil
				}
				return &panelEntryRow{fp: fp, entry: fp.Entries[index]}
			})
		} else {
			fp.Table.SetTableRowProvider(len(fp.Entries), func(index int) vtui.TableRow {
				if index < 0 || index >= len(fp.Entries) {
					return nil
				}
				return &mediumRow{fp: fp, r: index}
			})
		}
	} else if fp.gridColumnCount() == 1 {
		rows := make([]vtui.TableRow, len(fp.Entries))
		for i, e := range fp.Entries {
			rows[i] = &panelEntryRow{fp: fp, entry: e}
		}
		fp.Table.SetRows(rows)
	} else {
		rows := make([]vtui.TableRow, len(fp.Entries))
		for i := 0; i < len(rows); i++ {
			rows[i] = &mediumRow{fp: fp, r: i}
		}
		fp.Table.SetRows(rows)
	}
	fp.SetCursorIndex(idx)
	_, _, maxTop, _, _ := fp.panelScrollMetrics()
	if fp.Table.TopPos > maxTop {
		fp.Table.TopPos = maxTop
		fp.SetCursorIndex(idx)
	}
}

func (fp *FileSystemPanel) Show(scr *vtui.ScreenBuf) {
	fp.Frame.Show(scr)
	titleAttr := vtui.Palette[theme.ColPanelTitle]
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
		fp.Table.ColorSelectedTextIdx = theme.ColPanelInactiveCursor
		fp.Table.ColorItemSelectCursorIdx = theme.ColPanelInactiveSelectedCursor
		fp.Table.SetFocus(true)
	} else {
		fp.Table.ColorSelectedTextIdx = theme.ColPanelCursor
		fp.Table.ColorItemSelectCursorIdx = theme.ColPanelSelectedCursor
		fp.Table.SetFocus(fp.IsFocused())
	}
	fp.clampNameLeftPos()
	fp.Table.Show(scr)
	fp.drawFastFindMatches(scr)
	fp.drawCursorSeparators(scr)
	fp.drawNameScrollBrackets(scr)
	fp.drawScrollBar(scr)

	var totSize int64
	var totCount int
	var totFiles int
	var totDirs int
	for _, e := range fp.Entries {
		if e.Name == ".." {
			continue
		}
		totCount++
		if e.IsDir {
			totDirs++
		} else {
			totFiles++
			totSize += e.Size
		}
	}
	freeSpaceStr := ""
	if _, isLocal := fp.Vfs.(*vfs.OSVFS); isLocal {
		if info, ok := sysinfo.FS(fp.Vfs.GetPath()); ok {
			freeSpaceStr = formatBytes(info.Free)
		}
	}

	if config.App.ShowPanelFileInfo && fp.Y2-fp.Y1+1 > 6 {
		p := vtui.NewPainter(scr)
		attrBox := vtui.Palette[theme.ColPanelBox]
		// far2l paints the per-file status line with COL_PANELTEXT;
		// COL_PANELINFOTEXT (Panel.Text.Info) belongs to the info panel and
		// quick view, which use it in info_panel.go and quick_view_panel.go.
		attrInfo := vtui.Palette[theme.ColPanelText]

		p.DrawLine(fp.X1+1, fp.Y2-2, fp.X2-1, fp.Y2-2, '─', attrBox, false, false)
		scr.Write(fp.X1, fp.Y2-2, vtui.StringToCharInfo("├", attrBox))
		scr.Write(fp.X2, fp.Y2-2, vtui.StringToCharInfo("┤", attrBox))

		p.Fill(fp.X1+1, fp.Y2-1, fp.X2-1, fp.Y2-1, ' ', attrInfo)

		idx := fp.GetCursorIndex()
		if idx >= 0 && idx < len(fp.Entries) {
			e := fp.Entries[idx]

			dateStr := e.MTime.Format("02.01.06 15:04")
			sizeStr := ""
			if e.IsDir {
				if e.SizeCalculated {
					sizeStr = fileops.FormatIntWithSpaces(e.Size)
				} else if e.Name == ".." {
					sizeStr = "UP-DIR"
				} else if e.IsSymlink {
					sizeStr = "<LNK-DIR>"
				} else {
					sizeStr = "<DIR>"
				}
			} else if e.IsSymlink {
				sizeStr = "<LNK>"
			} else {
				sizeStr = fileops.FormatIntWithSpaces(e.Size)
			}

			nameStr := e.Name
			if e.IsSymlink && fp.Vfs != nil {
				if target, err := vfs.Readlink(context.Background(), fp.Vfs, fp.Vfs.Join(fp.Vfs.GetPath(), e.Name)); err == nil && target != "" {
					if fp.Vfs.GetPath() == "net://" {
						nameStr = e.Name + " -> " + target
					} else {
						sizeStr = "→ " + target
					}
				}
			}

			rightStr := fmt.Sprintf("%s  %s", sizeStr, dateStr)

			if fp.Vfs != nil && fp.Vfs.GetPath() == "net://" {
				rightStr = ""
			}

			availW := (fp.X2 - 1) - (fp.X1 + 1) + 1
			rightW := runewidth.StringWidth(rightStr)

			if availW > rightW+1 {
				// far2l's status line is a right-aligned name column: a name
				// that does not fit loses its beginning, not its extension.
				nameStr = truncateNameKeepingEnd(nameStr, availW-rightW-1)
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

	for _, e := range fp.Entries {
		if e.Name != ".." && e.Selected {
			if e.IsDir {
				selDirs++
				if e.SizeCalculated {
					selSize += e.Size
				}
			} else {
				selFiles++
				selSize += e.Size
			}
		}
	}

	// far2l (FileList::ShowSelectedSize) keeps the two summaries apart when
	// the status line is on: the selection summary sits centred on the
	// separator above the status line, and the panel total stays on the
	// bottom border underneath it. Only with the status line off do they
	// share the bottom border, where the selection summary wins (#394).
	fileInfoShown := config.App.ShowPanelFileInfo && fp.Y2-fp.Y1+1 > 6
	availBottom := fp.X2 - fp.X1 - 1

	selStr := ""
	if selFiles > 0 || selDirs > 0 {
		selStr = fmt.Sprintf(" "+i18n.Msg("Panel.SelectedInfo")+" ", fileops.FormatIntWithSpaces(selSize), selFiles, selDirs)
	}

	totalStr := ""
	var attrTotal uint64
	if selStr != "" && !fileInfoShown {
		totalStr = selStr
		attrTotal = vtui.Palette[theme.ColPanelSelectedInfo]
	} else if totCount > 0 {
		totalStr = fmt.Sprintf(" %s (%d/%d) ", fileops.FormatIntWithSpaces(totSize), totFiles, totDirs)
		if freeSpaceStr != "" {
			totalStr = fmt.Sprintf(" %s (%d/%d) — %s ", fileops.FormatIntWithSpaces(totSize), totFiles, totDirs, freeSpaceStr)
		}
		attrTotal = vtui.Palette[theme.ColPanelTotalInfo]
	}

	if selStr != "" && fileInfoShown {
		if selW := runewidth.StringWidth(selStr); selW < availBottom {
			p := vtui.NewPainter(scr)
			p.DrawString(fp.X1+1+(availBottom-selW)/2, fp.Y2-2, selStr, vtui.Palette[theme.ColPanelSelectedInfo])
		}
	}

	totalStart := fp.X2
	if totalStr != "" {
		totalW := runewidth.StringWidth(totalStr)
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
	if !config.App.ShowPanelFileInfo && fp.gridColumnCount() > 1 {
		if idx := fp.GetCursorIndex(); idx >= 0 && idx < len(fp.Entries) {
			e := fp.Entries[idx]
			curStr := fileops.FormatIntWithSpaces(e.Size)
			if e.IsDir && !e.SizeCalculated {
				curStr = "<DIR>"
				if e.Name == ".." {
					curStr = "UP-DIR"
				}
			}
			if e.IsSymlink && fp.Vfs != nil {
				if target, err := vfs.Readlink(context.Background(), fp.Vfs, fp.Vfs.Join(fp.Vfs.GetPath(), e.Name)); err == nil && target != "" {
					curStr = "→ " + target
				}
			}
			curStr = " ▸ " + curStr + " "
			if maxCurW := totalStart - (fp.X1 + 1); maxCurW > 0 {
				if runewidth.StringWidth(curStr) > maxCurW {
					curStr = runewidth.Truncate(curStr, maxCurW, "")
				}
				p := vtui.NewPainter(scr)
				p.DrawString(fp.X1+1, fp.Y2, curStr, vtui.Palette[theme.ColPanelText])
			}
		}
	}

	if fp.FastFindMode {
		// Ask the screen for the underline cursor instead of writing
		// DECSCUSR to stdout behind the renderer's back: the renderer
		// knows which terminals take the sequence and which must be
		// driven through the console API (f4 #219, classic conhost draws
		// DECSCUSR's underline as a one-pixel hairline).
		scr.SetCursorShape(vtui.CursorShapeUnderline)
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
		p.DrawTitle(fx1, fy1, fx2, i18n.Msg("Viewer.SearchTitle"), vtui.Palette[vtui.ColDialogBoxTitle])

		searchStr := fp.FastFindStr
		for runewidth.StringWidth(searchStr) > boxW-4 {
			runes := []rune(searchStr)
			searchStr = string(runes[1:])
		}

		searchColor := vtui.Palette[theme.ColPanelFastFindNoMatch]
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
	if !fp.FastFindMode || fp.FastFindStr == "" {
		return 0, 0, false
	}
	queryText := fp.FastFindStr
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
		fp.fastFindMatcherQueries = fp.fastFindMatcherQueries[:0]
		seen := make(map[string]struct{}, 2)
		for _, query := range []string{
			queryText,
			vtui.GlobalXlator.TranscodeString(queryText),
		} {
			if _, duplicate := seen[query]; duplicate {
				continue
			}
			seen[query] = struct{}{}
			if m := vtui.NewFuzzyMatcher(query, false); m != nil {
				fp.fastFindMatchers = append(fp.fastFindMatchers, m)
				fp.fastFindMatcherQueries = append(fp.fastFindMatcherQueries, query)
			}
		}
	}
	// One- and two-rune fuzzy queries accept zero edits. Avoid running the
	// bit-vector matcher over every long WinSxS component name while looking
	// for the first matching initial(s); a folded prefix/substring comparison
	// is exactly equivalent for this threshold and has no per-row allocation.
	if utf8.RuneCountInString(queryText) <= 2 {
		bestStart, bestLength := -1, 0
		for _, query := range fp.fastFindMatcherQueries {
			start, length, found := fastFindShortExactMatch(name, query, anywhere)
			if found && (bestStart < 0 || start < bestStart) {
				bestStart, bestLength = start, length
			}
		}
		if bestStart >= 0 {
			return bestStart, bestLength, true
		}
		return 0, 0, false
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

func fastFindShortExactMatch(name, query string, anywhere bool) (start, length int, ok bool) {
	queryRunes := utf8.RuneCountInString(query)
	if queryRunes == 0 || queryRunes > 2 {
		return 0, 0, false
	}
	if fastFindASCII(name) && fastFindASCII(query) {
		last := len(name) - len(query)
		if last < 0 {
			return 0, 0, false
		}
		if !anywhere {
			last = 0
		}
		for candidate := 0; candidate <= last; candidate++ {
			matched := true
			for offset := range len(query) {
				if fastFindLowerASCII(name[candidate+offset]) !=
					fastFindLowerASCII(query[offset]) {
					matched = false
					break
				}
			}
			if matched {
				return candidate, queryRunes, true
			}
		}
		return 0, 0, false
	}

	nameRunes := []rune(name)
	needle := []rune(query)
	last := len(nameRunes) - len(needle)
	if last < 0 {
		return 0, 0, false
	}
	if !anywhere {
		last = 0
	}
	for candidate := 0; candidate <= last; candidate++ {
		matched := true
		for offset, queryRune := range needle {
			nameRune := nameRunes[candidate+offset]
			if nameRune != queryRune &&
				unicode.ToLower(nameRune) != unicode.ToLower(queryRune) &&
				unicode.ToUpper(nameRune) != unicode.ToUpper(queryRune) {
				matched = false
				break
			}
		}
		if matched {
			return candidate, len(needle), true
		}
	}
	return 0, 0, false
}

func fastFindASCII(value string) bool {
	for index := range len(value) {
		if value[index] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func fastFindLowerASCII(value byte) byte {
	if value >= 'A' && value <= 'Z' {
		return value + ('a' - 'A')
	}
	return value
}

type fastFindIndexRange struct {
	first int
	end   int
}

const fastFindBinarySearchMinimumRows = 1024

// fastFindSortedShortCandidate uses the panel's existing name ordering as an
// index. One- and two-rune anchored queries are exact prefixes (the fuzzy
// threshold is zero), so every match is in one contiguous range per
// directory/file group and keyboard-layout variant. No panel-sized auxiliary
// index is retained.
func (fp *FileSystemPanel) fastFindSortedShortCandidate(
	searchStart, step int,
) (candidate, probes int, found, supported bool) {
	if fp == nil || len(fp.Entries) < fastFindBinarySearchMinimumRows ||
		fp.SortMode != SortName || fp.SortReverse ||
		strings.HasPrefix(fp.FastFindStr, "*") ||
		utf8.RuneCountInString(fp.FastFindStr) > 2 {
		return 0, 0, false, false
	}

	start := 0
	if fp.Entries[0] == nil {
		return 0, 0, false, false
	}
	if fp.Entries[0].Name == ".." {
		start = 1
	}
	split := start + sort.Search(len(fp.Entries)-start, func(offset int) bool {
		return !fp.Entries[start+offset].IsDir
	})
	groups := [...]fastFindIndexRange{
		{first: start, end: split},
		{first: split, end: len(fp.Entries)},
	}

	queries := []string{
		fp.FastFindStr,
		vtui.GlobalXlator.TranscodeString(fp.FastFindStr),
	}
	seen := make(map[string]struct{}, len(queries))
	ranges := make([]fastFindIndexRange, 0, len(groups)*len(queries))
	for _, query := range queries {
		if query == "" {
			continue
		}
		if _, duplicate := seen[query]; duplicate {
			continue
		}
		seen[query] = struct{}{}
		for _, group := range groups {
			length := group.end - group.first
			if length <= 0 {
				continue
			}
			lower := sort.Search(length, func(offset int) bool {
				probes++
				return comparePanelFolded(
					fp.Entries[group.first+offset].visibleName(), query) >= 0
			})
			if lower >= length {
				continue
			}
			if _, _, ok := fastFindShortExactMatch(
				fp.Entries[group.first+lower].visibleName(), query, false); !ok {
				continue
			}
			upper := sort.Search(length, func(offset int) bool {
				probes++
				name := fp.Entries[group.first+offset].visibleName()
				if comparePanelFolded(name, query) <= 0 {
					return false
				}
				_, _, prefix := fastFindShortExactMatch(name, query, false)
				return !prefix
			})
			if upper > lower {
				ranges = append(ranges, fastFindIndexRange{
					first: group.first + lower,
					end:   group.first + upper,
				})
			}
		}
	}
	if len(ranges) == 0 {
		return 0, probes, false, true
	}

	slices.SortFunc(ranges, func(left, right fastFindIndexRange) int {
		if left.first != right.first {
			return left.first - right.first
		}
		return left.end - right.end
	})
	merged := ranges[:0]
	for _, current := range ranges {
		last := len(merged) - 1
		if last >= 0 && current.first <= merged[last].end {
			merged[last].end = max(merged[last].end, current.end)
			continue
		}
		merged = append(merged, current)
	}

	if step >= 0 {
		for _, matchRange := range merged {
			if searchStart < matchRange.end {
				return max(searchStart, matchRange.first), probes, true, true
			}
		}
		return merged[0].first, probes, true, true
	}
	for index := len(merged) - 1; index >= 0; index-- {
		matchRange := merged[index]
		if searchStart >= matchRange.first {
			return min(searchStart, matchRange.end-1), probes, true, true
		}
	}
	last := merged[len(merged)-1]
	return last.end - 1, probes, true, true
}

type fastFindCachedMatch struct {
	start  int
	length int
	ok     bool
}

const maxFastFindRetainedRows = 2 * semanticFastFindRowsLimit

func (fp *FileSystemPanel) ensureFastFindMatchCache() {
	if fp.fastFindMatchCacheKey == fp.FastFindStr &&
		fp.fastFindMatchCacheCatalogGeneration == fp.semanticCatalogGeneration &&
		fp.fastFindMatchCacheEntryCount == len(fp.Entries) {
		return
	}
	preserveKnownNoMatch := fp.fastFindMatchCacheCatalogGeneration == fp.semanticCatalogGeneration &&
		fp.fastFindMatchCacheEntryCount == len(fp.Entries) &&
		fp.fastFindAnyMatchKnown && !fp.fastFindAnyMatch &&
		fastFindQueryStrictlyExtends(fp.fastFindMatchCacheKey, fp.FastFindStr)
	fp.fastFindMatchCacheKey = fp.FastFindStr
	fp.fastFindMatchCacheCatalogGeneration = fp.semanticCatalogGeneration
	fp.fastFindMatchCacheEntryCount = len(fp.Entries)
	if fp.fastFindMatchCache == nil {
		fp.fastFindMatchCache = make(map[int]fastFindCachedMatch, 128)
	} else {
		clear(fp.fastFindMatchCache)
	}
	fp.fastFindAnyMatchKnown = preserveKnownNoMatch
	fp.fastFindAnyMatch = false
}

func fastFindQueryStrictlyExtends(previous, current string) bool {
	previousAnywhere := strings.HasPrefix(previous, "*")
	currentAnywhere := strings.HasPrefix(current, "*")
	if previousAnywhere != currentAnywhere {
		return false
	}
	previous = strings.TrimPrefix(previous, "*")
	current = strings.TrimPrefix(current, "*")
	if previous == "" || len(current) <= len(previous) ||
		!strings.HasPrefix(current, previous) {
		return false
	}
	previousRunes := utf8.RuneCountInString(previous)
	currentRunes := utf8.RuneCountInString(current)
	// Up to two runes the matcher is exact, as it is again above its 64-rune
	// bit-vector limit. Inside the fuzzy range the accepted edit threshold can
	// grow when a character is appended, so an empty shorter query does not
	// prove that its extension is empty.
	return currentRunes <= 2 || previousRunes > 64
}

// fastFindMatchAt memoizes only rows actually inspected for the current
// query. The map normally remains viewport-sized; it grows beyond that only
// when finding the next/previous match genuinely has to cross more rows.
func (fp *FileSystemPanel) fastFindMatchAt(index int) (startRunes, matchedRunes int, ok bool) {
	if index < 0 || index >= len(fp.Entries) || fp.Entries[index] == nil {
		return 0, 0, false
	}
	fp.ensureFastFindMatchCache()
	if match, cached := fp.fastFindMatchCache[index]; cached {
		return match.start, match.length, match.ok
	}
	startRunes, matchedRunes, ok = fp.fastFindMatch(fp.Entries[index].visibleName())
	fp.fastFindMatchEvaluations++
	if len(fp.fastFindMatchCache) < maxFastFindRetainedRows || ok {
		fp.fastFindMatchCache[index] = fastFindCachedMatch{
			start: startRunes, length: matchedRunes, ok: ok,
		}
	}
	return startRunes, matchedRunes, ok
}

// compactFastFindMatchCache prevents a necessary long traversal (including
// the first proof that there are no matches) from becoming a panel-sized
// retained cache. Rendering repopulates only its bounded semantic/visible
// window after this point.
func (fp *FileSystemPanel) compactFastFindMatchCache(matchedIndex int) {
	if len(fp.fastFindMatchCache) <= maxFastFindRetainedRows {
		return
	}
	match, retainMatch := fp.fastFindMatchCache[matchedIndex]
	clear(fp.fastFindMatchCache)
	if retainMatch {
		fp.fastFindMatchCache[matchedIndex] = match
	}
}

func (fp *FileSystemPanel) fastFindHasMatches() bool {
	fp.ensureFastFindMatchCache()
	if fp.fastFindAnyMatchKnown {
		return fp.fastFindAnyMatch
	}
	// Production query edits always call doFastFind first, which proves the
	// answer while stopping at the first match. Keep direct/test construction
	// useful without turning rendering back into an implicit O(N) search.
	if _, _, ok := fp.fastFindMatchAt(fp.GetCursorIndex()); ok {
		fp.fastFindAnyMatchKnown = true
		fp.fastFindAnyMatch = true
		return true
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
	if !fp.FastFindMode || fp.FastFindStr == "" || !fp.Table.IsVisible() {
		return
	}
	fp.ensureFastFindMatchCache()
	if fp.fastFindAnyMatchKnown && !fp.fastFindAnyMatch {
		return
	}
	height := fp.Table.ViewHeight
	if height <= 0 {
		return
	}
	columns := fp.gridColumnCount()
	matchAttr := vtui.Palette[theme.ColPanelHighlightText]

	for rowOffset := 0; rowOffset < height; rowOffset++ {
		row := fp.Table.TopPos + rowOffset
		y := fp.Table.Y1 + fp.Table.MarginTop + rowOffset
		x := fp.Table.X1
		for column := 0; column < columns && column < len(fp.Table.Columns); column++ {
			entryIndex := row
			if columns > 1 {
				entryIndex += column * height
			}
			cellWidth := fp.Table.Columns[column].Width
			if entryIndex >= 0 && entryIndex < len(fp.Entries) {
				entry := fp.Entries[entryIndex]
				matchStart, matchedRunes, _ := fp.fastFindMatchAt(entryIndex)
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
	fp.Frame.SetPosition(x1, y1, x2, y2)
	// Table stays inside the frame, reserving space for status info only when enabled.
	if config.App.ShowPanelFileInfo && y2-y1+1 > 6 {
		fp.Table.SetPosition(x1+1, y1+1, x2-1, y2-3)
	} else {
		fp.Table.SetPosition(x1+1, y1+1, x2-1, y2-1)
	}
}

func (fp *FileSystemPanel) Resize(w, h int) {
	fp.SetPosition(fp.X1, fp.Y1, fp.X1+w-1, fp.Y1+h-1)

	switch fp.EffectiveViewMode() {
	case ViewModeWide:
		nameW := w - 2 - 2 - panelSizeColumnWidth - panelModifiedColumnWidth
		if nameW < 1 {
			nameW = 1
		}
		fp.Table.Columns = []vtui.TableColumn{
			{Title: i18n.Msg("Panel.Column.Name"), Width: nameW},
			{Title: i18n.Msg("Panel.Column.Size"), Width: panelSizeColumnWidth, Alignment: vtui.AlignRight},
			{Title: i18n.Msg("Panel.Column.Modified"), Width: panelModifiedColumnWidth},
		}
	case ViewModeDetailed:
		// The panel's inner table is w-2 characters wide. The size column
		// consumes 11 and its separator consumes 1, leaving w-14 for Name.
		nameW := w - 14
		if nameW < 5 {
			nameW = 5
		}
		fp.Table.Columns = []vtui.TableColumn{
			{Title: i18n.Msg("Panel.Column.Name"), Width: nameW},
			{Title: i18n.Msg("Panel.Column.Size"), Width: panelSizeColumnWidth, Alignment: vtui.AlignRight},
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
			columns[i] = vtui.TableColumn{Title: i18n.Msg("Panel.Column.Name"), Width: width}
			remaining -= width
		}
		fp.Table.Columns = columns
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
	return fp.processKey(e, false)
}

// EnterSelectedFromAction enters the selected directory or archive for an
// explicit panel action such as Ctrl+PgDn. It is intentionally distinct from
// ordinary Enter so providers can reserve that action for entries that should
// not open on Enter or a double-click.
func (fp *FileSystemPanel) EnterSelectedFromAction() bool {
	return fp.processKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	}, true)
}

func (fp *FileSystemPanel) processKey(e *vtinput.InputEvent, allowProviderPanelEnter bool) bool {
	if !e.KeyDown {
		return false
	}
	if fp.ProviderOpenTask != nil {
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
	if fp.ViewMode == ViewModeDetailed && config.App.NavigationMode != config.NavigationVim && !ctrl && !alt &&
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
	if !shift || !isShiftSelectNavKey(e.VirtualKeyCode) {
		fp.shiftSessionActive = false
	}

	if fp.FastFindMode {
		if e.VirtualKeyCode == vtinput.VK_UP || e.VirtualKeyCode == vtinput.VK_DOWN {
			fp.FastFindMode = false
			fp.FastFindStr = ""
			vtui.FrameManager.Redraw()
			// Reprocess the key as ordinary panel navigation now that Fast Find
			// no longer owns it.
			return fp.ProcessKey(e)
		}
		switch e.VirtualKeyCode {
		case vtinput.VK_LEFT, vtinput.VK_RIGHT, vtinput.VK_PRIOR, vtinput.VK_NEXT, vtinput.VK_HOME, vtinput.VK_END:
			fp.FastFindMode = false
			fp.FastFindStr = ""
			vtui.FrameManager.Redraw()
			// Проваливаемся дальше, чтобы обработать саму навигацию
		case vtinput.VK_ESCAPE:
			fp.FastFindMode = false
			fp.FastFindStr = ""
			vtui.FrameManager.Redraw()
			return true
		case vtinput.VK_DELETE:
			// Fast Find keeps its insertion point at the end, so forward Delete
			// has nothing to remove. Consume it here rather than allowing the
			// panel-level Del binding to hide the panels.
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_F2 && !shift && !ctrl && !alt {
			if strings.HasPrefix(fp.FastFindStr, "*") {
				fp.FastFindStr = strings.TrimPrefix(fp.FastFindStr, "*")
			} else {
				fp.FastFindStr = "*" + fp.FastFindStr
			}
			if fp.FastFindStr == "" {
				fp.FastFindMode = false
			} else {
				fp.doFastFind(0)
			}
			vtui.FrameManager.Redraw()
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_BACK {
			if len(fp.FastFindStr) > 0 {
				runes := []rune(fp.FastFindStr)
				fp.FastFindStr = string(runes[:len(runes)-1])
				if len(fp.FastFindStr) == 0 {
					fp.FastFindMode = false
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
			fp.FastFindMode = false
			fp.FastFindStr = ""
			vtui.FrameManager.Redraw()
			// Проваливаемся ниже, чтобы обработать Enter как вход в файл/директорию
		} else if r := fastFindRune(e, alt); r != 0 && !ctrl && unicode.IsPrint(r) {
			fp.FastFindStr += string(unicode.ToLower(r))
			fp.doFastFind(0)
			vtui.FrameManager.Redraw()
			return true
		}
	} else {
		searchFirstInput := config.App.NavigationMode == config.NavigationSearchFirst && fp.IsFocused() && !alt
		if r := fastFindRune(e, alt); r != 0 && (alt || searchFirstInput) && !ctrl && unicode.IsPrint(r) {
			fp.FastFindMode = true
			fp.FastFindStr = string(unicode.ToLower(r))
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
				if startIdx >= 0 && startIdx < len(fp.Entries) &&
					fp.Entries[startIdx].Name == ".." {
					if dir := shiftSelectDirection(e.VirtualKeyCode); dir != 0 {
						for i := startIdx + dir; i >= 0 && i < len(fp.Entries); i += dir {
							if fp.Entries[i].Name != ".." {
								modeIdx = i
								break
							}
						}
					}
				}
				if modeIdx >= 0 && modeIdx < len(fp.Entries) &&
					fp.Entries[modeIdx].Name != ".." &&
					fp.Entries[modeIdx].Selected {
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
		h := fp.Table.ViewHeight
		if h <= 0 {
			h = 1
		}

		handled := false
		if columns := fp.gridColumnCount(); columns > 1 {
			switch e.VirtualKeyCode {
			case vtinput.VK_UP:
				idx--
			case vtinput.VK_DOWN:
				idx++
			case vtinput.VK_LEFT:
				idx -= h
			case vtinput.VK_RIGHT:
				idx += h
			case vtinput.VK_PRIOR:
				idx -= h * columns
			case vtinput.VK_NEXT:
				idx += h * columns
			case vtinput.VK_HOME:
				idx = 0
			case vtinput.VK_END:
				idx = len(fp.Entries) - 1
			default:
				return false
			}
			fp.SetCursorIndex(idx)
			handled = true
		} else {
			// In Detailed mode, we let the table handle navigation but sync our index back
			if fp.Table.ProcessKey(e) {
				fp.CursorIdx = fp.Table.SelectPos
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
		if idx >= 0 && idx < len(fp.Entries) {
			selected := fp.Entries[idx]

			// Logic for leaving a virtual VFS (like an archive)
			if selected.Name == ".." {

				parent := fp.Vfs.ParentVFS()
				isRoot := fp.Vfs.IsAtRoot()

				if isRoot {
					if parent != nil {
						oldPath := fp.Vfs.GetPath()
						parentSelection := ""
						if temp, ok := fp.Vfs.(*TempPanelVFS); ok {
							parentSelection = temp.parentSelection
						}

						fp.cancelProviderOpen()
						// Закрываем текущую систему (удаляем временные файлы)
						_ = fp.Vfs.Close()

						fp.Vfs = parent
						if parentSelection != "" {
							fp.PendingSelection = parentSelection
						} else if fp.ProviderEntryName != "" {
							fp.PendingSelection = fp.ProviderEntryName
							fp.ProviderEntryName = ""
						} else {
							fp.PendingSelection = fp.Vfs.Base(oldPath)
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
			fullPath := fp.Vfs.Join(fp.Vfs.GetPath(), selected.Name)
			provider := vfs.FindProvider(context.Background(), fp.Vfs, fullPath)
			if selected.IsDir && provider != nil {
				directoryProvider, ok := provider.(vfs.VirtualDirectoryProvider)
				if !ok || !directoryProvider.OpensVirtualDirectories() {
					provider = nil
				}
			}
			if provider != nil && !allowProviderPanelEnter {
				if policy, ok := provider.(vfs.PanelEnterPolicyProvider); ok &&
					!policy.PanelEnterAllowed(context.Background(), fp.Vfs, fullPath) {
					return true
				}
			}
			if provider != nil {
				sourceVFS := fp.Vfs
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
						fp.ProviderEntryName = selectedName
						fp.Vfs = newVFS
						fp.PendingSelection = ".."
						fp.showCurrentVFSLoadingRows()
						fp.ReadDirectory()
					},
					func(err error) {
						fp.PendingSelection = selectedName
						if isArchiveProvider(provider) {
							vtui.ShowMessage(" Open Error ", fmt.Sprintf("Failed to open %s:\n%v", selectedName, err), []string{"&Ok"})
						} else {
							vtui.ShowMessage(" Connection Error ", fmt.Sprintf("Failed to connect to %s:\n%v", selectedName, err), []string{"&Ok"})
						}
					},
				)
				if started && showStatus {
					fp.showProviderOpenDialog(status)
				}
				return started
			}

			if selected.IsDir {
				oldPath := fp.Vfs.GetPath()
				newPath := fp.Vfs.Join(oldPath, selected.Name)
				vtui.DebugLog("PANEL: Navigating %q -> %q", oldPath, newPath)
				if err := fp.SetKnownDirectoryPath(newPath); err == nil {
					if selected.Name == ".." {
						fp.PendingSelection = fp.Vfs.Base(oldPath)
					} else {
						fp.PendingSelection = ".."
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
	if fp.ProviderOpenTask != nil {
		// The provider is still resolving while fp.vfs points at the source.
		// Consume panel mouse input until the switch so a double-click/context
		// action cannot run against the wrong filesystem.
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

	if fp.FastFindMode && e.ButtonState != 0 {
		fp.FastFindMode = false
		fp.FastFindStr = ""
		vtui.FrameManager.Redraw()
	}

	if e.WheelDirection != 0 {
		// Determine direction: up is -1, down is 1
		direction := 1
		speed := config.App.WheelPanelDown
		if e.WheelDirection > 0 {
			direction = -1
			speed = config.App.WheelPanelUp
		}
		step := direction * config.WheelScrollLines(speed)

		h := fp.Table.ViewHeight
		if h <= 0 {
			h = 1
		}

		if fp.gridColumnCount() == 1 {
			// Detailed view (1-column)
			idx := fp.GetCursorIndex()
			newIdx := idx + step
			if newIdx < 0 {
				newIdx = 0
			}
			if newIdx >= len(fp.Entries) {
				newIdx = len(fp.Entries) - 1
			}

			// Scroll the list if possible, keeping the cursor visually stable
			newTop := fp.Table.TopPos + step
			maxTop := len(fp.Entries) - h
			if maxTop < 0 {
				maxTop = 0
			}
			if newTop < 0 {
				newTop = 0
			}
			if newTop > maxTop {
				newTop = maxTop
			}

			fp.Table.TopPos = newTop
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
			if newIdx >= len(fp.Entries) {
				newIdx = len(fp.Entries) - 1
			}

			// Scroll the list if possible, keeping the cursor visually stable
			newTop := fp.Table.TopPos + step
			maxTop := len(fp.Entries) - fp.gridColumnCount()*h
			if maxTop < 0 {
				maxTop = 0
			}
			if newTop < 0 {
				newTop = 0
			}
			if newTop > maxTop {
				newTop = maxTop
			}

			fp.Table.TopPos = newTop
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
			if fp.Entries[idx].Name == ".." {
				return true
			}
			if e.MouseEventFlags&vtinput.DoubleClick != 0 {
				// Windows reports the second press as a double-click. The first
				// press has already established whether this is a select or
				// deselect operation, so propagate its result to the whole
				state := fp.Entries[idx].Selected
				fp.SetAllItemsSelected(state)
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

	handled := fp.Table.ProcessMouse(e)
	if handled {
		if e.KeyDown && !isMove && e.ButtonState&vtinput.FromLeft1stButtonPressed != 0 {
			fp.rowDragButton = vtinput.FromLeft1stButtonPressed
		}
		// Sync absolute index from table's visual selection
		if fp.gridColumnCount() == 1 {
			fp.CursorIdx = fp.Table.SelectPos
		} else {
			h := fp.Table.ViewHeight
			if h <= 0 {
				h = 1
			}
			// SelectPos is already absolute (TopPos + row) in Medium mode,
			// so we just add the column offset.
			newIdx := fp.Table.SelectPos + fp.Table.SelectCol*h

			// Fix for "click in empty space": if we selected an empty slot,
			// snap to the last valid entry.
			if newIdx >= len(fp.Entries) {
				fp.SetCursorIndex(len(fp.Entries) - 1)
			} else {
				fp.CursorIdx = newIdx
			}
		}
	}

	return handled
}

func (fp *FileSystemPanel) GetSelectedName() string {
	idx := fp.GetCursorIndex()
	if len(fp.Entries) == 0 || idx < 0 || idx >= len(fp.Entries) {
		return ""
	}
	entry := fp.Entries[idx]
	if entry.Name == ".." {
		return fp.Vfs.Dir(fp.Vfs.GetPath())
	}
	return entry.Name
}

func (fp *FileSystemPanel) GetRawSelectedName() string {
	idx := fp.GetCursorIndex()
	if len(fp.Entries) == 0 || idx < 0 || idx >= len(fp.Entries) {
		return ""
	}
	return fp.Entries[idx].Name
}

// SetSelectedByName picks or unpicks an entry by name and reports whether the
// panel shows such an entry at all. It is how the picture gallery keeps the
// panel underneath in step with what the reader has picked; a panel that has
// walked away to another directory simply answers no.
func (fp *FileSystemPanel) SetSelectedByName(name string, state bool) bool {
	for i, e := range fp.Entries {
		if e.Name == name {
			fp.SetItemSelected(i, state)
			return true
		}
	}
	return false
}

// IsNameSelected reports whether an entry has been picked explicitly.
func (fp *FileSystemPanel) IsNameSelected(name string) bool {
	return fp.SelectedItems[name]
}

// panelSelectionToken identifies one explicit mark without giving an async
// Apply batch permission to undo a later user action on the same filename.
type PanelSelectionToken struct {
	Panel          *FileSystemPanel
	Vfs            vfs.VFS
	path           string
	name           string
	DirectoryEpoch uint64
	selectionEpoch uint64
}

func (fp *FileSystemPanel) CaptureSelectionToken(name string) (PanelSelectionToken, bool) {
	if fp == nil || !fp.IsNameSelected(name) || fp.Vfs == nil {
		return PanelSelectionToken{}, false
	}
	return PanelSelectionToken{
		Panel:          fp,
		Vfs:            fp.Vfs,
		path:           fp.Vfs.GetPath(),
		name:           name,
		DirectoryEpoch: fp.DirectoryEpoch,
		selectionEpoch: fp.selectionEpoch[name],
	}, true
}

func (fp *FileSystemPanel) ClearSelectionIfUnchanged(token PanelSelectionToken) bool {
	if fp == nil || token.Panel != fp || fp.Vfs == nil || !fileops.SameVFSInstance(fp.Vfs, token.Vfs) || fp.Vfs.GetPath() != token.path ||
		fp.DirectoryEpoch != token.DirectoryEpoch || fp.selectionEpoch[token.name] != token.selectionEpoch ||
		!fp.IsNameSelected(token.name) {
		return false
	}
	return fp.SetSelectedByName(token.name, false)
}

// ImageSiblings lists the pictures of this panel in the order it shows them,
// together with the position of the one under the cursor, or minus one when
// the cursor is not on a picture.
func (fp *FileSystemPanel) ImageSiblings() ([]string, int) {
	current := fp.GetRawSelectedName()
	names := make([]string, 0, len(fp.Entries))
	index := -1
	for _, e := range fp.Entries {
		if e.IsDir || e.Name == ".." || !media.IsImageFile(e.Name) {
			continue
		}
		if e.Name == current {
			index = len(names)
		}
		names = append(names, e.Name)
	}
	return names, index
}

// AudioSiblings is ImageSiblings for recordings: the audio files of this
// panel in the order it shows them, and the position of the one under the
// cursor, or minus one when the cursor is not on one.
func (fp *FileSystemPanel) AudioSiblings() ([]string, int) {
	current := fp.GetRawSelectedName()
	names := make([]string, 0, len(fp.Entries))
	index := -1
	for _, e := range fp.Entries {
		if e.IsDir || e.Name == ".." || !media.IsAudioFile(e.Name) {
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
	for i, entry := range fp.Entries {
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
	for _, e := range fp.Entries {
		if e.Selected && e.Name != ".." {
			names = append(names, e.Name)
		}
	}
	// 2. If nothing is selected, fallback to the item under cursor
	if len(names) == 0 {
		idx := fp.GetCursorIndex()
		if idx >= 0 && idx < len(fp.Entries) {
			entry := fp.Entries[idx]
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
	for _, entry := range fp.Entries {
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
	for i, entry := range fp.Entries {
		_, state := selected[entry.Name]
		fp.SetItemSelected(i, state)
	}
	fp.Refresh()
}

// GetSuccessorName determines which file should receive focus after the current
// selection (or focused item) is deleted or moved.
func (fp *FileSystemPanel) doFastFind(dir int) {
	if fp.FastFindStr == "" || len(fp.Entries) == 0 {
		return
	}
	benchmark := navtrace.NavigationBenchmarkCurrentUI()
	benchmarkEnabled := navtrace.NavigationBenchmarkIsEnabled()
	startedNs := int64(0)
	if benchmarkEnabled {
		startedNs = navtrace.NavigationBenchmarkMonotonicNs()
	}
	fp.ensureFastFindMatchCache()
	evaluationsBefore := fp.fastFindMatchEvaluations
	emitBenchmark := func(checkedRows int, matched bool, matchedIndex int, knownNoMatch bool) {
		if !benchmarkEnabled {
			return
		}
		finishedNs := navtrace.NavigationBenchmarkMonotonicNs()
		navtrace.NavigationBenchmarkEmitAt(navtrace.NavigationBenchmarkTraceName(benchmark),
			"fast_find.search", "go.ui", finishedNs,
			"direction", dir,
			"queryRunes", len([]rune(fp.FastFindStr)),
			"totalRows", len(fp.Entries),
			"checkedRows", checkedRows,
			"evaluatedRows", fp.fastFindMatchEvaluations-evaluationsBefore,
			"retainedRows", len(fp.fastFindMatchCache),
			"matched", matched,
			"matchedIndex", matchedIndex,
			"knownNoMatch", knownNoMatch,
			"durationNs", finishedNs-startedNs)
	}
	if fp.fastFindAnyMatchKnown && !fp.fastFindAnyMatch {
		emitBenchmark(0, false, -1, true)
		return
	}
	startIdx := fp.GetCursorIndex()
	step := 1
	firstOffset := 0
	if dir > 0 {
		firstOffset = 1
	} else if dir < 0 {
		step = -1
		firstOffset = -1
	}
	count := len(fp.Entries)
	firstIndex := (startIdx + firstOffset) % count
	if firstIndex < 0 {
		firstIndex += count
	}
	acceptMatch := func(index, checked int) {
		fp.fastFindAnyMatchKnown = true
		fp.fastFindAnyMatch = true
		fp.compactFastFindMatchCache(index)
		fp.SetCursorIndex(index)
		fp.Refresh()
		emitBenchmark(checked, true, index, false)
	}
	if _, _, ok := fp.fastFindMatchAt(firstIndex); ok {
		acceptMatch(firstIndex, 1)
		return
	}
	if candidate, probes, found, supported :=
		fp.fastFindSortedShortCandidate(firstIndex, step); supported {
		if found {
			if _, _, ok := fp.fastFindMatchAt(candidate); ok {
				acceptMatch(candidate, probes+2)
				return
			}
			// A sorted range and the canonical matcher must agree. If a
			// provider ever violates that invariant, preserve correctness by
			// falling through to the ordinary bounded traversal.
		} else {
			fp.fastFindAnyMatchKnown = true
			fp.fastFindAnyMatch = false
			clear(fp.fastFindMatchCache)
			emitBenchmark(probes+1, false, -1, false)
			return
		}
	}
	for checked := 1; checked < count; checked++ {
		index := (firstIndex + checked*step) % count
		if index < 0 {
			index += count
		}
		if _, _, ok := fp.fastFindMatchAt(index); ok {
			acceptMatch(index, checked+1)
			return
		}
	}
	fp.fastFindAnyMatchKnown = true
	fp.fastFindAnyMatch = false
	clear(fp.fastFindMatchCache)
	emitBenchmark(count, false, -1, false)
}

// SaveSelection snapshots the current selection. The name map is the durable
// source of truth: fileEntry rows are rebuilt by asynchronous refreshes, so a
// row-only snapshot would make Ctrl+M forget Apply's original marks.
func (fp *FileSystemPanel) SaveSelection() {
	previous := make(map[string]bool)
	for _, e := range fp.Entries {
		e.PrevSelected = e.Selected
		if e.Name != ".." && e.Selected {
			previous[e.Name] = true
		}
	}
	fp.previousSelection = previous
	fp.previousSelectionVFS = fp.Vfs
	if fp.Vfs != nil {
		fp.previousSelectionPath = fp.Vfs.GetPath()
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
	if fp.Vfs == nil || !fp.previousSelectionMatches(fp.Vfs, fp.Vfs.GetPath()) {
		saved = nil
	}
	current := make(map[string]bool)
	for i, e := range fp.Entries {
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
	fp.previousSelectionVFS = fp.Vfs
	if fp.Vfs != nil {
		fp.previousSelectionPath = fp.Vfs.GetPath()
	} else {
		fp.previousSelectionPath = ""
	}
	vtui.FrameManager.Redraw()
}

func (fp *FileSystemPanel) InvertSelection() {
	fp.SaveSelection()
	for i, e := range fp.Entries {
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
	for i, e := range fp.Entries {
		if e.Name == ".." {
			continue
		}
		nameLower := strings.ToLower(e.visibleName())
		matched, _ := filepath.Match(maskLower, nameLower)
		if matched {
			fp.SetItemSelected(i, state)
		}
	}
	vtui.FrameManager.Redraw()
}

func (fp *FileSystemPanel) GetSuccessorName() string {
	if len(fp.Entries) <= 1 {
		return ".."
	}

	anySelected := false
	for _, e := range fp.Entries {
		if e.Selected && e.Name != ".." {
			anySelected = true
			break
		}
	}

	var firstIdx, lastIdx int

	if anySelected {
		// If something is selected, we only care about the selection range
		firstIdx = len(fp.Entries)
		lastIdx = -1
		for i, e := range fp.Entries {
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
		firstIdx = fp.CursorIdx
		lastIdx = fp.CursorIdx
	}

	// Helper to check if an item at index i is about to be removed
	isToBeRemoved := func(i int) bool {
		if anySelected {
			return fp.Entries[i].Selected && fp.Entries[i].Name != ".."
		}
		return i == fp.CursorIdx
	}

	// 1. Try to find the first valid item AFTER the removed block
	for i := lastIdx + 1; i < len(fp.Entries); i++ {
		if !isToBeRemoved(i) {
			return fp.Entries[i].Name
		}
	}

	// 2. If no "next" item, try to find the first valid item BEFORE the removed block
	for i := firstIdx - 1; i >= 0; i-- {
		if !isToBeRemoved(i) && fp.Entries[i].Name != ".." {
			return fp.Entries[i].Name
		}
	}

	// 3. Fallback to parent directory entry
	return ".."
}

// GetPredecessorName returns the first remaining entry before the item (or
// selected range) that an action is about to remove. If there is no such
// entry, it falls back to the first remaining entry after it. The fallback
// keeps the cursor on a useful row when the first item is removed.
func (fp *FileSystemPanel) GetPredecessorName() string {
	if len(fp.Entries) <= 1 {
		return ".."
	}

	anySelected := false
	for _, e := range fp.Entries {
		if e.Selected && e.Name != ".." {
			anySelected = true
			break
		}
	}

	var firstIdx, lastIdx int
	if anySelected {
		firstIdx = len(fp.Entries)
		lastIdx = -1
		for i, e := range fp.Entries {
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
		firstIdx = fp.CursorIdx
		lastIdx = fp.CursorIdx
	}

	isToBeRemoved := func(i int) bool {
		if anySelected {
			return fp.Entries[i].Selected && fp.Entries[i].Name != ".."
		}
		return i == fp.CursorIdx
	}

	for i := firstIdx - 1; i >= 0; i-- {
		if !isToBeRemoved(i) && fp.Entries[i].Name != ".." {
			return fp.Entries[i].Name
		}
	}

	for i := lastIdx + 1; i < len(fp.Entries); i++ {
		if !isToBeRemoved(i) && fp.Entries[i].Name != ".." {
			return fp.Entries[i].Name
		}
	}

	return ".."
}

// CurrentPanelEntryPath returns the full path represented by the panel cursor.
// As in Far, the parent entry represents the current directory for path-copy
// and path-insertion commands.
func CurrentPanelEntryPath(fsp *FileSystemPanel) string {
	if fsp == nil || fsp.Vfs == nil {
		return ""
	}
	idx := fsp.GetCursorIndex()
	if idx < 0 || idx >= len(fsp.Entries) {
		return ""
	}
	base := fsp.Vfs.GetPath()
	if fsp.Entries[idx].Name == ".." {
		return base
	}
	return fsp.Vfs.Join(base, fsp.Entries[idx].Name)
}
