package main

import (
	"encoding/base64"
	"sort"
	"sync"
	"time"

	"github.com/mattn/go-runewidth"
	"strings"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/textlayout"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// StyleChange фиксирует момент смены атрибутов в байтовом потоке лога.
type StyleChange struct {
	Offset int
	Attr   uint64
}

// TerminalView объединяет классическую сетку CharInfo и бесконечный лог.
type TerminalView struct {
	vtui.ScreenObject
	mu sync.Mutex

	// --- Состояние для ANSI Парсера (Grid) ---
	Lines        [][]vtui.CharInfo
	AltLines     [][]vtui.CharInfo
	WrapFlags    []bool // Tracks soft wrap state for each visual row
	UseAltScreen bool

	ScrollTop    int
	ScrollBottom int

	Width   int
	Height  int
	CursorX int
	CursorY int

	// Состояние терминала (сохранение координат)
	savedX, savedY       int
	decSavedX, decSavedY int
	Palette              [256]uint32

	// --- Бесконечный лог (History & Reflow) ---
	pt              *piecetable.PieceTable
	li              *piecetable.LineIndex
	engine          *textlayout.WrapEngine
	GridHistory     [][]vtui.CharInfo
	GridHistoryWrap []bool
	styles          []StyleChange
	lastAttr        uint64

	// Скроллинг истории (визуальный ряд)
	ScrollTopRow int

	Title                 string
	Win32InputMode        bool
	BracketedPasteMode    bool
	ApplicationCursorKeys bool
	KittyFlags            int
	KittyFlagsStack       []int
	AutoWrap              bool
	MouseTrackingMode     int
	MouseSGRMode          bool

	clipboardChunks []byte
	pty             PtyBackend
	kitty           *KittyGraphics
	images          []terminalImage
	kittyKeySeq     uint64
	cellW           int
	cellH           int

	Muted         bool
	lastCharWasCR bool
	authCache     map[string]int

	OnTitleChange func(string)
	OnBusyChange  func(bool)

	// --- Mouse-driven text selection over the visible viewport ---
	// Coordinates are absolute (screen) columns/rows, chosen so the
	// highlight stays visually anchored while PTY output scrolls the
	// underlying grid — matches xterm-style selection semantics.
	selActive  bool
	selStartX  int
	selStartY  int
	selEndX    int
	selEndY    int
	selBlock   bool
	showOffset int // last vertical "visual gravity" offset applied in Show
}

func NewTerminalView(w, h int) *TerminalView {
	tv := &TerminalView{
		Width:     w,
		Height:    h,
		AutoWrap:  true,
		authCache: make(map[string]int),
	}
	tv.ResetBuffer(w, h)
	return tv
}

func (tv *TerminalView) CloneStateFrom(other *TerminalView) {
	other.FlushLog()
	other.mu.Lock()
	defer other.mu.Unlock()
	tv.mu.Lock()
	defer tv.mu.Unlock()

	// 1. Match dimensions and re-allocate grids
	tv.Width = other.Width
	tv.Height = other.Height

	allocGrid := func(src [][]vtui.CharInfo) [][]vtui.CharInfo {
		dst := make([][]vtui.CharInfo, len(src))
		for y := range src {
			dst[y] = make([]vtui.CharInfo, len(src[y]))
			copy(dst[y], src[y])
		}
		return dst
	}
	tv.Lines = allocGrid(other.Lines)
	tv.AltLines = allocGrid(other.AltLines)
	tv.WrapFlags = make([]bool, len(other.WrapFlags))
	copy(tv.WrapFlags, other.WrapFlags)

	tv.GridHistory = make([][]vtui.CharInfo, len(other.GridHistory))
	for i := range other.GridHistory {
		tv.GridHistory[i] = make([]vtui.CharInfo, len(other.GridHistory[i]))
		copy(tv.GridHistory[i], other.GridHistory[i])
	}
	tv.GridHistoryWrap = make([]bool, len(other.GridHistoryWrap))
	copy(tv.GridHistoryWrap, other.GridHistoryWrap)

	// 2. Deep copy the PieceTable (History)
	bytes, _ := other.pt.Bytes()
	tv.pt = piecetable.New(bytes)

	// 3. Re-initialize indices and engine to point to the NEW pt
	tv.li = piecetable.NewLineIndex()
	tv.li.Rebuild(tv.pt)
	tv.engine = textlayout.NewWrapEngine(tv.pt, tv.li)
	tv.engine.SetWidth(tv.Width)

	// 4. Copy terminal state metadata
	tv.styles = append([]StyleChange(nil), other.styles...)
	tv.authCache = make(map[string]int)
	for k, v := range other.authCache {
		tv.authCache[k] = v
	}
	tv.lastAttr = other.lastAttr
	tv.Palette = other.Palette
	tv.CursorX, tv.CursorY = other.CursorX, other.CursorY
	tv.UseAltScreen = other.UseAltScreen
	tv.ScrollTop, tv.ScrollBottom = other.ScrollTop, other.ScrollBottom
	tv.KittyFlags = other.KittyFlags
	tv.KittyFlagsStack = append([]int(nil), other.KittyFlagsStack...)
	tv.pty = other.pty
	tv.images = append([]terminalImage(nil), other.images...)

	vtui.DebugLog("TERM_VIEW: CloneStateFrom completed. Cleaning active row for new shell.")

	// Clear the active visual row and reset horizontal cursor
	if tv.CursorY >= 0 && tv.CursorY < len(tv.Lines) {
		for x := range tv.Lines[tv.CursorY] {
			tv.Lines[tv.CursorY][x] = vtui.CharInfo{Char: ' ', Attributes: DefaultTermAttr}
		}
	}
	tv.CursorX = 0
}

func (tv *TerminalView) ResetBuffer(w, h int) {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	// Инициализация PieceTable (только один раз)
	if tv.pt == nil {
		tv.pt = piecetable.New([]byte{})
		tv.li = piecetable.NewLineIndex()
		tv.engine = textlayout.NewWrapEngine(tv.pt, tv.li)
		tv.styles = []StyleChange{{0, DefaultTermAttr}}
		tv.lastAttr = DefaultTermAttr
	}
	tv.engine.SetWidth(w)

	// Создание сеток (Grid)
	makeBuf := func() [][]vtui.CharInfo {
		b := make([][]vtui.CharInfo, h)
		for i := range b {
			b[i] = make([]vtui.CharInfo, w)
			for j := range b[i] {
				b[i][j] = vtui.CharInfo{Char: ' ', Attributes: DefaultTermAttr}
			}
		}
		return b
	}

	tv.Lines = makeBuf()
	tv.AltLines = makeBuf()
	tv.WrapFlags = make([]bool, h)
	tv.images = nil

	// Сброс параметров прокрутки и курсора
	tv.Width, tv.Height = w, h
	tv.ScrollTop = 0
	tv.ScrollBottom = h - 1
	tv.CursorX = 0
	tv.CursorY = h - 1 // Восстановлено выравнивание по нижнему краю для правильного визуала (прилипание к низу)
	tv.lastCharWasCR = true
	vtui.DebugLog("TERM_VIEW: ResetBuffer to %dx%d. VTE Mirror initialized at bottom (%d)", w, h, tv.CursorY)

	// Палитра по умолчанию (ANSI order)
	copy(tv.Palette[:], vtui.XTerm256Palette[:])
	tv.Palette[0] = far2lPalette[0] // Black
	tv.Palette[1] = far2lPalette[4] // Red
	tv.Palette[2] = far2lPalette[2] // Green
	tv.Palette[3] = far2lPalette[6] // Yellow
	tv.Palette[4] = far2lPalette[1] // Blue
	tv.Palette[5] = far2lPalette[5] // Magenta
	tv.Palette[6] = far2lPalette[3] // Cyan
	tv.Palette[7] = far2lPalette[7] // White
	for i := 0; i < 8; i++ {
		winIdx := []int{0, 4, 2, 6, 1, 5, 3, 7}[i]
		tv.Palette[i+8] = far2lPalette[winIdx+8]
	}
}

func (tv *TerminalView) getBuffer() [][]vtui.CharInfo {
	if tv.UseAltScreen {
		return tv.AltLines
	}
	return tv.Lines
}

func (tv *TerminalView) SetMuted(muted bool) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.Muted = muted
}
func (tv *TerminalView) PrintCleanCommand(cleanCmd string) {
	// Печатаем команду строго там, где сейчас находится курсор терминала (у промпта)
	for _, r := range cleanCmd {
		tv.PutChar(r, DefaultTermAttr)
	}
	tv.PutChar('\r', DefaultTermAttr)
	tv.PutChar('\n', DefaultTermAttr)
	tv.FlushLog()
}
func (tv *TerminalView) FlushLog() {}
func (tv *TerminalView) rowHasText(y int) bool {
	if y < 0 || y >= tv.Height {
		return false
	}
	for x := 0; x < tv.Width; x++ {
		if tv.Lines[y][x].Char != ' ' || tv.Lines[y][x].Attributes != DefaultTermAttr {
			return true
		}
	}
	return false
}
func (tv *TerminalView) flushLogUnsafe() {}

