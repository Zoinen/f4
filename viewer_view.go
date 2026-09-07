package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"golang.org/x/arch/x86/x86asm"
)

// ViewerView is a high-performance file viewer component.
type ViewerView struct {
	vtui.BaseFrame
	topBar  *TopBar
	menuBar *vtui.MenuBar
	backend *ViewerBackend
	vfs     vfs.VFS
	path    string

	HexMode    bool
	DecodeMode bool
	WrapMode   bool
	DisasmMode int   // 16, 32, or 64
	TopOffset  int64 // Current byte offset of the first visible line

	// For Text mode: offsets of lines currently on screen
	lineOffsets         []int64
	eofVisible          bool
	lastKnownSize       int64
	lastSearch          string
	lastSearchOffset    int64
	lastSearchTopOffset int64
	lastSearchFound     bool
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
		return nil, fmt.Errorf("read file header: %w", err)
	}
	header = header[:n]

	cpID := vfs.DetectEncoding(header, AppConfig.ViewerAutodetectCodePage, AppConfig.ViewerDefaultCodePage)
	binary := viewerHeaderLooksBinary(header, cpID)
	if binary {
		// Binary data has no text codepage to materialize. Keeping the remote
		// handle lets the hex viewer fetch only its small visible windows.
		cpID = 65001
	}

	var backend *ViewerBackend
	bCtx, bCancel := context.WithCancel(context.Background())
	if cpID == 65001 {
		backend = &ViewerBackend{
			file: f,
			size: size,
			// Detection already fetched these source bytes. Reuse them for
			// the first bounded window instead of returning ErrLoading and
			// fetching the same prefix again after the viewer is visible.
			cacheData:    header,
			path:         path,
			owner:        v,
			totalLines:   -1,
			totalForSize: -1,
			ctx:          bCtx,
			cancelCtx:    bCancel,
		}
		if indexer, ok := v.(vfs.LineIndexer); ok {
			backend.indexer = indexer
		}
	} else {
		fullData := make([]byte, size)
		copy(fullData, header)
		_, readErr := readDocumentBytes(ctx, f, fullData[len(header):], int64(len(header)))
		_ = f.Close()
		if readErr != nil {
			bCancel()
			return nil, fmt.Errorf("read file: %w", readErr)
		}

		decoded, err := vfs.DecodeBytes(fullData, cpID)
		if err != nil {
			decoded = fullData
			cpID = 65001
		}
		memFile := &vfs.MemoryReadAtCloser{Data: decoded}
		backend = &ViewerBackend{
			file:         memFile,
			size:         int64(len(decoded)),
			cacheData:    decoded[:min(len(decoded), 16*1024)],
			path:         path,
			totalLines:   -1,
			totalForSize: -1,
			ctx:          bCtx,
			cancelCtx:    bCancel,
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
	}
	vv.scrollBar = vtui.NewScrollBar(0, 0, 0)
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
			base := ""
			if vv.vfs != nil {
				base = vv.vfs.Base(vv.path)
			} else {
				base = filepath.Base(vv.path)
			}
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
				mode = fmt.Sprintf("Dec:%d", vv.effectiveDisasmMode())
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
	return vv, nil
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
	if cmd == CmSearch {
		actionViewerSearch(vv)
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
			data, _ := vv.backend.ReadAt(vv.TopOffset, 15)
			if len(data) > 0 {
				inst, err := x86asm.Decode(data, vv.effectiveDisasmMode())
				if err == nil {
					vv.TopOffset += int64(inst.Len)
				} else {
					vv.TopOffset += 1
				}
			}
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
				lineLen := 0
				visualWidth := 0
				tabSize := 8
				if AppConfig.EditorTabSize > 0 {
					tabSize = AppConfig.EditorTabSize
				}
				for lineLen < len(data) {
					r, size := utf8.DecodeRune(data[lineLen:])
					if r == '\n' {
						lineLen += size
						break
					}
					rw := 1
					if r == '\t' {
						rw = tabSize - (visualWidth % tabSize)
					} else {
						rw = runewidth.RuneWidth(r)
						if rw <= 0 {
							rw = 1
						}
					}
					if vv.WrapMode && visualWidth+rw > width {
						break
					}
					visualWidth += rw
					lineLen += size
				}
				if lineLen > 0 {
					vv.TopOffset += int64(lineLen)
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
			mode := vv.effectiveDisasmMode()
			for i := 0; i < int(contentHeight); i++ {
				data, _ := vv.backend.ReadAt(vv.TopOffset, 15)
				if len(data) > 0 {
					inst, err := x86asm.Decode(data, mode)
					if err == nil {
						vv.TopOffset += int64(inst.Len)
					} else {
						vv.TopOffset += 1
					}
				}
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
	title, prompt := " Go to line ", "Line number:"
	if vv.HexMode {
		title, prompt = " Go to offset ", "Byte offset:"
	}
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
	if vv.HexMode {
		size := vv.backend.Size()
		if n >= size {
			n = size - 1
		}
		if n < 0 {
			n = 0
		}
		vv.TopOffset = n &^ 0xF
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

	size := f.Size()
	var backend *ViewerBackend
	if cpID == 65001 {
		bCtx, bCancel := context.WithCancel(context.Background())
		backend = &ViewerBackend{
			file:         f,
			size:         size,
			path:         vv.path,
			owner:        vv.vfs,
			totalLines:   -1,
			totalForSize: -1,
			ctx:          bCtx,
			cancelCtx:    bCancel,
		}
		if indexer, ok := vv.vfs.(vfs.LineIndexer); ok {
			backend.indexer = indexer
		}
	} else {
		defer f.Close()
		fullData := make([]byte, size)
		_, _ = f.ReadAt(context.Background(), fullData, 0)

		decoded, err := vfs.DecodeBytes(fullData, cpID)
		if err != nil {
			decoded = fullData
			cpID = 65001
		}
		memFile := &vfs.MemoryReadAtCloser{Data: decoded}
		bCtx, bCancel := context.WithCancel(context.Background())
		backend = &ViewerBackend{
			file:         memFile,
			size:         int64(len(decoded)),
			path:         vv.path,
			totalLines:   -1,
			totalForSize: -1,
			ctx:          bCtx,
			cancelCtx:    bCancel,
		}
	}

	oldBackend := vv.backend
	vv.backend = backend
	vv.Codepage = cpID
	vv.semanticLayoutRevision++
	vv.TopOffset = vv.backend.FindLineStart(vv.TopOffset)

	if oldBackend != nil {
		oldBackend.Close()
	}
	vtui.FrameManager.Redraw()
}

func (vv *ViewerView) ReloadWithAutoDetect() {
	f, err := vv.vfs.Open(context.Background(), vv.path)
	if err != nil {
		return
	}
	defer f.Close()

	size := f.Size()
	detectLen := 16 * 1024
	if int64(detectLen) > size {
		detectLen = int(size)
	}
	header := make([]byte, detectLen)
	_, _ = f.ReadAt(context.Background(), header, 0)

	cpID := vfs.DetectEncoding(header, AppConfig.ViewerAutodetectCodePage, AppConfig.ViewerDefaultCodePage)
	vv.ReloadWithCodepage(cpID)
}

func (vv *ViewerView) showCodepageDialog() {
	items, currIdx := vfs.BuildCodepageMenuItems(vv.Codepage, AppConfig.ViewerAutodetectCodePage)
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
		if idx >= 0 && idx < len(menu.Items) {
			if cpID, ok := menu.Items[idx].UserData.(int); ok {
				if cpID == -1 {
					AppConfig.ViewerAutodetectCodePage = !AppConfig.ViewerAutodetectCodePage
					SaveConfig()
					vv.ReloadWithAutoDetect()
				} else {
					AppConfig.ViewerAutodetectCodePage = false
					AppConfig.ViewerDefaultCodePage = cpID
					SaveConfig()
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
	if vv.scrollBar != nil && vv.scrollBar.ProcessMouse(e) {
		return true
	}
	if e.WheelDirection != 0 {
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
func (vv *ViewerView) ResizeConsole(w, h int) {
	vv.SetPosition(0, vtui.FrameManager.WorkspaceTopInset(), w-1, h-2)
}

func (vv *ViewerView) Close() {
	vv.semanticProjection, vv.consoleProjection = nil, nil
	if vv.IsDone() {
		return
	}
	if GlobalFileState != nil && vv.path != "" {
		GlobalFileState.SaveViewerStateAsync(FileStateKey(vv.vfs, vv.path), vv.TopOffset, vv.WrapMode, vv.HexMode)
	}
	if vv.backend != nil {
		vv.backend.Close()
	}
	vv.BaseFrame.Close()
	if vv.OnClose != nil {
		vv.OnClose()
	}
}

func (vv *ViewerView) GetKeyLabels() *vtui.KeySet {
	nextCp := vfs.GetNextFastSwitchCodepage(vv.Codepage)
	nextCpName := vfs.DisplayCodepageName(nextCp)

	fallbacks := &vtui.KeySet{
		Normal: vtui.KeyBarLabels{
			Msg("KeyBar.ViewerF1"), Msg("KeyBar.ViewerF2"), Msg("KeyBar.ViewerF3"), Msg("KeyBar.ViewerF4"),
			"", "", Msg("KeyBar.ViewerF7"), nextCpName, "", Msg("KeyBar.ViewerF10"),
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
