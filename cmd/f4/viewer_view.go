package main

import (
	"context"
	"fmt"
	"golang.org/x/arch/x86/x86asm"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// ViewerView is a high-performance file viewer component.
type ViewerView struct {
	vtui.BaseFrame
	topBar  *TopBar
	menuBar *vtui.MenuBar
	backend *ViewerBackend
	vfs     vfs.VFS
	path    string

	HexMode bool
	// hexAuto records that hex mode came from the binary check rather than
	// from the user. Only an automatic verdict may be revisited when the
	// codepage changes: a hex view the user asked for has to survive an F8.
	hexAuto    bool
	DecodeMode bool
	WrapMode   bool
	// DisasmMode is the processor mode the decode view disassembles in:
	// 16, 32 or 64, or 0 while undecided. See disasm.go.
	DisasmMode int
	TopOffset  int64 // Current byte offset of the first visible line

	// For Text mode: offsets of lines currently on screen
	lineOffsets         []int64
	rowCells            []vtui.CharInfo
	visibleURLRows      [][]urlCellRange
	hoverURL            string
	eofVisible          bool
	lastKnownSize       int64
	lastSearch          string
	lastSearchOffset    int64
	lastSearchTopOffset int64
	lastSearchFound     bool
	lastSearchMatchLen  int64
	lastSearchCase      bool
	lastSearchReverse   bool
	lastSearchRegexp    bool
	lastSearchWholeWord bool
	// semanticWindowGeneration acknowledges GUI window/scroll requests even
	// when clamping leaves TopOffset unchanged at BOF/EOF.
	semanticWindowGeneration        uint64
	semanticWindowRequestGeneration uint64
	semanticPendingScroll           bool
	semanticPendingOffset           int64
	semanticPendingGeneration       uint64
	semanticWrapSeek                semanticWrapSeekState
	semanticLoadError               string
	semanticNeedsReflow             bool
	semanticProjection              *viewerWindowConstruction
	consoleProjection               *viewerWindowConstruction
	projectionContinuationPending   bool
	projectionContinuationKey       viewerWindowConstructionKey
	// viewerNavigationGeneration is the UI-thread ordering fence between a
	// direct keyboard destination and older asynchronous/native scrolling.
	// endNavigationAttempt distinguishes retries of the same logical End after
	// a viewport reflow from obsolete calculations using the previous layout.
	viewerNavigationGeneration uint64
	endNavigationAttempt       uint64
	endNavigationCancel        context.CancelFunc
	// nativeViewportRows is the number of complete text rows that fit in the
	// semantic frontend after its pixel-sized chrome has been laid out. The
	// terminal geometry remains authoritative when this is zero.
	nativeViewportRows     int
	nativeViewportColumns  int
	nativeViewportRevision uint64
	semanticLayoutRevision uint64
	layoutTabSize          int

	scrollBar *vtui.ScrollBar

	// tailStop closes when the viewer stops watching the file for changes.
	// Nil means nothing is watching -- a ViewerView built directly, as the
	// tests do, never starts the poll.
	tailStop chan struct{}

	OnClose  func()
	Codepage int
}

// semanticWrapSeekState keeps only the scalar cursor needed to resume a
// non-blocking wrapped-row seek plus a tiny ring of preceding fragment starts.
// The ring is sized from the semantic top overscan, never from file size.
type semanticWrapSeekState struct {
	active         bool
	ready          bool
	target         int64
	width          int
	curr           int64
	resolved       int64
	currColumn     int
	resolvedColumn int
	lineStartReady bool
	history        []viewerRowPosition
	historyHead    int
	historyCount   int
}

type viewerRowPosition struct {
	offset int64
	column int
}

func NewViewerView(ctx context.Context, v vfs.VFS, path string) (*ViewerView, error) {
	f, err := v.Open(ctx, path)
	if err != nil {
		return nil, err
	}

	size := f.Size()
	detectLen := 16 * 1024
	if int64(detectLen) > size {
		detectLen = int(size)
	}
	header := make([]byte, detectLen)
	n, err := readDocumentBytes(ctx, f, header, 0)
	if err != nil && err != io.EOF {
		_ = f.Close()
		return nil, err
	}
	header = header[:n]

	cpID := vfs.DetectEncoding(header, AppConfig.ViewerAutodetectCodePage, AppConfig.ViewerDefaultCodePage)
	if remembered, ok := rememberedCodepage(v, path); ok {
		cpID = remembered
	}
	binary := viewerHeaderLooksBinary(header, cpID)
	if binary {
		// Binary data has no text codepage to materialize. Keeping the remote
		// handle lets the hex viewer fetch only its small visible windows.
		cpID = 65001
	}
	dataOffset := int64(0)
	if cpID == 65001 && !binary && vfs.HasUTF8BOM(header) {
		dataOffset = vfs.UTF8BOMSize
	}

	backend, err := newViewerBackend(ctx, v, path, f, cpID, dataOffset)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if cpID == 65001 {
		// Detection already fetched these source bytes. Reuse the logical
		// prefix (excluding a BOM) for the first visible window.
		prefix := int(dataOffset)
		if prefix <= len(header) {
			backend.cacheData = header[prefix:]
			backend.cacheOff = 0
		}
	}

	vv := &ViewerView{
		backend:                backend,
		vfs:                    v,
		path:                   path,
		HexMode:                binary,
		WrapMode:               true,
		Codepage:               cpID,
		semanticLayoutRevision: 1,
		layoutTabSize:          effectiveViewerTabSize(),
		hexAuto:                binary,
		DisasmMode:             detectX86Mode(header),
	}
	vv.scrollBar = vtui.NewScrollBar(0, 0, 0)
	vv.scrollBar.ColorIdx = ColViewerScrollbar
	vv.scrollBar.SetOwner(vv)
	vv.scrollBar.OnScroll = func(v int) {
		newOff := int64(v)
		if vv.HexMode {
			newOff &= ^int64(0xF)
		} else {
			// Optimization: during fast drag, don't FindLineStart every pixel
			// unless we are close to the target or moving slowly.
			// For now, simple snap.
			newOff = vv.clampTextScrollOffset(newOff)
			newOff = vv.backend.FindLineStart(newOff)
		}
		if newOff != vv.TopOffset {
			vv.beginViewerNavigationIntent()
			vv.TopOffset = newOff
			vv.eofVisible = false
			vtui.FrameManager.Redraw()
		}
	}
	vv.scrollBar.OnStep = func(step int) {
		// Used for arrows and track clicks: perform logical steps
		switch step {
		case -1:
			vv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
		case 1:
			vv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
		case -2:
			vv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_PRIOR})
		case 2:
			vv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT})
		}
		vtui.FrameManager.Redraw()
	}
	vv.menuBar = vtui.NewMenuBar(nil)
	vv.menuBar.SetOwner(vv)
	vv.topBar = NewTopBar(
		func() string {
			base := displayFileTitle(vv.vfs, vv.path)
			return " " + base
		},
		func() string {
			percent := 0
			size := vv.backend.Size()
			if size > 0 {
				viewHeightBytes := int64(vv.viewportHeight())
				if vv.HexMode {
					viewHeightBytes *= 16
				} else {
					viewHeightBytes *= 80
				}
				if size <= viewHeightBytes {
					percent = 100
				} else {
					denominator := size - viewHeightBytes
					percent = int((vv.TopOffset * 100) / denominator)
				}
				if percent < 0 {
					percent = 0
				}
				if percent > 100 {
					percent = 100
				}
			}
			mode := Msg("Viewer.ModeText")
			if vv.DecodeMode {
				mode = disasmModeLabel(vv.disasmMode())
			} else if vv.HexMode {
				mode = Msg("Viewer.ModeHex")
			}
			cpName := vfs.DisplayCodepageName(vv.Codepage)
			return fmt.Sprintf(" %s │ %s │ %d%%     ", cpName, mode, percent)
		},
	)
	vv.topBar.SetVisible(true)
	vv.SetCanFocus(true)
	vv.SetFocus(true)
	vv.startTailWatch()
	return vv, nil
}

