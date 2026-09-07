package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	xdraw "golang.org/x/image/draw"
)

// QuickViewPanel is far2l's Ctrl+Q quick-view panel. It mirrors the
// source file panel's current cursor: for a directory it kicks off an
// async recursive scan and shows the running Folders / Files / Files-
// size totals; for a regular file it shows a text preview or a hex
// dump depending on a simple binary heuristic. Full-file viewer
// features (search, syntax highlighting, …) are deliberately deferred.
type QuickViewPanel struct {
	vtui.ScreenObject
	src     *FileSystemPanel
	frame   *vtui.BorderedFrame
	focused bool

	// Cache the last-computed preview so we don't re-read the file /
	// re-scan the directory on every redraw.
	cacheKey        quickViewSelectionKey
	cachePath       string
	cacheValid      bool
	cacheDir        bool // whether cache is for a directory or file
	cacheBinary     bool
	cacheImage      bool // whether cache is an image
	cacheLoading    bool
	cacheLabel      string
	imageSurf       *vtui.ImageSurface
	imageLoadGen    uint64
	gfxKey          string
	cacheLines      []string // raw preview lines (source lines or hex rows)
	cacheRaw        []byte   // raw bytes for the default text/hex preview
	cacheCodepage   int
	cacheAutoDetect bool
	cacheReadErr    error

	// Specialized providers may inspect an entire media/container header, so
	// they run away from the UI thread. previewGen rejects a late result after
	// the cursor, VFS session or file revision changed, even if provider code
	// did not return promptly when previewCancel was fired.
	previewCancel context.CancelFunc
	previewGen    uint64

	// Async recursive scan state for the directory case. Guarded by
	// scanMu so the goroutine and the UI thread can share it. scanGen
	// bumps on every new scan AND on every cancel; callbacks whose
	// gen mismatches are ignored (the goroutine may still be draining
	// after a cursor change or Close cancelled its ctx), so no stale
	// numbers can leak into a fresh state. scanDoneCh is closed on
	// completion and recreated per scan so tests can wait for
	// finalisation. scanClusterSize gates only the "Cluster size" row
	// in render; Physical/Ratio are gated by scanStats.PhysicalBytes.
	scanMu          sync.Mutex
	scanCancel      context.CancelFunc
	scanGen         uint64
	scanStats       vfs.OpStats
	scanClusterSize uint64
	scanDone        bool
	scanErr         error
	scanLastRedraw  time.Time
	scanDoneCh      chan struct{}

	// Display state driven by the keyboard while the panel is focused.
	wrap             bool
	scrollY          int
	scrollX          int
	hexMode          bool
	lastSearch       string
	lastSearchSource int
	codepages        map[quickViewSelectionKey]int

	// F2 (wrap toggle) sets these to re-anchor scrollY on the source
	// line the user was reading, so the new re-flow doesn't move the
	// text out from under them.
	pinSourceOnNextShow int
	hasPin              bool

	// Wrapped view: cacheLines re-flowed to fit innerW. Rebuilt when
	// content / wrap flag / innerW changes. displayToSource[i] holds
	// the index of the SOURCE line (cacheLines) the display line i
	// belongs to, so F2 can pin the currently-visible source line
	// while the display re-flows around it.
	displayLines    []string
	displayToSource []int
	displayWrap     bool
	displayWidth    int

	// Semantic Quick View uses the same bounded-window contract as the full
	// Viewer/Editor surfaces.  contentKey changes only when the mirrored file
	// changes, so a delayed native scroll request can never move a newer
	// preview. windowGeneration acknowledges viewport requests, including
	// clamped no-ops.
	semanticContentSerial    uint64
	semanticContentKey       string
	semanticHasSelection     bool
	semanticWindowGeneration uint64

	// Native frontends cannot consume vtui.ImageSurface directly. Cache one
	// bounded PNG representation per immutable decoded surface instead of
	// serialising the full source pixels on every semantic scene.
	semanticImageSurface    *vtui.ImageSurface
	semanticImageGeneration uint64
	semanticImageSource     string
	semanticImageWidth      int
	semanticImageHeight     int
}

// quickViewSelectionKey prevents identical-looking paths from different VFS
// sessions sharing a preview. Revision is preferred when a provider supplies
// one; size and mtime keep ordinary files responsive to a refreshed listing.
type quickViewSelectionKey struct {
	source   string
	revision string
	size     int64
	mtimeNS  int64
	isDir    bool
}

type quickViewPreparedSelection struct {
	item *fileEntry
	path string
	key  quickViewSelectionKey
}

func (q *QuickViewPanel) bumpSemanticContentKey() {
	q.semanticContentSerial++
	q.semanticContentKey = fmt.Sprintf("%s:%d", vtui.SemanticID(q), q.semanticContentSerial)
	q.semanticWindowGeneration++
}

// prepareSelection is the authoritative selection/cache transition shared by
// the raster TUI renderer and the semantic frontend. Native Quick View must
// remain fully functional when Show is never called for its covered panel.
func (q *QuickViewPanel) prepareSelection() (quickViewPreparedSelection, bool) {
	if q.src == nil || q.src.vfs == nil || len(q.src.entries) == 0 {
		if q.semanticHasSelection || q.semanticContentKey == "" {
			q.cancelScan()
			q.cancelFilePreview()
			q.imageLoadGen++
			q.cacheValid = false
			q.semanticHasSelection = false
			q.scrollY, q.scrollX = 0, 0
			q.displayLines, q.displayToSource = nil, nil
			q.clearSemanticImage()
			q.bumpSemanticContentKey()
		}
		return quickViewPreparedSelection{}, false
	}

	idx := q.src.GetCursorIndex()
	if idx < 0 || idx >= len(q.src.entries) || q.src.entries[idx] == nil {
		if q.semanticHasSelection || q.semanticContentKey == "" {
			q.cancelScan()
			q.cancelFilePreview()
			q.imageLoadGen++
			q.cacheValid = false
			q.semanticHasSelection = false
			q.scrollY, q.scrollX = 0, 0
			q.displayLines, q.displayToSource = nil, nil
			q.clearSemanticImage()
			q.bumpSemanticContentKey()
		}
		return quickViewPreparedSelection{}, false
	}

	item := q.src.entries[idx]
	path := q.src.vfs.Join(q.src.vfs.GetPath(), item.Name)
	if item.Name == ".." {
		// far2/far2l describe the current directory for the parent marker.
		path = q.src.vfs.GetPath()
		synth := &fileEntry{VFSItem: vfs.VFSItem{Name: path, IsDir: true}}
		item = synth
	}
	key := makeQuickViewSelectionKey(q.src.vfs, path, item.VFSItem)
	if !q.cacheValid || key != q.cacheKey {
		q.refreshCache(key, path, *item)
		q.semanticHasSelection = true
		q.scrollY, q.scrollX = 0, 0
		q.displayLines, q.displayToSource = nil, nil
		q.bumpSemanticContentKey()
	} else {
		q.semanticHasSelection = true
		if q.semanticContentKey == "" {
			q.bumpSemanticContentKey()
		}
	}
	return quickViewPreparedSelection{item: item, path: path, key: key}, true
}

