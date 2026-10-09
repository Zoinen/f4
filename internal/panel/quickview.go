package panel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/media"
	"github.com/unxed/f4/internal/numeric"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/f4/internal/wheel"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
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
	Frame   *vtui.BorderedFrame
	Focused bool

	// wheelCoast is what a fast wheel spin leaves behind: lines the preview
	// still owes the scroll position (see internal/wheel).
	wheelCoast wheel.Coast

	// Cache the last-computed preview so we don't re-read the file /
	// re-scan the directory on every redraw.
	CacheKey        quickViewSelectionKey
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
	Wrap    bool
	ScrollY int
	scrollX int
	hexMode bool
	// colorizer colours the text shown, started for colorizerKey; see
	// syntaxColors.
	colorizer        viewer.TextColorizer
	colorizerKey     quickViewColorKey
	colorizerStarted bool
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

	// The line cursor and the marks Ins / Shift+arrows set, as in the info
	// panel (#1804). cursorY is a display line; marks are source lines, so
	// a mark covers every row a wrapped line takes and survives F2.
	// followCursor asks the next Show to scroll the cursor into view; the
	// wheel leaves it unset and the cursor is dragged along instead.
	cursorY      int
	followCursor bool
	marks        map[int]bool
	// The folder summary has its own cursor and marks over its
	// label/value rows (quickview_dir.go). Marks are keyed by label: rows
	// such as Physical size appear while the scan runs, shifting indices.
	dirRows   []quickViewDirRow
	dirCursor int
	dirMarks  map[string]bool
}

// quickViewSelectionKey prevents identical-looking paths from different VFS
// sessions sharing a preview. Revision is preferred when a provider supplies
// one; size and mtime keep ordinary files responsive to a refreshed listing.
type quickViewSelectionKey struct {
	source   dirCacheKey
	revision string
	size     int64
	mtimeNS  int64
	isDir    bool
}

// NewQuickViewPanel creates a quick-view panel over src's slot.
func NewQuickViewPanel(src *FileSystemPanel) *QuickViewPanel {
	x1, y1, x2, y2 := src.GetPosition()
	q := &QuickViewPanel{src: src, Wrap: true, lastSearchSource: -1, codepages: make(map[quickViewSelectionKey]int)}
	q.SetVisible(true)
	q.Frame = vtui.NewBorderedFrame(x1, y1, x2, y2, vtui.SingleBox, i18n.Msg("QuickView.Title"))
	q.Frame.ColorBoxIdx = theme.ColPanelBox
	q.Frame.ColorTitleIdx = theme.ColPanelTitle
	q.Frame.ColorBackgroundIdx = theme.ColPanelInfoText
	q.gfxKey = fmt.Sprintf("f4.quickview:%p", q)
	q.SetPosition(x1, y1, x2, y2)
	return q
}

func (q *QuickViewPanel) SetPosition(x1, y1, x2, y2 int) {
	q.ScreenObject.SetPosition(x1, y1, x2, y2)
	if q.Frame != nil {
		q.Frame.SetPosition(x1, y1, x2, y2)
	}
}

func (q *QuickViewPanel) Source() *FileSystemPanel { return q.src }
func (q *QuickViewPanel) Kind() string             { return "quick_view" }

// SetFocus tracks the focused marker (title recolour). When focused
// the panel starts consuming navigation keys — see ProcessKey.
func (q *QuickViewPanel) SetFocus(f bool) {
	q.Focused = f
	if q.Frame != nil {
		if f {
			q.Frame.ColorTitleIdx = theme.ColPanelSelectedTitle
		} else {
			q.Frame.ColorTitleIdx = theme.ColPanelTitle
		}
	}
}
func (q *QuickViewPanel) IsFocused() bool { return q.Focused }