// viewerTailPollInterval is how often an open viewer looks at the file it is
// showing to see whether it changed. tail -f sleeps a second between looks;
// half of that keeps a log on screen feeling live without the poll itself
// becoming the workload.
const viewerTailPollInterval = 500 * time.Millisecond

// startTailWatch begins watching the file for changes. What it costs is one
// re-measure of an already-open handle per tick, and on a file system whose
// handles cannot do that -- a remote one -- it costs nothing at all, because
// ViewerBackend.Refresh is then a no-op. Nothing is read, and nothing is
// redrawn, until the file actually moves.
func (vv *ViewerView) startTailWatch() {
	if vv.tailStop != nil {
		return
	}
	stop := make(chan struct{})
	vv.tailStop = stop

	// Read the frame manager here, on the goroutine that starts the poll: the
	// poll outlives this call, and reading the global from inside it races
	// anything that reassigns vtui.FrameManager while it is still running.
	frames := vtui.FrameManager
	go func() {
		ticker := time.NewTicker(viewerTailPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				frames.PostTask(func() {
					// The viewer may have closed between the tick and this
					// task reaching the UI thread.
					if vv.tailStop == stop {
						vv.refreshFromFile()
					}
				})
			}
		}
	}()
}

// stopTailWatch puts the poll away. Closing the channel is what the goroutine
// is waiting on, so it stops at once rather than at the end of the interval,
// and a closed viewer leaves nothing running behind it.
func (vv *ViewerView) stopTailWatch() {
	if vv.tailStop == nil {
		return
	}
	close(vv.tailStop)
	vv.tailStop = nil
}

// Following belongs to the viewer lifecycle: native frontends may publish
// semantic state without ever calling the console painter.
func (vv *ViewerView) refreshFromFile() {
	if vv.backend == nil || vv.Busy {
		return
	}
	if !vv.backend.Refresh(context.Background()) {
		return
	}
	size := vv.backend.Size()
	follow := vv.eofVisible && size > vv.lastKnownSize
	vv.lastKnownSize = size
	if vv.TopOffset > size {
		// The file was truncated or rotated away under the viewport, and the
		// offset it was showing no longer exists.
		vv.TopOffset = 0
		vv.lastKnownSize = size
		vv.eofVisible = false
	}
	if follow {
		vv.jumpToEnd()
		return
	}
	vtui.FrameManager.Redraw()
}

// reload rereads the file on demand. Unlike the poll it drops the window cache
// even when the length did not change, so a file rewritten in place -- same
// size, different bytes -- also shows its new contents.
func (vv *ViewerView) reload() {
	if vv.backend == nil {
		return
	}
	vv.backend.Refresh(context.Background())
	vv.backend.DropCache()
	vv.beginViewerNavigationIntent()
	vv.lineOffsets = nil
	if size := vv.backend.Size(); vv.TopOffset > size {
		vv.TopOffset = 0
		vv.eofVisible = false
	}
	if vv.eofVisible {
		vv.jumpToEnd()
		return
	}
	vtui.FrameManager.Redraw()
}

// viewerDetectionHeader reads the prefix every codepage decision is made on.
// One helper so that opening a file, switching its codepage and going back to
// auto-detect all look at exactly the same bytes.
func viewerDetectionHeader(ctx context.Context, f vfs.ReadAtCloser) ([]byte, error) {
	size := f.Size()
	detectLen := 16 * 1024
	if int64(detectLen) > size {
		detectLen = int(size)
	}
	header := make([]byte, detectLen)
	n, err := f.ReadAt(ctx, header, 0)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("read file header: %w", err)
	}
	return header[:n], nil
}

func viewerHeaderLooksBinary(header []byte, cpID int) bool {
	decoded := header
	if cpID != 65001 {
		if converted, err := vfs.DecodeBytes(header, cpID); err == nil {
			decoded = converted
		}
	}
	return looksBinary(decoded)
}

func (vv *ViewerView) SetPosition(x1, y1, x2, y2 int) {
	previousWidth := vv.viewportWidth()
	vv.ScreenObject.SetPosition(x1, y1, x2, y2)
	if vv.viewportWidth() != previousWidth {
		vv.semanticNeedsReflow = true
		vv.semanticWrapSeek = semanticWrapSeekState{}
		vv.semanticLayoutRevision++
		vv.lineOffsets = nil
	}
	if vv.topBar != nil {
		vv.topBar.SetPosition(x1, y1, x2, y1)
	}
	if vv.menuBar != nil {
		vv.menuBar.SetPosition(x1, y1, x2, y1)
	}
	if vv.scrollBar != nil {
		vv.scrollBar.SetPosition(x2, y1+1, x2, y2)
	}
}