func (tv *TerminalView) pushRowToGridHistory(y int) {
	lineCopy := make([]vtui.CharInfo, len(tv.Lines[y]))
	copy(lineCopy, tv.Lines[y])
	tv.GridHistory = append(tv.GridHistory, lineCopy)
	tv.GridHistoryWrap = append(tv.GridHistoryWrap, tv.WrapFlags[y])

	if len(tv.GridHistory) > 2000 {
		tv.extrudeGridHistoryRow(0)
		tv.GridHistory = tv.GridHistory[1:]
		tv.GridHistoryWrap = tv.GridHistoryWrap[1:]
	}
}

func (tv *TerminalView) extrudeGridHistoryRow(idx int) {
	line := tv.GridHistory[idx]
	isWrapped := tv.GridHistoryWrap[idx]

	lastChar := len(line) - 1
	for lastChar >= 0 && line[lastChar].Char == ' ' && line[lastChar].Attributes == DefaultTermAttr {
		lastChar--
	}

	var sb strings.Builder
	for i := 0; i <= lastChar; i++ {
		// Saving attributes for the log
		if line[i].Attributes != tv.lastAttr {
			tv.styles = append(tv.styles, StyleChange{Offset: int(tv.pt.Size()) + sb.Len(), Attr: line[i].Attributes})
			tv.lastAttr = line[i].Attributes
		}
		if line[i].Char != vtui.WideCharFiller {
			sb.WriteRune(rune(line[i].Char))
		}
	}
	if !isWrapped {
		sb.WriteRune('\n')
	}
	text := sb.String()
	if len(text) > 0 {
		offset := tv.pt.Size()
		tv.pt.Insert(offset, []byte(text))
		tv.li.UpdateAfterInsert(offset, []byte(text))
		tv.engine.InvalidateFrom(tv.li.LineCount() - 2)
	}
}

func (tv *TerminalView) GetAllLogBytes() []byte {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	hist, _ := tv.pt.Bytes()
	var sb strings.Builder
	sb.Write(hist)

	for i := 0; i < len(tv.GridHistory); i++ {
		line := tv.GridHistory[i]
		isWrapped := tv.GridHistoryWrap[i]
		lastChar := len(line) - 1
		for lastChar >= 0 && line[lastChar].Char == ' ' && line[lastChar].Attributes == DefaultTermAttr {
			lastChar--
		}
		for j := 0; j <= lastChar; j++ {
			if line[j].Char != vtui.WideCharFiller {
				sb.WriteRune(rune(line[j].Char))
			}
		}
		if !isWrapped {
			sb.WriteRune('\n')
		}
	}

	if !tv.UseAltScreen {
		lastValidRow := 0
		for y := tv.Height - 1; y >= 0; y-- {
			if tv.rowHasText(y) {
				lastValidRow = y
				break
			}
		}
		if tv.CursorY > lastValidRow {
			lastValidRow = tv.CursorY
		}

		firstValidRow := 0
		if len(tv.GridHistory) == 0 && tv.pt.Size() == 0 {
			for y := 0; y <= lastValidRow; y++ {
				if tv.rowHasText(y) || y == tv.CursorY {
					firstValidRow = y
					break
				}
			}
		}

		for y := firstValidRow; y <= lastValidRow && y < tv.Height; y++ {
			line := tv.Lines[y]
			isWrapped := tv.WrapFlags[y]

			lastChar := len(line) - 1
			for lastChar >= 0 && line[lastChar].Char == ' ' && line[lastChar].Attributes == DefaultTermAttr {
				lastChar--
			}

			for i := 0; i <= lastChar; i++ {
				if line[i].Char != vtui.WideCharFiller {
					sb.WriteRune(rune(line[i].Char))
				}
			}
			if !isWrapped && y < lastValidRow {
				sb.WriteRune('\n')
			}
		}
	}
	return []byte(sb.String())
}

