package vtui

import (
	"fmt"
	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
	"golang.org/x/term"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FrameType defines the type of a frame for introspection.
type FrameType int

const (
	TypeDesktop FrameType = iota
	TypeDialog
	TypeMenu
	TypeUser
)

// WorkspaceTabMode controls how the workspace tab bar is presented.
type WorkspaceTabMode int

const (
	WorkspaceTabsAlways WorkspaceTabMode = iota
	WorkspaceTabsMultiple
	WorkspaceTabsOnCtrl
	WorkspaceTabsNever
)

// WorkspaceCtrlTabMode controls whether Ctrl+Tab cycles immediately or opens
// the existing Screens switcher and commits the selection on Ctrl release.
type WorkspaceCtrlTabMode int

const (
	WorkspaceCtrlTabDirect WorkspaceCtrlTabMode = iota
	WorkspaceCtrlTabMenu
)

type workspaceTabHit struct {
	x1, x2 int
	index  int
}

// Frame is the interface that all top-level screen objects (windows, dialogs, menus) must implement.
type Frame interface {
	ProcessKey(e *vtinput.InputEvent) bool
	ProcessMouse(e *vtinput.InputEvent) bool
	Show(scr *ScreenBuf)
	ResizeConsole(w, h int)
	GetType() FrameType
	SetExitCode(code int)
	IsDone() bool
	GetHelp() string
	IsBusy() bool // If true, FrameManager may skip the rendering phase
	HasShadow() bool
	GetKeyLabels() *KeySet
	HandleCommand(cmd int, args any) bool // Turbo Vision style command routing
	HandleBroadcast(cmd int, args any) bool
	Valid(cmd int) bool
	HitTest(x, y int) bool

	// MDI Methods
	GetMenuBar() *MenuBar
	SetPosition(x1, y1, x2, y2 int)
	GetPosition() (x1, y1, x2, y2 int)
	IsModal() bool
	GetWindowNumber() int
	SetWindowNumber(n int)
	RequestFocus() bool
	Close()
	GetTitle() string
	GetProgress() int // Returns 0-100, or -1 if no progress
}

// AppScreen represents an isolated workspace with its own frame stack.
type AppScreen struct {
	Number        int // Stable workspace number; never changes during its lifetime.
	Frames        []Frame
	CapturedFrame Frame
	Transparent   bool // Если true, под этим экраном будет рисоваться предыдущий
}

// WorkspaceTabTitleProvider lets an application provide a compact title for
// the tab strip without changing the fuller title used by the Screens menu.
type WorkspaceTabTitleProvider interface {
	GetWorkspaceTabTitle() string
}

// WorkspaceTabMarkerProvider exposes a short workspace-type marker that is
// rendered separately from the title so it can use a subdued foreground.
type WorkspaceTabMarkerProvider interface {
	GetWorkspaceTabMarker() string
}

// WorkspaceMenuInfo describes the richer, full-width representation of a
// workspace used by the Screens popup. Secondary is shown as an aligned second
// column when present (for example, the right panel path).
type WorkspaceMenuInfo struct {
	Icon      string
	Primary   string
	Secondary string
}

// WorkspaceMenuInfoProvider lets an application expose structured workspace
// information without overloading its window or compact tab title.
type WorkspaceMenuInfoProvider interface {
	GetWorkspaceMenuInfo() WorkspaceMenuInfo
}

func (s *AppScreen) GetTitle() string {
	if len(s.Frames) == 0 {
		return "Workspace"
	}
	// Возвращаем заголовок самого верхнего фрейма, очищенный от декоративных пробелов
	return strings.TrimSpace(s.Frames[len(s.Frames)-1].GetTitle())
}

// GetWorkspaceTitle returns the title of the active non-modal frame. Menus and
// dialogs are transient overlays and must not replace the host terminal's tab
// title while they are open.
func (s *AppScreen) GetWorkspaceTitle() string {
	for i := len(s.Frames) - 1; i >= 0; i-- {
		if s.Frames[i].IsModal() {
			continue
		}
		if title := strings.TrimSpace(s.Frames[i].GetTitle()); title != "" {
			return title
		}
	}
	return s.GetTitle()
}

func (s *AppScreen) getTabContentTitle() string {
	for i := len(s.Frames) - 1; i >= 0; i-- {
		if provider, ok := s.Frames[i].(WorkspaceTabTitleProvider); ok {
			if title := strings.TrimSpace(provider.GetWorkspaceTabTitle()); title != "" {
				return title
			}
		}
	}
	return s.GetTitle()
}

func (s *AppScreen) GetTabMarker() string {
	for i := len(s.Frames) - 1; i >= 0; i-- {
		if provider, ok := s.Frames[i].(WorkspaceTabMarkerProvider); ok {
			if marker := strings.TrimSpace(provider.GetWorkspaceTabMarker()); marker != "" {
				return marker
			}
		}
	}
	return ""
}

func (s *AppScreen) GetTabTitle() string {
	title := s.getTabContentTitle()
	if marker := s.GetTabMarker(); marker != "" {
		return marker + " " + title
	}
	return title
}

func (s *AppScreen) GetMenuInfo() WorkspaceMenuInfo {
	for i := len(s.Frames) - 1; i >= 0; i-- {
		if provider, ok := s.Frames[i].(WorkspaceMenuInfoProvider); ok {
			info := provider.GetWorkspaceMenuInfo()
			if strings.TrimSpace(info.Primary) != "" {
				return info
			}
		}
	}
	return WorkspaceMenuInfo{Icon: "▣", Primary: s.GetTitle()}
}

func (s *AppScreen) GetProgress() int {
	for i := len(s.Frames) - 1; i >= 0; i-- {
		if p := s.Frames[i].GetProgress(); p >= 0 {
			return p
		}
	}
	return -1
}

func (s *AppScreen) NeedsAttention() bool {
	if len(s.Frames) == 0 {
		return false
	}
	top := s.Frames[len(s.Frames)-1]
	// Проверяем флаг подавления внимания
	suppressed := false
	if bf, ok := top.(interface{ IsAttentionSuppressed() bool }); ok {
		suppressed = bf.IsAttentionSuppressed()
	}
	return top.IsModal() && !suppressed && top.GetType() != TypeMenu
}

// frameManager manages multiple screens and the main application loop.
type frameManager struct {
	Screens           []*AppScreen
	ActiveIdx         int
	activationHistory []*AppScreen

	frames         []Frame // Points to the active screen's frame stack
	scr            *ScreenBuf
	RedrawChan     chan struct{}
	TaskChan       chan func()
	taskChanIn     chan func()
	EventChan      chan *vtinput.InputEvent
	EventFilter    func(*vtinput.InputEvent) bool
	injectedEvents []*vtinput.InputEvent
	injectedMu     sync.Mutex
	OnRender       func(scr *ScreenBuf)

	pendingFar2l map[uint8]chan *vtinput.Far2lStack
	far2lMu      sync.Mutex

	// Global standard UI components
	DisabledCommands CommandSet
	MenuBar          *MenuBar
	StatusLine       *StatusLine
	KeyBar           *KeyBar

	// HideBars lets the frame on top claim the rows the global bars sit on.
	// ScreenObject.Show forces an object visible, so a frame cannot hide the
	// key bar on its own: it has to say so here.
	HideBars bool

	capturedFrame Frame // Points to the active screen's captured frame

	// Switcher State
	ctrlPressed              bool
	switcherMenu             *VMenu
	WorkspaceTabMode         WorkspaceTabMode
	WorkspaceCtrlTabMode     WorkspaceCtrlTabMode
	WorkspaceAltNumberSwitch bool
	WorkspaceTabBarAttr      uint64
	WorkspaceActiveAttr      uint64
	WorkspaceAccentAttr      uint64
	WorkspaceAttentionAttr   uint64
	workspaceColorsSet       bool
	workspaceTabHits         []workspaceTabHit
	workspaceNewTabX         int
	workspaceTabDrag         *AppScreen
	workspaceTabDragHits     []workspaceTabHit
	running                  bool

	lastMouseClickTime     time.Time
	lastMouseX, lastMouseY int
	lastMouseButton        uint32
	lastMouseClickCount    int
	lastMouseEventX        int
	lastMouseEventY        int
	mousePositionKnown     bool
	Reader                 *vtinput.Reader
	currentToast           *Toast
}

type Toast struct {
	Message string
	Expires time.Time
}

// ShowToast displays a non-blocking popup message at the top of the screen that disappears after the duration.
func ShowToast(msg string, dur time.Duration) {
	if FrameManager == nil {
		return
	}
	FrameManager.PostTask(func() {
		FrameManager.currentToast = &Toast{Message: msg, Expires: time.Now().Add(dur)}
		FrameManager.Redraw()
		go func() {
			time.Sleep(dur)
			if FrameManager != nil {
				FrameManager.Redraw()
			}
		}()
	})
}

// GetActiveToast returns the message of the currently active toast if it hasn't expired yet.
func (fm *frameManager) GetActiveToast() string {
	if fm.currentToast != nil && time.Now().Before(fm.currentToast.Expires) {
		return fm.currentToast.Message
	}
	return ""
}

// FrameManager is the global instance of the frame manager.
var FrameManager = &frameManager{}

// WorkspaceTopInset is the number of rows reserved above application frames
// for the persistent workspace tab bar.
func (fm *frameManager) WorkspaceTopInset() int {
	if fm.WorkspaceTabMode == WorkspaceTabsAlways ||
		(fm.WorkspaceTabMode == WorkspaceTabsMultiple && len(fm.Screens) > 1) {
		return 1
	}
	return 0
}

// ConfigureWorkspaceTabs applies workspace presentation and Ctrl+Tab policy.
// Frames are resized because the persistent modes can add or remove a top row.
func (fm *frameManager) ConfigureWorkspaceTabs(tabMode WorkspaceTabMode, ctrlTabMode WorkspaceCtrlTabMode) {
	if tabMode < WorkspaceTabsAlways || tabMode > WorkspaceTabsNever {
		tabMode = WorkspaceTabsMultiple
	}
	if ctrlTabMode < WorkspaceCtrlTabDirect || ctrlTabMode > WorkspaceCtrlTabMenu {
		ctrlTabMode = WorkspaceCtrlTabDirect
	}
	oldInset := fm.WorkspaceTopInset()
	fm.WorkspaceTabMode = tabMode
	fm.WorkspaceCtrlTabMode = ctrlTabMode
	if oldInset != fm.WorkspaceTopInset() {
		fm.ResizeAllScreens()
	}
	fm.Redraw()
}

// ConfigureWorkspaceAltNumberSwitch controls direct Alt+1..9 activation by
// stable workspace number. Applications opt in explicitly to avoid taking
// existing Alt combinations from embedders that do not expose workspace tabs.
func (fm *frameManager) ConfigureWorkspaceAltNumberSwitch(enabled bool) {
	fm.WorkspaceAltNumberSwitch = enabled
}

// ConfigureWorkspaceTabColors lets the host application match the tab strip
// to its theme-specific panel colors.
func (fm *frameManager) ConfigureWorkspaceTabColors(barAttr, activeAttr, accentAttr, attentionAttr uint64) {
	fm.WorkspaceTabBarAttr = barAttr
	fm.WorkspaceActiveAttr = activeAttr
	fm.WorkspaceAccentAttr = accentAttr
	fm.WorkspaceAttentionAttr = attentionAttr
	fm.workspaceColorsSet = true
	fm.Redraw()
}

// ResizeAllScreens reapplies the current terminal dimensions to every frame.
func (fm *frameManager) ResizeAllScreens() {
	if fm.scr == nil {
		return
	}
	for _, screen := range fm.Screens {
		for _, frame := range screen.Frames {
			frame.ResizeConsole(fm.scr.width, fm.scr.height)
		}
	}
	if fm.MenuBar != nil {
		top := fm.WorkspaceTopInset()
		fm.MenuBar.SetPosition(0, top, fm.scr.width-1, top)
	}
}

func (fm *frameManager) SyncCurrentScreen() {
	if len(fm.Screens) > 0 {
		fm.Screens[fm.ActiveIdx].Frames = fm.frames
		fm.Screens[fm.ActiveIdx].CapturedFrame = fm.capturedFrame
	}
}

func (fm *frameManager) screenIndex(target *AppScreen) int {
	for i, screen := range fm.Screens {
		if screen == target {
			return i
		}
	}
	return -1
}

func (fm *frameManager) rememberActiveScreen() {
	if fm.ActiveIdx < 0 || fm.ActiveIdx >= len(fm.Screens) {
		return
	}
	active := fm.Screens[fm.ActiveIdx]
	if n := len(fm.activationHistory); n == 0 || fm.activationHistory[n-1] != active {
		fm.activationHistory = append(fm.activationHistory, active)
	}
}

func (fm *frameManager) fallbackScreenIndex(defaultIdx int) int {
	for len(fm.activationHistory) > 0 {
		last := len(fm.activationHistory) - 1
		screen := fm.activationHistory[last]
		fm.activationHistory = fm.activationHistory[:last]
		if idx := fm.screenIndex(screen); idx >= 0 {
			return idx
		}
	}
	if defaultIdx >= len(fm.Screens) {
		defaultIdx = len(fm.Screens) - 1
	}
	if defaultIdx < 0 {
		defaultIdx = 0
	}
	return defaultIdx
}

func (fm *frameManager) GetActiveFrames(sIdx int) []Frame {
	if sIdx == fm.ActiveIdx {
		return fm.frames
	}
	if sIdx >= 0 && sIdx < len(fm.Screens) {
		return fm.Screens[sIdx].Frames
	}
	return nil
}

func (fm *frameManager) SwitchScreen(idx int) {
	if idx < 0 || idx >= len(fm.Screens) {
		return
	}
	if idx == fm.ActiveIdx && len(fm.frames) > 0 {
		return
	}

	// 1. Notify current screen it's losing focus
	if len(fm.frames) > 0 {
		fm.frames[len(fm.frames)-1].ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: false})
	}

	fm.SyncCurrentScreen()
	fm.rememberActiveScreen()

	// Workspace order is stable. Activation changes only the active index;
	// it never moves a workspace or changes its persistent Number.
	fm.ActiveIdx = idx
	fm.frames = fm.Screens[fm.ActiveIdx].Frames
	fm.capturedFrame = fm.Screens[fm.ActiveIdx].CapturedFrame
	DebugLog("FM: Switched to Screen %d (Workspace: %s)", fm.ActiveIdx, fm.Screens[fm.ActiveIdx].GetTitle())

	// 2. Notify new screen it's gaining focus
	if len(fm.frames) > 0 {
		fm.frames[len(fm.frames)-1].ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
	}

	fm.Redraw()
}