// NewQuickViewPanel creates a quick-view panel over src's slot.
func NewQuickViewPanel(src *FileSystemPanel) *QuickViewPanel {
	x1, y1, x2, y2 := src.GetPosition()
	q := &QuickViewPanel{src: src, wrap: true, lastSearchSource: -1, codepages: make(map[quickViewSelectionKey]int)}
	q.SetVisible(true)
	q.frame = vtui.NewBorderedFrame(x1, y1, x2, y2, vtui.SingleBox, Msg("QuickView.Title"))
	q.frame.ColorBoxIdx = ColPanelBox
	q.frame.ColorTitleIdx = ColPanelTitle
	q.frame.ColorBackgroundIdx = ColPanelInfoText
	q.gfxKey = fmt.Sprintf("f4.quickview:%p", q)
	q.SetPosition(x1, y1, x2, y2)
	return q
}

func (q *QuickViewPanel) SetPosition(x1, y1, x2, y2 int) {
	q.ScreenObject.SetPosition(x1, y1, x2, y2)
	if q.frame != nil {
		q.frame.SetPosition(x1, y1, x2, y2)
	}
}

func (q *QuickViewPanel) Source() *FileSystemPanel { return q.src }
func (q *QuickViewPanel) Kind() string             { return "quick_view" }

// SetFocus tracks the focused marker (title recolour). When focused
// the panel starts consuming navigation keys — see ProcessKey.
func (q *QuickViewPanel) SetFocus(f bool) {
	q.focused = f
	if q.frame != nil {
		if f {
			q.frame.ColorTitleIdx = ColPanelSelectedTitle
		} else {
			q.frame.ColorTitleIdx = ColPanelTitle
		}
	}
}
func (q *QuickViewPanel) IsFocused() bool { return q.focused }

// ProcessKey handles scroll / wrap-toggle keys while focused. Any
// key we don't recognise falls through (return false), letting the
// global handler chain deal with Ctrl+Q close, Tab, etc.
func (q *QuickViewPanel) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown || !q.focused {
		return false
	}
	ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
	alt := e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
	shift := e.ControlKeyState&vtinput.ShiftPressed != 0
	// Ctrl+Q / Ctrl+L / Alt+F8 and the other global combinations need
	// to reach the panel frame unchanged.
	if ctrl || alt {
		return false
	}
	switch e.VirtualKeyCode {
	case vtinput.VK_UP:
		q.scrollY--
	case vtinput.VK_DOWN:
		q.scrollY++
	case vtinput.VK_PRIOR: // PgUp
		q.scrollY -= q.pageHeight()
	case vtinput.VK_NEXT: // PgDn
		q.scrollY += q.pageHeight()
	case vtinput.VK_HOME:
		q.scrollY = 0
		q.scrollX = 0
	case vtinput.VK_END:
		q.scrollY = 1 << 30 // clamped by Show
	case vtinput.VK_LEFT:
		if !q.wrap {
			q.scrollX--
		}
	case vtinput.VK_RIGHT:
		if !q.wrap {
			q.scrollX++
		}
	case vtinput.VK_F2:
		// Pin the currently-visible source line before re-flowing so
		// the user's reading position doesn't scroll away. Falls back
		// to 0 if we haven't computed a mapping yet.
		pinnedSrc := 0
		if q.scrollY >= 0 && q.scrollY < len(q.displayToSource) {
			pinnedSrc = q.displayToSource[q.scrollY]
		}
		q.wrap = !q.wrap
		q.scrollX = 0
		q.displayLines = nil // force re-flow on next Show
		q.pinSourceOnNextShow = pinnedSrc
		q.hasPin = true
	case vtinput.VK_F3:
		return q.openSelectedInViewer()
	case vtinput.VK_F4:
		if !q.toggleHexMode() {
			return false
		}
	case vtinput.VK_F7:
		if shift {
			return q.repeatSearch(false)
		}
		q.showSearchDialog()
		return true
	case vtinput.VK_F8:
		if shift {
			q.showCodepageDialog()
		} else {
			q.switchToCodepage(vfs.GetNextFastSwitchCodepage(q.cacheCodepage))
		}
		return true
	default:
		return false
	}
	if q.scrollX < 0 {
		q.scrollX = 0
	}
	if q.scrollY < 0 {
		q.scrollY = 0
	}
	q.semanticWindowGeneration++
	vtui.FrameManager.HardRefresh()
	return true
}

func (q *QuickViewPanel) selectedFile() (string, *fileEntry, bool) {
	if q.src == nil || q.src.vfs == nil {
		return "", nil, false
	}
	idx := q.src.GetCursorIndex()
	if idx < 0 || idx >= len(q.src.entries) {
		return "", nil, false
	}
	item := q.src.entries[idx]
	if item == nil || item.IsDir || item.Name == ".." {
		return "", nil, false
	}
	return q.src.vfs.Join(q.src.vfs.GetPath(), item.Name), item, true
}

func (q *QuickViewPanel) openSelectedInViewer() bool {
	path, _, ok := q.selectedFile()
	if !ok || q.src == nil || q.src.vfs == nil {
		return false
	}
	if pf := findPanelsFrameAnyScreen(); pf != nil {
		openViewerInternal(pf, q.src.vfs, path)
		return true
	}
	return false
}