func (tv *TerminalView) PutChar(r rune, attr uint64) {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	if tv.Muted {
		return
	}

	if r == '\r' {
		// vtui.DebugLog("TERM_VIEW: CR (CursorX: %d -> 0)", tv.CursorX)
		tv.CursorX = 0
		tv.lastCharWasCR = true
		return
	}
	if r == '\n' {
		// vtui.DebugLog("TERM_VIEW: LF (CursorY: %d -> %d)", tv.CursorY, tv.CursorY+1)
		if !tv.UseAltScreen && tv.CursorY >= 0 && tv.CursorY < tv.Height {
			tv.WrapFlags[tv.CursorY] = false // Hard break
		}
		tv.newline()
		return
	}
	if r == '\b' {
		if tv.CursorX > 0 {
			tv.CursorX--
		}
		return
	}
	if r == '\t' {
		tv.CursorX = (tv.CursorX + 8) & ^7
		if tv.CursorX >= tv.Width {
			tv.CursorX = tv.Width - 1
		}
		return
	}
	if r < 0x20 {
		return
	}

	w := runewidth.RuneWidth(r)
	if w <= 0 {
		w = 1
	}

	if tv.CursorX >= tv.Width {
		if tv.AutoWrap {
			if !tv.UseAltScreen && tv.CursorY >= 0 && tv.CursorY < tv.Height {
				tv.WrapFlags[tv.CursorY] = true // Soft wrap (reached edge)
			}
			tv.newline()
		} else {
			tv.CursorX = tv.Width - 1 // Overwrite last character instead of wrapping
		}
	}

	buf := tv.getBuffer()
	if tv.CursorY >= 0 && tv.CursorY < tv.Height && tv.CursorX >= 0 && tv.CursorX+w <= tv.Width {
		buf[tv.CursorY][tv.CursorX] = vtui.CharInfo{Char: uint64(r), Attributes: attr}
		for i := 1; i < w; i++ {
			buf[tv.CursorY][tv.CursorX+i] = vtui.CharInfo{Char: vtui.WideCharFiller, Attributes: attr}
		}
		tv.CursorX += w
	}
	tv.lastCharWasCR = false
}

func (tv *TerminalView) newline() {
	// vtui.DebugLog("TERM: newline at Y=%d (ScrollBottom=%d)", tv.CursorY, tv.ScrollBottom)
	tv.CursorX = 0
	tv.CursorY++
	if tv.CursorY > tv.ScrollBottom {
		tv.scrollUp(tv.ScrollTop, tv.ScrollBottom, 1)
		tv.CursorY = tv.ScrollBottom
	}
}
func (tv *TerminalView) ReverseIndex() {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if tv.Muted {
		return
	}
	if tv.CursorY == tv.ScrollTop {
		tv.scrollDown(tv.ScrollTop, tv.ScrollBottom, 1)
	} else if tv.CursorY > 0 {
		tv.CursorY--
	}
}

func (tv *TerminalView) Index() {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if tv.Muted {
		return
	}
	tv.CursorY++
	if tv.CursorY > tv.ScrollBottom {
		tv.scrollUp(tv.ScrollTop, tv.ScrollBottom, 1)
		tv.CursorY = tv.ScrollBottom
	}
}

func (tv *TerminalView) NextLine() {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if tv.Muted {
		return
	}
	tv.CursorX = 0
	tv.CursorY++
	if tv.CursorY > tv.ScrollBottom {
		tv.scrollUp(tv.ScrollTop, tv.ScrollBottom, 1)
		tv.CursorY = tv.ScrollBottom
	}
}

func (tv *TerminalView) scrollUp(top, bottom, n int) {
	buf := tv.getBuffer()
	if top < 0 {
		top = 0
	}
	if bottom >= len(buf) {
		bottom = len(buf) - 1
	}
	if top >= bottom {
		return
	}

	for i := 0; i < n; i++ {
		if !tv.UseAltScreen && top == 0 {
			// Не пушим пустые строки в лог, если он еще девственно чист
			// Это предотвращает появление 23 пустых строк при старте bash
			if len(tv.GridHistory) > 0 || tv.pt.Size() > 0 || tv.rowHasText(top) {
				vtui.DebugLog("TERM_VIEW: ScrollUp extruding row %d to history", top)
				tv.pushRowToGridHistory(top)
			} else {
				vtui.DebugLog("TERM_VIEW: ScrollUp skipped extruding row %d (Extrusion Guard active)", top)
			}
		}
		recycledLine := buf[top]
		copy(buf[top:bottom], buf[top+1:bottom+1])
		buf[bottom] = recycledLine
		for j := range buf[bottom] {
			buf[bottom][j] = vtui.CharInfo{Char: ' ', Attributes: DefaultTermAttr}
		}
		if !tv.UseAltScreen {
			copy(tv.WrapFlags[top:bottom], tv.WrapFlags[top+1:bottom+1])
			tv.WrapFlags[bottom] = false
		}
	}
	tv.kittyScrollPlacements(top, bottom, n)
}
func (tv *TerminalView) ScrollDown(top, bottom, n int) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if tv.Muted {
		return
	}
	tv.scrollDown(top, bottom, n)
}

func (tv *TerminalView) scrollDown(top, bottom, n int) {
	buf := tv.getBuffer()
	if top < 0 {
		top = 0
	}
	if bottom >= len(buf) {
		bottom = len(buf) - 1
	}
	if top >= bottom {
		return
	}

	for i := 0; i < n; i++ {
		recycledLine := buf[bottom]
		for y := bottom; y > top; y-- {
			buf[y] = buf[y-1]
		}
		buf[top] = recycledLine
		for j := range buf[top] {
			buf[top][j] = vtui.CharInfo{Char: ' ', Attributes: DefaultTermAttr}
		}
		if !tv.UseAltScreen {
			for y := bottom; y > top; y-- {
				tv.WrapFlags[y] = tv.WrapFlags[y-1]
			}
			tv.WrapFlags[top] = false
		}
	}
	tv.kittyScrollPlacements(top, bottom, -n)
}

func (tv *TerminalView) DeleteCharacters(n int, attr uint64) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if tv.Muted {
		return
	}
	buf := tv.getBuffer()
	if tv.CursorY < 0 || tv.CursorY >= len(buf) {
		return
	}
	line := buf[tv.CursorY]
	if tv.CursorX < 0 || tv.CursorX >= tv.Width {
		return
	}

	if tv.CursorX+n < len(line) {
		copy(line[tv.CursorX:], line[tv.CursorX+n:])
	}

	clearStart := len(line) - n
	if clearStart < tv.CursorX {
		clearStart = tv.CursorX
	}
	for i := clearStart; i < len(line); i++ {
		line[i] = vtui.CharInfo{Char: ' ', Attributes: attr}
	}
}