func (fm *frameManager) createScreen(f Frame, transparent bool) *AppScreen {
	number := 1
	for _, screen := range fm.Screens {
		if screen.Number >= number {
			number = screen.Number + 1
		}
	}
	newScreen := &AppScreen{Number: number, Frames: make([]Frame, 0, 10), Transparent: transparent}
	if !transparent {
		newScreen.Frames = append(newScreen.Frames, NewDesktop())
	}
	newScreen.Frames = append(newScreen.Frames, f)
	return newScreen
}

func (fm *frameManager) insertScreenAfterActive(screen *AppScreen) int {
	idx := fm.ActiveIdx + 1
	if idx < 0 {
		idx = 0
	}
	if idx > len(fm.Screens) {
		idx = len(fm.Screens)
	}
	fm.Screens = append(fm.Screens, nil)
	copy(fm.Screens[idx+1:], fm.Screens[idx:])
	fm.Screens[idx] = screen
	return idx
}

func (fm *frameManager) AddScreen(f Frame) {
	// If we are already shutting down or in an inconsistent state, bail out.
	if fm.Screens == nil {
		return
	}

	oldInset := fm.WorkspaceTopInset()
	fm.SyncCurrentScreen()
	newIdx := fm.insertScreenAfterActive(fm.createScreen(f, false))
	if oldInset != fm.WorkspaceTopInset() {
		fm.ResizeAllScreens()
	}
	fm.SwitchScreen(newIdx)
	fm.Redraw()
}

func (fm *frameManager) AddScreenHeadless(f Frame) {
	if fm.Screens == nil {
		return
	}
	oldInset := fm.WorkspaceTopInset()
	fm.SyncCurrentScreen()
	fm.rememberActiveScreen()
	fm.ActiveIdx = fm.insertScreenAfterActive(fm.createScreen(f, true))
	fm.frames = fm.Screens[fm.ActiveIdx].Frames
	fm.capturedFrame = nil
	if oldInset != fm.WorkspaceTopInset() {
		fm.ResizeAllScreens()
	}
	f.ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
	fm.Redraw()
}

func (fm *frameManager) AddScreenBackground(f Frame) {
	oldInset := fm.WorkspaceTopInset()
	fm.SyncCurrentScreen()
	// Place it next to its source workspace and leave focus unchanged.
	newIdx := fm.insertScreenAfterActive(fm.createScreen(f, false))
	if oldInset != fm.WorkspaceTopInset() {
		fm.ResizeAllScreens()
	}
	DebugLog("FM: Added background screen at index %d. Current ActiveIdx: %d", newIdx, fm.ActiveIdx)
	fm.Redraw()
}

// RestoreScreenNumbers restores stable display numbers in workspace order.
// New workspaces derive their number from the current maximum when created.
func (fm *frameManager) RestoreScreenNumbers(numbers []int) {
	for i, number := range numbers {
		if i >= len(fm.Screens) || number < 1 {
			continue
		}
		fm.Screens[i].Number = number
	}
}

func (fm *frameManager) CloseActiveScreen() {
	fm.CloseScreen(fm.ActiveIdx)
}

// CloseScreen closes one workspace by index. Background workspaces can be
// closed without activating them first, which is used by middle-click tabs.
func (fm *frameManager) CloseScreen(idx int) {
	if idx < 0 || idx >= len(fm.Screens) {
		return
	}
	if len(fm.Screens) <= 1 {
		fm.EmitCommand(CmQuit, nil)
		return
	}

	oldInset := fm.WorkspaceTopInset()
	fm.SyncCurrentScreen()
	closedIdx := idx
	activeScreen := fm.Screens[fm.ActiveIdx]
	closingActive := closedIdx == fm.ActiveIdx
	screenToClose := fm.Screens[closedIdx]
	for i := len(screenToClose.Frames) - 1; i >= 0; i-- {
		screenToClose.Frames[i].Close()
	}

	fm.Screens = append(fm.Screens[:closedIdx], fm.Screens[closedIdx+1:]...)
	newIdx := fm.screenIndex(activeScreen)
	if closingActive || newIdx < 0 {
		newIdx = fm.fallbackScreenIndex(closedIdx)
	}
	fm.ActiveIdx = newIdx
	fm.frames = fm.Screens[newIdx].Frames
	fm.capturedFrame = fm.Screens[newIdx].CapturedFrame
	if oldInset != fm.WorkspaceTopInset() {
		fm.ResizeAllScreens()
	}
	if closingActive && len(fm.frames) > 0 {
		fm.frames[len(fm.frames)-1].ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
	}
	fm.Redraw()
}

// GetActiveMenuBar returns the menu bar of the topmost frame that provides one,
// or the global MenuBar if none do.
func (fm *frameManager) GetActiveMenuBar() *MenuBar {
	for i := len(fm.frames) - 1; i >= 0; i-- {
		if mb := fm.frames[i].GetMenuBar(); mb != nil {
			return mb
		}
	}
	return fm.MenuBar
}

// Screen returns the ScreenBuf the frames are painted into. A frame needs it
// outside of its Show method, for example to find out whether the backend can
// display images before it offers to open a picture.
func (fm *frameManager) Screen() *ScreenBuf {
	return fm.scr
}

// Init initializes the FrameManager with a ScreenBuf.
func (fm *frameManager) Init(scr *ScreenBuf) {
	fm.scr = scr
	fm.frames = make([]Frame, 0, 10)
	fm.Screens = []*AppScreen{{Number: 1, Frames: fm.frames}}
	fm.ActiveIdx = 0
	fm.activationHistory = nil
	fm.mousePositionKnown = false
	fm.WorkspaceTabMode = WorkspaceTabsMultiple
	fm.WorkspaceCtrlTabMode = WorkspaceCtrlTabDirect
	fm.WorkspaceAltNumberSwitch = false
	fm.workspaceColorsSet = false
	fm.workspaceTabHits = nil
	fm.workspaceNewTabX = -1
	fm.workspaceTabDrag = nil
	fm.workspaceTabDragHits = nil

	if fm.RedrawChan == nil {
		fm.RedrawChan = make(chan struct{}, 1)
	}

	if fm.taskChanIn == nil {
		fm.taskChanIn = make(chan func())
		fm.TaskChan = make(chan func())

		go func() {
			var queue []func()
			for {
				if len(queue) == 0 {
					task, ok := <-fm.taskChanIn
					if !ok {
						return
					}
					queue = append(queue, task)
				} else {
					select {
					case task, ok := <-fm.taskChanIn:
						if !ok {
							return
						}
						queue = append(queue, task)
					case fm.TaskChan <- queue[0]:
						queue[0] = nil
						queue = queue[1:]
					}
				}
			}
		}()
	}

	fm.injectedEvents = make([]*vtinput.InputEvent, 0)
	SetDefaultPalette()

	fm.scr.ThemePalette = &ThemePalette

	// Hide cursor globally at start
	fm.scr.SetCursorVisible(false)

	// Ensure terminal is in a known state before sending escape sequences
	if _, ok := fm.scr.Renderer.(*AnsiRenderer); ok {
		initTerminalOS()
		// Reset terminal palette to default to clear state from possible previous crashes
		os.Stdout.WriteString("\x1b]104\x07")
	}

}

// Push adds a new frame to the top of the stack and assigns a number if it's non-modal.
func (fm *frameManager) Push(f Frame) {
	if !f.IsModal() && f.GetType() != TypeDesktop {
		// Find a free number from 1 to 9
		used := make(map[int]bool)
		for _, frame := range fm.frames {
			if frame.GetWindowNumber() > 0 {
				used[frame.GetWindowNumber()] = true
			}
		}
		for i := 1; i <= 9; i++ {
			if !used[i] {
				f.SetWindowNumber(i)
				break
			}
		}
	}

	if len(fm.frames) > 0 {
		fm.frames[len(fm.frames)-1].ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: false})
	}

	fm.frames = append(fm.frames, f)
	fm.SyncCurrentScreen() // Ensure the Screen object is aware of the new frame immediately
	f.ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
}