func quickViewTextLines(data []byte) []string {
	lines := splitTextLines(string(data))
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

func (q *QuickViewPanel) applyPreviewCodepage(cpID int, autoDetect bool) bool {
	if q.cacheRaw == nil {
		return false
	}
	decoded, err := vfs.DecodeBytes(q.cacheRaw, cpID)
	if err != nil {
		q.cacheReadErr = err
		return false
	}
	q.cacheCodepage = cpID
	q.cacheAutoDetect = autoDetect
	q.cacheBinary = looksBinary(decoded)
	if q.hexMode {
		q.cacheLines = hexDumpLines(q.cacheRaw)
	} else {
		q.cacheLines = quickViewTextLines(decoded)
	}
	q.displayLines = nil
	q.displayToSource = nil
	q.updateFrameTitle()
	return true
}

func (q *QuickViewPanel) switchToCodepage(cpID int) bool {
	if q.cacheRaw == nil {
		return false
	}
	if !q.applyPreviewCodepage(cpID, false) {
		return false
	}
	if q.codepages == nil {
		q.codepages = make(map[quickViewSelectionKey]int)
	}
	q.codepages[q.cacheKey] = cpID
	q.persistCodepage(cpID)
	vtui.FrameManager.HardRefresh()
	return true
}

func (q *QuickViewPanel) persistCodepage(cpID int) {
	if GlobalFileState == nil || q.src == nil || q.src.vfs == nil || q.cachePath == "" {
		return
	}
	GlobalFileState.SaveQuickViewCodepageAsync(FileStateKey(q.src.vfs, q.cachePath), cpID)
}

func (q *QuickViewPanel) rememberedCodepage() (int, bool) {
	if cpID, ok := q.codepages[q.cacheKey]; ok {
		return cpID, true
	}
	if GlobalFileState == nil || q.src == nil || q.src.vfs == nil || q.cachePath == "" {
		return 0, false
	}
	state := GlobalFileState.GetState(FileStateKey(q.src.vfs, q.cachePath))
	if state == nil || state.QuickViewCodepage <= 0 {
		return 0, false
	}
	return state.QuickViewCodepage, true
}

func (q *QuickViewPanel) showCodepageDialog() {
	if q.cacheRaw == nil {
		return
	}
	items, currIdx := vfs.BuildCodepageMenuItems(q.cacheCodepage, q.cacheAutoDetect)
	menu := vtui.NewVMenu(Msg("Codepage.Title"))
	for _, item := range items {
		menu.AddItem(item)
	}
	w, h := 45, len(items)+2
	scrW := vtui.FrameManager.GetScreenSize()
	scrH := vtui.FrameManager.GetScreenHeight()
	maxH := scrH - 2
	if maxH < 5 {
		maxH = 5
	}
	if h > maxH {
		h = maxH
	}
	x := (scrW - w) / 2
	y := (scrH - h) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	menu.SetPosition(x, y, x+w-1, y+h-1)
	menu.OnAction = func(idx int) {
		menu.Close()
		if idx < 0 || idx >= len(menu.Items) {
			return
		}
		cpID, ok := menu.Items[idx].UserData.(int)
		if !ok {
			return
		}
		if cpID == vfs.CodepageAutoDetect {
			delete(q.codepages, q.cacheKey)
			q.persistCodepage(0)
			q.applyPreviewCodepage(vfs.DetectEncoding(q.cacheRaw, AppConfig.ViewerAutodetectCodePage, AppConfig.ViewerDefaultCodePage), true)
			vtui.FrameManager.HardRefresh()
			return
		}
		q.switchToCodepage(cpID)
	}
	menu.SetSelectPos(currIdx)
	vtui.FrameManager.Push(menu)
}

func (q *QuickViewPanel) toggleHexMode() bool {
	if q.cacheRaw == nil {
		return false
	}
	q.hexMode = !q.hexMode
	if q.hexMode {
		q.cacheLines = hexDumpLines(q.cacheRaw)
	} else {
		if !q.applyPreviewCodepage(q.cacheCodepage, q.cacheAutoDetect) {
			return false
		}
	}
	q.displayLines = nil
	q.displayToSource = nil
	vtui.FrameManager.HardRefresh()
	return true
}

func (q *QuickViewPanel) showSearchDialog() {
	if q.cacheLoading || len(q.cacheLines) == 0 {
		return
	}
	vtui.InputBox(Msg("Viewer.SearchTitle"), "Search for:", q.lastSearch, func(pattern string) {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			return
		}
		q.lastSearch = pattern
		q.lastSearchSource = -1
		q.repeatSearch(false)
	})
}

func (q *QuickViewPanel) repeatSearch(reverse bool) bool {
	if q.lastSearch == "" || len(q.cacheLines) == 0 {
		return false
	}
	start := 0
	if reverse {
		start = len(q.cacheLines) - 1
		if q.lastSearchSource >= 0 {
			start = q.lastSearchSource - 1
		}
	} else if q.lastSearchSource >= 0 {
		start = q.lastSearchSource + 1
	}
	if start < 0 {
		start = len(q.cacheLines) - 1
	}
	if start >= len(q.cacheLines) {
		start = 0
	}
	needle := strings.ToLower(q.lastSearch)
	for n := 0; n < len(q.cacheLines); n++ {
		idx := start + n
		if reverse {
			idx = start - n
		}
		for idx < 0 {
			idx += len(q.cacheLines)
		}
		idx %= len(q.cacheLines)
		if strings.Contains(strings.ToLower(q.cacheLines[idx]), needle) {
			q.lastSearchSource = idx
			q.scrollToSource(idx)
			vtui.FrameManager.HardRefresh()
			return true
		}
	}
	return true
}

func (q *QuickViewPanel) scrollToSource(source int) {
	innerW := q.X2 - q.X1 - 1
	if innerW < 1 {
		return
	}
	q.displayLines, q.displayToSource = q.buildDisplayLines(innerW)
	q.displayWrap = q.wrap
	q.displayWidth = innerW
	q.scrollY = firstDisplayForSource(q.displayToSource, source)
	q.hasPin = false
}

func (q *QuickViewPanel) updateFrameTitle() {
	if q.frame == nil {
		return
	}
	title := Msg("QuickView.Title")
	if !q.cacheDir && q.cacheCodepage > 0 {
		title = fmt.Sprintf("%s │ %s", title, vfs.DisplayCodepageName(q.cacheCodepage))
	}
	q.frame.SetTitle(title)
}

// ProcessMouse handles the wheel over the panel. Uses WheelDirection
// as the wheel signal (universal across platforms — Linux SGR mouse
// only sets WheelDirection, Windows ConPTY sets both MouseWheeled
// flag and WheelDirection; the flag-based check misses Linux).
// PanelsFrame's dispatch routes wheel-on-active-alt here.
func (q *QuickViewPanel) ProcessMouse(e *vtinput.InputEvent) bool {
	if e.WheelDirection == 0 {
		return false
	}
	step := 3
	if e.WheelDirection > 0 {
		q.scrollY -= step
	} else {
		q.scrollY += step
	}
	if q.scrollY < 0 {
		q.scrollY = 0
	}
	q.semanticWindowGeneration++
	vtui.FrameManager.HardRefresh()
	return true
}

func (q *QuickViewPanel) GetSelectedName() string {
	if q.src == nil {
		return ""
	}
	return q.src.GetSelectedName()
}

func (q *QuickViewPanel) pageHeight() int {
	h := q.Y2 - q.Y1 - 1 // room between borders
	if h < 1 {
		return 1
	}
	return h
}