func (tv *TerminalView) InsertBlankCharacters(n int, attr uint64) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if tv.Muted {
		return
	}
	buf := tv.getBuffer()
	if tv.CursorY < 0 || tv.CursorY >= len(buf) {
		return
	}
	line := buf[tv.CursorY]
	if tv.CursorX < 0 || tv.CursorX >= tv.Width {
		return
	}

	if tv.CursorX+n < len(line) {
		copy(line[tv.CursorX+n:], line[tv.CursorX:])
	}

	end := tv.CursorX + n
	if end > len(line) {
		end = len(line)
	}
	for i := tv.CursorX; i < end; i++ {
		line[i] = vtui.CharInfo{Char: ' ', Attributes: attr}
	}
}

func (tv *TerminalView) SetCursor(x, y int) {
	// vtui.DebugLog("TERM: SetCursor to (%d,%d)", x, y)
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if tv.Muted {
		return
	}
	if x < 0 {
		x = 0
	}
	if x >= tv.Width {
		x = tv.Width - 1
	}
	if y < 0 {
		y = 0
	}
	if y >= tv.Height {
		y = tv.Height - 1
	}
	tv.CursorX, tv.CursorY = x, y
	if x == 0 {
		tv.lastCharWasCR = true
	}
}

func (tv *TerminalView) SaveCursor() {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if tv.Muted {
		return
	}
	tv.decSavedX, tv.decSavedY = tv.CursorX, tv.CursorY
}

func (tv *TerminalView) RestoreCursor() {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if tv.Muted {
		return
	}
	tv.CursorX, tv.CursorY = tv.decSavedX, tv.decSavedY
}

func (tv *TerminalView) RepeatLastChar(n int, r rune, attr uint64) {
	for i := 0; i < n; i++ {
		tv.PutChar(r, attr)
	}
}

func (tv *TerminalView) EraseCharacter(n int, attr uint64) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if tv.Muted {
		return
	}
	buf := tv.getBuffer()
	if tv.CursorY < 0 || tv.CursorY >= len(buf) {
		return
	}
	line := buf[tv.CursorY]
	for i := 0; i < n && (tv.CursorX+i) < len(line); i++ {
		line[tv.CursorX+i] = vtui.CharInfo{Char: ' ', Attributes: attr}
	}
}

func (tv *TerminalView) EraseDisplay(mode int, attr uint64) {
	// vtui.DebugLog("TERM_VIEW: EraseDisplay mode=%d", mode)
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if tv.Muted {
		return
	}

	if (mode == 2 || mode == 3) && !tv.UseAltScreen {
		// Сохраняем экран в историю перед очисткой (игнорируя пустоту снизу)
		lastRow := -1
		for y := 0; y < tv.Height; y++ {
			if tv.rowHasText(y) {
				lastRow = y
			}
		}
		// vtui.DebugLog("TERM_VIEW: EraseDisplay(%d) pushing viewport up to row %d to history", mode, lastRow)
		for y := 0; y <= lastRow; y++ {
			tv.pushRowToGridHistory(y)
		}
		for i := range tv.WrapFlags {
			tv.WrapFlags[i] = false
		}
	}

	buf := tv.getBuffer()
	if mode == 2 {
		tv.CursorX = 0
		tv.CursorY = 0
		tv.lastCharWasCR = true
		tv.kittyClearPlacements(tv.UseAltScreen)
		for i := range buf {
			for j := range buf[i] {
				buf[i][j] = vtui.CharInfo{Char: ' ', Attributes: attr}
			}
		}
	} else if mode == 0 {
		if tv.CursorY >= 0 && tv.CursorY < tv.Height {
			line := buf[tv.CursorY]
			for j := (tv.CursorX); j < len(line); j++ {
				if j >= 0 {
					line[j] = vtui.CharInfo{Char: ' ', Attributes: attr}
				}
			}
			if !tv.UseAltScreen {
				tv.WrapFlags[tv.CursorY] = false
			}
		}
		for i := tv.CursorY + 1; i < tv.Height; i++ {
			if i >= 0 && i < len(buf) {
				line := buf[i]
				for j := range line {
					line[j] = vtui.CharInfo{Char: ' ', Attributes: attr}
				}
				if !tv.UseAltScreen {
					tv.WrapFlags[i] = false
				}
			}
		}
	}
}

func (tv *TerminalView) EraseLine(mode int, attr uint64) {
	// vtui.DebugLog("TERM_VIEW: EraseLine mode=%d at Y=%d (X=%d)", mode, tv.CursorY, tv.CursorX)
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if tv.Muted {
		return
	}
	buf := tv.getBuffer()
	if tv.CursorY < 0 || tv.CursorY >= len(buf) {
		return
	}
	line := buf[tv.CursorY]
	start, end := 0, len(line)
	if mode == 0 {
		start = tv.CursorX
	} else if mode == 1 {
		end = tv.CursorX + 1
	}
	for j := start; j < end; j++ {
		if j >= 0 && j < len(line) {
			line[j] = vtui.CharInfo{Char: ' ', Attributes: attr}
		}
	}
	if !tv.UseAltScreen && (mode == 2 || (mode == 0 && tv.CursorX == 0)) {
		tv.WrapFlags[tv.CursorY] = false
	}
}

func (tv *TerminalView) SetAltScreen(enable bool) {
	vtui.DebugLog("TERM: SetAltScreen %v", enable)
	vtui.DebugLog("TERM_VIEW: Switching screen buffer. AltScreen enabled: %v (Current Cursor: %d,%d)", enable, tv.CursorX, tv.CursorY)
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if tv.UseAltScreen == enable {
		return
	}
	if enable {
		tv.savedX, tv.savedY = tv.CursorX, tv.CursorY
		tv.CursorX, tv.CursorY = 0, 0
	} else {
		tv.CursorX, tv.CursorY = tv.savedX, tv.savedY
		// The alternate screen is discarded rather than remembered: the
		// next program to raise it is given an empty one, and the erase on
		// the way in proves it. Its pictures go with it, or they would keep
		// their pixels alive for as long as the session lasts.
		tv.kittyClearPlacements(true)
	}
	tv.UseAltScreen = enable
}