// PushToFrameScreen adds a frame to the screen that contains the anchor frame.
func (fm *frameManager) PushToFrameScreen(anchor Frame, f Frame) {
	fm.SyncCurrentScreen()
	for i, s := range fm.Screens {
		for _, existing := range s.Frames {
			if existing == anchor {
				if i == fm.ActiveIdx {
					// Target is active screen, use standard Push to ensure proper focus and slice update
					fm.Push(f)
				} else {
					// Target is background screen
					s.Frames = append(s.Frames, f)
					// Initialize focus state for the new frame
					f.ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
					// Auto-switch to this screen if the frame is modal (pull user attention)
					if f.IsModal() {
						fm.SwitchScreen(i)
					}
				}
				return
			}
		}
	}
	// Fallback to active screen if anchor is lost
	fm.Push(f)
}

// Flash provides visual feedback for screen transitions (fork/close).
func (fm *frameManager) Flash() {
	if fm.scr == nil {
		return
	}
	prevOverlay := fm.scr.OverlayMode
	fm.scr.SetOverlayMode(false)

	// Pure black blink
	fm.scr.FillRect(0, 0, fm.scr.width-1, fm.scr.height-1, ' ', SetRGBBoth(0, 0, 0))
	fm.scr.Flush()

	time.Sleep(30 * time.Millisecond)

	fm.scr.SetOverlayMode(prevOverlay)
	fm.Redraw()
}

// Broadcast sends a command to ALL frames in ALL screens, bypassing focus and modality.
// Returns true if at least one element handled the broadcast.
func (fm *frameManager) Broadcast(cmd int, args any) bool {
	if fm.Screens == nil {
		return false
	}
	handled := false
	for _, s := range fm.Screens {
		for i := len(s.Frames) - 1; i >= 0; i-- {
			if s.Frames[i].HandleBroadcast(cmd, args) {
				handled = true
			}
		}
	}
	if handled {
		fm.Redraw()
	}
	return handled
}

// RequestFocus moves the given frame to the top of the stack (brings it to front).
// Returns false if a modal dialog blocks the focus change.
func (fm *frameManager) RequestFocus(f Frame) bool {
	// If there's a modal dialog on top, we cannot change focus
	for i := len(fm.frames) - 1; i >= 0; i-- {
		if fm.frames[i] == f {
			break
		}
		if fm.frames[i].IsModal() {
			return false
		}
	}

	idx := -1
	for i, frame := range fm.frames {
		if frame == f {
			idx = i
			break
		}
	}

	if idx == -1 || idx == len(fm.frames)-1 {
		return true // Already on top or not found
	}

	// Tell current top frame it lost focus
	fm.frames[len(fm.frames)-1].ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: false})

	// Move the frame to the end of the slice
	fm.frames = append(fm.frames[:idx], fm.frames[idx+1:]...)
	fm.frames = append(fm.frames, f)

	// Tell new top frame it got focus
	f.ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})

	fm.Redraw()
	return true
}

// Pop removes the top-most frame from the stack.
func (fm *frameManager) Pop() {
	if len(fm.frames) > 0 {
		top := fm.frames[len(fm.frames)-1]
		if fm.capturedFrame == top {
			fm.capturedFrame = nil
		}
		fm.frames = fm.frames[:len(fm.frames)-1]
		if len(fm.frames) > 0 {
			fm.frames[len(fm.frames)-1].ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
		}
	}
}

// RemoveFrame deletes a specific frame from the stack, regardless of its position.
func (fm *frameManager) RemoveFrame(f Frame) {
	isTop := len(fm.frames) > 0 && fm.frames[len(fm.frames)-1] == f
	for i, frame := range fm.frames {
		if frame == f {
			if fm.capturedFrame == f {
				fm.capturedFrame = nil
			}
			fm.frames = append(fm.frames[:i], fm.frames[i+1:]...)
			fm.SyncCurrentScreen() // Critical: update the slice header in Screens array
			if isTop && len(fm.frames) > 0 {
				fm.frames[len(fm.frames)-1].ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
			}
			return
		}
	}
}

// PopFramesAbove removes all frames above the given frame from the stack.
// Used when a drag starts on a window, to close popups (combobox dropdowns, menus) on top.
func (fm *frameManager) PopFramesAbove(f Frame) {
	for i := len(fm.frames) - 1; i >= 0; i-- {
		if fm.frames[i] == f {
			if i < len(fm.frames)-1 {
				fm.frames = fm.frames[:i+1]
				fm.SyncCurrentScreen()
				f.ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
			}
			return
		}
	}
}

// HardRefresh clears the terminal shadow buffer and forces a complete redraw.
func (fm *frameManager) HardRefresh() {
	if fm.scr != nil {
		fm.scr.HardReset()
	}
	fm.Redraw()
}

// Redraw triggers an asynchronous redraw request.
func (fm *frameManager) Redraw() {
	select {
	case fm.RedrawChan <- struct{}{}:
		DebugLog("FM: Redraw requested")
	default:
	}
}

// PostTask schedules a function to be executed safely on the main UI thread.
func (fm *frameManager) PostTask(task func()) {
	if fm.taskChanIn != nil {
		fm.taskChanIn <- task
	}
}

// EmitCommand broadcasts a command starting from the top-most frame
// and going down the stack until a frame handles it. (Turbo Vision style)
func (fm *frameManager) EmitCommand(cmd int, args any) bool {
	if fm.DisabledCommands.IsDisabled(cmd) {
		DebugLog("COMMAND: %d is DISABLED, ignoring", cmd)
		return false
	}
	DebugLog("COMMAND: Emitting %d", cmd)
	// First, if MenuBar is active, give it a chance
	activeMenu := fm.GetActiveMenuBar()
	if activeMenu != nil && activeMenu.Active {
		if activeMenu.HandleCommand(cmd, args) {
			DebugLog("COMMAND: Handled by MenuBar")
			return true
		}
	}

	// Route down the frame stack
	for i := len(fm.frames) - 1; i >= 0; i-- {
		DebugLog("COMMAND: Checking frame %d (type %d)", i, fm.frames[i].GetType())
		if fm.frames[i].HandleCommand(cmd, args) {
			DebugLog("COMMAND: Handled by frame %d", i)
			fm.Redraw()
			return true
		}
	}
	DebugLog("COMMAND: No one handled %d", cmd)
	return false
}

// InjectEvents adds simulated input events to the front of the queue.
func (fm *frameManager) InjectEvents(events []*vtinput.InputEvent) {
	fm.injectedMu.Lock()
	fm.injectedEvents = append(fm.injectedEvents, events...)
	fm.injectedMu.Unlock()
}

// Shutdown clears all frames, effectively stopping the application loop.
func (fm *frameManager) Shutdown() {
	fm.Screens = nil
	fm.frames = nil
	fm.capturedFrame = nil
	// In tests we don't actually want to close the channel because
	// other tests might still have pending background goroutines.
}

// IsShutdown returns true if the FrameManager has been shut down explicitly.
func (fm *frameManager) IsShutdown() bool {
	return fm.Screens == nil
}

func (fm *frameManager) RegisterFar2lWaiter(id uint8) chan *vtinput.Far2lStack {
	ch := make(chan *vtinput.Far2lStack, 1)
	fm.far2lMu.Lock()
	if fm.pendingFar2l == nil {
		fm.pendingFar2l = make(map[uint8]chan *vtinput.Far2lStack)
	}
	fm.pendingFar2l[id] = ch
	fm.far2lMu.Unlock()
	return ch
}

func (fm *frameManager) UnregisterFar2lWaiter(id uint8) {
	fm.far2lMu.Lock()
	if fm.pendingFar2l != nil {
		delete(fm.pendingFar2l, id)
	}
	fm.far2lMu.Unlock()
}

// WaitFar2lResponse blocks until a response with the given ID is received,
// while pumping the event and task queues to prevent deadlocks on the UI thread.
func (fm *frameManager) WaitFar2lResponse(id uint8, timeout time.Duration) *vtinput.Far2lStack {
	ch := fm.RegisterFar2lWaiter(id)
	defer fm.UnregisterFar2lWaiter(id)

	deadline := time.Now().Add(timeout)
	for {
		select {
		case res := <-ch:
			return res
		case task := <-fm.TaskChan:
			task()
			fm.cleanupDoneFrames()
			fm.Redraw()
		case e, ok := <-fm.EventChan:
			if !ok {
				return nil
			}
			if e == nil {
				continue
			}
			fm.dispatchEvent(e, false)
		case <-time.After(10 * time.Millisecond):
			if time.Now().After(deadline) {
				DebugLog("FM: WaitFar2lResponse TIMEOUT for ID=%d", id)
				return nil
			}
		}
	}
}

// CycleWindows updates the selection in the switcher overlay
func (fm *frameManager) CycleWindows(forward bool) bool {
	if len(fm.Screens) < 2 {
		return false
	}

	if fm.switcherMenu == nil {
		if fm.GetTopFrameType() == TypeMenu && strings.TrimSpace(fm.GetTopFrame().GetTitle()) == "Screens" {
			fm.switcherMenu = fm.GetTopFrame().(*VMenu)
		} else {
			fm.showScreensMenu()
			fm.switcherMenu = fm.frames[len(fm.frames)-1].(*VMenu)
			fm.switcherMenu.SetSelectPos(fm.ActiveIdx)
		}
	}

	menu := fm.switcherMenu
	if forward {
		newPos := (menu.SelectPos + 1) % len(menu.Items)
		menu.SetSelectPos(newPos)
	} else {
		newPos := menu.SelectPos - 1
		if newPos < 0 {
			newPos = len(menu.Items) - 1
		}
		menu.SetSelectPos(newPos)
	}
	fm.Redraw()
	return true
}

func (fm *frameManager) getScreenInfo(idx int, maxTitleLen int) (prefix, title, suffix string, needsAttn bool) {
	s := fm.Screens[idx]
	rawTitle := s.GetTitle()
	needsAttn = s.NeedsAttention()
	isCurrent := (idx == fm.ActiveIdx)

	prefix = "  "
	if isCurrent && needsAttn {
		prefix = "? "
	} else if isCurrent {
		prefix = "* "
	} else if needsAttn {
		prefix = "! "
	}

	suffix = ""
	if p := s.GetProgress(); p >= 0 {
		barLen := 10
		filled := (p * barLen) / 100
		bar := "["
		for b := 0; b < barLen; b++ {
			if b < filled {
				bar += "#"
			} else {
				bar += "."
			}
		}
		suffix = " " + bar + "]"
	}

	title = TruncateMiddle(rawTitle, maxTitleLen)
	return
}

func (fm *frameManager) workspaceCounterText() string {
	if len(fm.Screens) < 2 {
		return ""
	}
	return fmt.Sprintf("[%d/%d]", fm.ActiveIdx+1, len(fm.Screens))
}