func (q *QuickViewPanel) Show(scr *vtui.ScreenBuf) {
	if q.frame != nil {
		q.frame.Show(scr)
	}
	// Bottom-border hint reminding the user of the units toggle, same
	// pattern InfoPanel uses (same string too — the toggle behaves
	// identically). Drawn always while the panel is up because B
	// affects both the "Files size" number for directories and the
	// header "Size" for files.
	if q.frame != nil && q.Y2 > q.Y1+1 {
		hint := Msg("InfoPanel.UnitsHint")
		if runewidth.StringWidth(hint) < q.X2-q.X1-1 {
			attrBox := vtui.Palette[ColPanelBox]
			scr.Write(q.X1+2, q.Y2, vtui.StringToCharInfo(hint, attrBox))
		}
	}
	innerW := q.X2 - q.X1 - 1
	if innerW < 1 || q.src == nil {
		return
	}
	attr := vtui.Palette[ColPanelInfoText]
	y := q.Y1 + 1
	maxY := q.Y2 - 1

	writeLine := func(s string) {
		if y > maxY {
			return
		}
		if runewidth.StringWidth(s) > innerW {
			s = runewidth.Truncate(s, innerW, "…")
		}
		pad := innerW - runewidth.StringWidth(s)
		if pad > 0 {
			s += strings.Repeat(" ", pad)
		}
		ci := vtui.StringToCharInfo(s, attr)
		// Hard cap on cell count — defends the right border against
		// any pathological width mismatch between StringWidth and
		// StringToCharInfo (double-width edge cases, etc.).
		if len(ci) > innerW {
			ci = ci[:innerW]
		}
		scr.Write(q.X1+1, y, ci)
		y++
	}

	selection, ok := q.prepareSelection()
	if !ok {
		writeLine(" " + Msg("QuickView.NoSelection"))
		return
	}
	item := selection.item

	if q.cacheDir {
		q.renderDir(item, writeLine)
		return
	}
	q.renderFile(item, innerW, writeLine, attr, scr)

	// Vertical scrollbar over the right border. Repaints column X2
	// with scrollbar glyphs, so if a wide content line ever bled
	// into the border position it gets restored. Skipped when the
	// content fits entirely (DrawScrollBar returns false).
	if q.Y2 > q.Y1+1 && len(q.displayLines) > 0 {
		vtui.DrawScrollBar(scr, q.X2, q.Y1+1, q.Y2-q.Y1-1,
			q.scrollY, len(q.displayLines), vtui.Palette[ColPanelScrollbar])
	}
}

func (q *QuickViewPanel) renderDir(item *fileEntry, writeLine func(string)) {
	writeLine(" " + Msg("QuickView.Folder") + " \"" + item.Name + "\"")
	writeLine("")
	q.scanMu.Lock()
	stats := q.scanStats
	cluster := q.scanClusterSize
	done := q.scanDone
	serr := q.scanErr
	q.scanMu.Unlock()

	// vfs.CalculateStats counts the passed-in directory itself as one
	// of Dirs, but far2/far2l show "Folders" as the child-folder count.
	// Subtract 1 (clamped) so the two match.
	dirs := stats.Dirs - 1
	if dirs < 0 {
		dirs = 0
	}

	writeLine(" " + Msg("QuickView.Contains") + ":")
	writeLine("")
	writeLine(fmt.Sprintf(" %-14s %d", Msg("QuickView.FolderCount"), dirs))
	writeLine(fmt.Sprintf(" %-14s %d", Msg("QuickView.FileCount"), stats.Files))
	// "Files size" adds dir-inode Sizes to file bytes — that's what
	// far2l puts in "Размер файлов" (see far2l/src/dirinfo.cpp:
	// FileSize += FindData.nFileSize for directories). On Windows,
	// though, Far/Explorer count only file bytes: child dirs report
	// Size 0 via ReadDir, and the sole non-zero contributor is the
	// scanned root itself — os.Stat of a directory returns a 4096
	// index size via GetFileInformationByHandle. Drop DirBytes there
	// so the total matches the platform's native tools.
	logical := stats.Bytes + stats.DirBytes
	if runtime.GOOS == "windows" {
		logical = stats.Bytes
	}
	writeLine(fmt.Sprintf(" %-14s %s", Msg("QuickView.FilesSize"), formatBytes(nonNegativeUint64(logical))))
	// Physical size + Ratio need per-item on-disk footprint. Stub /
	// remote VFSes leave PhysicalBytes at 0 during the whole scan —
	// hide the rows in that case. Ratio is also hidden when it would
	// just read "100%" — on Unix that's every uncompressed tree, and
	// a constant carries no information for the reader.
	if stats.PhysicalBytes > 0 {
		writeLine(fmt.Sprintf(" %-14s %s", Msg("QuickView.PhysicalSize"), formatBytes(nonNegativeUint64(stats.PhysicalBytes))))
		if stats.PhysicalBytes < logical {
			// Ratio interpretation matches far/far2l — >100% means "on
			// disk it takes less than the logical size", i.e. real
			// NTFS compression / sparse regions.
			ratio := int((logical * 100) / stats.PhysicalBytes)
			writeLine(fmt.Sprintf(" %-14s %d%%", Msg("QuickView.Ratio"), ratio))
		}
	}
	// Cluster size stands on its own — shown even when PhysicalBytes
	// couldn't be filled (VFS without per-item support).
	if cluster > 0 {
		writeLine("")
		writeLine(fmt.Sprintf(" %-14s %s", Msg("QuickView.ClusterSize"), formatBytes(cluster)))
	}
	// Single "scanning" hint per far2l — one trailing line at the
	// bottom, not repeated on every row.
	if !done && serr == nil {
		writeLine("")
		writeLine(" " + Msg("QuickView.Scanning"))
	}
	if serr != nil {
		writeLine("")
		writeLine(" " + Msg("QuickView.ReadError") + ": " + serr.Error())
	}
}

// startDirScan cancels any running scan and kicks off a fresh async
// vfs.CalculateStats for the given directory. Progress lands in
// scanStats under scanMu; the UI is nudged via HardRefresh no more
// than every 200ms while the scan runs. On completion the final stats
// (plus scanErr if any) are latched and scanDone becomes true.
// fsInfo (statfs / GetDiskFreeSpace) is done inside the goroutine —
// it can block for seconds on a hung NFS/SMB mount and must not sit
// on the UI thread.
func (q *QuickViewPanel) startDirScan(fullPath string) {
	q.scanMu.Lock()
	if q.scanCancel != nil {
		q.scanCancel()
	}
	q.scanGen++
	gen := q.scanGen
	ctx, cancel := context.WithCancel(context.Background())
	q.scanCancel = cancel
	q.scanStats = vfs.OpStats{}
	q.scanClusterSize = 0
	q.scanDone = false
	q.scanErr = nil
	q.scanLastRedraw = time.Time{}
	done := make(chan struct{})
	q.scanDoneCh = done
	source := q.src.vfs
	// CalculateStats derives its target via Join(basePath, name), so
	// split fullPath on its parent+basename. Works for both children
	// of GetPath() and for GetPath() itself (the ".." case).
	basePath := source.Dir(fullPath)
	name := source.Base(fullPath)
	q.scanMu.Unlock()

	// Read on the goroutine that starts this work, not inside it: the
	// work outlives the call, and reading the global from it races
	// anything that reassigns vtui.FrameManager meanwhile.
	frames := vtui.FrameManager
	go func() {
		defer close(done)
		// fsInfo() is a syscall that may block on stuck network
		// mounts; do it here, off the UI thread. Cluster size is a
		// display-only field.
		if fs, ok := fsInfo(fullPath); ok {
			q.scanMu.Lock()
			if q.scanGen == gen {
				q.scanClusterSize = fs.ClusterSize
			}
			q.scanMu.Unlock()
		}
		// QuickView explicitly does NOT follow symlink-to-dir and
		// DEDUPS hard links (same-inode counted once) — matches
		// far2/far2l and `find`. The copy/move code path keeps the
		// historical follow-through and no-dedup so pre-scan ETAs
		// there still line up with the actual walk.
		scanOpts := vfs.ScanOptions{FollowSymlinkDirs: false, DedupInodes: true}
		stats, err := vfs.CalculateStatsWithOptions(ctx, source, basePath, []string{name}, scanOpts, func(_ string, s vfs.OpStats) {
			q.scanMu.Lock()
			if q.scanGen != gen {
				q.scanMu.Unlock()
				return
			}
			q.scanStats = s
			redraw := time.Since(q.scanLastRedraw) > 200*time.Millisecond
			if redraw {
				q.scanLastRedraw = time.Now()
			}
			q.scanMu.Unlock()
			if redraw {
				frames.PostTask(frames.HardRefresh)
			}
		})
		q.scanMu.Lock()
		if q.scanGen == gen {
			q.scanStats = stats
			if err != nil && ctx.Err() == nil {
				q.scanErr = err
			}
			q.scanDone = true
		}
		q.scanMu.Unlock()
		frames.PostTask(frames.HardRefresh)
	}()
}

