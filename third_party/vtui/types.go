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

// SemanticSceneIncrementalRenderer gives a native renderer the first chance to
// publish an authoritative semantic delta without walking every screen/frame.
// Returning true means the current semantic state is fully accounted for,
// including the no-change case. Returning false requests the ordinary complete
// ExportSemanticScene fallback.
type SemanticSceneIncrementalRenderer interface {
	SemanticSceneRenderer
	SetSemanticSceneIncremental(ctx *SemanticContext) bool
}

// SemanticSceneTransitionRenderer accepts the complete bounded semantic state
// after FrameManager has proved that an input transaction changed the non-menu
// frame stack. This is the latency-sensitive path for opening or closing a
// document: implementations may publish the exact scene transition before the
// regular render starts, without walking hidden frame trees or panel catalogs.
//
// Returning true means the renderer has already delivered (or proved
// unchanged) the visible result and may suppress the following render/export.
// Returning false keeps the ordinary conservative render path.
type SemanticSceneTransitionRenderer interface {
	SemanticSceneRenderer
	SetSemanticSceneTransition(ctx *SemanticContext) bool
}

// SemanticInputUnchangedRenderer accepts an explicit application proof that
// the current input transaction scheduled future work but made no immediate
// visible or semantic change. Native renderers may use it to omit the otherwise
// automatic render between an asynchronous request and its authoritative UI
// completion task.
type SemanticInputUnchangedRenderer interface {
	SemanticSceneRenderer
	SetSemanticInputUnchanged() bool
}

// SemanticMenuStateRenderer accepts the complete bounded menu/global-chrome
// state after FrameManager has proved that an input transaction did not invoke
// a menu item or mutate a non-menu frame stack. Returning true means the
// renderer has already delivered (or proved unchanged) that visible result and
// may suppress the following complete semantic export.
//
// Implementations must not inspect frame SemanticProvider trees from this
// method: the capability exists specifically so popup interaction is
// independent of large panel catalogs and documents.
type SemanticMenuStateRenderer interface {
	SemanticSceneRenderer
	SetSemanticMenuState(ctx *SemanticContext) bool
}

// SemanticSceneExportSuppressor lets a semantic renderer replace exactly one
// full semantic export with an authoritative compact update it has already
// queued. FrameManager still paints and flushes the cell surface; only the
// expensive semantic tree traversal is omitted.
//
// ConsumeSemanticSceneExportSuppression must be one-shot. Returning true for
// one render must not suppress a later render unless the renderer explicitly
// arms another compact update.
type SemanticSceneExportSuppressor interface {
	ConsumeSemanticSceneExportSuppression() bool
}

// SemanticRenderPhaseDeferrer lets a native semantic renderer omit exactly
// one render phase after it has already delivered the complete visible state.
// Unlike SemanticSceneExportSuppressor, this also skips Frame.Show, cell
// composition, and Flush. It is intended only for native surfaces whose cell
// grid is hidden; terminal and fallback renderers must not implement it unless
// they can make the same guarantee.
//
// BindSemanticRenderPhaseDeferral associates an armed direct update with the
// redraw generation observed after its complete input/task mutation boundary.
// ConsumeSemanticRenderPhaseDeferral must be one-shot and return true only for
// that exact generation. A later redraw, input, or task must render normally
// unless the renderer proves and arms another complete direct update.
type SemanticRenderPhaseDeferrer interface {
	BindSemanticRenderPhaseDeferral(redrawGeneration uint64)
	ConsumeSemanticRenderPhaseDeferral(redrawGeneration uint64) bool
}

// SemanticSceneUpdateTracker brackets input dispatches and queued UI tasks.
// A renderer may use the boundary to invalidate a compact update when another
// unverified model mutation is processed before the next semantic export.
type SemanticSceneUpdateTracker interface {
	BeginSemanticSceneUpdate()
	EndSemanticSceneUpdate()
}

// SemanticSceneUnchangedUpdateTracker lets a renderer close a task mutation
// boundary without invalidating already-proven compact updates when the task
// has established that its visible and semantic state did not change. The
// bool result is false when the renderer observed any mutation inside the
// boundary and therefore requires the ordinary conservative render path.
//
// FrameManager only omits a task-owned redraw when this optional capability is
// present and accepts the unchanged boundary. Renderers which do not implement
// it retain the historical redraw-after-every-task behavior.
type SemanticSceneUnchangedUpdateTracker interface {
	EndSemanticSceneUpdateUnchanged() bool
}

// CoveredTerminalRedrawDeferrer lets a native semantic renderer prove that a
// terminal-output update is currently hidden behind the application's native
// surface. Callers may omit only the redraw requested for the terminal bytes
// themselves; any concurrent redraw, semantic mutation, task, or later reveal
// retains its normal render path. Cell, text, and fallback renderers should not
// implement this capability unless they provide the same visibility guarantee.
type CoveredTerminalRedrawDeferrer interface {
	CanDeferCoveredTerminalRedraw() bool
}

// SemanticBenchmarkHooks optionally observes the semantic render/export
// boundaries. Applications leave this nil in normal operation; benchmark
// builds can install callbacks without making vtui depend on an application
// tracing package.
type SemanticBenchmarkHooks struct {
	RenderBegin func()
	ExportBegin func()
	ExportEnd   func(scene map[string]any)
	RenderEnd   func()
}

// SemanticSceneBenchmarkHooks is intentionally nil unless an application
// explicitly enables semantic-scene benchmarking.
var SemanticSceneBenchmarkHooks *SemanticBenchmarkHooks

// FrameManagerBenchmarkHooks optionally observes redraw scheduling and task /
// render boundaries. The variadic field shape keeps vtui independent of a
// particular trace schema; hooks are nil in ordinary sessions, so caller
// discovery and event construction stay completely off the production path.
type FrameManagerBenchmarkHooks struct {
	Event func(event string, fields ...any)
}

// FrameManagerLifecycleBenchmarkHooks is intentionally nil unless an
// application explicitly enables scheduling diagnostics.
var FrameManagerLifecycleBenchmarkHooks *FrameManagerBenchmarkHooks

// InputBenchmarkHooks optionally observes the complete FrameManager dispatch
// boundary for an input event. Applications can associate metadata with the
// event pointer without adding application-specific fields to vtinput.
type InputBenchmarkHooks struct {
	DispatchBegin func(event *vtinput.InputEvent)
	DispatchEnd   func(event *vtinput.InputEvent)
}

// InputEventBenchmarkHooks is intentionally nil unless an application enables
// input pipeline benchmarking.
var InputEventBenchmarkHooks *InputBenchmarkHooks