func withForeground(backgroundAttr, foregroundAttr uint64) uint64 {
	if foregroundAttr&IsFgRGB != 0 {
		return SetRGBFore(backgroundAttr, GetRGBFore(foregroundAttr))
	}
	return SetIndexFore(backgroundAttr, GetIndexFore(foregroundAttr))
}

func workspaceTitleSeparatorAttr(attr uint64) uint64 {
	if attr&IsFgRGB == 0 {
		return DimColor(attr)
	}

	foreground := GetRGBFore(attr)
	background := uint32(0)
	if attr&IsBgRGB != 0 {
		background = GetRGBBack(attr)
	}
	blend := func(foreground, background uint32) uint32 {
		return (foreground + background) / 2
	}
	return SetRGBFore(attr,
		blend((foreground>>16)&0xFF, (background>>16)&0xFF)<<16|
			blend((foreground>>8)&0xFF, (background>>8)&0xFF)<<8|
			blend(foreground&0xFF, background&0xFF),
	)
}

func (fm *frameManager) workspaceBarAttr() uint64 {
	if fm.workspaceColorsSet {
		return fm.WorkspaceTabBarAttr
	}
	return Palette[ColMenuBarItem]
}

func (fm *frameManager) workspaceActiveAttr() uint64 {
	if fm.workspaceColorsSet {
		return fm.WorkspaceActiveAttr
	}
	return Palette[ColMenuBarSelected]
}

func (fm *frameManager) workspaceNumberAttr(backgroundAttr uint64) uint64 {
	accentAttr := Palette[ColMenuBarHighlight]
	if fm.workspaceColorsSet {
		accentAttr = fm.WorkspaceAccentAttr
	}
	return withForeground(backgroundAttr, accentAttr)
}

func (fm *frameManager) workspaceAttentionAttr(backgroundAttr uint64) uint64 {
	attentionAttr := Palette[ColMenuBarSelectedHighlight]
	if fm.workspaceColorsSet {
		attentionAttr = fm.WorkspaceAttentionAttr
	}
	return withForeground(backgroundAttr, attentionAttr)
}

func (fm *frameManager) hasBackgroundAttention() bool {
	for i, screen := range fm.Screens {
		if i != fm.ActiveIdx && screen.NeedsAttention() {
			return true
		}
	}
	return false
}

func (fm *frameManager) drawWorkspaceCounter() {
	indicator := fm.workspaceCounterText()
	if indicator == "" || fm.scr == nil {
		return
	}

	baseAttr := Palette[ColMenuBarItem]
	if fm.workspaceTabsVisible() {
		baseAttr = fm.workspaceBarAttr()
	}
	currentAttr := fm.workspaceNumberAttr(baseAttr)
	if fm.hasBackgroundAttention() {
		currentAttr = fm.workspaceAttentionAttr(baseAttr)
	}

	current := strconv.Itoa(fm.ActiveIdx + 1)
	total := strconv.Itoa(len(fm.Screens))
	x := fm.scr.width - runewidth.StringWidth(indicator)
	fm.scr.Write(x, 0, StringToCharInfo("[", baseAttr))
	x++
	fm.scr.Write(x, 0, StringToCharInfo(current, currentAttr))
	x += runewidth.StringWidth(current)
	fm.scr.Write(x, 0, StringToCharInfo("/"+total+"]", baseAttr))
}

func (fm *frameManager) workspaceTabsVisible() bool {
	switch fm.WorkspaceTabMode {
	case WorkspaceTabsAlways:
		return true
	case WorkspaceTabsMultiple:
		return len(fm.Screens) > 1
	case WorkspaceTabsOnCtrl:
		return fm.ctrlPressed
	default:
		return false
	}
}

func (fm *frameManager) drawWorkspaceTabs() {
	fm.workspaceTabHits = fm.workspaceTabHits[:0]
	fm.workspaceNewTabX = -1
	if fm.scr == nil || !fm.workspaceTabsVisible() || len(fm.Screens) == 0 {
		return
	}

	baseAttr := fm.workspaceBarAttr()
	fm.scr.FillRect(0, 0, fm.scr.width-1, 0, ' ', baseAttr)
	counterWidth := runewidth.StringWidth(fm.workspaceCounterText())
	available := fm.scr.width - counterWidth
	if available <= 0 {
		return
	}

	tabsLimit := available
	if tabsLimit >= 2 {
		tabsLimit -= 2 // Reserve │+ after the compact tab sequence.
	}

	x := 0
	for i, screen := range fm.Screens {
		remaining := tabsLimit - x
		if remaining < 1 {
			break
		}
		tabsRemaining := len(fm.Screens) - i
		separatorsWidth := tabsRemaining - 1
		contentAvailable := remaining - separatorsWidth
		maxTabWidth := 0
		if contentAvailable > 0 {
			maxTabWidth = contentAvailable / tabsRemaining
		}
		if maxTabWidth < 1 {
			maxTabWidth = remaining
		}

		number := strconv.Itoa(screen.Number)
		numberWidth := runewidth.StringWidth(number)
		marker := screen.GetTabMarker()
		markerWidth := runewidth.StringWidth(marker)
		titleWidth := maxTabWidth - numberWidth - 3
		if markerWidth > 0 {
			titleWidth -= markerWidth
		}
		if titleWidth < 0 {
			titleWidth = 0
		}
		title := TruncateMiddle(screen.getTabContentTitle(), titleWidth)

		attr := baseAttr
		if i == fm.ActiveIdx {
			attr = fm.workspaceActiveAttr()
		} else if screen.NeedsAttention() {
			attr = fm.workspaceAttentionAttr(baseAttr)
		}
		numberAttr := fm.workspaceNumberAttr(attr)
		if i != fm.ActiveIdx && screen.NeedsAttention() {
			numberAttr = fm.workspaceAttentionAttr(attr)
		}
		tabStart := x
		fm.scr.Write(x, 0, StringToCharInfo(" ", attr))
		x++
		fm.scr.Write(x, 0, StringToCharInfo(number, numberAttr))
		x += numberWidth
		if marker != "" && x < tabsLimit {
			fm.scr.Write(x, 0, StringToCharInfo(marker, workspaceTitleSeparatorAttr(attr)))
			x += markerWidth
		}
		if title != "" && x < tabsLimit {
			parts := strings.Split(" "+title, "─")
			for partIndex, part := range parts {
				if part != "" {
					fm.scr.Write(x, 0, StringToCharInfo(part, attr))
					x += runewidth.StringWidth(part)
				}
				if partIndex < len(parts)-1 && x < tabsLimit {
					fm.scr.Write(x, 0, StringToCharInfo("─", workspaceTitleSeparatorAttr(attr)))
					x++
				}
			}
		}
		if x < tabsLimit {
			fm.scr.Write(x, 0, StringToCharInfo(" ", attr))
			x++
		}
		fm.workspaceTabHits = append(fm.workspaceTabHits, workspaceTabHit{x1: tabStart, x2: x - 1, index: i})
		if i < len(fm.Screens)-1 && x < tabsLimit {
			fm.scr.Write(x, 0, StringToCharInfo("│", baseAttr))
			x++
		}
	}
	if x+2 <= available {
		fm.scr.Write(x, 0, StringToCharInfo("│", baseAttr))
		fm.scr.Write(x+1, 0, StringToCharInfo("+", fm.workspaceNumberAttr(baseAttr)))
		fm.workspaceNewTabX = x + 1
	}
}

func workspaceTabAt(hits []workspaceTabHit, x int) int {
	for _, hit := range hits {
		if x <= hit.x2 {
			return hit.index
		}
	}
	if len(hits) > 0 {
		return hits[len(hits)-1].index
	}
	return -1
}

func (fm *frameManager) moveWorkspaceTab(screen *AppScreen, target int) bool {
	from := fm.screenIndex(screen)
	if from < 0 || target < 0 || target >= len(fm.Screens) || from == target {
		return false
	}
	activeScreen := fm.Screens[fm.ActiveIdx]
	if from < target {
		copy(fm.Screens[from:target], fm.Screens[from+1:target+1])
	} else {
		copy(fm.Screens[target+1:from+1], fm.Screens[target:from])
	}
	fm.Screens[target] = screen
	fm.ActiveIdx = fm.screenIndex(activeScreen)
	fm.frames = fm.Screens[fm.ActiveIdx].Frames
	fm.capturedFrame = fm.Screens[fm.ActiveIdx].CapturedFrame
	// Refresh hit targets immediately so several move events arriving before
	// the next render continue to reorder against the new visual order.
	fm.drawWorkspaceTabs()
	fm.drawWorkspaceCounter()
	fm.Redraw()
	return true
}

func (fm *frameManager) processWorkspaceTabDrag(ev *vtinput.InputEvent, mx int) bool {
	if fm.workspaceTabDrag == nil {
		return false
	}
	if ev.ButtonState == 0 {
		fm.workspaceTabDrag = nil
		fm.workspaceTabDragHits = nil
		return true
	}
	if ev.ButtonState&vtinput.FromLeft1stButtonPressed == 0 {
		fm.workspaceTabDrag = nil
		fm.workspaceTabDragHits = nil
		return true
	}
	if ev.MouseEventFlags&vtinput.MouseMoved != 0 {
		// Use the slot geometry captured on mouse-down. Re-rendering after a
		// reorder can move the boundary dramatically when a short tab trades
		// places with a long one; using the new boundary would immediately move
		// it back on the next one-pixel mouse event.
		fm.moveWorkspaceTab(fm.workspaceTabDrag, workspaceTabAt(fm.workspaceTabDragHits, mx))
	}
	return true
}

func (fm *frameManager) cycleScreensDirect(forward bool) bool {
	if len(fm.Screens) < 2 {
		return false
	}
	idx := fm.ActiveIdx - 1
	if forward {
		idx = fm.ActiveIdx + 1
	}
	if idx < 0 {
		idx = len(fm.Screens) - 1
	} else if idx >= len(fm.Screens) {
		idx = 0
	}
	fm.SwitchScreen(idx)
	return true
}

func (fm *frameManager) switchScreenNumber(number int) bool {
	for idx, screen := range fm.Screens {
		if screen.Number == number {
			if idx == fm.ActiveIdx {
				// Already on that workspace, so switching would do nothing
				// and swallowing the key costs whatever is below it. This is
				// the single workspace case in particular: it is always
				// number 1 and always active, so reporting a switch here
				// would make Alt+1 permanently unreachable for the panel's
				// quick search while Alt+2 and the rest still worked.
				return false
			}
			fm.SwitchScreen(idx)
			return true
		}
	}
	return false
}