// viewportHeight returns the height used by navigation and semantic windows.
// Native chrome is measured in pixels and is not necessarily an integral
// number of terminal cells, so the QML surface reports the complete rows that
// actually remain visible.
func (vv *ViewerView) viewportHeight() int {
	height := vv.Y2 - vv.Y1
	if vv.nativeViewportRows > 0 {
		return vv.nativeViewportRows
	}
	return height
}

func (vv *ViewerView) viewportWidth() int {
	if vv.nativeViewportColumns > 0 {
		return vv.nativeViewportColumns
	}
	width := vv.X2 - vv.X1 + 1
	if vv.scrollBar != nil {
		width--
	}
	return max(1, width)
}

// GetMenuBar returns the viewer's menu bar. Items are regenerated from
// the action registry on every call, so shortcuts and toggle states are
// always current.
func (vv *ViewerView) GetMenuBar() *vtui.MenuBar {
	vv.menuBar.Items = BuildMenuBarItems("Viewer")
	return vv.menuBar
}

func (vv *ViewerView) HandleCommand(cmd int, args any) bool {
	if cmd == vtui.CmClose {
		vv.Close()
		return true
	}
	if cmd == CmSwitchToEditor {
		actionSwitchViewerToEditor(vv)
		return true
	}
	if cmd == CmSearch {
		actionViewerSearch(vv)
		return true
	}
	if handleWorkspaceForkCommand(cmd, args) {
		return true
	}
	return vv.BaseFrame.HandleCommand(cmd, args)
}

func (vv *ViewerView) Show(scr *vtui.ScreenBuf) {
	vv.ScreenObject.Show(scr)
	if vv.topBar != nil {
		vv.topBar.Show(scr)
	}
	vv.DisplayObject(scr)
}

func (vv *ViewerView) DisplayObject(scr *vtui.ScreenBuf) {
	if !vv.IsVisible() {
		return
	}
	vv.ensureTextLayoutSettings()

	// AUTO-SCROLL LOGIC (tail -f)
	currentSize := vv.backend.Size()
	if vv.eofVisible && currentSize > vv.lastKnownSize && !vv.Busy {
		vv.lastKnownSize = currentSize
		vv.jumpToEnd()
		return
	}
	vv.lastKnownSize = currentSize
	if nativeDocumentCellPaintOwned(scr) {
		return
	}

	width := vv.viewportWidth()
	// Rendering also adjusts TopOffset and EOF state. Use the same complete-row
	// viewport as navigation, or the hidden terminal render scrolls native EOF
	// pages backwards and leaves the last lines below the QML viewport.
	contentHeight := vv.viewportHeight()

	bgAttr := vtui.Palette[ColViewerText]

	// 1. Draw Background
	scr.FillRect(vv.X1, vv.Y1+1, vv.X2, vv.Y2, ' ', bgAttr)

	if vv.Busy {
		scr.Write(vv.X1, vv.Y1+1, vtui.StringToCharInfo(" [ Loading... ] ", bgAttr))
		return
	}

	if contentHeight > 0 {
		if vv.DecodeMode {
			vv.renderDecode(scr, width, contentHeight)
		} else if vv.HexMode {
			vv.renderHex(scr, width, contentHeight)
		} else {
			vv.renderText(scr, width, contentHeight)
		}
	}

	if vv.scrollBar != nil && vv.backend.Size() > 0 {
		maxOffset := int(vv.backend.Size())
		if vv.HexMode {
			if contentHeight > 0 {
				lastLineOffset := int((vv.backend.Size() - 1) &^ 0xF)
				maxOffset = lastLineOffset - (contentHeight-1)*16
				if maxOffset < 0 {
					maxOffset = 0
				}
			}
		}
		vv.scrollBar.SetParams(int(vv.TopOffset), 0, maxOffset)
		vv.scrollBar.Show(scr)
	}
}

func (vv *ViewerView) renderHex(scr *vtui.ScreenBuf, width, contentHeight int) {
	attr := vtui.Palette[ColViewerText]
	currOffset := vv.TopOffset &^ 0xF
	for y := 0; y < contentHeight && currOffset < vv.backend.Size(); y++ {
		row, err := vv.projectRow(currOffset, width)
		if err != nil {
			message := fmt.Sprintf(" [ Error: %v ] ", err)
			if viewerProjectionLoading(err) {
				message = " [ Loading... ] "
			}
			scr.Write(vv.X1, vv.Y1+1+y, vtui.StringToCharInfo(message, attr))
			break
		}
		scr.Write(vv.X1, vv.Y1+1+y, row.cells)
		currOffset = row.end
	}
	vv.eofVisible = currOffset >= vv.backend.Size()
}

func (vv *ViewerView) effectiveDisasmMode() int {
	if !vv.DecodeMode {
		return 0
	}
	if vv.DisasmMode == 0 {
		if vv.backend != nil {
			header, _ := vv.backend.ReadAt(0, 1024)
			vv.DisasmMode = detectX86Mode(header)
		}
	}
	if vv.DisasmMode == 0 {
		return 64
	}
	return vv.DisasmMode
}

func (vv *ViewerView) findPrecedingInstructionOffset(target int64) int64 {
	if target <= 0 || vv.backend == nil {
		return 0
	}
	if build := vv.semanticProjection; build != nil {
		for i := len(build.rows) - 1; i >= 0; i-- {
			if build.rows[i].projection.end == target {
				return build.rows[i].start
			}
		}
	}
	for i := 1; i < len(vv.lineOffsets); i++ {
		if vv.lineOffsets[i] == target {
			return vv.lineOffsets[i-1]
		}
	}
	mode := vv.effectiveDisasmMode()
	minAnchor := max(int64(0), target-64)
	for anchor := minAnchor; anchor < target; anchor++ {
		curr := anchor
		var prev int64 = -1
		for curr < target {
			data, err := vv.backend.ReadAt(curr, 15)
			if err != nil && len(data) == 0 {
				break
			}
			instLen := 1
			inst, decErr := x86asm.Decode(data, mode)
			if decErr == nil {
				instLen = inst.Len
			}
			prev = curr
			curr += int64(instLen)
		}
		if curr == target && prev >= 0 {
			return prev
		}
	}
	return max(int64(0), target-1)
}

