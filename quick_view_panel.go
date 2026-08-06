package main

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
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
	frame   *vtui.BorderedFrame
	focused bool

	// Cache the last-computed preview so we don't re-read the file /
	// re-scan the directory on every redraw.
	cacheKey     string // full path we last previewed
	cacheDir     bool   // whether cache is for a directory or file
	cacheBinary  bool
	cacheImage   bool // whether cache is an image
	imageSurf    *vtui.ImageSurface
	imageLoadGen uint64
	gfxKey       string
	cacheLines   []string // raw preview lines (source lines or hex rows)
	cacheReadErr error

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
	wrap    bool
	scrollY int
	scrollX int

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
}

// NewQuickViewPanel creates a quick-view panel over src's slot.
func NewQuickViewPanel(src *FileSystemPanel) *QuickViewPanel {
	x1, y1, x2, y2 := src.GetPosition()
	q := &QuickViewPanel{src: src, wrap: true}
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
	// Ignore anything with modifiers — Ctrl+Q / Ctrl+L etc. need
	// to reach the global handler chain unchanged.
	if e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed|vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0 {
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
	default:
		return false
	}
	if q.scrollX < 0 {
		q.scrollX = 0
	}
	if q.scrollY < 0 {
		q.scrollY = 0
	}
	vtui.FrameManager.HardRefresh()
	return true
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

	idx := q.src.GetCursorIndex()
	if idx < 0 || idx >= len(q.src.entries) {
		q.cancelScan()
		q.cacheKey = ""
		writeLine(" " + Msg("QuickView.NoSelection"))
		return
	}
	item := q.src.entries[idx]

	// On "..", far2/far2l scan the CURRENT directory (parent of the
	// listing) rather than showing a static "Parent directory" note.
	// We synthesize a fileEntry that points at the current dir and
	// funnel it through the same refreshCache path as regular items.
	// The header shows the full path so it's unambiguous even when
	// several panels sit in similarly-named leaf folders.
	var path string
	if item.Name == ".." {
		path = q.src.vfs.GetPath()
		synth := fileEntry{VFSItem: vfs.VFSItem{Name: path, IsDir: true}}
		item = &synth
	} else {
		path = q.src.vfs.Join(q.src.vfs.GetPath(), item.Name)
	}
	if path != q.cacheKey {
		q.refreshCache(path, *item)
		q.scrollY = 0
		q.scrollX = 0
		q.displayLines = nil
	}

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
	writeLine(fmt.Sprintf(" %-14s %s", Msg("QuickView.FilesSize"), formatBytes(uint64(logical))))
	// Physical size + Ratio need per-item on-disk footprint. Stub /
	// remote VFSes leave PhysicalBytes at 0 during the whole scan —
	// hide the rows in that case. Ratio is also hidden when it would
	// just read "100%" — on Unix that's every uncompressed tree, and
	// a constant carries no information for the reader.
	if stats.PhysicalBytes > 0 {
		writeLine(fmt.Sprintf(" %-14s %s", Msg("QuickView.PhysicalSize"), formatBytes(uint64(stats.PhysicalBytes))))
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
				vtui.FrameManager.HardRefresh()
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
		vtui.FrameManager.HardRefresh()
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
func (q *QuickViewPanel) Close() { q.cancelScan() }

func (q *QuickViewPanel) renderFile(item *fileEntry, innerW int, writeLine func(string), attr uint64, scr *vtui.ScreenBuf) {
	// Header block (name + size + optional binary note). Two rows.
	writeLine(" " + item.Name)
	writeLine(fmt.Sprintf(" %s: %s", Msg("QuickView.Size"), formatBytes(uint64(item.Size))))
	if q.cacheReadErr != nil {
		writeLine("")
		writeLine(" " + Msg("QuickView.ReadError") + ": " + q.cacheReadErr.Error())
		return
	}
	if q.cacheImage {
		writeLine(" " + Msg("QuickView.Image"))
	} else if q.cacheBinary {
		writeLine(" " + Msg("QuickView.Binary"))
	} else {
		writeLine("")
	}
	writeLine(" " + strings.Repeat("─", innerW-2))

	if q.cacheImage {
		q.renderImage(innerW, writeLine, attr, scr)
		return
	}

	// Re-flow if wrap flag / innerW changed.
	if q.displayLines == nil || q.displayWrap != q.wrap || q.displayWidth != innerW {
		q.displayLines, q.displayToSource = q.buildDisplayLines(innerW)
		q.displayWrap = q.wrap
		q.displayWidth = innerW
		if q.hasPin {
			q.scrollY = firstDisplayForSource(q.displayToSource, q.pinSourceOnNextShow)
			q.hasPin = false
		}
	}

	// Clamp scroll offsets against fresh display.
	viewH := (q.Y2 - 1) - (q.Y1 + 1 + 4) + 1 // rows left after the 4-line header
	if viewH < 0 {
		viewH = 0
	}
	maxScroll := len(q.displayLines) - viewH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if q.scrollY > maxScroll {
		q.scrollY = maxScroll
	}

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
	top := y1 + 1 + 4 // Below the 4-line header
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

// refreshCache reads a fresh preview for path. Best-effort: errors
// are captured into cache*Err so the render path can surface them
// without blowing up.
func (q *QuickViewPanel) refreshCache(path string, item fileEntry) {
	q.cacheKey = path
	q.cacheDir = item.IsDir
	q.cacheBinary = false
	q.cacheImage = false
	q.imageSurf = nil
	q.cacheLines = nil
	q.cacheReadErr = nil

	if item.IsDir {
		q.startDirScan(path)
		return
	}
	q.cancelScan()

	if IsImageFile(path) {
		q.cacheImage = true
		q.imageLoadGen++
		gen := q.imageLoadGen

		if res, ok := ImagePipe.PreviewSync(context.Background(), q.src.vfs, path); ok {
			if res.Surface != nil && res.Surface.Valid() {
				q.imageSurf = res.Surface
			}
		}

		ImagePipe.Load(q.src.vfs, path, func(res ImageResult) {
			vtui.FrameManager.PostTask(func() {
				if q.imageLoadGen == gen {
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

	// Regular file: read up to previewMax bytes, split into lines or
	// classify as binary. Small budget (16 KiB) keeps this cheap even
	// on network VFSes.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	rc, err := q.src.vfs.Open(ctx, path)
	if err != nil {
		q.cacheReadErr = err
		return
	}
	defer rc.Close()
	buf := make([]byte, previewMax)
	n, rerr := rc.ReadAt(ctx, buf, 0)
	if rerr != nil && rerr != io.EOF {
		q.cacheReadErr = rerr
		return
	}
	buf = buf[:n]

	cpID := vfs.DetectEncoding(buf, AppConfig.ViewerAutodetectCodePage, AppConfig.ViewerDefaultCodePage)
	var decodedBuf []byte = buf
	if cpID != 65001 {
		if decoded, err := vfs.DecodeBytes(buf, cpID); err == nil {
			decodedBuf = decoded
		}
	}

	if looksBinary(decodedBuf) {
		q.cacheBinary = true
		q.cacheLines = hexDumpLines(buf)
	} else {
		lines := splitTextLines(string(decodedBuf))
		if n := len(lines); n > 0 && lines[n-1] == "" {
			lines = lines[:n-1]
		}
		q.cacheLines = lines
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