func (fm *frameManager) showScreensMenu() {
	fm.SyncCurrentScreen()
	menu := NewVMenu(" Screens ")

	scrW := fm.GetScreenSize()
	scrH := 25
	if fm.scr != nil {
		scrH = fm.scr.height
	}
	// Silent/headless screens used by embedders and tests may not have been
	// allocated yet. Keep the popup layout valid until a real size arrives.
	if scrW < 20 {
		scrW = 40
	}

	maxNumberWidth := 1
	maxLeftWidth := 0
	maxRightWidth := 0
	maxIconWidth := 1
	infos := make([]WorkspaceMenuInfo, len(fm.Screens))
	for _, screen := range fm.Screens {
		if width := len(strconv.Itoa(screen.Number)); width > maxNumberWidth {
			maxNumberWidth = width
		}
	}
	for i, screen := range fm.Screens {
		infos[i] = screen.GetMenuInfo()
		if width := runewidth.StringWidth(infos[i].Primary); width > maxLeftWidth {
			maxLeftWidth = width
		}
		if width := runewidth.StringWidth(infos[i].Secondary); width > maxRightWidth {
			maxRightWidth = width
		}
		if width := runewidth.StringWidth(infos[i].Icon); width > maxIconWidth {
			maxIconWidth = width
		}
	}

	// number + marker + icon + primary + optional separator/secondary + frame
	contentWidth := maxNumberWidth + 3 + maxIconWidth + 1 + maxLeftWidth
	if maxRightWidth > 0 {
		contentWidth += 3 + maxRightWidth
	}
	menuW := contentWidth + 4
	if menuW < 40 {
		menuW = 40
	}
	maxMenuW := scrW - 4
	if maxMenuW < 20 {
		maxMenuW = scrW
	}
	if menuW > maxMenuW {
		menuW = maxMenuW
	}
	availablePaths := menuW - 4 - maxNumberWidth - 3 - maxIconWidth - 1
	if maxRightWidth > 0 {
		availablePaths -= 3
		if maxLeftWidth+maxRightWidth > availablePaths {
			leftShare := availablePaths / 2
			if leftShare < 4 {
				leftShare = 4
			}
			maxLeftWidth = min(maxLeftWidth, leftShare)
			maxRightWidth = max(0, availablePaths-maxLeftWidth)
		}
	} else {
		maxLeftWidth = min(maxLeftWidth, availablePaths)
	}

	for i := range fm.Screens {
		pre, _, suf, _ := fm.getScreenInfo(i, 0)
		info := infos[i]
		primary := truncateMiddleCells(info.Primary, maxLeftWidth)
		marker := strings.TrimSpace(pre)
		line := " " + marker + strings.Repeat(" ", 1-runewidth.StringWidth(marker)) + " "
		line += info.Icon + strings.Repeat(" ", maxIconWidth-runewidth.StringWidth(info.Icon)) + " "
		line += primary + strings.Repeat(" ", maxLeftWidth-runewidth.StringWidth(primary))
		if maxRightWidth > 0 {
			secondary := truncateMiddleCells(info.Secondary, maxRightWidth)
			line += " ─ " + secondary
		}
		line += suf
		menu.AddItem(MenuItem{
			AccentPrefix: fmt.Sprintf("%*d", maxNumberWidth, fm.Screens[i].Number),
			Text:         line,
			UserData:     i,
		})
	}

	menu.OnAction = func(idx int) {
		fm.SwitchScreen(menu.Items[idx].UserData.(int))
	}

	menuH := len(fm.Screens) + 2
	if menuH > 15 {
		menuH = 15
	}
	x := (scrW - menuW) / 2
	y := (scrH - menuH) / 2
	menu.SetPosition(x, y, x+menuW-1, y+menuH-1)
	fm.Push(menu)
}

func truncateMiddleCells(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	leftWidth := (width - 1) / 2
	rightWidth := width - 1 - leftWidth
	left := runewidth.Truncate(value, leftWidth, "")
	runes := []rune(value)
	start := len(runes)
	used := 0
	for start > 0 {
		cellWidth := runewidth.RuneWidth(runes[start-1])
		if used+cellWidth > rightWidth {
			break
		}
		start--
		used += cellWidth
	}
	return left + "…" + string(runes[start:])
}

func (fm *frameManager) cleanupDoneFrames() {
	oldInset := fm.WorkspaceTopInset()
	fm.SyncCurrentScreen()
	oldActiveIdx := fm.ActiveIdx
	var activeScreen *AppScreen
	if fm.ActiveIdx >= 0 && fm.ActiveIdx < len(fm.Screens) {
		activeScreen = fm.Screens[fm.ActiveIdx]
	}

	var oldTop Frame
	if len(fm.frames) > 0 {
		oldTop = fm.frames[len(fm.frames)-1]
	}

	for sIdx := len(fm.Screens) - 1; sIdx >= 0; sIdx-- {
		s := fm.Screens[sIdx]
		wasModified := false
		for i := len(s.Frames) - 1; i >= 0; i-- {
			if s.Frames[i].IsDone() {
				if s.CapturedFrame == s.Frames[i] {
					s.CapturedFrame = nil
				}
				s.Frames = append(s.Frames[:i], s.Frames[i+1:]...)
				wasModified = true
				DebugLog("FM: Frame removed from Screen %d. Remaining: %d", sIdx, len(s.Frames))
			}
		}

		// Экран считается мертвым, если:
		// 1. В нем вообще нет фреймов.
		// 2. В нем остался только Desktop, и МЫ ТОЛЬКО ЧТО закрыли в нем
		//    последнее окно (wasModified), и это НЕ единственный экран.
		isDead := len(s.Frames) == 0
		if !isDead && wasModified && len(s.Frames) == 1 && s.Frames[0].GetType() == TypeDesktop && len(fm.Screens) > 1 {
			isDead = true
		}

		if isDead {
			DebugLog("FM: Removing dead Screen %d (Total screens: %d)", sIdx, len(fm.Screens))
			fm.Screens = append(fm.Screens[:sIdx], fm.Screens[sIdx+1:]...)
		}
	}

	if len(fm.Screens) > 0 {
		if idx := fm.screenIndex(activeScreen); idx >= 0 {
			fm.ActiveIdx = idx
		} else {
			fm.ActiveIdx = fm.fallbackScreenIndex(oldActiveIdx)
		}
		fm.frames = fm.Screens[fm.ActiveIdx].Frames
		fm.capturedFrame = fm.Screens[fm.ActiveIdx].CapturedFrame

		var newTop Frame
		if len(fm.frames) > 0 {
			newTop = fm.frames[len(fm.frames)-1]
		}
		if newTop != nil && newTop != oldTop {
			newTop.ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
		}
		if oldInset != fm.WorkspaceTopInset() {
			fm.ResizeAllScreens()
		}
	} else {
		fm.Shutdown()
	}
}
func (fm *frameManager) cleanupOrphanedMenus() {
	activeMenu := fm.GetActiveMenuBar()
	if activeMenu != nil && !activeMenu.Active && activeMenu.activeSubMenu != nil {
		activeMenu.closeSub()
	}
}

// SetWindowTitle updates the terminal or application window title.
func (fm *frameManager) SetWindowTitle(title string) {
	if fm.scr != nil && fm.scr.Renderer != nil {
		fm.scr.Renderer.SetWindowTitle(title)
		fm.scr.Renderer.Flush()
	}
}

// SetWindowTitle changes the terminal or GUI window title globally.
func SetWindowTitle(title string) {
	if FrameManager != nil {
		FrameManager.SetWindowTitle(title)
	}
}

// GetTopFrameType returns the type of the topmost frame or -1 if empty.
func (fm *frameManager) GetTopFrameType() FrameType {
	if len(fm.frames) == 0 {
		DebugLog("FRAMEWORK: GetTopFrameType(), len(fm.frames) == 0")
		return -1
	}
	return fm.frames[len(fm.frames)-1].GetType()
}
func (fm *frameManager) GetTopFrame() Frame {
	if len(fm.frames) == 0 {
		return nil
	}
	return fm.frames[len(fm.frames)-1]
}

func (fm *frameManager) GetScreenSize() int {
	if fm.scr == nil {
		return 80
	}
	return fm.scr.width
}
func (fm *frameManager) GetScreenHeight() int {
	if fm.scr == nil {
		return 25
	}
	return fm.scr.height
}

// GetBackendName returns the name of the active rendering backend.
func (fm *frameManager) GetBackendName() string {
	if fm.scr == nil || fm.scr.Renderer == nil {
		return "Console"
	}
	rName := fmt.Sprintf("%T", fm.scr.Renderer)
	if strings.Contains(rName, "Gogpu") {
		return "GUI (GoGPU)"
	}
	if strings.Contains(rName, "X11") {
		return "GUI (X11)"
	}
	if strings.Contains(rName, "Wayland") {
		return "GUI (Wayland)"
	}
	return "Console"
}
func (fm *frameManager) GetSyncStats() string {
	tLen, tCap := 0, 0
	if fm.TaskChan != nil {
		tLen, tCap = len(fm.TaskChan), cap(fm.TaskChan)
	}
	eLen, eCap := 0, 0
	if fm.EventChan != nil {
		eLen, eCap = len(fm.EventChan), cap(fm.EventChan)
	}
	return fmt.Sprintf("Tasks:%d/%d, Events:%d/%d", tLen, tCap, eLen, eCap)
}

// GetTerminalSize is a variable to allow mocking terminal size in tests.
var GetTerminalSize = func() (int, int, error) {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		w, h, err = term.GetSize(int(os.Stdin.Fd()))
	}
	return w, h, err
}

func (fm *frameManager) ResizeWindow(cols, rows int) {
	if fm.scr != nil && fm.scr.Renderer != nil {
		if r, ok := fm.scr.Renderer.(interface{ ResizeWindow(int, int) }); ok {
			r.ResizeWindow(cols, rows)
		}
	}
}

func rendererWantsPeriodicRedraw(renderer SurfaceRenderer) bool {
	if renderer, ok := renderer.(PeriodicRedrawRenderer); ok {
		return renderer.WantsPeriodicRedraw()
	}
	return true
}

// Stop signals the main loop to exit.
func (fm *frameManager) Stop() {
	DebugLog("FM: Stop() requested. Deactivating menus and exiting loop.")
	// Safety: deactivate top menu before leaving to avoid stale sub-menus on return
	if fm.MenuBar != nil {
		fm.MenuBar.Active = false
	}
	fm.running = false
	// Wake up the select loop immediately
	select {
	case fm.RedrawChan <- struct{}{}:
	default:
	}
}