func (vv *ViewerView) renderDecode(scr *vtui.ScreenBuf, width, contentHeight int) {
	attr := vtui.Palette[ColViewerText]
	currOffset := vv.TopOffset
	for y := 0; y < contentHeight && currOffset < vv.backend.Size(); y++ {
		row, err := vv.projectRow(currOffset, width)
		if err != nil {
			message := fmt.Sprintf(" [ Error: %v ] ", err)
			if viewerProjectionLoading(err) {
				message = " [ Loading... ] "
			}
			scr.Write(vv.X1, vv.Y1+1+y, vtui.StringToCharInfo(message, attr))
			break
		}
		scr.Write(vv.X1, vv.Y1+1+y, row.cells)
		currOffset = row.end
	}
	vv.eofVisible = currOffset >= vv.backend.Size()
}

// disasmMode returns the processor mode the decode view uses. A view built
// without a header (NewViewerView reads one) decides it here, from the
// file's first bytes, the first time an instruction is needed.
func (vv *ViewerView) disasmMode() int {
	if !disasmModeValid(vv.DisasmMode) {
		header, _ := vv.backend.ReadAt(0, 1024)
		vv.DisasmMode = detectX86Mode(header)
	}
	return vv.DisasmMode
}

// cycleDisasmMode switches the decode view to the next processor mode in
// the 64 -> 32 -> 16 -> 64 cycle and returns the mode now in effect.
func (vv *ViewerView) cycleDisasmMode() int {
	vv.DisasmMode = nextDisasmMode(vv.disasmMode())
	return vv.DisasmMode
}

// decodeStep returns how many bytes the instruction at off occupies in the
// current mode: the distance to the next line of the decode view. It is
// zero while the bytes at off are still being fetched.
func (vv *ViewerView) decodeStep(off int64) int64 {
	data, _ := vv.backend.ReadAt(off, disasmMaxInstLen)
	return int64(disasmInstLen(data, vv.disasmMode()))
}

func (vv *ViewerView) renderText(scr *vtui.ScreenBuf, width, contentHeight int) {
	vv.renderTextRows(scr, width, contentHeight, true)
}

// renderTextRows renders a bounded set of visual rows.  alignEnd is enabled
// for the real viewer and disabled for semantic off-screen rendering: the
// latter must keep its requested row-to-byte mapping intact.
func (vv *ViewerView) renderTextRows(scr *vtui.ScreenBuf, width, contentHeight int, alignEnd bool) {
	vv.ensureTextLayoutSettings()
	attr := vtui.Palette[ColViewerText]
	if vv.semanticNeedsReflow {
		resolved, ready := vv.semanticResolveTextWindowOffset(vv.TopOffset)
		if !ready {
			message := " [ Loading... ] "
			if vv.semanticLoadError != "" {
				message = " [ Error: " + vv.semanticLoadError + " ] "
			}
			scr.Write(vv.X1, vv.Y1+1, vtui.StringToCharInfo(message, attr))
			return
		}
		vv.TopOffset, vv.semanticNeedsReflow = resolved, false
	}
	vv.lineOffsets = vv.lineOffsets[:0]
	build, _, err := vv.constructWindow(width, contentHeight, 0, &vv.consoleProjection)
	for y, constructed := range build.rows {
		// Only ready rows participate in paging and EOF alignment.
		vv.lineOffsets = append(vv.lineOffsets, constructed.start)
		scr.Write(vv.X1, vv.Y1+1+y, constructed.projection.cells)
	}
	if err != nil {
		message := fmt.Sprintf(" [ Error: %v ] ", err)
		if viewerProjectionLoading(err) {
			message = " [ Loading... ] "
		}
		scr.Write(vv.X1, vv.Y1+1+len(build.rows), vtui.StringToCharInfo(message, attr))
	}
	reachedEOF := build.current >= vv.backend.Size()
	if alignEnd && reachedEOF && len(vv.lineOffsets) < contentHeight && vv.TopOffset > 0 {
		aligned, ready := vv.finalTextPageOffset(vv.TopOffset, width,
			contentHeight-len(vv.lineOffsets))
		if !ready {
			// The tail is already drawable, but finding the preceding visual rows
			// may have started an asynchronous cache fill.  Leave EOF unset so a
			// redraw can retry the alignment instead of freezing an incomplete
			// final page.
			vv.eofVisible = false
			return
		}
		if aligned < vv.TopOffset {
			vv.TopOffset = aligned
			vv.renderTextRows(scr, width, contentHeight, false)
			return
		}
	}
	vv.eofVisible = reachedEOF
}

// finalTextPageOffset returns the first visual row needed to fill the final
// page whose current top row is offset.  It walks only the missing rows, so
// reaching the end of a large file remains proportional to the viewport.
func (vv *ViewerView) finalTextPageOffset(offset int64, width, rows int) (int64, bool) {
	if rows <= 0 || offset <= 0 {
		return offset, true
	}

	current := offset
	for i := 0; i < rows && current > 0; i++ {
		var previous int64
		var ready bool
		if vv.WrapMode {
			previous, ready = vv.semanticPreviousTextRowStart(current, width)
		} else {
			previous, ready = vv.backend.TryFindLineStart(current - 1)
		}
		if !ready || previous >= current {
			return offset, false
		}
		current = previous
	}
	return current, true
}