// cancelScan tears down any in-flight scan and clears scan state.
// Called both when switching from a dir to a file entry and on Close.
// Bumps scanGen so any still-draining callback of the old goroutine is
// rejected and can't stamp stale numbers onto the freshly-cleared state.
func (q *QuickViewPanel) cancelScan() {
	q.scanMu.Lock()
	if q.scanCancel != nil {
		q.scanCancel()
		q.scanCancel = nil
	}
	q.scanGen++
	q.scanStats = vfs.OpStats{}
	q.scanClusterSize = 0
	q.scanDone = false
	q.scanErr = nil
	q.scanMu.Unlock()
}

// Close cancels any running scan. Called by PanelsFrame.toggleAltPanel
// when the QuickView panel is being removed (Ctrl+Q toggle-off,
// Ctrl+L replacing it, etc.), so the scan goroutine doesn't outlive
// the panel it's populating.
func (q *QuickViewPanel) Close() {
	q.cancelScan()
	q.cancelFilePreview()
	q.imageLoadGen++
	q.cacheValid = false
	q.clearSemanticImage()
}

func (q *QuickViewPanel) fileViewportRows() int {
	// Interior rows remaining below the four-line file header.
	return max(0, q.Y2-q.Y1-5)
}

func (q *QuickViewPanel) ensureDisplayLayout(innerW int) {
	if innerW <= 0 || q.cacheDir || q.cacheImage || q.cacheLoading || q.cacheReadErr != nil {
		return
	}
	if q.displayLines == nil || q.displayWrap != q.wrap || q.displayWidth != innerW {
		q.displayLines, q.displayToSource = q.buildDisplayLines(innerW)
		q.displayWrap = q.wrap
		q.displayWidth = innerW
		if q.hasPin {
			q.scrollY = firstDisplayForSource(q.displayToSource, q.pinSourceOnNextShow)
			q.hasPin = false
		}
	}

	maxScroll := len(q.displayLines) - q.fileViewportRows()
	if maxScroll < 0 {
		maxScroll = 0
	}
	q.scrollY = max(0, min(q.scrollY, maxScroll))
}

func (q *QuickViewPanel) renderFile(item *fileEntry, innerW int, writeLine func(string), attr uint64, scr *vtui.ScreenBuf) {
	if q.cacheReadErr != nil {
		writeLine(" " + Msg("QuickView.ReadError") + ": " + q.cacheReadErr.Error())
		return
	}
	// The file name and size are already visible in the source panel. Keep
	// Quick View's header for the mode/encoding only, so the codepage remains
	// visible even for long names and never shifts with the panel selection.
	if q.cacheLoading {
		writeLine(" " + Msg("QuickView.Loading"))
	} else if q.cacheLabel != "" {
		writeLine(" " + q.cacheLabel)
	} else if q.cacheImage {
		writeLine(" " + Msg("QuickView.Image"))
	} else if q.hexMode {
		writeLine(" " + Msg("QuickView.Binary"))
	} else {
		writeLine("")
	}
	writeLine(" " + strings.Repeat("─", innerW-2))
	if q.cacheLoading {
		return
	}

	if q.cacheImage {
		q.renderImage(innerW, writeLine, attr, scr)
		return
	}

	q.ensureDisplayLayout(innerW)
	viewH := q.fileViewportRows()

	// Emit visible slice with optional horizontal shift.
	end := q.scrollY + viewH
	if end > len(q.displayLines) {
		end = len(q.displayLines)
	}
	for i := q.scrollY; i < end; i++ {
		line := q.displayLines[i]
		if !q.wrap && q.scrollX > 0 {
			line = trimLeftCells(line, q.scrollX)
		}
		writeLine(line)
	}
}
func (q *QuickViewPanel) renderImage(innerW int, writeLine func(string), attr uint64, scr *vtui.ScreenBuf) {
	if q.imageSurf == nil || !q.imageSurf.Valid() {
		writeLine(" [ Loading image... ]")
		return
	}
	if !scr.SupportsGraphics() {
		writeLine(" [ Image graphics not supported ]")
		return
	}

	x1, y1, x2, y2 := q.GetPosition()
	top := y1 + 1 + 2 // Below the mode/encoding header and separator
	cols := x2 - x1 - 1
	rows := y2 - top

	if cols <= 0 || rows <= 0 {
		return
	}

	cw, ch := scr.Graphics().CellSize()
	if cw <= 0 || ch <= 0 {
		cw, ch = imageViewFallbackCellW, imageViewFallbackCellH
	}

	boxW := cols * cw
	boxH := rows * ch

	fitW, fitH := vtui.FitInside(q.imageSurf.Width, q.imageSurf.Height, boxW, boxH)
	if fitW <= 0 || fitH <= 0 {
		return
	}

	p := vtui.ImagePlacement{Surface: q.imageSurf}
	p.Cols, p.Rows = cellsFor(fitW, cw, cols), cellsFor(fitH, ch, rows)
	p.Col = x1 + 1 + (cols-p.Cols)/2
	p.Row = top + (rows-p.Rows)/2
	p.SrcX, p.SrcY = 0, 0
	p.SrcW, p.SrcH = q.imageSurf.Width, q.imageSurf.Height
	p.ZIndex = -1 // Keep picture below panel borders if they overlap

	scr.Graphics().DrawImage(q.gfxKey, p)
}

const quickViewSemanticImageMaxDimension = 1024

