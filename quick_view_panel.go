package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// QuickViewPanel is far2l's Ctrl+Q quick-view panel. It mirrors the
// source file panel's current cursor: for a directory it shows a
// small "info" block (name + immediate file/folder counts), for a
// regular file it shows a text preview or a hex dump depending on a
// simple binary heuristic. Recursive directory sizing and full-file
// viewer features are deliberately deferred.
type QuickViewPanel struct {
	vtui.ScreenObject
	src     *FileSystemPanel
	frame   *vtui.BorderedFrame
	focused bool

	// Cache the last-computed preview so we don't re-read the file /
	// re-scan the directory on every redraw.
	cacheKey        string // full path we last previewed
	cacheDir        bool   // whether cache is for a directory or file
	cacheDirFiles   int
	cacheDirFolders int
	cacheDirErr     error
	cacheBinary     bool
	cacheLines      []string // raw preview lines (source lines or hex rows)
	cacheReadErr    error

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
		writeLine(" " + Msg("QuickView.NoSelection"))
		return
	}
	item := q.src.entries[idx]
	if item.Name == ".." {
		writeLine(" " + Msg("QuickView.ParentDir"))
		return
	}

	path := q.src.vfs.Join(q.src.vfs.GetPath(), item.Name)
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
	if q.cacheDirErr != nil {
		writeLine(" " + Msg("QuickView.ReadError") + ": " + q.cacheDirErr.Error())
		return
	}
	writeLine(fmt.Sprintf(" %s: %d", Msg("QuickView.FileCount"), q.cacheDirFiles))
	writeLine(fmt.Sprintf(" %s: %d", Msg("QuickView.FolderCount"), q.cacheDirFolders))
}

func (q *QuickViewPanel) renderFile(item *fileEntry, innerW int, writeLine func(string), attr uint64, scr *vtui.ScreenBuf) {
	// Header block (name + size + optional binary note). Two rows.
	writeLine(" " + item.Name)
	writeLine(fmt.Sprintf(" %s: %s", Msg("QuickView.Size"), formatBytes(uint64(item.Size))))
	if q.cacheReadErr != nil {
		writeLine("")
		writeLine(" " + Msg("QuickView.ReadError") + ": " + q.cacheReadErr.Error())
		return
	}
	if q.cacheBinary {
		writeLine(" " + Msg("QuickView.Binary"))
	} else {
		writeLine("")
	}
	writeLine(" " + strings.Repeat("─", innerW-2))

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
	q.cacheLines = nil
	q.cacheReadErr = nil
	q.cacheDirErr = nil
	q.cacheDirFiles = 0
	q.cacheDirFolders = 0

	if item.IsDir {
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()
		err := q.src.vfs.ReadDir(ctx, path, func(chunk []vfs.VFSItem) {
			for i := range chunk {
				if chunk[i].Name == ".." {
					continue
				}
				if chunk[i].IsDir {
					q.cacheDirFolders++
				} else {
					q.cacheDirFiles++
				}
			}
		})
		if err != nil {
			q.cacheDirErr = err
		}
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
	if looksBinary(buf) {
		q.cacheBinary = true
		q.cacheLines = hexDumpLines(buf)
	} else {
		lines := splitTextLines(string(buf))
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