func (tv *TerminalView) getAttrAt(offset int) uint64 {
	idx := sort.Search(len(tv.styles), func(i int) bool {
		return tv.styles[i].Offset > offset
	})
	if idx > 0 {
		return tv.styles[idx-1].Attr
	}
	return DefaultTermAttr
}

func (tv *TerminalView) Show(scr *vtui.ScreenBuf) {
	tv.ScreenObject.Show(scr)

	scr.ActivePalette = &tv.Palette
	// Terminal content must always be rendered without Early Binding
	// to allow the host terminal to use its native indexed palette.
	prevOverlay := scr.OverlayMode
	scr.SetOverlayMode(false)
	defer func() { scr.SetOverlayMode(prevOverlay) }()

	tv.mu.Lock()
	defer tv.mu.Unlock()

	// Очищаем всю область терминала черным цветом
	// The placement layer needs the real size of a cell to turn the pixel
	// geometry of an image into a rectangle of cells, and so does the
	// program running in the terminal, which learns it from the pty.
	if cw, ch := scr.Graphics().CellSize(); cw > 0 && ch > 0 && (cw != tv.cellW || ch != tv.cellH) {
		tv.cellW, tv.cellH = cw, ch
		tv.syncPtyPixelSize()
		// A span we chose ourselves was worked out from the old cell, and
		// on the new one it would stretch the picture.
		tv.kittyRecomputeSpans()
	}

	scr.FillRect(tv.X1, tv.Y1, tv.X1+tv.Width-1, tv.Y1+tv.Height-1, ' ', DefaultTermAttr)

	buf := tv.Lines
	if tv.UseAltScreen {
		buf = tv.AltLines
	}

	offset := 0
	if !tv.UseAltScreen {
		lowestRow := 0
		for y := tv.Height - 1; y >= 0; y-- {
			if tv.rowHasText(y) {
				lowestRow = y
				break
			}
		}
		if tv.CursorY > lowestRow {
			lowestRow = tv.CursorY // Убеждаемся, что курсор также остается видимым
		}
		// A picture occupies rows that hold no text of their own, and
		// gravity must not push it out of the visible area.
		for i := range tv.images {
			if tv.images[i].Alt {
				continue
			}
			if bottom := tv.images[i].Row + tv.images[i].Rows - 1; bottom > lowestRow {
				lowestRow = bottom
			}
		}
		// Visual Gravity: сдвигаем весь активный рендер вниз, если он не достает до дна
		if lowestRow < tv.Height-1 {
			offset = (tv.Height - 1) - lowestRow
		}
	}
	tv.showOffset = offset

	for y, line := range buf {
		if y >= tv.Height {
			break
		}
		drawY := tv.Y1 + y + offset
		if tv.UseAltScreen {
			drawY = tv.Y1 + y
		}
		// Проверка выхода за пределы экрана
		if drawY >= tv.Y1 && drawY <= tv.Y1+tv.Height-1 {
			scr.Write(tv.X1, drawY, line)
		}
	}

	tv.kittyDrawPlacements(scr, offset)

	if tv.selActive {
		tv.paintSelectionHighlight(scr)
	}

	if tv.IsVisible() && tv.IsFocused() {
		cursorDrawY := tv.Y1 + tv.CursorY + offset
		if tv.UseAltScreen {
			cursorDrawY = tv.Y1 + tv.CursorY
		}
		if cursorDrawY >= tv.Y1 && cursorDrawY <= tv.Y1+tv.Height-1 {
			scr.SetCursorPos(tv.X1+tv.CursorX, cursorDrawY)
			scr.SetCursorVisible(true)
		}
	}
}

// selectionScreenRect returns the normalised screen-coordinate
// rectangle currently painted as selection, clamped to the terminal's
// visible area. Ok is false when there's no active selection.
func (tv *TerminalView) selectionScreenRect() (x1, y1, x2, y2 int, ok bool) {
	if !tv.selActive {
		return 0, 0, 0, 0, false
	}
	x1, x2 = tv.selStartX, tv.selEndX
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	y1, y2 = tv.selStartY, tv.selEndY
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	if x1 < tv.X1 {
		x1 = tv.X1
	}
	if y1 < tv.Y1 {
		y1 = tv.Y1
	}
	if x2 > tv.X1+tv.Width-1 {
		x2 = tv.X1 + tv.Width - 1
	}
	if y2 > tv.Y1+tv.Height-1 {
		y2 = tv.Y1 + tv.Height - 1
	}
	if x1 > x2 || y1 > y2 {
		return 0, 0, 0, 0, false
	}
	return x1, y1, x2, y2, true
}

// paintSelectionHighlight inverts fg↔bg for every cell inside the
// selection area. Stream selections span from the start column on the
// first row to the end column on the last row, filling middle rows
// edge-to-edge; block selections are strict rectangles.
func (tv *TerminalView) paintSelectionHighlight(scr *vtui.ScreenBuf) {
	x1, y1, x2, y2, ok := tv.selectionScreenRect()
	if !ok {
		return
	}
	rowLeft := func(y int) int {
		if tv.selBlock {
			return x1
		}
		if y == y1 && !singleRow(y1, y2) {
			return normalizedStart(tv.selStartX, tv.selStartY, tv.selEndX, tv.selEndY, true)
		}
		if y == y1 && singleRow(y1, y2) {
			return x1
		}
		return tv.X1
	}
	rowRight := func(y int) int {
		if tv.selBlock {
			return x2
		}
		if y == y2 && !singleRow(y1, y2) {
			return normalizedStart(tv.selStartX, tv.selStartY, tv.selEndX, tv.selEndY, false)
		}
		if y == y2 && singleRow(y1, y2) {
			return x2
		}
		return tv.X1 + tv.Width - 1
	}
	for y := y1; y <= y2; y++ {
		l, r := rowLeft(y), rowRight(y)
		if l < tv.X1 {
			l = tv.X1
		}
		if r > tv.X1+tv.Width-1 {
			r = tv.X1 + tv.Width - 1
		}
		for x := l; x <= r; x++ {
			ci := scr.GetCell(x, y)
			ci.Attributes = invertAttrColors(ci.Attributes)
			scr.Write(x, y, []vtui.CharInfo{ci})
		}
	}
}

func singleRow(y1, y2 int) bool { return y1 == y2 }