// Run starts the main application event loop.
func (fm *frameManager) Run(reader *vtinput.Reader) {
	DebugLog("FM: Run() ENTERED with Reader[%p]", reader)
	fm.Reader = reader
	fm.running = true
	// Restore cursor visibility on exit
	defer func() {
		if r := recover(); r != nil {
			// Note: RecordCrash now generates its own full stack dump
			DebugLog("FATAL PANIC IN RUN LOOP: %v", r)
			crashPath := RecordCrash(r, nil)
			Suspend()
			fmt.Fprintf(os.Stderr, "\n[%s] FATAL PANIC: %v\n", AppName, r)
			if crashPath != "" {
				fmt.Fprintf(os.Stderr, "[%s] Crash report saved to: %s\n", AppName, crashPath)
			}
			CleanupStderrLog()
			os.Exit(2)
		}
		fm.running = false
		fm.scr.SetCursorVisible(true)
		fm.scr.Flush()
		CleanupStderrLog()
	}()

	fm.EventChan = reader.GetEventChan()
	defer reader.Close()

	// Configure channel for tracking window resizing
	sigChan := make(chan os.Signal, 1)
	watchResizeSignal(sigChan)

	// Terminal renderers own cursor blinking and animation, while native hosts
	// can opt out and remain blocked until a real event or redraw request.
	if rendererWantsPeriodicRedraw(fm.scr.Renderer) {
		go func() {
			for fm.running {
				time.Sleep(250 * time.Millisecond)
				fm.Redraw()
			}
		}()
	}

	// Terminal size polling (handles Windows and fallback for missed SIGWINCH)
	sizeChan := make(chan struct{}, 1)
	go func() {
		lastW, lastH, _ := GetTerminalSize()
		for fm.running {
			time.Sleep(200 * time.Millisecond)
			w, h, err := GetTerminalSize()
			if err == nil && w > 0 && h > 0 && (w != lastW || h != lastH) {
				lastW, lastH = w, h
				select {
				case sizeChan <- struct{}{}:
				default:
				}
			}
		}
	}()

	handleResize := func() {
		width, height, err := GetTerminalSize()
		DebugLog("FM_RESIZE: handleResize triggered. GetTerminalSize returned: %dx%d (err: %v). Current scr: %dx%d", width, height, err, fm.scr.width, fm.scr.height)
		if err != nil {
			return // Keep existing size if we can't determine the new one
		}

		if width > 0 && height > 0 && (width != fm.scr.width || height != fm.scr.height) {

			fm.scr.AllocBuf(width, height)

			for _, s := range fm.Screens {
				for _, f := range s.Frames {
					f.ResizeConsole(width, height)
				}
			}

			// Re-dock global UI elements to the new screen edges.
			// This ensures they stay at the top/bottom regardless of whether
			// the active frame also tries to resize them.
			if fm.MenuBar != nil {
				top := fm.WorkspaceTopInset()
				fm.MenuBar.SetPosition(0, top, width-1, top)
			}
			if fm.KeyBar != nil {
				fm.KeyBar.SetPosition(0, height-1, width-1, height-1)
			}
			if fm.StatusLine != nil {
				fm.StatusLine.SetPosition(0, height-1, width-1, height-1)
			}

			fm.Redraw()
		}
	}

	// --- Main application loop ---
	// Persistent timer to avoid allocations in the drain loop
	idleTimer := time.NewTimer(time.Hour)
	if !idleTimer.Stop() {
		select {
		case <-idleTimer.C:
		default:
		}
	}
	DebugLog("FM: Entering Run loop. MenuBar set: %v, KeyBar set: %v", fm.MenuBar != nil, fm.KeyBar != nil)
	for fm.running {
		if len(fm.frames) == 0 {
			DebugLog("FM: No frames left, exiting Run loop.")
			return
		}

		fm.renderPhase()

		// 3. Event waiting (Blocking)
		var e *vtinput.InputEvent
		injected := false
		loopAgain := false

		fm.injectedMu.Lock()
		if len(fm.injectedEvents) > 0 {
			e = fm.injectedEvents[0]
			fm.injectedEvents = fm.injectedEvents[1:]
			injected = true
		}
		fm.injectedMu.Unlock()

		if !injected {
			select {
			case <-fm.RedrawChan:
				loopAgain = true
			case task := <-fm.TaskChan:
				task()
				fm.cleanupDoneFrames()
				fm.Redraw()
				loopAgain = true
			case <-sigChan:
				handleResize()
				loopAgain = true
			case <-sizeChan:
				handleResize()
				loopAgain = true
			case ev, ok := <-fm.EventChan:
				if !ok {
					DebugLog("FM: eventChan closed, exiting Run() // in Event waiting (Blocking)")
					return
				}
				e = ev
			}
		}

		if loopAgain {
			continue
		}
		if e == nil {
			continue
		}
		if e.Type == vtinput.Far2lEventType {
			// Protocol level events handled inside dispatchEvent to cover both main loop and drain loop
			fm.dispatchEvent(e, injected)
			continue
		}
		if e.Type == vtinput.ResizeEventType {
			handleResize()
			continue
		}

		if e.Type == vtinput.KeyEventType && e.KeyDown {
			DebugLog("KEY: VK=%s Char=%d Src=%s ActiveFrames=%d", vtinput.VKString(e.VirtualKeyCode), e.Char, e.InputSource, len(fm.frames))
		}

		fm.dispatchEvent(e, injected)

		// 4. Queue "Drain"
		// Burst process pending events and tasks to avoid redundant renders.
		// This naturally throttles the UI when a background thread spams updates.
		drainStart := time.Now()
		drainCount := 0
		for fm.running && !fm.IsShutdown() {
			// Prevent event flood from starving the renderer (e.g. held down key)
			if time.Since(drainStart) > 20*time.Millisecond {
				DebugLog("FM_PERF: Drain loop break due to 20ms timeout. Processed %d events/tasks.", drainCount)
				break
			}

			idleTimer.Reset(2 * time.Millisecond)
			select {
			case ev, ok := <-fm.EventChan:
				if !idleTimer.Stop() {
					select {
					case <-idleTimer.C:
					default:
					}
				}
				if !ok {
					return
				}
				if ev == nil {
					continue
				}
				if len(fm.frames) > 0 {
					fm.dispatchEvent(ev, false)
				}
				drainCount++
				continue
			case task := <-fm.TaskChan:
				if !idleTimer.Stop() {
					select {
					case <-idleTimer.C:
					default:
					}
				}
				task()
				fm.cleanupDoneFrames()
				fm.Redraw()
				drainCount++
				continue
			case <-idleTimer.C:
			}
			break
		}
	}
}

func (fm *frameManager) renderPhase() {
	if len(fm.frames) == 0 {
		return
	}
	renderPhaseStart := time.Now()
	if fm.scr != nil && fm.scr.Renderer != nil {
		// Only log periodically to avoid performance hit
		if (time.Now().UnixMilli()/1000)%5 == 0 {
			DebugLog("FM: renderPhase() for screen %dx%d, stack depth: %d, top frame: %q",
				fm.scr.width, fm.scr.height, len(fm.frames), fm.frames[len(fm.frames)-1].GetTitle())
		}
	}
	topFrame := fm.frames[len(fm.frames)-1]

	// Update global status line context automatically
	if fm.StatusLine != nil {
		topic := ""
		// Priority: Focused item's help -> Frame's help -> Menu context
		if fm.MenuBar != nil && fm.MenuBar.Active {
			topic = "menu"
		} else {
			if fc, ok := topFrame.(FocusContainer); ok {
				if foc := fc.GetFocusedItem(); foc != nil && foc.GetHelp() != "" {
					topic = foc.GetHelp()
				}
			}
			if topic == "" {
				topic = topFrame.GetHelp()
			}
		}
		fm.StatusLine.UpdateContext(topic)
	}

	// Update KeyBar content from the active frame
	if fm.KeyBar != nil {
		// Find the topmost frame that provides key labels
		for i := len(fm.frames) - 1; i >= 0; i-- {
			if ks := fm.frames[i].GetKeyLabels(); ks != nil {
				fm.KeyBar.Normal = ks.Normal
				fm.KeyBar.Shift = ks.Shift
				fm.KeyBar.Ctrl = ks.Ctrl
				fm.KeyBar.Alt = ks.Alt
				break
			}
		}
	}

	// If the frame is "busy" (e.g., mass insertion in progress), skip drawing
	// and Flush to avoid flickering and save CPU.
	if !topFrame.IsBusy() {
		// Cleanup orphaned menus safely outside the frames iteration loop
		// to avoid "index out of range" during rendering.
		fm.cleanupOrphanedMenus()

		fm.scr.Graphics().BeginFrame()
		fm.scr.SetCursorVisible(false)
		fm.scr.ActivePalette = nil
		// By default, we use OverlayMode (Early Binding) for host UI elements.
		// Desktop and TerminalView will explicitly disable it during their render.
		fm.scr.SetOverlayMode(true)

		// 1. Находим "базовый" экран (первый непрозрачный, идя назад от активного)
		baseIdx := fm.ActiveIdx
		for baseIdx > 0 && fm.Screens[baseIdx].Transparent {
			baseIdx--
		}

		// If even the bottom screen of the painted stack is transparent,
		// nothing below will paint the background: clear the pending buffer
		// so cells vacated by moved frames do not leave stale content.
		if fm.Screens[baseIdx].Transparent {
			fm.scr.ClearBuf()
		}

		// 2. Отрисовываем стэк экранов от базового до текущего
		for sIdx := baseIdx; sIdx <= fm.ActiveIdx; sIdx++ {
			frames := fm.GetActiveFrames(sIdx)
			for _, frame := range frames {
				if frame.HasShadow() {
					x1, y1, x2, y2 := frame.GetPosition()
					isFullScreen := x1 <= 0 && y1 <= 0 && x2 >= fm.scr.width-1 && y2 >= fm.scr.height-1
					if !isFullScreen {
						fm.scr.ApplyShadow(x1+2, y2+1, x2+2, y2+1)
						fm.scr.ApplyShadow(x2+1, y1+1, x2+2, y2)
					}
				}
				frame.Show(fm.scr)
			}
		}

		// Render Standard Global UI
		activeMenu := fm.GetActiveMenuBar()
		if activeMenu != nil && activeMenu.Active {
			activeMenu.Show(fm.scr)
			fm.scr.SetCursorVisible(false) // Hide underlying cursor when menu is active
		}
		if fm.KeyBar != nil {
			if fm.HideBars {
				// Hide rather than merely skip the drawing: an invisible bar
				// that still reports itself visible keeps eating clicks on
				// the bottom row in dispatchEvent.
				fm.KeyBar.Hide(fm.scr)
			} else {
				fm.KeyBar.Show(fm.scr)
			}
		}
		if fm.StatusLine != nil {
			fm.StatusLine.Show(fm.scr)
		}

		if fm.OnRender != nil {
			fm.OnRender(fm.scr)
		}
		fm.drawWorkspaceTabs()
		fm.drawWorkspaceCounter()

		// Draw the toast after the workspace tab bar so it overlays the top row
		// instead of being erased by drawWorkspaceTabs' FillRect.
		if fm.currentToast != nil {
			if time.Now().After(fm.currentToast.Expires) {
				fm.currentToast = nil
			} else {
				msg := " " + fm.currentToast.Message + " "
				attr := SetRGBBoth(0, 0xFFFFFF, 0x444444) // White on Dark Gray
				x := (fm.scr.width - runewidth.StringWidth(msg)) / 2
				if x < 0 {
					x = 0
				}
				fm.scr.Write(x, 0, StringToCharInfo(msg, attr))
			}
		}

		fm.scr.Graphics().EndFrame()
		if semanticRenderer, ok := fm.scr.Renderer.(SemanticSceneRenderer); ok {
			semanticRenderer.SetSemanticScene(fm.ExportSemanticScene())
		}

		fm.scr.Flush()
	}
	renderPhaseDur := time.Since(renderPhaseStart)
	if renderPhaseDur > 10*time.Millisecond {
		DebugLog("FM_PERF: renderPhase took %v", renderPhaseDur)
	}
}