func (q *QuickViewPanel) clearSemanticImage() {
	q.semanticImageSurface = nil
	q.semanticImageGeneration = 0
	q.semanticImageSource = ""
	q.semanticImageWidth = 0
	q.semanticImageHeight = 0
}

// semanticImageDataURL encodes at most a 1024x1024 representation once for a
// decoded surface. The original may contain tens of millions of pixels; the
// semantic protocol never repeats or transports that unbounded buffer.
func (q *QuickViewPanel) semanticImageDataURL() (string, int, int) {
	surface := q.imageSurf
	if surface == nil || !surface.Valid() {
		q.clearSemanticImage()
		return "", 0, 0
	}
	if q.semanticImageSurface == surface && q.semanticImageGeneration == q.imageLoadGen &&
		q.semanticImageSource != "" {
		return q.semanticImageSource, q.semanticImageWidth, q.semanticImageHeight
	}

	// ImageSurface stores straight-alpha RGBA8 pixels, which is exactly
	// image.NRGBA's memory contract. Wrap the source buffer without copying it:
	// a very large decoded image must not cause a second full-size allocation
	// merely to produce the bounded semantic thumbnail.
	source := &image.NRGBA{
		Pix:    surface.Pix,
		Stride: surface.Stride,
		Rect:   image.Rect(0, 0, surface.Width, surface.Height),
	}
	var encodedImage image.Image = source
	width, height := surface.Width, surface.Height
	maxDimension := max(width, height)
	if maxDimension > quickViewSemanticImageMaxDimension {
		width = max(1, width*quickViewSemanticImageMaxDimension/maxDimension)
		height = max(1, height*quickViewSemanticImageMaxDimension/maxDimension)
		destination := image.NewNRGBA(image.Rect(0, 0, width, height))
		xdraw.ApproxBiLinear.Scale(destination, destination.Bounds(), source,
			source.Bounds(), xdraw.Src, nil)
		encodedImage = destination
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, encodedImage); err != nil {
		q.clearSemanticImage()
		return "", 0, 0
	}
	q.semanticImageSurface = surface
	q.semanticImageGeneration = q.imageLoadGen
	q.semanticImageWidth = width
	q.semanticImageHeight = height
	q.semanticImageSource = "data:image/png;base64," +
		base64.StdEncoding.EncodeToString(encoded.Bytes())
	return q.semanticImageSource, width, height
}

func quickViewRows(lines []string) []extui.TextRowModel {
	rows := make([]extui.TextRowModel, 0, len(lines))
	for index, text := range lines {
		rows = append(rows, extui.TextRowModel{
			Index:     index,
			VisualRow: index,
			Offset:    int64(index),
			EndOffset: int64(index + 1),
			Text:      text,
		})
	}
	return rows
}

func (q *QuickViewPanel) semanticHeaderRows(item *fileEntry, innerW int) []extui.TextRowModel {
	if item == nil || item.IsDir {
		return nil
	}
	lines := []string{
		" " + item.Name,
		fmt.Sprintf(" %s: %s", Msg("QuickView.Size"), formatBytes(uint64(item.Size))),
	}
	if q.cacheReadErr != nil {
		lines = append(lines, "", " "+Msg("QuickView.ReadError")+": "+q.cacheReadErr.Error())
		return quickViewRows(lines)
	}
	status := ""
	switch {
	case q.cacheLoading:
		status = Msg("QuickView.Loading")
	case q.cacheLabel != "":
		status = q.cacheLabel
	case q.cacheImage:
		status = Msg("QuickView.Image")
	case q.cacheBinary:
		status = Msg("QuickView.Binary")
	}
	lines = append(lines, " "+status,
		" "+strings.Repeat("─", max(0, innerW-2)))
	return quickViewRows(lines)
}

func (q *QuickViewPanel) semanticDirectoryLines(item *fileEntry) []string {
	var lines []string
	q.renderDir(item, func(text string) { lines = append(lines, text) })
	return lines
}

func (q *QuickViewPanel) semanticWindowForLines(lines []string, viewportRows int) semanticSurfaceWindow {
	var window semanticSurfaceWindow
	viewportRows = max(1, viewportRows)
	maxTop := max(0, len(lines)-viewportRows)
	q.scrollY = max(0, min(q.scrollY, maxTop))
	bufferRows := semanticWindowBufferRows(viewportRows)
	windowStart := max(0, q.scrollY-bufferRows)
	windowEnd := min(len(lines), q.scrollY+viewportRows+bufferRows)
	window.start = int64(windowStart)
	window.end = int64(windowEnd)
	window.viewportRow = q.scrollY - windowStart
	window.viewportRows = viewportRows
	window.viewportSpan = int64(min(viewportRows, max(0, len(lines)-q.scrollY)))
	for visualRow := windowStart; visualRow < windowEnd; visualRow++ {
		text := lines[visualRow]
		if !q.wrap && q.scrollX > 0 {
			text = trimLeftCells(text, q.scrollX)
		}
		window.rows = append(window.rows, extui.TextRowModel{
			Index:     len(window.rows),
			VisualRow: visualRow,
			Offset:    int64(visualRow),
			EndOffset: int64(visualRow + 1),
			Text:      text,
		})
	}
	return window
}