// ProcessKey handles cursor, marking, copy and wrap-toggle keys while
// focused. The arrows move a line cursor the view follows; Shift+arrows and
// Ins mark lines the way the info panel marks rows, and C copies them. Any
// key we don't recognise falls through (return false), letting the global
// handler chain deal with Ctrl+Q close, Tab, B etc.
func (q *QuickViewPanel) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown || !q.Focused {
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
		if shift {
			q.toggleMarkAtCursor()
			q.moveCursorToSource(-1)
		} else {
			q.moveCursor(-1)
		}
	case vtinput.VK_DOWN:
		if shift {
			q.toggleMarkAtCursor()
			q.moveCursorToSource(+1)
		} else {
			q.moveCursor(+1)
		}
	case vtinput.VK_PRIOR: // PgUp
		if shift {
			q.toggleMarkAtCursor()
		}
		q.moveCursor(-q.textPageHeight())
	case vtinput.VK_NEXT: // PgDn
		if shift {
			q.toggleMarkAtCursor()
		}
		q.moveCursor(q.textPageHeight())
	case vtinput.VK_HOME:
		if shift {
			q.toggleMarkAtCursor()
		}
		q.moveCursor(-(1 << 30))
		q.scrollX = 0
	case vtinput.VK_END:
		if shift {
			q.toggleMarkAtCursor()
		}
		q.moveCursor(1 << 30)
	case vtinput.VK_INSERT:
		if shift {
			return false
		}
		q.toggleMarkAtCursor()
		q.moveCursorToSource(+1)
	case vtinput.VK_C:
		if shift {
			return false
		}
		q.copyCurrent()
		return true
	case vtinput.VK_LEFT:
		if !q.Wrap {
			q.scrollX--
		}
	case vtinput.VK_RIGHT:
		if !q.Wrap {
			q.scrollX++
		}
	case vtinput.VK_F2:
		// Pin the currently-visible source line before re-flowing so
		// the user's reading position doesn't scroll away. Falls back
		// to 0 if we haven't computed a mapping yet.
		pinnedSrc := 0
		if q.ScrollY >= 0 && q.ScrollY < len(q.displayToSource) {
			pinnedSrc = q.displayToSource[q.ScrollY]
		}
		q.Wrap = !q.Wrap
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
	if q.ScrollY < 0 {
		q.ScrollY = 0
	}
	vtui.FrameManager.HardRefresh()
	return true
}

// moveCursor shifts the line cursor by delta display lines, clamped to the
// text, and has the next Show bring it into view.
func (q *QuickViewPanel) moveCursor(delta int) {
	if q.cacheDir {
		q.moveDirCursor(delta)
		return
	}
	n := len(q.displayLines)
	if n == 0 {
		q.cursorY = 0
		return
	}
	q.cursorY = min(max(q.cursorY+delta, 0), n-1)
	q.followCursor = true
}

// moveCursorToSource steps to the first display line of the next (+1) or
// previous (-1) source line, so marking a wrapped line moves past all of it.
func (q *QuickViewPanel) moveCursorToSource(direction int) {
	if q.cacheDir {
		q.moveDirCursor(direction)
		return
	}
	src, ok := q.cursorSource()
	if !ok {
		return
	}
	target := src + direction
	if target < 0 || target >= len(q.cacheLines) {
		return
	}
	q.cursorY = firstDisplayForSource(q.displayToSource, target)
	q.followCursor = true
}

// cursorSource is the source line under the cursor.
func (q *QuickViewPanel) cursorSource() (int, bool) {
	if !q.hasTextLines() || q.cursorY < 0 || q.cursorY >= len(q.displayToSource) {
		return 0, false
	}
	return q.displayToSource[q.cursorY], true
}

// hasTextLines reports whether the preview is lines a cursor can walk:
// text, hex rows or a provider's report — not a folder or a picture.
func (q *QuickViewPanel) hasTextLines() bool {
	return q.cacheValid && !q.cacheDir && !q.cacheImage && !q.cacheLoading && q.cacheReadErr == nil && len(q.cacheLines) > 0
}

func (q *QuickViewPanel) toggleMarkAtCursor() {
	if q.cacheDir {
		q.toggleDirMark()
		return
	}
	src, ok := q.cursorSource()
	if !ok {
		return
	}
	if q.marks[src] {
		delete(q.marks, src)
		return
	}
	if q.marks == nil {
		q.marks = make(map[int]bool)
	}
	q.marks[src] = true
}

// clearMarks drops the marks and the cursor along with the text they point
// into: a new file, the hex toggle or another codepage.
func (q *QuickViewPanel) clearMarks() {
	q.marks = nil
	q.cursorY = 0
	q.dirMarks = nil
	q.dirCursor = -1
}

// copyCurrent is C. Marked lines are copied in file order, one per line;
// with none marked, the line under the cursor. A folder copies its rows the
// way the info panel does, a picture goes to the clipboard as an image.
func (q *QuickViewPanel) copyCurrent() {
	switch {
	case !q.cacheValid || q.cacheLoading:
		return
	case q.cacheDir:
		q.copyDirRows()
	case q.cacheImage:
		q.copyImage()
	case len(q.marks) > 0:
		var lines []string
		for i, line := range q.cacheLines {
			if q.marks[i] {
				lines = append(lines, line)
			}
		}
		q.copyText(strings.Join(lines, "\n"), len(lines))
	default:
		if src, ok := q.cursorSource(); ok {
			q.copyText(q.cacheLines[src], 0)
		}
	}
}

// copyText puts text on the clipboard; rows > 0 reports a count instead of
// echoing the text, as the info panel does for its marked rows.
func (q *QuickViewPanel) copyText(text string, rows int) {
	if text == "" {
		return
	}
	terminal.SetF4Clipboard(text)
	if rows > 0 {
		toast.Show(fmt.Sprintf("%s: %d", i18n.Msg("InfoPanel.CopiedRows"), rows), 2*time.Second)
		return
	}
	shown := strings.ReplaceAll(text, "\n", " ")
	toast.Show(fmt.Sprintf("%s: %s", i18n.Msg("InfoPanel.Copied"), runewidth.Truncate(shown, 60, "…")), 2*time.Second)
}