// isDuplicateMouseMove filters backend motion notifications that do not change
// the pointer's cell position. Windows consoles can emit many such records
// while the pointer is stationary; treating the first one after a menu opens as
// hover would unexpectedly move the menu selection.
func (fm *frameManager) isDuplicateMouseMove(ev *vtinput.InputEvent) bool {
	if ev.Type != vtinput.MouseEventType {
		return false
	}

	x, y := int(ev.MouseX), int(ev.MouseY)
	isMove := ev.MouseEventFlags&vtinput.MouseMoved != 0
	duplicate := isMove && fm.mousePositionKnown && x == fm.lastMouseEventX && y == fm.lastMouseEventY
	fm.lastMouseEventX = x
	fm.lastMouseEventY = y
	fm.mousePositionKnown = true
	return duplicate
}

func (fm *frameManager) dispatchEvent(ev *vtinput.InputEvent, is_injected bool) {
	if fm.isDuplicateMouseMove(ev) {
		return
	}
	DebugLog("FM_DISPATCH: Received event: %s", ev.String())
	// Translator Tool: Ctrl+Alt+RightClick
	if ev.Type == vtinput.MouseEventType && ev.ButtonState == vtinput.RightmostButtonPressed && ev.KeyDown {
		ctrl := (ev.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
		alt := (ev.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0
		if ctrl && alt {
			mx, my := int(ev.MouseX), int(ev.MouseY)

			// Find the top-most frame under the mouse
			var hitFrame Frame
			for i := len(fm.frames) - 1; i >= 0; i-- {
				if fm.frames[i].HitTest(mx, my) {
					hitFrame = fm.frames[i]
					break
				}
			}

			if hitFrame != nil {
				var target UIElement
				if c, ok := hitFrame.(interface{ GetElementAt(x, y int) UIElement }); ok {
					target = c.GetElementAt(mx, my)
				}

				if target != nil {
					// 1. Extract text (using interfaces)
					text := ""
					if txtObj, ok := target.(interface{ GetText() string }); ok {
						text = txtObj.GetText()
					}

					// 2. Reverse lookup key
					key := ""
					if text != "" {
						key = ReverseLookup(text)
					}

					// 3. Build Help Context Stack
					var contexts []string
					if target.GetHelp() != "" {
						contexts = append(contexts, target.GetHelp())
					}
					owner := target.GetOwner()
					for owner != nil {
						if owner.GetHelp() != "" {
							contexts = append([]string{owner.GetHelp()}, contexts...) // Prepend
						}
						if obj, ok := owner.(interface{ GetOwner() CommandHandler }); ok {
							owner = obj.GetOwner()
						} else {
							break
						}
					}

					// 4. Format and copy to clipboard
					report := "--- f4 Translator Tool ---\n"
					if key != "" {
						report += fmt.Sprintf("Key:  %s\nText: %s\n", key, text)
					} else if text != "" {
						report += fmt.Sprintf("Key:  <HARDCODED>\nText: %s\n", text)
					} else {
						report += "Key:  <NO TEXT>\n"
					}

					ctxStr := "None"
					if len(contexts) > 0 {
						ctxStr = strings.Join(contexts, " -> ")
					}
					report += fmt.Sprintf("Help Context: %s\n", ctxStr)

					SetClipboard(report)
					ShowToast("Translator info copied to clipboard", 3*time.Second)
					return // Consume event
				}
			}
		}
	}
	RecordEvent(ev.String())
	if ev.Type == vtinput.Far2lEventType {
		DebugLog("FM_DISPATCH: Processing Far2l event: cmd=%q", ev.Far2lCommand)
		if ev.Far2lCommand == "ok" {
			DebugLog("FM_DISPATCH: Far2l extensions successfully negotiated with host. Setting Far2lEnabled = true")
			Far2lEnabled = true
			return
		}
		if ev.Far2lCommand == "reply" {
			DebugLog("FM_DISPATCH: Processing Far2l reply...")
			stk := vtinput.Far2lStack(ev.Far2lData)
			id := stk.PopU8()
			fm.far2lMu.Lock()
			if fm.pendingFar2l != nil {
				if ch, ok := fm.pendingFar2l[id]; ok {
					ch <- &stk
					delete(fm.pendingFar2l, id)
				}
			}
			fm.far2lMu.Unlock()
			return
		}

		// Interaction requests (from remote terminal to app) are handled by the active frame
		// (usually PanelsFrame) which manages the terminal view.
	}

	if len(fm.frames) == 0 {
		return
	}

	fm.markMultiClick(ev, time.Now())

	topFrame := fm.frames[len(fm.frames)-1]
	activeMenu := fm.GetActiveMenuBar()
	// Track input for XLat transliteration
	if ev.Type == vtinput.KeyEventType && ev.KeyDown && ev.Char != 0 {
		GlobalXlator.Track(ev.Char)
	}

	// Update KeyBar modifiers automatically if present.
	// Reset modifiers to a clean state on FocusEvents to prevent modifiers
	// from getting stuck during focus transitions (such as system layout switching).
	if fm.KeyBar != nil {
		if ev.Type == vtinput.FocusEventType {
			fm.KeyBar.SetModifiers(false, false, false)
		} else {
			shift := (ev.ControlKeyState & vtinput.ShiftPressed) != 0
			ctrl := (ev.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
			alt := (ev.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0

			// Workaround for X11/macOS where the event's modifier state reflects the
			// logical state *prior* to the keypress/keyrelease of the modifier itself.
			if ev.Type == vtinput.KeyEventType {
				if ev.VirtualKeyCode == vtinput.VK_SHIFT || ev.VirtualKeyCode == vtinput.VK_LSHIFT || ev.VirtualKeyCode == vtinput.VK_RSHIFT {
					shift = ev.KeyDown
				}
				if ev.VirtualKeyCode == vtinput.VK_CONTROL || ev.VirtualKeyCode == vtinput.VK_LCONTROL || ev.VirtualKeyCode == vtinput.VK_RCONTROL {
					ctrl = ev.KeyDown
				}
				if ev.VirtualKeyCode == vtinput.VK_MENU || ev.VirtualKeyCode == vtinput.VK_LMENU || ev.VirtualKeyCode == vtinput.VK_RMENU {
					alt = ev.KeyDown
				}
			}

			fm.KeyBar.SetModifiers(shift, ctrl, alt)
		}
	}

	// User-defined filter has first say
	if !is_injected && fm.EventFilter != nil && fm.EventFilter(ev) {
		DebugLog("FM_DISPATCH: Event CONSUMED by EventFilter (Macro?).")
		// Filters may execute actions that close a frame. Preserve the normal
		// end-of-dispatch cleanup even though the event itself is consumed.
		fm.cleanupDoneFrames()
		return
	}

	// Track Ctrl state for Switcher logic
	if ev.Type == vtinput.KeyEventType {
		wasCtrlPressed := fm.ctrlPressed
		ctrl := (ev.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
		if ev.VirtualKeyCode == vtinput.VK_CONTROL || ev.VirtualKeyCode == vtinput.VK_LCONTROL || ev.VirtualKeyCode == vtinput.VK_RCONTROL {
			ctrl = ev.KeyDown
		}
		fm.ctrlPressed = ctrl
		if wasCtrlPressed != fm.ctrlPressed && fm.WorkspaceTabMode == WorkspaceTabsOnCtrl {
			fm.Redraw()
		}

		// Commit Switcher selection on Ctrl release
		if !fm.ctrlPressed && fm.switcherMenu != nil {
			if !fm.switcherMenu.IsDone() {
				idx := fm.switcherMenu.SelectPos
				if idx >= 0 && idx < len(fm.switcherMenu.Items) {
					userData := fm.switcherMenu.Items[idx].UserData.(int)
					fm.switcherMenu.Close()
					fm.SwitchScreen(userData)
				}
			}
			fm.switcherMenu = nil
		}
	}

	// --- Menu Interception ---
	if ev.Type == vtinput.KeyEventType && ev.KeyDown {

		DebugLog("INPUT: KeyPress VK=%s Char=%d (Stack: %d frames, ActiveIdx: %d)", vtinput.VKString(ev.VirtualKeyCode), ev.Char, len(fm.frames), fm.ActiveIdx)

		// 1. If Menu is Active, it has priority.
		// We allow it even if topFrame is modal, provided topFrame IS the menu itself
		// or the frame that owns the menu.
		isMenuRelated := topFrame.GetType() == TypeMenu || topFrame.GetMenuBar() == activeMenu
		if activeMenu != nil && activeMenu.Active && (!topFrame.IsModal() || isMenuRelated) {
			// Exception: if a VMenu is open, it MUST handle navigation keys
			if fm.GetTopFrameType() == TypeMenu {
				menuFrame := fm.frames[len(fm.frames)-1]
				if menuFrame.ProcessKey(ev) {
					if menuFrame.IsDone() {
						fm.RemoveFrame(menuFrame)
					}
					return
				}
			}
			// Otherwise, MenuBar processes keys (Arrows, Esc, Hotkeys)
			if ev.VirtualKeyCode == vtinput.VK_ESCAPE || ev.VirtualKeyCode == vtinput.VK_F10 {
				activeMenu.Active = false
				return
			}
			if activeMenu.ProcessKey(ev) {
				return
			}
			return // Don't pass keys to background frames when menu is active
		}
	} else if ev.Type == vtinput.KeyEventType && !ev.KeyDown {
		DebugLog("INPUT: KeyRelease VK=%s Char=%d (Stack: %d frames, ActiveIdx: %d)", vtinput.VKString(ev.VirtualKeyCode), ev.Char, len(fm.frames), fm.ActiveIdx)
	}

	// Alt+1..9 selects the workspace whose stable displayed number matches the
	// digit. In the transient Ctrl-only tab-bar mode, Ctrl+Alt+1..9 does the
	// same: Ctrl is already being held to reveal the bar, so requiring its
	// release before choosing a tab would make the visible shortcuts awkward.
	// Menus retain priority above, and a missing number falls through so
	// applications and terminals can keep using that combination.
	if fm.WorkspaceAltNumberSwitch && ev.Type == vtinput.KeyEventType && ev.KeyDown &&
		ev.VirtualKeyCode >= vtinput.VK_1 && ev.VirtualKeyCode <= vtinput.VK_9 {
		mods := ev.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed |
			vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed | vtinput.ShiftPressed)
		alt := mods&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
		ctrl := mods&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
		shift := mods&vtinput.ShiftPressed != 0
		validModifiers := alt && !shift && (!ctrl || fm.WorkspaceTabMode == WorkspaceTabsOnCtrl)
		if validModifiers {
			if fm.switchScreenNumber(int(ev.VirtualKeyCode - vtinput.VK_0)) {
				return
			}
		}
	}

	// 3. Regular Dispatch (MDI Hit-Testing)
	handled := false

	if ev.Type == vtinput.KeyEventType || ev.Type == vtinput.PasteEventType || ev.Type == vtinput.FocusEventType {
		handled = topFrame.ProcessKey(ev)
		DebugLog("FM_DISPATCH: TopFrame.ProcessKey handled=%v", handled)
	} else if ev.Type == vtinput.MouseEventType {
		mx, my := int(ev.MouseX), int(ev.MouseY)
		if ev.ButtonState != 0 || ev.WheelDirection != 0 {
			DebugLog("FM: Mouse Event at (%d,%d) State:%X Wheel:%d", mx, my, ev.ButtonState, ev.WheelDirection)
		}
		// Workspace-tab dragging is global capture: once a press starts on a
		// tab, no part of the held gesture may leak into a frame or menu. Handle
		// it before bounds checks so releasing outside the window still clears
		// the capture.
		if fm.processWorkspaceTabDrag(ev, mx) {
			return
		}

		if mx < -1 || my < -1 {
			return
		}
		if fm.scr != nil && fm.scr.width > 0 && fm.scr.height > 0 {
			if mx > fm.scr.width || my > fm.scr.height {
				return
			}
		}

		// 3.1. Active Mouse Capture (Dragging/Resizing)
		if fm.capturedFrame != nil {
			handled = fm.capturedFrame.ProcessMouse(ev)
			if ev.ButtonState == 0 {
				fm.capturedFrame = nil // Release capture
			}
		} else {
			if my == 0 && ev.KeyDown && ev.ButtonState&vtinput.FromLeft2ndButtonPressed != 0 {
				for _, hit := range fm.workspaceTabHits {
					if mx >= hit.x1 && mx <= hit.x2 {
						fm.CloseScreen(hit.index)
						return
					}
				}
			}
			// 3.1.2. Hit test for workspace counter [current/total] and tabs.
			if len(fm.Screens) > 1 && my == 0 {
				indicatorLen := runewidth.StringWidth(fm.workspaceCounterText())
				if mx >= fm.scr.width-indicatorLen && mx < fm.scr.width {
					if ev.ButtonState == vtinput.FromLeft1stButtonPressed && ev.KeyDown {
						fm.showScreensMenu()
						return
					}
				}
			}
			if my == 0 && ev.ButtonState == vtinput.FromLeft1stButtonPressed && ev.KeyDown {
				if mx == fm.workspaceNewTabX {
					fm.EmitCommand(CmResize, "fork")
					return
				}
				for _, hit := range fm.workspaceTabHits {
					if mx >= hit.x1 && mx <= hit.x2 {
						fm.workspaceTabDrag = fm.Screens[hit.index]
						fm.workspaceTabDragHits = append(fm.workspaceTabDragHits[:0], fm.workspaceTabHits...)
						fm.SwitchScreen(hit.index)
						return
					}
				}
			}
			// 3.1.5. Global UI components hit-testing (MenuBar, KeyBar)
			if fm.KeyBar != nil && fm.KeyBar.IsVisible() && fm.KeyBar.HitTest(mx, my) {
				if fm.KeyBar.ProcessMouse(ev) {
					return
				}
			}
			canActivateMenu := !topFrame.IsModal() || topFrame.GetType() == TypeMenu || topFrame.GetMenuBar() == activeMenu
			if activeMenu != nil && canActivateMenu && activeMenu.HitTest(mx, my) {
				if activeMenu.ProcessMouse(ev) {
					return
				}
			}

			// 3.2. Mouse Hit-Testing: check frames from top to bottom
			for i := len(fm.frames) - 1; i >= 0; i-- {
				f := fm.frames[i]

				// Desktop always gets mouse if nothing above it handled it
				if f.GetType() == TypeDesktop {
					handled = f.ProcessMouse(ev)
					if handled && ev.ButtonState != 0 {
						fm.capturedFrame = f
					}
					break
				}

				// Account for shadow dimensions in the hit test (+2 X, +1 Y)
				x1, y1, x2, y2 := f.GetPosition()
				hitShadow := f.HasShadow() && mx >= x1 && mx <= x2+2 && my >= y1 && my <= y2+1

				if f.HitTest(mx, my) || hitShadow {
					if i != len(fm.frames)-1 {
						DebugLog("FM: Mouse hit background frame %d (type %d), requesting focus.", i, f.GetType())
						// Try to bring it to front before passing the event
						if fm.RequestFocus(f) {
							handled = f.ProcessMouse(ev)
						}
					} else {
						handled = f.ProcessMouse(ev)
					}

					// If a frame handled a click, it captures the mouse until release
					if handled && ev.ButtonState != 0 {
						fm.capturedFrame = f
					}

					// If the frame is modal, it eats the click even if it didn't handle it
					// (to prevent clicking on windows behind it)
					if f.IsModal() || handled {
						break
					}
				}

				if f.IsModal() {
					// Logic for clicking OUTSIDE of a modal frame:
					// LMB -> ESC (cancel).
					// RMB -> ENTER (confirm the default action) for a
					//        regular dialog, but ESC for a menu — an
					//        outside click on a drop-down / context
					//        menu should dismiss, not silently activate
					//        whichever row happens to be under the
					//        cursor (typically the first item, e.g.
					//        "Other panel" for f4's drive menu — the
					//        exact symptom reported in unxed/f4#396).
					if ev.KeyDown && ev.ButtonState != 0 {
						if ev.ButtonState == vtinput.FromLeft1stButtonPressed {
							f.ProcessKey(&vtinput.InputEvent{
								Type:           vtinput.KeyEventType,
								KeyDown:        true,
								VirtualKeyCode: vtinput.VK_ESCAPE,
							})
						} else if ev.ButtonState == vtinput.RightmostButtonPressed {
							vk := uint16(vtinput.VK_RETURN)
							if f.GetType() == TypeMenu {
								vk = vtinput.VK_ESCAPE
							}
							f.ProcessKey(&vtinput.InputEvent{
								Type:           vtinput.KeyEventType,
								KeyDown:        true,
								VirtualKeyCode: vk,
							})
						}
					}
					break
				}
			}
		}
	}

	// 3. Fallbacks (F9, Alt+Hotkey, Global Shortcuts) if top frame didn't want the key
	if !handled && ev.Type == vtinput.KeyEventType && ev.KeyDown {

		// Workspace cycling (Ctrl+Tab / Ctrl+Shift+Tab).
		if ev.VirtualKeyCode == vtinput.VK_TAB && (fm.ctrlPressed || fm.switcherMenu != nil) {
			shift := (ev.ControlKeyState & vtinput.ShiftPressed) != 0
			cycled := false
			if fm.WorkspaceCtrlTabMode == WorkspaceCtrlTabMenu || fm.WorkspaceTabMode == WorkspaceTabsNever {
				cycled = fm.CycleWindows(!shift)
			} else {
				cycled = fm.cycleScreensDirect(!shift)
			}
			if cycled {
				return
			}
		}
		// Screen Dump (Ctrl+Shift+P)
		if ev.VirtualKeyCode == 'P' && fm.ctrlPressed && (ev.ControlKeyState&vtinput.ShiftPressed) != 0 {
			home, _ := os.UserHomeDir()
			if home != "" {
				dumpPath := filepath.Join(home, "vtui.screen.log")
				f, err := os.Create(dumpPath)
				if err == nil {
					fm.scr.Dump(f)
					f.Close()
					DebugLog("FM: Screen dump saved to %s", dumpPath)
				}
			}
			return
		}

		// Ctrl+N - Fork Active Frame into new Screen
		if ev.VirtualKeyCode == vtinput.VK_N && fm.ctrlPressed {
			fm.Flash()
			// We need a way to clone the top-level frame.
			// For now, let's trigger a Command that Panels can handle.
			fm.EmitCommand(CmResize, "fork") // Temporary hack or use specialized Command
			return
		}

		// Ctrl+W - Close Active Screen
		if ev.VirtualKeyCode == vtinput.VK_W && fm.ctrlPressed {
			fm.Flash()
			fm.CloseActiveScreen()
			return
		}

		// F12 - Screens Menu (Window List)
		// We must ignore NumLock, CapsLock, and EnhancedKey flags
		modifierMask := vtinput.ControlKeyState(vtinput.LeftAltPressed | vtinput.RightAltPressed | vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed | vtinput.ShiftPressed)
		if ev.VirtualKeyCode == vtinput.VK_F1 && (ev.ControlKeyState&modifierMask) == 0 {
			DebugLog("FM: F1 triggered Help for topic context.")
			topic := topFrame.GetHelp()
			if fc, ok := topFrame.(FocusContainer); ok {
				if foc := fc.GetFocusedItem(); foc != nil && foc.GetHelp() != "" {
					topic = foc.GetHelp()
				}
			}
			if topic == "" {
				topic = "Contents"
			}
			if topic != "" && GlobalHelpEngine != nil {
				hv := NewHelpView(GlobalHelpEngine, topic)
				fm.Push(hv)
				return
			}
		}
		if ev.VirtualKeyCode == vtinput.VK_F12 && (ev.ControlKeyState&modifierMask) == 0 {
			if fm.GetTopFrameType() != TypeMenu {
				fm.showScreensMenu()
				return
			}
		}

		// Allow F9 if not modal, OR if the modal frame itself has a menu
		canActivateMenu := !topFrame.IsModal() || topFrame.GetType() == TypeMenu || topFrame.GetMenuBar() != nil
		if ev.VirtualKeyCode == vtinput.VK_F9 {
			if activeMenu == nil {
				DebugLog("FM: F9 pressed but activeMenu is NIL")
			} else if activeMenu.Active {
				DebugLog("FM: F9 pressed but Menu is already active")
			} else if !canActivateMenu {
				DebugLog("FM: F9 pressed but Menu activation blocked by modal frame")
			} else {
				DebugLog("FM: F9 accepted, activating menu")
				activeMenu.Active = true
				if len(activeMenu.Items) > 0 {
					if activeMenu.SelectPos < 0 || activeMenu.SelectPos >= len(activeMenu.Items) {
						activeMenu.SelectPos = 0
					}
					activeMenu.ActivateSubMenu(activeMenu.SelectPos)
				}
				return
			}
		}
		if activeMenu != nil && !activeMenu.Active && canActivateMenu {
			alt := (ev.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0
			if alt && ev.Char != 0 {
				if activeMenu.ProcessKey(ev) {
					return
				}
				DebugLog("FM: Hotkey Alt+%c matched MenuBar item.", ev.Char)
			}
		}
	}

	// 4. Cleanup: Remove all frames that are marked as done.
	fm.cleanupDoneFrames()
}

func (fm *frameManager) markMultiClick(ev *vtinput.InputEvent, now time.Time) {
	if ev.Type != vtinput.MouseEventType || ev.ButtonState == 0 || !ev.KeyDown || ev.MouseEventFlags&vtinput.MouseMoved != 0 {
		return
	}

	sameClick := ev.ButtonState == fm.lastMouseButton &&
		int(ev.MouseX) == fm.lastMouseX && int(ev.MouseY) == fm.lastMouseY &&
		now.Sub(fm.lastMouseClickTime) < 400*time.Millisecond
	if sameClick {
		fm.lastMouseClickCount++
	} else {
		fm.lastMouseClickCount = 1
	}

	fm.lastMouseButton = ev.ButtonState
	fm.lastMouseX = int(ev.MouseX)
	fm.lastMouseY = int(ev.MouseY)
	fm.lastMouseClickTime = now

	switch fm.lastMouseClickCount {
	case 2:
		ev.MouseEventFlags |= vtinput.DoubleClick
		DebugLog("FM: DoubleClick generated at (%d,%d)", ev.MouseX, ev.MouseY)
	case 3:
		ev.MouseEventFlags &^= vtinput.DoubleClick
		ev.MouseEventFlags |= TripleClick
		fm.lastMouseClickCount = 0
		DebugLog("FM: TripleClick generated at (%d,%d)", ev.MouseX, ev.MouseY)
	}
}