func (vv *ViewerView) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown {
		return false
	}
	vv.ensureTextLayoutSettings()

	ctrl := (e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
	alt := (e.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0
	if e.VirtualKeyCode == vtinput.VK_TAB && ctrl {
		return false
	}

	//height := int64(vv.Y2 - vv.Y1 + 1)
	step := int64(1)
	if vv.HexMode {
		step = 16
	}

	contentHeight := int64(vv.viewportHeight())

	switch e.VirtualKeyCode {
	case vtinput.VK_DOWN:
		// Arrow and wheel navigation are direct destinations too. Retire any
		// native window request before changing TopOffset, otherwise its
		// delayed acknowledgement can replay over this input.
		vv.beginViewerNavigationIntent()
		if vv.eofVisible {
			return true // Prevent scrolling past End of File
		}
		if vv.DecodeMode {
			vv.TopOffset += vv.decodeStep(vv.TopOffset)
		} else if vv.HexMode {
			if vv.TopOffset+16 < vv.backend.Size() {
				vv.TopOffset += 16
			}
		} else if len(vv.lineOffsets) > 1 {
			vv.TopOffset = vv.lineOffsets[1]
		} else {
			// Fail-safe: if lineOffsets not populated (e.g. before first render),
			// try to proactively find the next line start from current offset.
			width := vv.viewportWidth()
			data, err := vv.backend.ReadAt(vv.TopOffset, width*4)
			if err == nil && len(data) > 0 {
				tabSize := 8
				if AppConfig.EditorTabSize > 0 {
					tabSize = AppConfig.EditorTabSize
				}
				row := layoutViewerTextRow(data, width, tabSize, vv.WrapMode)
				if row.lineLen > 0 {
					vv.TopOffset += int64(row.lineLen)
				}
			}
		}
		vv.eofVisible = false
		return true

	case vtinput.VK_UP:
		vv.beginViewerNavigationIntent()
		if vv.DecodeMode {
			vv.TopOffset = vv.findPrecedingInstructionOffset(vv.TopOffset)
		} else if vv.HexMode {
			vv.TopOffset -= step
		} else {
			vv.TopOffset = vv.backend.FindLineStart(vv.TopOffset - 1)
		}
		if vv.TopOffset < 0 {
			vv.TopOffset = 0
		}
		vv.eofVisible = false
		return true

	case vtinput.VK_NEXT: // PgDn
		vv.beginViewerNavigationIntent()
		oldOffset := vv.TopOffset
		if vv.DecodeMode {
			for i := 0; i < int(contentHeight); i++ {
				vv.TopOffset += vv.decodeStep(vv.TopOffset)
			}
			if vv.TopOffset >= vv.backend.Size() {
				vv.TopOffset = vv.backend.Size() - 1
				if vv.TopOffset < 0 {
					vv.TopOffset = 0
				}
			}
		} else if vv.HexMode {
			vv.TopOffset += 16 * contentHeight
			if vv.TopOffset >= vv.backend.Size() {
				vv.TopOffset = (vv.backend.Size() - 1) &^ 0xF
				if vv.TopOffset < 0 {
					vv.TopOffset = 0
				}
			}
		} else if len(vv.lineOffsets) > 0 {
			if vv.eofVisible {
				return true
			}
			nextOffset := vv.lineOffsets[len(vv.lineOffsets)-1]
			if nextOffset <= vv.TopOffset || nextOffset >= vv.backend.Size() {
				return true
			}
			vv.TopOffset = nextOffset
		}
		if vv.TopOffset != oldOffset {
			vv.eofVisible = false
		}
		return true

	case vtinput.VK_PRIOR: // PgUp
		vv.beginViewerNavigationIntent()
		oldOffset := vv.TopOffset
		if vv.DecodeMode {
			for i := 0; i < int(contentHeight); i++ {
				vv.TopOffset = vv.findPrecedingInstructionOffset(vv.TopOffset)
				if vv.TopOffset == 0 {
					break
				}
			}
		} else if vv.HexMode {
			vv.TopOffset -= step * contentHeight
		} else {
			for i := 0; i < int(contentHeight); i++ {
				vv.TopOffset = vv.backend.FindLineStart(vv.TopOffset - 1)
			}
		}
		if vv.TopOffset < 0 {
			vv.TopOffset = 0
		}
		if vv.TopOffset != oldOffset {
			vv.eofVisible = false
		}
		return true

	case vtinput.VK_HOME:
		vv.beginViewerNavigationIntent()
		vv.TopOffset = 0
		vv.eofVisible = false
		return true

	case vtinput.VK_END:
		vv.jumpToEnd()
		return true

	case vtinput.VK_F8:
		if alt {
			vv.askGoto()
			return true
		}
	}

	// Injected-event fallback: KeyBar mouse clicks reach ProcessKey via
	// InjectEvents, which skips FrameManager.EventFilter and therefore the
	// hotkey manager. Route them through the same lookup so clicking F2/F5/
	// F7/… on the bottom bar triggers the configured Viewer action.
	if MacroMgr.LookupHotkey(e) {
		return true
	}

	return false
}

// askGoto prompts for a position. In text mode that is a line number, which
// only means something once someone has counted the newlines; in hex mode it
// is a byte offset, which needs no counting at all.
func (vv *ViewerView) askGoto() {
	if vv.HexMode || vv.DecodeMode {
		title, prompt := gotoText("Viewer.GotoOffsetTitle", " Go to offset "), gotoText("Viewer.GotoOffsetPrompt", "Byte offset:")
		showGotoOffsetDialog(vv, title, prompt, vv.TopOffset, func(offset int64) {
			vv.gotoPosition(offset)
		})
		return
	}
	title, prompt := " Go to line ", "Line number:"
	vtui.InputBoxOn(vv, title, prompt, "", func(s string) {
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil || n < 0 {
			return
		}
		vv.gotoPosition(n)
	})
}

func (vv *ViewerView) gotoPosition(n int64) {
	intent := vv.beginViewerNavigationIntent()
	if vv.HexMode || vv.DecodeMode {
		size := vv.backend.Size()
		if size == 0 {
			n = 0
		}
		if n >= size {
			n = size - 1
		}
		if n < 0 {
			n = 0
		}
		if vv.HexMode {
			vv.TopOffset = n &^ 0xF
		} else {
			vv.TopOffset = n
		}
		vtui.FrameManager.Redraw()
		return
	}

	// Finding a line can mean a remote round trip or, on a file system that
	// cannot index, a walk over the file, so it does not happen on the UI
	// thread and the user can cancel it.
	vv.Busy = true
	backend := vv.backend
	vtui.RunAsync(func(ctx *vtui.TaskContext) {
		stop := context.AfterFunc(backend.ctx, ctx.Cancel)
		defer stop()
		off, ok := backend.LineStart(ctx.Context, n)
		ctx.RunOnUIWithRedrawDecision(func() bool {
			if vv.IsDone() || vv.backend != backend ||
				vv.viewerNavigationGeneration != intent || backend.ctx.Err() != nil {
				return false
			}
			vv.Busy = false
			if !ok {
				if ctx.Err() == nil {
					vtui.ShowMessageOn(vv, " Go to line ",
						fmt.Sprintf("Line %d is past the end of the file.", n), []string{"&Ok"})
				}
				return true
			}
			vv.TopOffset = off
			vv.eofVisible = false
			vtui.FrameManager.Redraw()
			return true
		})
	})
}

// beginViewerNavigationIntent makes a direct keyboard destination newer than
// any native window request that was already accepted by the core. Advancing
// the acknowledgement is important: the native surface may still be waiting
// for that generation and would otherwise reject the keyboard-produced scene
// as older, then replay its pending destination over Home/End.
func (vv *ViewerView) beginViewerNavigationIntent() uint64 {
	if vv.endNavigationCancel != nil {
		vv.endNavigationCancel()
		vv.endNavigationCancel = nil
	}
	vv.viewerNavigationGeneration++
	// A direct keyboard destination is itself a new presentation generation.
	// Merely acknowledging the newest native request is insufficient: QML may
	// have one replaceable destination that has not been sent yet.  The native
	// surface cancels that local intent synchronously on Home/End and rejects
	// the in-flight request generation, so the keyboard-produced window must be
	// newer than both the last published and last accepted native generations.
	latestGeneration := max(vv.semanticWindowGeneration,
		vv.semanticWindowRequestGeneration, vv.semanticPendingGeneration)
	if latestGeneration != ^uint64(0) {
		latestGeneration++
	}
	vv.semanticWindowGeneration = latestGeneration
	vv.semanticWindowRequestGeneration = latestGeneration
	vv.semanticPendingScroll = false
	vv.semanticPendingOffset = 0
	vv.semanticPendingGeneration = 0
	vv.semanticProjection = nil
	vv.consoleProjection = nil
	vv.projectionContinuationPending = false
	vv.semanticWrapSeek = semanticWrapSeekState{}
	// A superseded End computation may still finish on a source that cannot
	// cancel an issued read. Its generation fence below prevents publication;
	// it must not keep the replacement keyboard destination visually busy.
	vv.Busy = false
	return vv.viewerNavigationGeneration
}

func (vv *ViewerView) jumpToEnd() {
	if vv.backend != nil {
		vv.backend.Refresh(context.Background())
	}
	intent := vv.beginViewerNavigationIntent()
	vv.jumpToEndForIntent(intent)
}

func (vv *ViewerView) jumpToEndForIntent(intent uint64) {
	if intent != vv.viewerNavigationGeneration || vv.backend == nil {
		return
	}
	if vv.endNavigationCancel != nil {
		vv.endNavigationCancel()
		vv.endNavigationCancel = nil
	}
	vv.ensureTextLayoutSettings()
	vv.endNavigationAttempt++
	attempt := vv.endNavigationAttempt
	contentHeight := int64(max(1, vv.viewportHeight()))
	if vv.HexMode {
		if vv.backend.Size() == 0 {
			vv.TopOffset = 0
		} else {
			lastLineOffset := (vv.backend.Size() - 1) &^ 0xF
			vv.TopOffset = lastLineOffset - (contentHeight-1)*16
			if vv.TopOffset < 0 {
				vv.TopOffset = 0
			}
		}
		return
	}
	if vv.DecodeMode {
		if vv.backend.Size() == 0 {
			vv.TopOffset = 0
		} else {
			curr := max(int64(0), vv.backend.Size()-1)
			for i := int64(0); i < contentHeight-1 && curr > 0; i++ {
				prev := vv.findPrecedingInstructionOffset(curr)
				if prev >= curr {
					break
				}
				curr = prev
			}
			vv.TopOffset = curr
		}
		return
	}

	if vv.backend.Size() == 0 {
		vv.TopOffset = 0
		return
	}

	vv.Busy = true
	width := vv.viewportWidth()
	backend := vv.backend
	size := backend.Size()
	wrapMode := vv.WrapMode
	layoutRevision := vv.semanticLayoutRevision
	viewportRevision := vv.nativeViewportRevision
	layoutTabSize := effectiveViewerTabSize()
	requestGeneration := vv.semanticWindowRequestGeneration
	scanContext, cancelScan := context.WithCancel(backend.ctx)
	vv.endNavigationCancel = cancelScan
	vtui.RunAsync(func(ctx *vtui.TaskContext) {
		stopTaskCancellation := context.AfterFunc(ctx.Context, cancelScan)
		defer stopTaskCancellation()
		defer cancelScan()
		defer ctx.RunOnUIWithRedrawDecision(func() bool {
			if !vv.IsDone() && vv.backend == backend &&
				vv.viewerNavigationGeneration == intent &&
				vv.endNavigationAttempt == attempt {
				vv.endNavigationCancel = nil
				changed := vv.Busy
				vv.Busy = false
				return changed
			}
			return false
		})

		// The common End path remains one bounded tail range. If that arbitrary
		// byte boundary cuts a logical line whose suffix cannot fill the
		// viewport, correctness requires resolving a real newline/BOF anchor;
		// that explicit fallback remains cancellable and uses fixed-size reads.
		tailLength := int(min(size, int64(viewerEndTailWindow)))
		startOff := size - int64(tailLength)
		tail, err := backend.ReadContext(scanContext, startOff, tailLength)
		if err != nil || len(tail) != tailLength {
			return
		}

		endResult, ready, err := viewerFastEndOffset(scanContext, tail, startOff, size,
			width, int(contentHeight), wrapMode, layoutTabSize)
		if err != nil {
			return
		}
		if !ready {
			// A tail boundary inside a logical line has no trustworthy wrap or
			// tab origin. Prefer a remote line index when one exists; otherwise
			// seek backward in cancellable bounded slices until a real line
			// start (and enough preceding logical rows) is proven.
			anchor, indexed := backend.LineStartFromEnd(scanContext, contentHeight)
			if !indexed || anchor < 0 || anchor >= size {
				anchor, err = viewerEndLogicalAnchor(scanContext, backend, size,
					int(contentHeight), startOff, tail)
				if err != nil {
					return
				}
			}
			endResult, err = viewerEndOffsetFromAnchor(scanContext, backend, anchor,
				startOff, size, tail, width, int(contentHeight), wrapMode, layoutTabSize)
			if err != nil {
				return
			}
		}

		ctx.RunOnUIWithRedrawDecision(func() bool {
			if vv.IsDone() || vv.backend != backend || backend.ctx.Err() != nil ||
				vv.viewerNavigationGeneration != intent ||
				vv.endNavigationAttempt != attempt ||
				vv.semanticWindowRequestGeneration != requestGeneration {
				return false
			}
			// End names a logical destination, not coordinates in the layout
			// that happened to exist when its bounded tail read began. If native
			// geometry or wrapping changed meanwhile, keep the same one-press
			// intent and recalculate against the current complete-row viewport.
			if vv.semanticLayoutRevision != layoutRevision ||
				vv.nativeViewportRevision != viewportRevision ||
				vv.viewportWidth() != width ||
				int64(vv.viewportHeight()) != contentHeight ||
				vv.WrapMode != wrapMode || effectiveViewerTabSize() != layoutTabSize ||
				backend.Size() != size {
				vv.jumpToEndForIntent(intent)
				return false
			}
			vv.Busy = false
			cancelScan()
			vv.endNavigationCancel = nil
			vv.TopOffset = endResult.top.offset
			if wrapMode {
				vv.seedViewerEndWrapSeek(endResult, width)
			}
			vtui.FrameManager.Redraw()
			return true
		})
	})
}
func (vv *ViewerView) ReloadWithCodepage(cpID int) {
	if vv.Codepage == cpID {
		return
	}

	f, err := vv.vfs.Open(context.Background(), vv.path)
	if err != nil {
		return
	}

	header, err := viewerDetectionHeader(context.Background(), f)
	if err != nil {
		_ = f.Close()
		return
	}

	hexMode := vv.HexMode
	if vv.hexAuto {
		// The hex view was the binary check's guess, so the codepage the
		// user just picked gets to overturn it. Without this, a file the
		// check misread -- UTF-16 with no byte-order mark, say -- had no way
		// back to text: choosing its codepage relabelled the status bar and
		// changed nothing else on screen.
		hexMode = viewerHeaderLooksBinary(header, cpID)
	}

	backendCP := cpID
	if hexMode {
		// Hex mode displays raw bytes; changing the label must not replace
		// those bytes with a decoded text stream.
		backendCP = 65001
	}
	dataOffset := int64(0)
	if backendCP == 65001 && !hexMode &&
		!viewerHeaderLooksBinary(header, backendCP) && vfs.HasUTF8BOM(header) {
		dataOffset = vfs.UTF8BOMSize
	}
	backend, err := newViewerBackend(context.Background(), vv.vfs, vv.path, f, backendCP, dataOffset)
	if err != nil {
		_ = f.Close()
		return
	}

	oldBackend := vv.backend
	oldOffset := vv.TopOffset
	oldSize := int64(0)
	if oldBackend != nil {
		oldSize = oldBackend.Size()
	}
	vv.backend = backend
	vv.Codepage = cpID
	vv.semanticLayoutRevision++
	vv.HexMode = hexMode
	newSize := vv.backend.Size()
	if newSize <= 0 {
		vv.TopOffset = 0
	} else {
		// TopOffset is an offset in the decoded stream. Its byte density can
		// change when the same raw file is viewed as CP1251, CP866, UTF-8,
		// or UTF-16, so carrying the old value verbatim can put the viewport
		// past EOF. Preserve the relative position first, then snap to a line.
		if oldSize > 0 && oldSize != newSize {
			oldOffset = oldOffset * newSize / oldSize
		}
		if oldOffset < 0 {
			oldOffset = 0
		}
		if oldOffset >= newSize {
			oldOffset = newSize - 1
		}
		if vv.HexMode {
			vv.TopOffset = oldOffset &^ 0xF
		} else {
			vv.TopOffset = vv.backend.FindLineStart(oldOffset)
		}
	}

	if oldBackend != nil {
		oldBackend.Close()
	}
	vtui.FrameManager.Redraw()
}

// newViewerBackend gives text mode one consistent coordinate system. A
// ViewerView's offsets are offsets in the UTF-8 stream it renders, not offsets
// in the raw file. Keeping a raw CP1251/CP866/UTF-16 window while exposing its
// decoded bytes made every multi-byte character change the meaning of the
// next offset; the cursor eventually ran past EOF, especially after Ctrl+End
// followed by an F8 switch. Non-UTF-8 files are materialized into the same
// memory-backed stream that the old viewer used, while UTF-8 keeps the lazy
// windowed backend for large files and remote VFSes.
func newViewerBackend(ctx context.Context, owner vfs.VFS, path string, f vfs.ReadAtCloser, cpID int, dataOffset int64) (*ViewerBackend, error) {
	if cpID != 65001 {
		size := f.Size()
		maxInt := int64(int(^uint(0) >> 1))
		if size < 0 || size > maxInt {
			return nil, fmt.Errorf("viewer: file is too large to decode: %d bytes", size)
		}
		raw := make([]byte, int(size))
		n, err := f.ReadAt(ctx, raw, 0)
		if err != nil && err != io.EOF {
			return nil, err
		}
		decoded, err := vfs.DecodeBytes(raw[:n], cpID)
		if err != nil {
			return nil, err
		}
		_ = f.Close()
		bCtx, bCancel := context.WithCancel(context.Background())
		backend := &ViewerBackend{
			file:         &vfs.MemoryReadAtCloser{Data: decoded},
			size:         int64(len(decoded)),
			path:         path,
			totalLines:   -1,
			totalForSize: -1,
			ctx:          bCtx,
			cancelCtx:    bCancel,
		}
		backend.cacheData = decoded[:min(len(decoded), 16*1024)]
		return backend, nil
	}

	logicalSize := f.Size() - dataOffset
	if logicalSize < 0 {
		logicalSize = 0
	}
	bCtx, bCancel := context.WithCancel(context.Background())
	backend := &ViewerBackend{
		file:         f,
		size:         logicalSize,
		path:         path,
		owner:        owner,
		codepage:     cpID,
		dataOffset:   dataOffset,
		totalLines:   -1,
		totalForSize: -1,
		ctx:          bCtx,
		cancelCtx:    bCancel,
	}
	if indexer, ok := owner.(vfs.LineIndexer); ok {
		backend.indexer = indexer
	}
	return backend, nil
}

func (vv *ViewerView) ReloadWithAutoDetect() {
	f, err := vv.vfs.Open(context.Background(), vv.path)
	if err != nil {
		return
	}
	defer f.Close()

	header, err := viewerDetectionHeader(context.Background(), f)
	if err != nil {
		return
	}

	// The user asked for this file to be detected, so detect it -- the
	// global switch decides what happens at open, not here (#875).
	cpID := vfs.DetectEncoding(header, true, AppConfig.ViewerDefaultCodePage)
	saveCodepageOverride(vv.vfs, vv.path, 0)
	vv.ReloadWithCodepage(cpID)
}

func (vv *ViewerView) showCodepageDialog() {
	_, overridden := rememberedCodepage(vv.vfs, vv.path)
	items, currIdx := vfs.BuildCodepageMenuItems(vv.Codepage, !overridden)
	menu := newCodepageMenu(Msg("Codepage.Title"), items)

	// This menu is about the file on screen, as Shift+F8 is in Far: a
	// codepage picked here is remembered for this file, and Auto-detect
	// forgets that and detects it again. Neither touches the global
	// viewer settings -- flipping AutodetectCodePage off and rewriting the
	// default codepage from here is what made every later file open in
	// whatever the previous one was switched to (#875).
	menu.OnAction = func(idx int) {
		menu.Close()
		if idx >= 0 && idx < len(menu.Items) {
			if cpID, ok := menu.Items[idx].UserData.(int); ok {
				if cpID == vfs.CodepageAutoDetect {
					vv.ReloadWithAutoDetect()
				} else {
					saveCodepageOverride(vv.vfs, vv.path, cpID)
					vv.ReloadWithCodepage(cpID)
				}
			}
		}
	}
	menu.SetSelectPos(currIdx)
	vtui.FrameManager.PushMenu(menu)
}

func (vv *ViewerView) ProcessMouse(e *vtinput.InputEvent) bool {
	if e.Type != vtinput.MouseEventType {
		return false
	}
	if e.WheelDirection == 0 {
		if changed := vv.updateURLHover(int(e.MouseX), int(e.MouseY)); changed {
			vtui.FrameManager.Redraw()
		}
		if ctrlMouseClick(e) {
			if link, ok := vv.urlLinkAtMouse(int(e.MouseX), int(e.MouseY)); ok {
				openExternalURLAsync(link.URL)
				return true
			}
		}
	}
	if vv.scrollBar != nil && vv.scrollBar.ProcessMouse(e) {
		return true
	}
	if e.WheelDirection != 0 {
		vv.hoverURL = ""
		speed := AppConfig.WheelViewerDown
		vk := uint16(vtinput.VK_DOWN)
		if e.WheelDirection > 0 {
			speed = AppConfig.WheelViewerUp
			vk = vtinput.VK_UP
		}
		for i := 0; i < wheelScrollLines(speed); i++ {
			vv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk})
		}
		return true
	}
	return false
}