// semanticModel exports Quick View as panel chrome plus a nested bounded
// document surface. It intentionally calls prepareSelection and builds the
// wrapped layout itself: native mode does not rasterise the covered panel.
func (q *QuickViewPanel) semanticModel(side, sourceSide int, active bool) extui.QuickViewModel {
	selection, selected := q.prepareSelection()
	innerW := max(1, q.X2-q.X1-1)
	id := vtui.SemanticID(q)
	model := extui.QuickViewModel{
		ID:          id,
		Side:        side,
		SourceSide:  sourceSide,
		Active:      active,
		Title:       Msg("QuickView.Title"),
		BottomHint:  Msg("InfoPanel.UnitsHint"),
		ContentKey:  q.semanticContentKey,
		PreviewKind: "empty",
		Wrap:        q.wrap,
	}

	var bodyLines []string
	viewportRows := max(1, q.fileViewportRows())
	if !selected {
		model.HeaderRows = quickViewRows([]string{" " + Msg("QuickView.NoSelection")})
	} else {
		item := selection.item
		model.Name = item.Name
		model.Path = selection.path
		model.SizeText = formatBytes(uint64(item.Size))
		model.Label = q.cacheLabel
		model.HeaderRows = q.semanticHeaderRows(item, innerW)

		switch {
		case q.cacheDir:
			model.PreviewKind = "directory"
			viewportRows = max(1, q.Y2-q.Y1-1)
			bodyLines = q.semanticDirectoryLines(item)
			q.scanMu.Lock()
			model.Loading = !q.scanDone && q.scanErr == nil
			if q.scanErr != nil {
				model.Error = q.scanErr.Error()
			}
			q.scanMu.Unlock()
		case q.cacheReadErr != nil:
			model.PreviewKind = "error"
			model.Error = q.cacheReadErr.Error()
		case q.cacheLoading:
			model.PreviewKind = "loading"
			model.Loading = true
		case q.cacheImage:
			model.PreviewKind = "image"
			model.ImageSource, model.ImageWidth, model.ImageHeight = q.semanticImageDataURL()
			model.Loading = model.ImageSource == ""
		case q.cacheBinary:
			model.PreviewKind = "hex"
			q.ensureDisplayLayout(innerW)
			bodyLines = q.displayLines
		default:
			model.PreviewKind = "text"
			q.ensureDisplayLayout(innerW)
			bodyLines = q.displayLines
		}
	}

	window := q.semanticWindowForLines(bodyLines, viewportRows)
	visibleEnd := min(window.viewportRow+window.viewportRows, len(window.rows))
	visibleRows := window.rows
	if window.viewportRow >= 0 && window.viewportRow <= visibleEnd {
		visibleRows = window.rows[window.viewportRow:visibleEnd]
	}
	model.Surface = extui.SurfaceModel{
		ID:                 id,
		Kind:               "quick_view",
		Title:              model.Title,
		Path:               model.Path,
		BaseName:           model.Name,
		Mode:               model.PreviewKind,
		Busy:               model.Loading,
		WrapMode:           q.wrap,
		ScrollLeft:         q.scrollX,
		DocumentKey:        q.semanticContentKey,
		ScrollAction:       "quickView.scroll",
		ScrollUnit:         "rows",
		WindowStart:        window.start,
		WindowEnd:          window.end,
		ViewportStart:      int64(q.scrollY),
		ViewportSpan:       window.viewportSpan,
		ContentExtent:      int64(len(bodyLines)),
		ContentExtentKnown: true,
		ViewportRow:        window.viewportRow,
		ViewportRows:       window.viewportRows,
		WindowGeneration:   q.semanticWindowGeneration,
		Rows:               visibleRows,
		WindowRows:         window.rows,
	}
	return model
}

// HandleSemanticAction accepts only a request for the currently mirrored
// content. The contentKey guard is what makes an in-flight touchpad request
// harmless when the source panel cursor switches to another file.
func (q *QuickViewPanel) HandleSemanticAction(action map[string]any) bool {
	if q == nil || semanticString(action["target"]) != vtui.SemanticID(q) ||
		semanticString(action["action"]) != "quickView.scroll" {
		return false
	}
	q.prepareSelection()
	if key := semanticString(action["contentKey"]); key == "" || key != q.semanticContentKey {
		return false
	}
	innerW := max(1, q.X2-q.X1-1)
	q.ensureDisplayLayout(innerW)
	viewportRows := max(1, q.fileViewportRows())
	contentRows := len(q.displayLines)
	if q.cacheDir {
		selection, ok := q.prepareSelection()
		if ok {
			contentRows = len(q.semanticDirectoryLines(selection.item))
			viewportRows = max(1, q.Y2-q.Y1-1)
		}
	}
	requested := semanticInt(action["visualRow"])
	q.scrollY = max(0, min(requested, max(0, contentRows-viewportRows)))
	q.semanticWindowGeneration++
	return true
}

// buildDisplayLines converts cacheLines into what should actually be
// on screen: with wrap on, long lines are re-flowed to fit innerW;
// with wrap off, we pass them through and rely on scrollX/right-clip
// at render time. Returns the display lines plus a parallel slice
// mapping each display line back to its source index so wrap toggle
// can pin the reading position.
func (q *QuickViewPanel) buildDisplayLines(innerW int) ([]string, []int) {
	if innerW <= 0 {
		return nil, nil
	}
	if !q.wrap {
		out := make([]string, len(q.cacheLines))
		copy(out, q.cacheLines)
		src := make([]int, len(q.cacheLines))
		for i := range src {
			src[i] = i
		}
		return out, src
	}
	var out []string
	var src []int
	for srcIdx, raw := range q.cacheLines {
		if raw == "" {
			out = append(out, "")
			src = append(src, srcIdx)
			continue
		}
		for len(raw) > 0 {
			cut := cellCut(raw, innerW)
			if cut == 0 { // guard against zero-width impossibility
				cut = len(raw)
			}
			out = append(out, raw[:cut])
			src = append(src, srcIdx)
			raw = raw[cut:]
		}
	}
	return out, src
}

// firstDisplayForSource returns the smallest i for which m[i]==src.
// If no line maps to src (out of range), returns 0.
func firstDisplayForSource(m []int, src int) int {
	for i, s := range m {
		if s == src {
			return i
		}
	}
	return 0
}

// cellCut finds the byte offset that keeps runewidth.StringWidth
// under width. Handles multibyte runes and double-width cells.
func cellCut(s string, width int) int {
	if width <= 0 || s == "" {
		return len(s)
	}
	used := 0
	for i := 0; i < len(s); {
		r, sz := utf8.DecodeRuneInString(s[i:])
		w := runewidth.RuneWidth(r)
		if used+w > width {
			return i
		}
		used += w
		i += sz
	}
	return len(s)
}

// trimLeftCells drops `cells` display columns from the front. Used
// for horizontal scroll (wrap = off).
func trimLeftCells(s string, cells int) string {
	dropped := 0
	for i := 0; i < len(s); {
		r, sz := utf8.DecodeRuneInString(s[i:])
		w := runewidth.RuneWidth(r)
		if dropped+w > cells {
			return s[i:]
		}
		dropped += w
		i += sz
	}
	return ""
}

// refreshCache starts a fresh preview for path. Best-effort errors are stored
// in cacheReadErr so rendering remains side-effect free.
func (q *QuickViewPanel) refreshCache(key quickViewSelectionKey, path string, item fileEntry) {
	q.cancelFilePreview()
	q.imageLoadGen++
	q.cacheKey = key
	q.cachePath = path
	q.cacheValid = true
	q.cacheDir = item.IsDir
	q.cacheBinary = false
	q.cacheImage = false
	q.cacheLoading = false
	q.cacheLabel = ""
	q.cacheRaw = nil
	q.cacheCodepage = 0
	q.cacheAutoDetect = false
	q.hexMode = false
	q.imageSurf = nil
	q.clearSemanticImage()
	q.cacheLines = nil
	q.cacheReadErr = nil
	q.lastSearchSource = -1
	q.updateFrameTitle()

	if item.IsDir {
		q.startDirScan(path)
		return
	}
	q.cancelScan()

	if IsImageFile(path) {
		q.cacheImage = true
		gen := q.imageLoadGen
		source := q.src.vfs

		if res, ok := ImagePipe.PreviewSync(context.Background(), source, path); ok {
			if res.Surface != nil && res.Surface.Valid() {
				q.imageSurf = res.Surface
			}
		}

		ImagePipe.Load(source, path, func(res ImageResult) {
			vtui.FrameManager.PostTask(func() {
				if q.imageLoadGen == gen && q.cacheValid && q.cacheKey == key {
					if res.Err != nil {
						q.cacheReadErr = res.Err
					} else if res.Surface != nil && res.Surface.Valid() {
						q.imageSurf = res.Surface
					}
					vtui.FrameManager.Redraw()
				}
			})
		})
		return
	}

	request := vfs.QuickViewRequest{VFS: q.src.vfs, Path: path, Item: item.VFSItem}
	providers := vfs.QuickViewProvidersFor(request)
	if len(providers) != 0 {
		q.startFilePreview(key, request, providers)
		return
	}

	q.applyFilePreview(loadDefaultQuickView(context.Background(), request.VFS, request.Path))
}