// copyImage decodes the full picture away from the UI goroutine and puts it
// on the clipboard as PNG. Where no image clipboard exists (a TTY over SSH)
// the path is copied as text instead, so C still yields something useful.
func (q *QuickViewPanel) copyImage() {
	path, source, frames := q.cachePath, q.src.Vfs, vtui.FrameManager
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := quickViewCopyImage(ctx, source, path)
		frames.PostTask(func() {
			if err == nil {
				toast.Show(i18n.Msg("QuickView.ImageCopied"), 2*time.Second)
				return
			}
			vtui.DebugLog("quick view: image clipboard: %v", err)
			terminal.SetF4Clipboard(path)
			toast.Show(fmt.Sprintf("%s: %s", i18n.Msg("QuickView.ImagePathCopied"), path), 3*time.Second)
		})
	}()
}

// quickViewCopyImage is the seam tests replace to keep the system clipboard
// out of their way.
var quickViewCopyImage = func(ctx context.Context, source vfs.VFS, path string) error {
	surf, _, err := media.LoadImage(ctx, source, path)
	if err != nil {
		return err
	}
	if surf == nil || !surf.Valid() {
		return terminal.ErrImageClipboardUnavailable
	}
	var buf bytes.Buffer
	if err := media.EncodeClipboardImage(&buf, surf.ToRGBA(), "png", "speed", 0); err != nil {
		return err
	}
	return terminal.SetImageClipboard(ctx, buf.Bytes())
}

func (q *QuickViewPanel) selectedFile() (string, *FileEntry, bool) {
	if q.src == nil || q.src.Vfs == nil {
		return "", nil, false
	}
	idx := q.src.GetCursorIndex()
	if idx < 0 || idx >= len(q.src.Entries) {
		return "", nil, false
	}
	item := q.src.Entries[idx]
	if item == nil || item.IsDir || item.Name == ".." {
		return "", nil, false
	}
	return q.src.Vfs.Join(q.src.Vfs.GetPath(), item.Name), item, true
}