// normalizedStart returns the column that starts / ends a stream
// selection across multiple rows. When returnStart is true it returns
// the left edge on the row that owns the top-most anchor; otherwise
// it returns the right edge on the row that owns the bottom-most.
func normalizedStart(sx, sy, ex, ey int, returnStart bool) int {
	topX, botX := sx, ex
	if sy > ey {
		topX, botX = ex, sx
	}
	if returnStart {
		return topX
	}
	return botX
}

// HasSelection reports whether the terminal currently owns a
// user-driven text selection over its viewport.
func (tv *TerminalView) HasSelection() bool {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	return tv.selActive
}

// StartSelection begins a new selection anchored at the given
// screen-absolute cell. block controls stream vs. rectangular mode.
func (tv *TerminalView) StartSelection(x, y int, block bool) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.selActive = true
	tv.selBlock = block
	tv.selStartX, tv.selStartY = x, y
	tv.selEndX, tv.selEndY = x, y
}

// ExtendSelection moves the loose end of an active selection.
func (tv *TerminalView) ExtendSelection(x, y int) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if !tv.selActive {
		return
	}
	tv.selEndX, tv.selEndY = x, y
}

// ClearSelection drops the highlight without touching the clipboard.
func (tv *TerminalView) ClearSelection() {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.selActive = false
}

// SelectionIsEmpty reports whether the current selection covers a
// single cell (i.e. a click without drag).
func (tv *TerminalView) SelectionIsEmpty() bool {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	return !tv.selActive || (tv.selStartX == tv.selEndX && tv.selStartY == tv.selEndY)
}

// gridRowForScreenY maps a screen-absolute Y to the index into
// tv.Lines / tv.AltLines that is currently visible at that row, or
// -1 if the row falls outside the visible viewport.
func (tv *TerminalView) gridRowForScreenY(y int) int {
	off := 0
	if !tv.UseAltScreen {
		off = tv.showOffset
	}
	logical := y - tv.Y1 - off
	if logical < 0 || logical >= tv.Height {
		return -1
	}
	return logical
}

// ExtractSelection returns the plain-text content of the current
// selection, read from the terminal's own grid. Trailing spaces on
// stream-selected rows are trimmed; block selections keep alignment.
// WideCharFiller cells are skipped so wide glyphs don't emit stray
// runes.
func (tv *TerminalView) ExtractSelection() string {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if !tv.selActive {
		return ""
	}
	x1, y1, x2, y2, ok := tv.selectionScreenRect()
	if !ok {
		return ""
	}

	buf := tv.Lines
	if tv.UseAltScreen {
		buf = tv.AltLines
	}

	var sb strings.Builder
	for y := y1; y <= y2; y++ {
		gy := tv.gridRowForScreenY(y)
		if gy < 0 || gy >= len(buf) {
			if y < y2 {
				sb.WriteByte('\n')
			}
			continue
		}
		row := buf[gy]

		var l, r int
		if tv.selBlock {
			l, r = x1, x2
		} else if y1 == y2 {
			l, r = x1, x2
		} else if y == y1 {
			l = normalizedStart(tv.selStartX, tv.selStartY, tv.selEndX, tv.selEndY, true)
			r = tv.X1 + tv.Width - 1
		} else if y == y2 {
			l = tv.X1
			r = normalizedStart(tv.selStartX, tv.selStartY, tv.selEndX, tv.selEndY, false)
		} else {
			l = tv.X1
			r = tv.X1 + tv.Width - 1
		}
		if l < tv.X1 {
			l = tv.X1
		}
		if r > tv.X1+tv.Width-1 {
			r = tv.X1 + tv.Width - 1
		}

		var line []rune
		for x := l; x <= r; x++ {
			ci := row[x-tv.X1]
			if ci.Char == vtui.WideCharFiller {
				continue
			}
			ch := rune(ci.Char)
			if ch == 0 {
				ch = ' '
			}
			line = append(line, ch)
		}
		if !tv.selBlock {
			sb.WriteString(strings.TrimRight(string(line), " "))
		} else {
			sb.WriteString(string(line))
		}
		if y < y2 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// SelectWordAt sets a selection covering the whitespace-delimited
// word under the given screen cell. If the cell is whitespace,
// nothing changes.
func (tv *TerminalView) SelectWordAt(x, y int) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	gy := tv.gridRowForScreenY(y)
	if gy < 0 {
		return
	}
	buf := tv.Lines
	if tv.UseAltScreen {
		buf = tv.AltLines
	}
	if gy >= len(buf) {
		return
	}
	row := buf[gy]
	col := x - tv.X1
	if col < 0 || col >= len(row) {
		return
	}
	isWord := func(ch uint64) bool {
		if ch == 0 || ch == vtui.WideCharFiller {
			return false
		}
		return ch != ' '
	}
	if !isWord(row[col].Char) {
		return
	}
	left := col
	for left > 0 && isWord(row[left-1].Char) {
		left--
	}
	right := col
	for right < len(row)-1 && isWord(row[right+1].Char) {
		right++
	}
	tv.selActive = true
	tv.selBlock = false
	tv.selStartX, tv.selStartY = tv.X1+left, y
	tv.selEndX, tv.selEndY = tv.X1+right, y
}

// SelectLineAt selects the whole visible row at the given screen Y.
func (tv *TerminalView) SelectLineAt(y int) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if tv.gridRowForScreenY(y) < 0 {
		return
	}
	tv.selActive = true
	tv.selBlock = false
	tv.selStartX, tv.selStartY = tv.X1, y
	tv.selEndX, tv.selEndY = tv.X1+tv.Width-1, y
}

// InTerminalArea reports whether a screen cell falls inside the
// terminal's visible viewport.
func (tv *TerminalView) InTerminalArea(x, y int) bool {
	return x >= tv.X1 && x <= tv.X1+tv.Width-1 && y >= tv.Y1 && y <= tv.Y1+tv.Height-1
}

