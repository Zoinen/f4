package vtui

import "github.com/unxed/vtinput"

// TripleClick is a VTUI mouse event flag generated for the third consecutive
// click at the same position. The native console flags only define DoubleClick.
const TripleClick uint32 = 0x0010

// Coord defines the coordinates in the console.
type Coord struct {
	X int16
	Y int16
}

// SmallRect defines a rectangular area in the console.
// Rect defines a generic rectangle with absolute coordinates.
type Rect struct {
	X1, Y1, X2, Y2 int
}

// SmallRect defines a rectangular area in the console.
type SmallRect struct {
	Left   int16
	Top    int16
	Right  int16
	Bottom int16
}

// CharInfo contains a character and its visual attributes (including colors).
// In far2l, Char (UnicodeChar) is uint64 (COMP_CHAR) to support composite characters.
// Let's use the same bit length.
type CharInfo struct {
	Char       uint64 // Equivalent to union with COMP_CHAR UnicodeChar
	Attributes uint64 // DWORD64 Equivalent Attributes (lower 16 bits are flags, 16-39 are Fore RGB, 40-63 are Back RGB)
} // GrowMode flags for responsive layout resizing (analogous to Turbo Vision)
type GrowMode int

const (
	GrowNone GrowMode = 0
	GrowLoX  GrowMode = 0x01
	GrowHiX  GrowMode = 0x02
	GrowLoY  GrowMode = 0x04
	GrowHiY  GrowMode = 0x08
	GrowAll  GrowMode = 0x0f
	GrowRel  GrowMode = 0x10
)

// LinkAction defines how a target element reacts to a source element's state change.
type LinkAction int

const (
	LinkEnableIfChecked LinkAction = iota
	LinkDisableIfChecked
	LinkShowIfChecked
	LinkHideIfChecked
)

// UIElement is the interface that all screen objects (widgets, frames, windows) implement.
type UIElement interface {
	GetPosition() (int, int, int, int)
	SetPosition(int, int, int, int)
	GetGrowMode() GrowMode
	Show(scr *ScreenBuf)
	Hide(scr *ScreenBuf)
	IsVisible() bool
	SetVisible(bool)
	SetFocus(bool)
	IsFocused() bool
	CanFocus() bool
	IsDisabled() bool
	SetDisabled(bool)
	SetOwner(CommandHandler)
	GetOwner() CommandHandler
	GetHotkey() rune
	GetId() string
	GetHelp() string
	ProcessKey(e *vtinput.InputEvent) bool
	ProcessMouse(e *vtinput.InputEvent) bool
	HandleCommand(cmd int, args any) bool
	HandleBroadcast(cmd int, args any) bool
	Valid(cmd int) bool
	HitTest(x, y int) bool
	WantsChars() bool
	GetFocusLink() UIElement
	MoveRelative(dx, dy int)
}

// Container is an interface for elements that have child UI elements.
type Container interface {
	GetChildren() []UIElement
	GetElementAt(x, y int) UIElement
}

// DataControl is an interface for UI elements that can store and return data.
type DataControl interface {
	SetData(value any)
	GetData() any
}

// FocusContainer is an interface for UI elements that manage a focusable child.
type FocusContainer interface {
	GetFocusedItem() UIElement
}

// Highlighter defines a capability to provide syntax coloring.
type Highlighter interface {
	// Highlight processes a line of text.
	// line: text to highlight.
	// prevState: state returned by the previous line (nil for the first line).
	// baseAttr: default text attributes.
	//
	// attrs is indexed by rune: attrs[i] colours the i-th rune of line,
	// counted the way utf8.DecodeRune walks it. len(attrs) is therefore the
	// rune count of the line, except that a highlighter with nothing to say
	// may return nil, and a short slice is allowed; the rest of the line then
	// takes baseAttr.
	//
	// It is not indexed by byte, and not indexed by screen cell. The three
	// units coincided for plain text and stopped coinciding the moment text
	// carrying emoji or combining marks arrived: a grapheme cluster is one
	// cell, one or two columns, one to seven runes and up to twenty five
	// bytes. A caller that mixes the units up desynchronises at the first such
	// character and stays wrong to the end of the line, which is exactly the
	// artifact that made this contract necessary.
	//
	// The runes of one cluster need not agree on their attribute:
	// StringToCharInfoWithAttrs gives the cell to the first rune of the
	// cluster, so a mark cannot shift anything.
	Highlight(line string, prevState any, baseAttr uint64) (attrs []uint64, nextState any)
}

// HighlighterProvider defines a factory for highlighters.
type HighlighterProvider interface {
	Name() string
	// Match returns true if this provider can handle the file.
	Match(filename string, content string) bool
	// Create generates a new Highlighter instance for a specific file.
	Create(filename string, content string) Highlighter
}

// SurfaceRenderer определяет, как логический буфер CharInfo переносится на экран.
type CursorShape int

const (
	CursorShapeUnderline CursorShape = iota
	CursorShapeBlock
)

// SurfaceRenderer определяет, как логический буфер CharInfo переносится на экран.
type SurfaceRenderer interface {
	Render(buf, shadow []CharInfo, width, height int, forceRedraw bool)
	SetCursor(x, y int, visible bool, shape CursorShape)
	SetPalette(palette *[256]uint32)
	SetWindowTitle(title string)
	Flush() // Combined atomic output
}

// PeriodicRedrawRenderer lets a renderer opt out of the terminal heartbeat.
// Native hosts generally own cursor blinking and animations themselves, so
// rebuilding the complete terminal and semantic scene while idle is wasteful.
type PeriodicRedrawRenderer interface {
	WantsPeriodicRedraw() bool
}

// SemanticContext содержит контекст для генерации семантического дерева.
type SemanticContext struct {
	Width        int
	Height       int
	ActiveScreen int
}

// SemanticProvider должен быть реализован UI элементами, которые хотят
// экспортировать свое семантическое состояние для внешних GUI.
type SemanticProvider interface {
	SemanticNode(ctx *SemanticContext) map[string]any
}

// SemanticActionHandler обрабатывает действия, приходящие от внешнего GUI.
type SemanticActionHandler interface {
	HandleSemanticAction(action map[string]any) bool
}

// SemanticSceneRenderer расширяет SurfaceRenderer возможностью принимать семантическую сцену.
type SemanticSceneRenderer interface {
	SurfaceRenderer
	SetSemanticScene(scene map[string]any)
}