func (vv *ViewerView) urlLinkAtMouse(mx, my int) (urlCellRange, bool) {
	if vv.HexMode || vv.DecodeMode || mx < vv.X1 || mx > vv.X2 || my < vv.Y1+1 || my > vv.Y2 {
		return urlCellRange{}, false
	}
	row := my - (vv.Y1 + 1)
	if row < 0 || row >= len(vv.visibleURLRows) {
		return urlCellRange{}, false
	}
	col := mx - vv.X1
	for _, link := range vv.visibleURLRows[row] {
		if col >= link.Start && col < link.End {
			return link, true
		}
	}
	return urlCellRange{}, false
}

func (vv *ViewerView) updateURLHover(mx, my int) bool {
	var next string
	if link, ok := vv.urlLinkAtMouse(mx, my); ok {
		next = link.URL
	}
	if next == vv.hoverURL {
		return false
	}
	vv.hoverURL = next
	return true
}
func (vv *ViewerView) ResizeConsole(w, h int) {
	vv.SetPosition(0, vtui.FrameManager.WorkspaceTopInset(), w-1, h-2)
}

func (vv *ViewerView) Close() {
	vv.stopTailWatch()
	vv.semanticProjection, vv.consoleProjection = nil, nil
	if vv.IsDone() {
		return
	}
	if GlobalFileState != nil && vv.path != "" {
		GlobalFileState.SaveViewerStateAsync(FileStateKey(vv.vfs, vv.path), vv.TopOffset, vv.WrapMode, vv.HexMode)
	}
	var size int64
	if vv.backend != nil {
		size = vv.backend.Size()
		vv.backend.Close()
	}
	vv.lineOffsets = nil
	vv.rowCells = nil
	vv.scrollBar = nil
	vv.BaseFrame.Close()
	if vv.OnClose != nil {
		vv.OnClose()
	}
	ReleaseHeavyMemory(size)
}