func (tv *TerminalView) Resize(w, h int) {
	if tv.Width == w && tv.Height == h {
		return
	}

	tv.mu.Lock()
	defer tv.mu.Unlock()

	tv.engine.SetWidth(w)

	makeBuf := func() [][]vtui.CharInfo {
		b := make([][]vtui.CharInfo, h)
		for i := range b {
			b[i] = make([]vtui.CharInfo, w)
			for j := range b[i] {
				b[i][j] = vtui.CharInfo{Char: ' ', Attributes: DefaultTermAttr}
			}
		}
		return b
	}

	newLines := makeBuf()
	newAltLines := makeBuf()
	newWrap := make([]bool, h)

	// 1. Сохраняем основной экран (Primary Screen).
	yOffset := 0
	yShift := 0

	if !tv.UseAltScreen {
		if h < tv.Height {
			lostRows := tv.Height - h
			for y := 0; y < lostRows; y++ {
				if tv.rowHasText(y) {
					tv.pushRowToGridHistory(y)
				}
			}
			yOffset = lostRows
		} else if h > tv.Height {
			yShift = h - tv.Height
			pullCount := yShift
			if pullCount > len(tv.GridHistory) {
				pullCount = len(tv.GridHistory)
			}

			startIdx := len(tv.GridHistory) - pullCount
			dstStart := yShift - pullCount

			// Возвращаем строки из GridHistory обратно на экран
			for i := 0; i < pullCount; i++ {
				dstY := dstStart + i
				srcLine := tv.GridHistory[startIdx+i]
				copyLen := w
				if len(srcLine) > copyLen {
					copyLen = len(srcLine) // Horizontal Preservation
				}
				newLines[dstY] = make([]vtui.CharInfo, copyLen)
				copy(newLines[dstY], srcLine)
				for j := len(srcLine); j < copyLen; j++ {
					newLines[dstY][j] = vtui.CharInfo{Char: ' ', Attributes: DefaultTermAttr}
				}
				newWrap[dstY] = tv.GridHistoryWrap[startIdx+i]
			}
			tv.GridHistory = tv.GridHistory[:startIdx]
			tv.GridHistoryWrap = tv.GridHistoryWrap[:startIdx]

			// Заполняем пустоты сверху
			for dstY := 0; dstY < dstStart; dstY++ {
				newLines[dstY] = make([]vtui.CharInfo, w)
				for j := 0; j < w; j++ {
					newLines[dstY][j] = vtui.CharInfo{Char: ' ', Attributes: DefaultTermAttr}
				}
			}
		}
	}

	// Копируем видимые строки в новую сетку, сохраняя данные, вышедшие за пределы ширины окна
	for dstY := yShift; dstY < h; dstY++ {
		srcY := dstY - yShift + yOffset
		if srcY >= 0 && srcY < tv.Height {
			srcLine := tv.Lines[srcY]
			copyLen := w
			if len(srcLine) > copyLen {
				copyLen = len(srcLine) // Horizontal Preservation
			}
			newLines[dstY] = make([]vtui.CharInfo, copyLen)
			copy(newLines[dstY], srcLine)
			for j := len(srcLine); j < copyLen; j++ {
				newLines[dstY][j] = vtui.CharInfo{Char: ' ', Attributes: DefaultTermAttr}
			}
			newWrap[dstY] = tv.WrapFlags[srcY]
		} else {
			newLines[dstY] = make([]vtui.CharInfo, w)
			for j := 0; j < w; j++ {
				newLines[dstY][j] = vtui.CharInfo{Char: ' ', Attributes: DefaultTermAttr}
			}
		}
	}

	// 2. Сохраняем содержимое AltScreen (для TUI приложений типа nano/mc).
	minH := h
	if tv.Height < minH {
		minH = tv.Height
	}
	for y := 0; y < minH; y++ {
		copyLen := w
		if tv.Width < w {
			copyLen = tv.Width
		}
		copy(newAltLines[y][:copyLen], tv.AltLines[y][:copyLen])
	}

	tv.Lines = newLines
	tv.AltLines = newAltLines
	tv.WrapFlags = newWrap

	tv.Width = w
	tv.Height = h
	tv.ScrollTop = 0
	tv.ScrollBottom = h - 1

	// The pictures follow the text through the reflow, and then take
	// whatever room the new size gives them.
	tv.kittyResizePlacements(yShift-yOffset, h)
	tv.kittyRecomputeSpans()

	if !tv.UseAltScreen {
		tv.CursorY = tv.CursorY - yOffset + yShift
		if tv.CursorY < 0 {
			tv.CursorY = 0
		}
		if tv.CursorY >= h {
			tv.CursorY = h - 1
		}
	} else {
		if tv.CursorY >= h {
			tv.CursorY = h - 1
		}
	}

	if tv.CursorX >= w {
		tv.CursorX = w - 1
	}
	tv.lastCharWasCR = (tv.CursorX == 0)
}
func (tv *TerminalView) IsModal() bool         { return false }
func (tv *TerminalView) RequestFocus() bool    { return true }
func (tv *TerminalView) Close()                {}
func (tv *TerminalView) GetWindowNumber() int  { return 0 }
func (tv *TerminalView) SetWindowNumber(n int) {}

func (tv *TerminalView) HandleFar2lAPC(s string) {
	// vtui.DebugLog("TERM_APC: Incoming Far2l sequence: %q", s)
	// Robustness: skip any garbage before the actual marker
	idx := strings.Index(s, "far2l")
	if idx == -1 {
		return
	}
	s = s[idx:]

	if s == "far2l1" {
		if tv.pty != nil {
			tv.pty.Write([]byte("\x1b_far2lok\x07"))
		}
	} else if s == "far2l0" {
		// Disable
	} else if s == "far2lok" {
		// Acknowledgement from the host terminal. This is not for the internal shell to process visually.
		// Consume and do nothing.
	} else if strings.HasPrefix(s, "far2l:") {
		b64 := s[6:]
		if m := len(b64) % 4; m != 0 {
			b64 += strings.Repeat("=", 4-m)
		}
		decoded, _ := base64.StdEncoding.DecodeString(b64)
		if len(decoded) > 0 {
			go tv.ProcessFar2lInteract(decoded)
		}
	}
}

// CellSize reports the pixel size of one character cell as the host renderer
// last told us, falling back to the size the terminal advertises when nobody
// knows better.
func (tv *TerminalView) CellSize() (int, int) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	return tv.cellSizeUnsafe()
}

func (tv *TerminalView) cellSizeUnsafe() (int, int) {
	cw, ch := tv.cellW, tv.cellH
	if cw <= 0 {
		cw = kittyFallbackCellW
	}
	if ch <= 0 {
		ch = kittyFallbackCellH
	}
	return cw, ch
}

// syncPtyPixelSize tells the child how large its terminal is in pixels. The
// caller holds the lock.
func (tv *TerminalView) syncPtyPixelSize() {
	if tv.pty == nil {
		return
	}
	sizer, ok := tv.pty.(PtyPixelSizer)
	if !ok {
		return
	}
	cw, ch := tv.cellSizeUnsafe()
	sizer.SetSizePixels(tv.Width, tv.Height, tv.Width*cw, tv.Height*ch)
}