func (q *QuickViewPanel) openSelectedInViewer() bool {
	path, _, ok := q.selectedFile()
	if !ok || q.src == nil || q.src.Vfs == nil {
		return false
	}
	if pf := FindPanelsFrameAnyScreen(); pf != nil {
		OpenViewerInternal(pf, q.src.Vfs, path)
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
	q.cacheBinary = viewer.LooksBinary(decoded)
	if q.hexMode {
		q.cacheLines = hexDumpLines(q.cacheRaw)
	} else {
		q.cacheLines = quickViewTextLines(decoded)
	}
	q.clearMarks()
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
	q.codepages[q.CacheKey] = cpID
	q.persistCodepage(cpID)
	vtui.FrameManager.HardRefresh()
	return true
}

func (q *QuickViewPanel) persistCodepage(cpID int) {
	if fileops.GlobalFileState == nil || q.src == nil || q.src.Vfs == nil || q.cachePath == "" {
		return
	}
	fileops.GlobalFileState.SaveQuickViewCodepageAsync(fileops.FileStateKey(q.src.Vfs, q.cachePath), cpID)
}

func (q *QuickViewPanel) rememberedCodepage() (int, bool) {
	if cpID, ok := q.codepages[q.CacheKey]; ok {
		return cpID, true
	}
	if fileops.GlobalFileState == nil || q.src == nil || q.src.Vfs == nil || q.cachePath == "" {
		return 0, false
	}
	state := fileops.GlobalFileState.GetState(fileops.FileStateKey(q.src.Vfs, q.cachePath))
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
	menu := vtui.NewVMenu(i18n.Msg("Codepage.Title"))
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
			delete(q.codepages, q.CacheKey)
			q.persistCodepage(0)
			q.applyPreviewCodepage(vfs.DetectEncoding(q.cacheRaw, config.App.ViewerAutodetectCodePage, config.App.ViewerDefaultCodePage), true)
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
	q.clearMarks()
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
	vtui.InputBox(i18n.Msg("Viewer.SearchTitle"), "Search for:", q.lastSearch, func(pattern string) {
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
	q.displayWrap = q.Wrap
	q.displayWidth = innerW
	q.ScrollY = firstDisplayForSource(q.displayToSource, source)
	q.cursorY = q.ScrollY
	q.hasPin = false
}

func (q *QuickViewPanel) updateFrameTitle() {
	if q.Frame == nil {
		return
	}
	title := i18n.Msg("QuickView.Title")
	if !q.cacheDir && q.cacheCodepage > 0 {
		title = fmt.Sprintf("%s │ %s", title, vfs.DisplayCodepageName(q.cacheCodepage))
	}
	q.Frame.SetTitle(title)
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
	direction := 1
	if e.WheelDirection > 0 {
		direction = -1
	}
	// A spin faster than one notch per spin window queues extra lines the
	// preview keeps scrolling on its own (see internal/wheel).
	q.wheelCoast.Notch(direction, q.scrollWheelBy)
	q.scrollWheelBy(direction * 3)
	vtui.FrameManager.HardRefresh()
	return true
}

// scrollWheelBy moves the preview by step lines, positive down the file,
// and reports whether anything moved so a coast stops at an end of the
// preview instead of spinning in place.
func (q *QuickViewPanel) scrollWheelBy(step int) bool {
	before := q.ScrollY
	q.ScrollY += step
	if q.ScrollY < 0 {
		q.ScrollY = 0
	}
	return q.ScrollY != before
}

func (q *QuickViewPanel) GetSelectedName() string {
	if q.src == nil {
		return ""
	}
	return q.src.GetSelectedName()
}

// textPageHeight is how many text rows fit under the two-line header.
func (q *QuickViewPanel) textPageHeight() int {
	return max(q.pageHeight()-2, 1)
}

func (q *QuickViewPanel) pageHeight() int {
	h := q.Y2 - q.Y1 - 1 // room between borders
	if h < 1 {
		return 1
	}
	return h
}

func (q *QuickViewPanel) Show(scr *vtui.ScreenBuf) {
	if q.Frame != nil {
		q.Frame.Show(scr)
	}
	// Bottom-border hint reminding the user of the units toggle, same
	// pattern InfoPanel uses (same string too — the toggle behaves
	// identically). Drawn always while the panel is up because B
	// affects both the "Files size" number for directories and the
	// header "Size" for files.
	if q.Frame != nil && q.Y2 > q.Y1+1 {
		hint := i18n.Msg("InfoPanel.UnitsHint")
		if runewidth.StringWidth(hint) < q.X2-q.X1-1 {
			attrBox := vtui.Palette[theme.ColPanelBox]
			scr.Write(q.X1+2, q.Y2, vtui.StringToCharInfo(hint, attrBox))
		}
	}
	innerW := q.X2 - q.X1 - 1
	if innerW < 1 || q.src == nil {
		return
	}
	attr := vtui.Palette[theme.ColPanelInfoText]
	y := q.Y1 + 1
	maxY := q.Y2 - 1

	writeLineAttr := func(s string, attr uint64) {
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
	writeLine := func(s string) { writeLineAttr(s, attr) }

	writeColored := func(s string, runeStart int, attrs []uint64) {
		if y > maxY {
			return
		}
		scr.Write(q.X1+1, y, quickViewColoredCells(s, runeStart, attrs, attr, innerW))
		y++
	}

	idx := q.src.GetCursorIndex()
	if idx < 0 || idx >= len(q.src.Entries) {
		q.cancelScan()
		q.cancelFilePreview()
		q.imageLoadGen++
		q.cacheValid = false
		q.cacheDir = false
		q.cacheCodepage = 0
		q.updateFrameTitle()
		writeLine(" " + i18n.Msg("QuickView.NoSelection"))
		return
	}
	item := q.src.Entries[idx]

	// On "..", far2/far2l scan the CURRENT directory (parent of the
	// listing) rather than showing a static "Parent directory" note.
	// We synthesize a FileEntry that points at the current dir and
	// funnel it through the same refreshCache path as regular items.
	// The header shows the full path so it's unambiguous even when
	// several panels sit in similarly-named leaf folders.
	var path string
	if item.Name == ".." {
		path = q.src.Vfs.GetPath()
		synth := FileEntry{VFSItem: vfs.VFSItem{Name: path, IsDir: true}}
		item = &synth
	} else {
		path = q.src.Vfs.Join(q.src.Vfs.GetPath(), item.Name)
	}
	key := makeQuickViewSelectionKey(q.src.Vfs, path, item.VFSItem)
	if !q.cacheValid || key != q.CacheKey {
		q.refreshCache(key, path, *item)
		q.ScrollY = 0
		q.scrollX = 0
		q.displayLines = nil
	}

	if q.cacheDir {
		q.renderDir(item, writeLineAttr, attr)
		return
	}
	q.renderFile(item, innerW, writeLine, writeLineAttr, writeColored, attr, scr)

	// Vertical scrollbar over the right border. Repaints column X2
	// with scrollbar glyphs, so if a wide content line ever bled
	// into the border position it gets restored. Skipped when the
	// content fits entirely (DrawScrollBar returns false).
	if q.Y2 > q.Y1+1 && len(q.displayLines) > 0 {
		vtui.DrawScrollBar(scr, q.X2, q.Y1+1, q.Y2-q.Y1-1,
			q.ScrollY, len(q.displayLines), vtui.Palette[theme.ColPanelScrollbar])
	}
}

func (q *QuickViewPanel) renderDir(item *FileEntry, writeLineAttr func(string, uint64), attr uint64) {
	q.dirRows = q.dirSummary(item.Name)
	q.settleDirCursor()
	for i, row := range q.dirRows {
		writeLineAttr(row.text, q.dirRowAttr(i, attr))
	}
}

// dirSummary is the folder report renderDir draws and C copies.
func (q *QuickViewPanel) dirSummary(name string) []quickViewDirRow {
	var rows []quickViewDirRow
	writeLine := func(s string) { rows = append(rows, quickViewDirRow{text: s}) }
	field := func(label, value string) {
		rows = append(rows, quickViewDirRow{label: label, value: value, text: fmt.Sprintf(" %-14s %s", label, value)})
	}
	rows = append(rows, quickViewDirRow{
		label: i18n.Msg("QuickView.Folder"), value: name,
		text: " " + i18n.Msg("QuickView.Folder") + " \"" + name + "\"",
	})
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

	writeLine(" " + i18n.Msg("QuickView.Contains") + ":")
	writeLine("")
	field(i18n.Msg("QuickView.FolderCount"), fmt.Sprint(dirs))
	field(i18n.Msg("QuickView.FileCount"), fmt.Sprint(stats.Files))
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
	field(i18n.Msg("QuickView.FilesSize"), formatBytes(numeric.NonNegativeUint64(logical)))
	// Physical size + Ratio need per-item on-disk footprint. Stub /
	// remote VFSes leave PhysicalBytes at 0 during the whole scan —
	// hide the rows in that case. Ratio is also hidden when it would
	// just read "100%" — on Unix that's every uncompressed tree, and
	// a constant carries no information for the reader.
	if stats.PhysicalBytes > 0 {
		field(i18n.Msg("QuickView.PhysicalSize"), formatBytes(numeric.NonNegativeUint64(stats.PhysicalBytes)))
		if stats.PhysicalBytes < logical {
			// Ratio interpretation matches far/far2l — >100% means "on
			// disk it takes less than the logical size", i.e. real
			// NTFS compression / sparse regions.
			ratio := int((logical * 100) / stats.PhysicalBytes)
			field(i18n.Msg("QuickView.Ratio"), fmt.Sprintf("%d%%", ratio))
		}
	}
	// Cluster size stands on its own — shown even when PhysicalBytes
	// couldn't be filled (VFS without per-item support).
	if cluster > 0 {
		writeLine("")
		field(i18n.Msg("QuickView.ClusterSize"), formatBytes(cluster))
	}
	// Single "scanning" hint per far2l — one trailing line at the
	// bottom, not repeated on every row.
	if !done && serr == nil {
		writeLine("")
		writeLine(" " + i18n.Msg("QuickView.Scanning"))
	}
	if serr != nil {
		writeLine("")
		writeLine(" " + i18n.Msg("QuickView.ReadError") + ": " + serr.Error())
	}
	return rows
}

// startDirScan cancels any running scan and kicks off a fresh async
// vfs.CalculateStats for the given directory. Progress lands in
// scanStats under scanMu; the UI is nudged via HardRefresh no more
// than every 200ms while the scan runs. On completion the final stats
// (plus scanErr if any) are latched and scanDone becomes true.
// sysinfo.FsInfo (statfs / GetDiskFreeSpace) is done inside the goroutine —
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
	source := q.src.Vfs
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
		// sysinfo.FS() is a syscall that may block on stuck network
		// mounts; do it here, off the UI thread. Cluster size is a
		// display-only field.
		if fs, ok := sysinfo.FS(fullPath); ok {
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
	q.closeColorizer()
	q.cancelScan()
	q.cancelFilePreview()
	q.imageLoadGen++
	q.cacheValid = false
}

func (q *QuickViewPanel) renderFile(item *FileEntry, innerW int, writeLine func(string), writeLineAttr func(string, uint64), writeColored func(string, int, []uint64), attr uint64, scr *vtui.ScreenBuf) {
	if q.cacheReadErr != nil {
		writeLine(" " + i18n.Msg("QuickView.ReadError") + ": " + q.cacheReadErr.Error())
		return
	}
	// The file name and size are already visible in the source panel. Keep
	// Quick View's header for the mode/encoding only, so the codepage remains
	// visible even for long names and never shifts with the panel selection.
	if q.cacheLoading {
		writeLine(" " + i18n.Msg("QuickView.Loading"))
	} else if q.cacheLabel != "" {
		writeLine(" " + q.cacheLabel)
	} else if q.cacheImage {
		writeLine(" " + i18n.Msg("QuickView.Image"))
	} else if q.hexMode {
		writeLine(" " + i18n.Msg("QuickView.Binary"))
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

	// Re-flow if wrap flag / innerW changed.
	if q.displayLines == nil || q.displayWrap != q.Wrap || q.displayWidth != innerW {
		cursorSrc, keepCursor := q.cursorSource()
		q.displayLines, q.displayToSource = q.buildDisplayLines(innerW)
		if keepCursor {
			q.cursorY = firstDisplayForSource(q.displayToSource, cursorSrc)
		}
		q.displayWrap = q.Wrap
		q.displayWidth = innerW
		if q.hasPin {
			q.ScrollY = firstDisplayForSource(q.displayToSource, q.pinSourceOnNextShow)
			q.hasPin = false
		}
	}

	// Clamp scroll offsets against fresh display.
	viewH := (q.Y2 - 1) - (q.Y1 + 1 + 2) + 1 // rows left after the 2-line header
	if viewH < 0 {
		viewH = 0
	}
	maxScroll := len(q.displayLines) - viewH
	if maxScroll < 0 {
		maxScroll = 0
	}
	q.cursorY = min(max(q.cursorY, 0), max(len(q.displayLines)-1, 0))
	if q.followCursor && viewH > 0 {
		if q.cursorY < q.ScrollY {
			q.ScrollY = q.cursorY
		} else if q.cursorY >= q.ScrollY+viewH {
			q.ScrollY = q.cursorY - viewH + 1
		}
	}
	q.followCursor = false
	if q.ScrollY > maxScroll {
		q.ScrollY = maxScroll
	}
	// The wheel scrolled without the cursor: drag it along.
	if viewH > 0 {
		q.cursorY = min(max(q.cursorY, q.ScrollY), max(q.ScrollY+viewH-1, 0))
	}

	// Emit visible slice with optional horizontal shift.
	end := q.ScrollY + viewH
	if end > len(q.displayLines) {
		end = len(q.displayLines)
	}
	colors := q.syntaxColors(attr)
	for i := q.ScrollY; i < end; i++ {
		line := q.displayLines[i]
		skipped := 0
		if !q.Wrap && q.scrollX > 0 {
			trimmed := trimLeftCells(line, q.scrollX)
			skipped = len(line) - len(trimmed)
			line = trimmed
		}
		if lineAttr, special := q.lineAttr(i); special {
			writeLineAttr(line, lineAttr)
			continue
		}
		if colors != nil && i < len(q.displayToSource) {
			if attrs := colors.LineAttrs(q.displayToSource[i]); attrs != nil {
				writeColored(line, q.displayRuneOffset(i)+utf8.RuneCountInString(q.displayLines[i][:skipped]), attrs)
				continue
			}
		}
		writeLine(line)
	}
}

// lineAttr is the colour of display line i when the cursor or a mark sits
// on it, in the info panel's order: marked, cursor, cursor on a mark.
func (q *QuickViewPanel) lineAttr(i int) (uint64, bool) {
	marked := i < len(q.displayToSource) && q.marks[q.displayToSource[i]]
	cursor := q.Focused && i == q.cursorY
	switch {
	case cursor && marked:
		return vtui.Palette[theme.ColPanelSelectedCursor], true
	case cursor:
		return vtui.Palette[theme.ColPanelCursor], true
	case marked:
		return vtui.Palette[theme.ColPanelSelectedText], true
	}
	return 0, false
}

// quickViewColorKey identifies the text a colorizer was started for.
type quickViewColorKey struct {
	path     string
	codepage int
	lines    int
	sum      uint64
}

// syntaxColors is the colorizer for the text the quick view shows: started
// when the text changes, stopped when it goes away or is not text.
func (q *QuickViewPanel) syntaxColors(base uint64) viewer.TextColorizer {
	if viewer.NewTextColorizer == nil || q.hexMode || q.cacheBinary || q.cacheImage || q.cacheLabel != "" || len(q.cacheLines) == 0 {
		q.closeColorizer()
		return nil
	}
	h := fnv.New64a()
	for _, line := range q.cacheLines {
		_, _ = h.Write([]byte(line))
		_, _ = h.Write([]byte{'\n'})
	}
	key := quickViewColorKey{path: q.cachePath, codepage: q.cacheCodepage, lines: len(q.cacheLines), sum: h.Sum64()}
	if !q.colorizerStarted || q.colorizerKey != key {
		q.closeColorizer()
		q.colorizer = viewer.NewTextColorizer(q.cachePath, q.cacheLines, base, true, func() {
			if vtui.FrameManager != nil {
				vtui.FrameManager.Redraw()
			}
		})
		q.colorizerKey, q.colorizerStarted = key, true
	}
	return q.colorizer
}

func (q *QuickViewPanel) closeColorizer() {
	if q.colorizer != nil {
		q.colorizer.Close()
	}
	q.colorizer = nil
	q.colorizerStarted = false
}

// displayRuneOffset is where display line i starts within its source line, in
// runes: wrapping cuts a source line into consecutive display lines.
func (q *QuickViewPanel) displayRuneOffset(i int) int {
	if !q.Wrap || i >= len(q.displayToSource) {
		return 0
	}
	n := 0
	for j := i - 1; j >= 0 && q.displayToSource[j] == q.displayToSource[i]; j-- {
		n += utf8.RuneCountInString(q.displayLines[j])
	}
	return n
}

// quickViewColoredCells lays out a display line whose runes, from runeStart
// on, take their colours from attrs, runes past attrs taking base. Runs of
// one colour are laid out together, so a character and its combining marks
// stay one cell. The row is cut to width with an ellipsis and padded, as
// Show's plain rows are.
func quickViewColoredCells(line string, runeStart int, attrs []uint64, base uint64, width int) []vtui.CharInfo {
	var cells []vtui.CharInfo
	runeIdx := runeStart
	runStart, runAttr := 0, uint64(0)
	flush := func(end int) {
		if end > runStart {
			cells = append(cells, vtui.StringToCharInfo(line[runStart:end], runAttr)...)
		}
		runStart = end
	}
	for i := range line {
		a := base
		if runeIdx >= 0 && runeIdx < len(attrs) {
			a = attrs[runeIdx]
		}
		if i == 0 {
			runAttr = a
		} else if a != runAttr {
			flush(i)
			runAttr = a
		}
		runeIdx++
	}
	flush(len(line))
	if len(cells) > width {
		cells = append(cells[:max(width-1, 0)], vtui.StringToCharInfo("…", base)...)
		if len(cells) > width {
			cells = cells[:width]
		}
	}
	for len(cells) < width {
		cells = append(cells, vtui.CharInfo{Char: ' ', Attributes: base})
	}
	return cells
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
		cw, ch = media.ImageViewFallbackCellW, media.ImageViewFallbackCellH
	}

	boxW := cols * cw
	boxH := rows * ch

	fitW, fitH := vtui.FitInside(q.imageSurf.Width, q.imageSurf.Height, boxW, boxH)
	if fitW <= 0 || fitH <= 0 {
		return
	}

	p := vtui.ImagePlacement{Surface: q.imageSurf}
	p.Cols, p.Rows = media.CellsFor(fitW, cw, cols), media.CellsFor(fitH, ch, rows)
	p.Col = x1 + 1 + (cols-p.Cols)/2
	p.Row = top + (rows-p.Rows)/2
	p.SrcX, p.SrcY = 0, 0
	p.SrcW, p.SrcH = q.imageSurf.Width, q.imageSurf.Height
	p.ZIndex = -1 // Keep picture below panel borders if they overlap

	scr.Graphics().DrawImage(q.gfxKey, p)
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
	if !q.Wrap {
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
func (q *QuickViewPanel) refreshCache(key quickViewSelectionKey, path string, item FileEntry) {
	q.cancelFilePreview()
	q.imageLoadGen++
	q.CacheKey = key
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
	q.cacheLines = nil
	q.cacheReadErr = nil
	q.lastSearchSource = -1
	q.clearMarks()
	q.updateFrameTitle()

	if item.IsDir {
		q.startDirScan(path)
		return
	}
	q.cancelScan()

	if media.IsImageFile(path) {
		q.cacheImage = true
		gen := q.imageLoadGen
		source := q.src.Vfs

		if res, ok := media.ImagePipe.PreviewSync(context.Background(), source, path); ok {
			if res.Surface != nil && res.Surface.Valid() {
				q.imageSurf = res.Surface
			}
		}

		media.ImagePipe.Load(source, path, func(res media.ImageResult) {
			vtui.FrameManager.PostTask(func() {
				if q.imageLoadGen == gen && q.cacheValid && q.CacheKey == key {
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

	request := vfs.QuickViewRequest{VFS: q.src.Vfs, Path: path, Item: item.VFSItem}
	providers := vfs.QuickViewProvidersFor(request)
	if len(providers) != 0 {
		q.startFilePreview(key, request, providers, LoadDefaultQuickView)
		return
	}

	// The plain text-or-hex preview is read right here, on the UI goroutine,
	// so it is read without sudo: the password prompt is a dialog this
	// goroutine has to show, and an elevated read started from here would
	// hold it up for as long as SudoClient waits for the dispatcher, five
	// minutes, with f4 frozen meanwhile. A file only root can read goes to
	// the worker instead, where the prompt can appear.
	loaded := LoadDefaultQuickView(vfs.WithoutElevation(context.Background()), request.VFS, request.Path)
	if errors.Is(loaded.Err, fs.ErrPermission) {
		q.startFilePreview(key, request, nil, loadRefusedQuickView)
		return
	}
	q.applyFilePreview(loaded)
}

type quickViewFileResult struct {
	Label      string
	Lines      []string
	raw        []byte
	codepage   int
	autoDetect bool
	Binary     bool
	Err        error
}

func makeQuickViewSelectionKey(filesystem vfs.VFS, path string, item vfs.VFSItem) quickViewSelectionKey {
	return quickViewSelectionKey{
		source:   DirectoryCacheKey(filesystem, path),
		revision: item.Revision,
		size:     item.Size,
		mtimeNS:  item.MTime.UnixNano(),
		isDir:    item.IsDir,
	}
}

// startFilePreview tries matching providers in priority order away from the
// UI thread. Only an explicit ErrQuickViewUnsupported advances to the next
// provider; an actual parse/read error is useful information and is shown.
// When no provider takes the file, fallback reads it, on the same worker.
func (q *QuickViewPanel) startFilePreview(key quickViewSelectionKey, request vfs.QuickViewRequest, providers []vfs.QuickViewProvider, fallback func(context.Context, vfs.VFS, string) quickViewFileResult) {
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
				loaded.Err = fmt.Errorf("%s: %w", provider.Name(), err)
			} else {
				loaded.Label = result.Label
				loaded.Lines = append([]string(nil), result.Lines...)
			}
			break
		}
		if !handled {
			loaded = fallback(ctx, request.VFS, request.Path)
		}
		if ctx.Err() != nil {
			return
		}

		frames.PostTask(func() {
			if q.previewGen != gen || !q.cacheValid || q.CacheKey != key {
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
	q.cacheLabel = result.Label
	q.cacheBinary = result.Binary
	q.cacheRaw = append(q.cacheRaw[:0], result.raw...)
	q.cacheCodepage = result.codepage
	q.cacheAutoDetect = result.autoDetect
	q.cacheLines = append(q.cacheLines[:0], result.Lines...)
	q.cacheReadErr = result.Err
	q.hexMode = result.Binary
	q.clearMarks()
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
func LoadDefaultQuickView(parent context.Context, filesystem vfs.VFS, path string) quickViewFileResult {
	ctx, cancel := context.WithTimeout(parent, 500*time.Millisecond)
	defer cancel()
	rc, err := filesystem.Open(ctx, path)
	if err != nil {
		return quickViewFileResult{Err: err}
	}
	defer rc.Close()
	return readQuickViewPreview(ctx, rc)
}

// loadRefusedQuickView is LoadDefaultQuickView for a file the UI goroutine
// was refused, and runs on the preview worker. Opening it may go through sudo
// and wait while the password is typed, so the 500 ms budget covers the read
// alone and starts once the file is open; counted from before the open, it
// would run out during the prompt and turn the read into a deadline error.
func loadRefusedQuickView(parent context.Context, filesystem vfs.VFS, path string) quickViewFileResult {
	rc, err := filesystem.Open(parent, path)
	if err != nil {
		return quickViewFileResult{Err: err}
	}
	defer rc.Close()
	ctx, cancel := context.WithTimeout(parent, 500*time.Millisecond)
	defer cancel()
	return readQuickViewPreview(ctx, rc)
}

// readQuickViewPreview reads the first previewMax bytes of rc as text or hex.
func readQuickViewPreview(ctx context.Context, rc vfs.ReadAtCloser) quickViewFileResult {
	buf := make([]byte, previewMax)
	n, readErr := rc.ReadAt(ctx, buf, 0)
	if readErr != nil && readErr != io.EOF {
		return quickViewFileResult{Err: readErr}
	}
	buf = buf[:n]

	autoDetect := config.App.ViewerAutodetectCodePage
	cpID := vfs.DetectEncoding(buf, autoDetect, config.App.ViewerDefaultCodePage)
	decodedBuf := buf
	if cpID != 65001 {
		if decoded, decodeErr := vfs.DecodeBytes(buf, cpID); decodeErr == nil {
			decodedBuf = decoded
		}
	}

	if viewer.LooksBinary(decodedBuf) {
		return quickViewFileResult{raw: append([]byte{}, buf...), codepage: cpID, autoDetect: autoDetect, Binary: true, Lines: hexDumpLines(buf)}
	}
	decodedBuf = vfs.StripUTF8BOM(decodedBuf)
	return quickViewFileResult{
		raw:        append([]byte{}, buf...),
		codepage:   cpID,
		autoDetect: autoDetect,
		Lines:      quickViewTextLines(decodedBuf),
	}
}

const previewMax = 16 * 1024

// viewer.LooksBinary returns true if the buffer contains a NUL byte or an
// unusually high proportion of non-printable / non-UTF-8 sequences.
// Simple heuristic — same shape as Far/far2l's viewer classification.

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