func (vv *ViewerView) GetKeyLabels() *vtui.KeySet {
	nextCp := vfs.GetNextFastSwitchCodepage(vv.Codepage)
	nextCpName := vfs.DisplayCodepageName(nextCp)

	fallbacks := &vtui.KeySet{
		Normal: vtui.KeyBarLabels{
			Msg("KeyBar.ViewerF1"), Msg("KeyBar.ViewerF2"), Msg("KeyBar.ViewerF3"), Msg("KeyBar.ViewerF4"),
			"", Msg("KeyBar.F4"), Msg("KeyBar.ViewerF7"), nextCpName, "", Msg("KeyBar.ViewerF10"),
		},
		NormalIcons: vtui.KeyBarIconNames{
			"circle-question-mark", "text-wrap", "x", "binary", "", "", "search", "languages", "", "x", "", "",
		},
		Alt: vtui.KeyBarLabels{
			"", "", "", "", "", "", "", Msg("KeyBar.ViewerAltF8"), "", "",
		},
		AltIcons: vtui.KeyBarIconNames{"", "", "", "", "", "", "", "locate-fixed", "", "", "", ""},
	}
	res := KeyBarLabelsForArea("Viewer", fallbacks)
	if hm := GlobalHotkeysMgr; hm != nil {
		if hm.GetAction("Viewer", "F8") == "Viewer.CodepageNext" {
			res.Normal[7] = nextCpName
		}
	}
	return res
}

func (vv *ViewerView) GetType() vtui.FrameType { return vtui.TypeUser + 3 }
func (vv *ViewerView) GetTitle() string {
	if vv.path != "" {
		return "View: " + filepath.Base(vv.path)
	}
	return "Viewer"
}

// GetWorkspaceTabTitle provides a compact title for the workspace
// tab bar while leaving GetTitle available for contexts that need the fuller
// textual description.
func (vv *ViewerView) GetWorkspaceTabTitle() string {
	if vv.path != "" {
		return filepath.Base(vv.path)
	}
	return "Viewer"
}
func (vv *ViewerView) GetWorkspaceTabMarker() string { return "V" }

func (vv *ViewerView) GetWorkspaceTabSurfaceKind() string { return "viewer" }