type quickViewFileResult struct {
	label      string
	lines      []string
	raw        []byte
	codepage   int
	autoDetect bool
	binary     bool
	err        error
}

func makeQuickViewSelectionKey(filesystem vfs.VFS, path string, item vfs.VFSItem) quickViewSelectionKey {
	return quickViewSelectionKey{
		source:   mediaSourceKey(filesystem, path),
		revision: item.Revision,
		size:     item.Size,
		mtimeNS:  item.MTime.UnixNano(),
		isDir:    item.IsDir,
	}
}

// startFilePreview tries matching providers in priority order away from the
// UI thread. Only an explicit ErrQuickViewUnsupported advances to the next
// provider; an actual parse/read error is useful information and is shown.
func (q *QuickViewPanel) startFilePreview(key quickViewSelectionKey, request vfs.QuickViewRequest, providers []vfs.QuickViewProvider) {
	ctx, cancel := context.WithCancel(context.Background())
	q.previewCancel = cancel
	gen := q.previewGen
	q.cacheLoading = true

	// Read on the goroutine that starts this work, not inside it: the
	// work outlives the call, and reading the global from it races
	// anything that reassigns vtui.FrameManager meanwhile.
	frames := vtui.FrameManager
	go func() {
		var loaded quickViewFileResult
		handled := false
		for _, provider := range providers {
			result, err := provider.Preview(ctx, request)
			if ctx.Err() != nil {
				return
			}
			if errors.Is(err, vfs.ErrQuickViewUnsupported) {
				continue
			}
			handled = true
			if err != nil {
				loaded.err = fmt.Errorf("%s: %w", provider.Name(), err)
			} else {
				loaded.label = result.Label
				loaded.lines = append([]string(nil), result.Lines...)
			}
			break
		}
		if !handled {
			loaded = loadDefaultQuickView(ctx, request.VFS, request.Path)
		}
		if ctx.Err() != nil {
			return
		}

		frames.PostTask(func() {
			if q.previewGen != gen || !q.cacheValid || q.cacheKey != key {
				return
			}
			q.previewCancel = nil
			q.applyFilePreview(loaded)
			frames.HardRefresh()
		})
	}()
}

func (q *QuickViewPanel) cancelFilePreview() {
	if q.previewCancel != nil {
		q.previewCancel()
		q.previewCancel = nil
	}
	q.previewGen++
}

func (q *QuickViewPanel) applyFilePreview(result quickViewFileResult) {
	q.cacheLoading = false
	q.cacheLabel = result.label
	q.cacheBinary = result.binary
	q.cacheRaw = append(q.cacheRaw[:0], result.raw...)
	q.cacheCodepage = result.codepage
	q.cacheAutoDetect = result.autoDetect
	q.cacheLines = append(q.cacheLines[:0], result.lines...)
	q.cacheReadErr = result.err
	q.hexMode = result.binary
	if remembered, ok := q.rememberedCodepage(); ok && q.cacheRaw != nil {
		q.applyPreviewCodepage(remembered, false)
	}
	q.displayLines = nil
	q.displayToSource = nil
	q.updateFrameTitle()
}

// loadDefaultQuickView preserves the existing 16 KiB/500 ms text-or-hex
// fallback. It can run synchronously for unmatched files or in the provider
// worker after every specialized provider declined the file.
func loadDefaultQuickView(parent context.Context, filesystem vfs.VFS, path string) quickViewFileResult {
	ctx, cancel := context.WithTimeout(parent, 500*time.Millisecond)
	defer cancel()
	rc, err := filesystem.Open(ctx, path)
	if err != nil {
		return quickViewFileResult{err: err}
	}
	defer rc.Close()
	buf := make([]byte, previewMax)
	n, readErr := rc.ReadAt(ctx, buf, 0)
	if readErr != nil && readErr != io.EOF {
		return quickViewFileResult{err: readErr}
	}
	buf = buf[:n]

	autoDetect := AppConfig.ViewerAutodetectCodePage
	cpID := vfs.DetectEncoding(buf, autoDetect, AppConfig.ViewerDefaultCodePage)
	decodedBuf := buf
	if cpID != 65001 {
		if decoded, decodeErr := vfs.DecodeBytes(buf, cpID); decodeErr == nil {
			decodedBuf = decoded
		}
	}

	if looksBinary(decodedBuf) {
		return quickViewFileResult{raw: append([]byte{}, buf...), codepage: cpID, autoDetect: autoDetect, binary: true, lines: hexDumpLines(buf)}
	}
	decodedBuf = vfs.StripUTF8BOM(decodedBuf)
	return quickViewFileResult{
		raw:        append([]byte{}, buf...),
		codepage:   cpID,
		autoDetect: autoDetect,
		lines:      quickViewTextLines(decodedBuf),
	}
}

const previewMax = 16 * 1024

// looksBinary returns true if the buffer contains a NUL byte or an
// unusually high proportion of non-printable / non-UTF-8 sequences.
// Simple heuristic — same shape as Far/far2l's viewer classification.
func looksBinary(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	if !utf8.Valid(b) {
		return true
	}
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return false
}

func hexDumpLines(b []byte) []string {
	const perLine = 16
	var out []string
	for off := 0; off < len(b); off += perLine {
		end := off + perLine
		if end > len(b) {
			end = len(b)
		}
		row := b[off:end]
		hex := make([]byte, 0, perLine*3)
		ascii := make([]byte, 0, perLine)
		for i := 0; i < perLine; i++ {
			if i < len(row) {
				hex = append(hex, hexNibble(row[i]>>4), hexNibble(row[i]&0xF))
			} else {
				hex = append(hex, ' ', ' ')
			}
			hex = append(hex, ' ')
			if i < len(row) {
				c := row[i]
				if c < 32 || c == 127 {
					c = '.'
				}
				ascii = append(ascii, c)
			}
		}
		out = append(out, fmt.Sprintf(" %08X  %s  %s", off, hex, ascii))
	}
	return out
}

func hexNibble(n byte) byte {
	if n < 10 {
		return '0' + n
	}
	return 'A' + (n - 10)
}

var _ AltPanel = (*QuickViewPanel)(nil)