// kittyGraphics lazily creates the receiver of the kitty graphics protocol:
// a session that never sends an image never pays for one.
func (tv *TerminalView) kittyGraphics() *KittyGraphics {
	tv.mu.Lock()
	kg := tv.kitty
	tv.mu.Unlock()
	if kg != nil {
		return kg
	}

	kg = NewKittyGraphics(func(b []byte) {
		if tv.pty != nil {
			tv.pty.Write(b)
		}
	})
	// The placement layer is attached before the receiver becomes reachable,
	// so the two locks are never taken at the same time.
	kg.SetDisplay(kittyDisplay{tv})

	tv.mu.Lock()
	if tv.kitty == nil {
		tv.kitty = kg
	}
	kg = tv.kitty
	tv.mu.Unlock()
	return kg
}

// HandleKittyAPC consumes one graphics escape code, without the leading G.
func (tv *TerminalView) HandleKittyAPC(s string) {
	tv.kittyGraphics().Handle(s)
}

func (tv *TerminalView) HandleOSC133(payload string) {
	vtui.DebugLog("TERM_OSC133: %s", payload)
	if payload == "C" {
		tv.SetMuted(false)
		if tv.OnBusyChange != nil {
			tv.OnBusyChange(true)
		}
	} else if payload == "D" || strings.HasPrefix(payload, "D;") {
		if tv.OnBusyChange != nil {
			tv.OnBusyChange(false)
		}
	}
}
func (tv *TerminalView) ProcessFar2lInteract(data []byte) {
	stk := (*vtinput.Far2lStack)(&data)
	id := stk.PopU8()
	cmd := stk.PopU8()
	// vtui.DebugLog("TERM_APC: ProcessFar2lInteract: cmd=%c, id=%d", cmd, id)

	reply := vtinput.Far2lStack{}

	switch cmd {
	case 'c': // Clipboard
		sub := stk.PopU8()
		// vtui.DebugLog("TERM_APC: Clipboard sub-command: %c", sub)
		switch sub {
		case 'o':
			clientID := stk.PopString()
			tv.mu.Lock()
			auth, cached := tv.authCache[clientID]
			tv.mu.Unlock()

			if !cached {
				if vtui.GlobalClipboardAccessManager != nil {
					auth = vtui.GlobalClipboardAccessManager.Authorize(clientID)
					if auth != 0 {
						tv.mu.Lock()
						tv.authCache[clientID] = auth
						tv.mu.Unlock()
					}
				}
			}

			respAuth := auth
			if auth == -1 {
				respAuth = 1 // Tell child success, we'll handle it locally
			}
			reply.PushU64(2) // FARTTY_FEATCLIP_CHUNKED_SET
			reply.PushU8(uint8(respAuth))
		case 'c':
			tv.mu.Lock()
			tv.clipboardChunks = nil
			tv.mu.Unlock()
			reply.PushU8(1)
		case 'e':
			vtui.SetClipboard("")
			tv.mu.Lock()
			tv.clipboardChunks = nil
			tv.mu.Unlock()
			reply.PushU8(1)
		case 'a':
			_ = stk.PopU32() // fmt
			reply.PushU8(1)
		case 'S':
			size := stk.PopU16()
			tv.mu.Lock()
			if size == 0 {
				tv.clipboardChunks = nil
			} else {
				chunk := stk.PopBytes(int(size) << 8)
				tv.clipboardChunks = append(tv.clipboardChunks, chunk...)
			}
			tv.mu.Unlock()
		case 's':
			_ = stk.PopU32() // fmt
			len := stk.PopU32()
			textBytes := stk.PopBytes(int(len))
			tv.mu.Lock()
			fullData := append(tv.clipboardChunks, textBytes...)
			tv.clipboardChunks = nil
			tv.mu.Unlock()
			if !vtui.SetOSClipboard(string(fullData)) {
				vtui.SetClipboard(string(fullData))
			}
			// Guest expects: dataID (U64) + status (U8)
			reply.PushU64(0)
			reply.PushU8(1)
		case 'g':
			_ = stk.PopU32() // fmt
			clipData := vtui.GetClipboard()
			if len(clipData) > 64*1024 {
				clipData = clipData[:64*1024]
			}
			// Guest expects: dataID (U64) + data (Bytes) + length (U32)
			reply.PushU64(0)
			reply.PushBytes([]byte(clipData))
			reply.PushU32(uint32(len(clipData)))
		case 'i':
			_ = stk.PopU32()
			reply.PushU64(0)
		case 'r':
			_ = stk.PopString()
			reply.PushU32(0xC000)
		}
	case 'w': // Window size
		reply.PushU16(uint16(tv.Height))
		reply.PushU16(uint16(tv.Width))
	case 'h': // Cursor height
		_ = stk.PopU8()
	case 'n': // Desktop notification
		text := stk.PopString()
		title := stk.PopString()
		vtui.FrameManager.PostTask(func() {
			vtui.ShowToast(title+": "+text, 3*time.Second)
		})
	case 'f': // FKey titles
		for i := 0; i < 12; i++ {
			state := stk.PopU8()
			if state != 0 {
				_ = stk.PopString() // Just pop, we can ignore it for now or implement KeyBar update
			}
		}
		reply.PushU8(1)
	case 'x': // Extra features
		feats := stk.PopU64()
		if feats&2 != 0 { // FARTTY_FEAT_TERMINAL_SIZE
			tv.SendFar2lTerminalSize()
		}
	case 'p': // Palette info
		reply.PushU8(0)  // reserved
		reply.PushU8(24) // bits
	case 'i': // Image operations (stub)
		_ = stk.PopU8() // subcmd
		reply.PushU8(0)
	}

	if len(reply) > 0 || id != 0 {
		reply.PushU8(id)
		b64 := base64.StdEncoding.EncodeToString(reply)
		if tv.pty != nil {
			// Reply from terminal to app MUST NOT have a colon after 'far2l'.
			// The colon is used as a discriminator: 'far2l:' indicates a request,
			// while 'far2l' (without colon) indicates a reply.
			tv.pty.Write([]byte("\x1b_far2l" + b64 + "\x07"))
		}
	}
}

func (tv *TerminalView) SendFar2lTerminalSize() {
	stk := vtinput.Far2lStack{}
	stk.PushU16(uint16(tv.Height))
	stk.PushU16(uint16(tv.Width))
	stk.PushU8('S')
	b64 := base64.StdEncoding.EncodeToString(stk)
	if tv.pty != nil {
		tv.pty.Write([]byte("\x1b_f2l:" + b64 + "\x07"))
	}
}
