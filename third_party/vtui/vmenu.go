package vtui

import (
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
)

// MenuItem represents a single menu item.
type MenuItem struct {
	// Details contains named display fields for native renderers. Console Text
	// remains authoritative for terminal layout and keyboard mnemonics.
	Details map[string]string

	// ID is a stable identity used to preserve selection while an asynchronous
	// menu snapshot is replaced. It is optional for legacy callers.
	ID string
	// AccentPrefix is drawn immediately before Text using the menu highlight
	// color. It is useful for non-hotkey metadata such as stable item numbers.
	AccentPrefix string
	Text         string
	// Icon is an optional semantic icon name for graphical frontends. The
	// terminal renderer deliberately ignores it and keeps the classic layout.
	Icon string
	// IconColor is an optional graphical-frontend color (for example a Finder
	// tag color). Terminal rendering keeps using the configured menu palette.
	IconColor   string
	Description string
	SubItems    []MenuItem
	Shortcut    string // Optional right-aligned hotkey hint (e.g. "F3")
	Command     int    // TV-style Command ID to emit when selected
	OnClick     func() // Closure called when selected
	UserData    any
	Separator   bool
	Header      bool
	Disabled    bool
	// KeepOpen leaves the menu chain active after invoking this leaf. It is
	// useful for Retry/refresh commands that replace rows asynchronously.
	KeepOpen bool
	// Submenu creates the anchored child lazily. A fresh child is requested on
	// every opening so callers can start an asynchronous refresh at that point.
	Submenu func() *VMenu
	// SubmenuFrame is the custom-frame counterpart to Submenu. The returned
	// frame must expose its embedded menu through MenuFrameProvider. Keeping the
	// actual frame in the stack preserves custom keyboard, mouse, rendering, and
	// close behavior while VMenu continues to own the common cascade lifecycle.
	// If both factories are set, SubmenuFrame takes precedence.
	SubmenuFrame func() Frame
}

// MenuFrameProvider exposes the VMenu control hosted by a custom menu frame.
// VMenu implements this itself, and a frame embedding *VMenu inherits the
// implementation automatically.
type MenuFrameProvider interface {
	MenuControl() *VMenu
}

// menuItemDisabled reports whether item is unavailable: either the caller
// marked it Disabled directly, or its Command id is one FrameManager.
// DisabledCommands currently disables. Both dim and both block activation
// the same way, so every call site that used to test DisabledCommands alone
// tests this instead.
func menuItemDisabled(item MenuItem) bool {
	return item.Disabled || FrameManager.DisabledCommands.IsDisabled(item.Command)
}

// VMenu implements a vertical menu with navigation support.
type VMenu struct {
	ScrollView
	title string
	// bottomTitle is drawn centred on the lower border, where far2l's
	// VMenu::SetBottomTitle puts a menu's key hints.
	bottomTitle     string
	bottomTextLines int
	TruncateMark    string
	Items           []MenuItem
	// SemanticBottomHint exposes a custom renderer's footer to native frontends.
	SemanticBottomHint string
	// SemanticPresentation selects an optional native menu layout.
	SemanticPresentation string
	windowFrame          *Window
	// SemanticItemsRevision opts into immutable native row snapshots. Owners
	// must increment it after every row/content change; zero disables reuse.
	SemanticItemsRevision uint64
	done                  bool
	exitCode              int
	// selectAtOpen is SelectPos as of the last ClearDone. Browsing moves
	// SelectPos live (arrows, mouse hover), so cancelling has to put it
	// back: dialogs read SelectPos as the confirmed choice, and without the
	// restore an Esc'd dropdown silently commits whatever row the user
	// happened to stop on.
	mouseSelecting bool
	selectAtOpen   int
	OnAction       func(int)
	OnKeyDown      func(*vtinput.InputEvent) bool
	HideShadow     bool
	// IgnoreSingleClick keeps a single left click from confirming the menu;
	// a double click still confirms it.
	// DisableFilter turns off the optional item filter for consumers that own
	// their rows and key handling, such as the Colorer outline.
	BoxType int
	// OnClose is invoked exactly once for one shown lifetime. Dynamic menus use
	// it to cancel native requests and live queries when their chain closes.
	OnClose func()

	parentMenu    *VMenu
	parentFrame   Frame
	parentIndex   int
	childMenu     *VMenu
	childFrame    Frame
	childIndex    int
	hostFrame     Frame
	closeNotified bool
	hoverMu       sync.Mutex
	hoverTimer    *time.Timer
	hoverGen      uint64

	// parentMenu and activeSub link a chain of nested menus. Only the
	// deepest one is on top of the frame stack and sees input, so closing
	// or confirming has to walk the chain explicitly: a submenu left
	// behind would keep painting over the screen with nothing to close it.
	activeSub *VMenu

	// Palette entries the menu paints with. They default to the Menu.* group;
	// a ComboBox points them at Dialog.Combo.* so its dropdown stands apart
	// from the dialog underneath it.
	ColorTextIdx              int
	ColorSelectedTextIdx      int
	ColorHighlightIdx         int
	ColorSelectedHighlightIdx int
	ColorBoxIdx               int
	ColorTitleIdx             int

	// DisableFilter turns the item filter (Ctrl+Alt+F, see vmenu_filter.go)
	// off. A menu that filters its own items, or paints rows from TopPos
	// and Items by itself, sets it: the filter hides rows the consumer
	// would still paint.
	DisableFilter bool
	// FilterOnType starts the filter with the first printable key, without
	// Ctrl+Alt+F. It suits a list whose letters do nothing else, such as the
	// history of an input field.
	FilterOnType bool
	// IgnoreSingleClick is far2l's VMENU_IGNORE_SINGLECLICK: a single left
	// click only selects the row it lands on, and a double click confirms
	// it. It suits a list read rather than chosen from, such as an about
	// box, where a stray click must not close it.
	IgnoreSingleClick bool

	filterOn     bool
	filterLocked bool
	filterText   []rune
	filterTop    int
}

// menuStopHeldArrowAtEdge holds the inverse of SetMenuLoopScroll, so that
// the zero value keeps the behaviour menus always had: arrows loop.
var menuStopHeldArrowAtEdge atomic.Bool

// SetMenuLoopScroll is far2l's "Loop list scrolling" option (Menu settings;
// Opt.VMenu.MenuLoopScroll, stored as [VMenu] MenuStopWrapOnEdge). On, the
// default, Up on the first item and Down on the last wrap round the menu
// even while the arrow is held. Off, a held arrow stops at the first or the
// last item and only a separate press wraps, which is what Far Manager 3
// always does. It concerns menus whose Wrap is set; the wheel and the page
// keys stop at the ends either way.
func SetMenuLoopScroll(loop bool) {
	menuStopHeldArrowAtEdge.Store(!loop)
}

// MenuLoopScroll reports the value last given to SetMenuLoopScroll.
func MenuLoopScroll() bool {
	return !menuStopHeldArrowAtEdge.Load()
}

// NewVMenu creates a new vertical menu instance.
func NewVMenu(title string) *VMenu {
	clean, _, _ := ParseAmpersandString(title)
	m := &VMenu{
		title:                     clean,
		Items:                     []MenuItem{},
		ColorTextIdx:              ColMenuText,
		ColorSelectedTextIdx:      ColMenuSelectedText,
		ColorHighlightIdx:         ColMenuHighlight,
		ColorSelectedHighlightIdx: ColMenuSelectedHighlight,
		ColorBoxIdx:               ColMenuBox,
		ColorTitleIdx:             ColMenuTitle,
		BoxType:                   DoubleBox,
	}
	m.canFocus = true
	m.Wrap = true
	m.WheelArea = WheelAreaMenu
	m.parentIndex = -1
	m.childIndex = -1
	m.IsSelectable = func(i int) bool {
		return m.itemSelectable(i)
	}
	m.ShowScrollBar = true
	m.MarginTop = 1
	m.MarginBottom = 1
	m.InitScrollBar(m)
	m.ScrollBar.ColorIdx = ColMenuScrollbar
	// The bar counts shown rows while the filter hides items.
	m.ScrollBar.OnScroll = func(v int) {
		if m.filtering() {
			m.scrollFilteredBy(m.visibleRows(), v-m.filterTop)
			return
		}
		m.ScrollBy(v - m.scrollBarTop())
	}
	return m
}

// AddItem adds a new item to the menu.
func (m *VMenu) AddItem(item MenuItem) {
	m.Items = append(m.Items, item)
	m.ItemCount = len(m.Items)
	if len(m.Items) == 1 || !m.itemSelectable(m.SelectPos) {
		m.selectFirstSelectable()
	}
}

// AddSeparator adds a separator line.
func (m *VMenu) AddSeparator() {
	m.Items = append(m.Items, MenuItem{Separator: true})
	m.ItemCount = len(m.Items)
}

func (m *VMenu) GetItemCount() int { return len(m.Items) }

func (m *VMenu) itemSelectable(i int) bool {
	return i >= 0 && i < len(m.Items) && !m.Items[i].Separator &&
		!m.Items[i].Header && !m.Items[i].Disabled
}

func (m *VMenu) selectFirstSelectable() {
	for i := range m.Items {
		if m.itemSelectable(i) {
			m.ScrollView.SetSelectPos(i)
			return
		}
	}
	m.ScrollView.SetSelectPos(0)
}

// SetSelectPos updates the selection and closes a child anchored to a row the
// cursor has left. ScrollView's internal key navigation is handled similarly
// by handleSemanticNavigation below.
func (m *VMenu) SetSelectPos(pos int) {
	oldPos := m.SelectPos
	m.ScrollView.SetSelectPos(pos)
	if m.filtering() {
		m.steerSelection(m.visibleRows())
	}
	if oldPos != m.SelectPos {
		m.cancelSubmenuHover()
	}
	if oldPos != m.SelectPos && m.childMenu != nil && m.childIndex != m.SelectPos {
		m.CloseSubmenu()
	}
}

// ReplaceItems atomically installs an asynchronous snapshot while preserving
// selection by stable item ID. If the selected ID disappeared, the first
// selectable row becomes active.
func (m *VMenu) ReplaceItems(items []MenuItem) {
	selectedID := ""
	childID := ""
	if m.SelectPos >= 0 && m.SelectPos < len(m.Items) {
		selectedID = m.Items[m.SelectPos].ID
	}
	if m.childMenu != nil && m.childIndex >= 0 && m.childIndex < len(m.Items) {
		childID = m.Items[m.childIndex].ID
	}
	m.Items = append([]MenuItem(nil), items...)
	m.ItemCount = len(m.Items)
	m.TopPos = 0
	next := -1
	if selectedID != "" {
		for i := range m.Items {
			if m.Items[i].ID == selectedID && m.itemSelectable(i) {
				next = i
				break
			}
		}
	}
	if next >= 0 {
		m.ScrollView.SetSelectPos(next)
	} else {
		m.selectFirstSelectable()
	}
	if m.filtering() {
		m.steerSelection(m.visibleRows())
	}
	if m.childMenu != nil {
		nextChildIndex := -1
		if childID != "" {
			for i := range m.Items {
				if m.Items[i].ID == childID && m.itemSelectable(i) &&
					m.HasSubmenu(i) && (!m.filtering() || rowOfItem(m.visibleRows(), i) >= 0) {
					nextChildIndex = i
					break
				}
			}
		}
		if nextChildIndex < 0 {
			m.CloseSubmenu()
		} else {
			m.childIndex = nextChildIndex
			m.childMenu.parentIndex = nextChildIndex
			m.childMenu.parentFrame = m.menuFrame()
			m.positionSubmenu(m.childMenu, nextChildIndex)
		}
	}
	if m.parentMenu != nil {
		m.parentMenu.positionSubmenu(m, m.parentIndex)
	}
	m.declareSemanticMenuState()
}

func (m *VMenu) MenuControl() *VMenu { return m }
func (m *VMenu) ParentMenu() *VMenu  { return m.parentMenu }
func (m *VMenu) ParentFrame() Frame  { return m.parentFrame }
func (m *VMenu) ParentIndex() int    { return m.parentIndex }

// RepositionSubmenu asks the parent to recompute this menu's anchored
// placement. Dynamic custom menu frames call it after replacing asynchronous
// rows or when their available screen geometry changes.
func (m *VMenu) RepositionSubmenu() {
	if m != nil && m.parentMenu != nil {
		m.parentMenu.positionSubmenu(m, m.parentIndex)
	}
}

func (m *VMenu) menuFrame() Frame {
	if m != nil && m.hostFrame != nil {
		return m.hostFrame
	}
	return m
}

func bindMenuFrame(frame Frame) *VMenu {
	provider, ok := frame.(MenuFrameProvider)
	if !ok {
		return nil
	}
	menu := provider.MenuControl()
	if menu != nil {
		menu.hostFrame = frame
	}
	return menu
}

func (m *VMenu) HasSubmenu(index int) bool {
	return index >= 0 && index < len(m.Items) &&
		(m.Items[index].Submenu != nil || m.Items[index].SubmenuFrame != nil || len(m.Items[index].SubItems) > 0)
}

func (m *VMenu) positionSubmenu(child *VMenu, index int) {
	if child == nil {
		return
	}
	width := 24
	for itemIndex, item := range child.Items {
		clean, _, _ := ParseAmpersandString(item.Text)
		itemWidth := runewidth.StringWidth(item.AccentPrefix) +
			runewidth.StringWidth(clean) + runewidth.StringWidth(item.Shortcut) + 6
		if child.HasSubmenu(itemIndex) {
			itemWidth += 2
		}
		if itemWidth > width {
			width = itemWidth
		}
	}
	height := len(child.Items) + 2
	if height < 3 {
		height = 3
	}
	screenW, screenH := 80, 25
	if FrameManager != nil {
		screenW = FrameManager.GetScreenSize()
		screenH = FrameManager.GetScreenHeight()
	}
	if width > screenW {
		width = screenW
	}
	if height > screenH {
		height = screenH
	}
	x := m.X2 + 1
	if x+width > screenW {
		x = m.X1 - width
	}
	if x < 0 {
		x = 0
	}
	y := m.Y1 + m.MarginTop + m.rowOffset(index)
	if y+height > screenH {
		y = screenH - height
	}
	if y < 0 {
		y = 0
	}
	child.SetPosition(x, y, x+width-1, y+height-1)
}

// OpenSubmenu opens the selected item's child as a separate modal frame while
// leaving its parent in the stack. It returns false for a leaf or disabled row.
func (m *VMenu) OpenSubmenu(index int) bool {
	m.cancelSubmenuHover()
	if !m.itemSelectable(index) || !m.HasSubmenu(index) || FrameManager == nil {
		return false
	}
	if m.childMenu != nil && m.childIndex == index && !m.childMenu.IsDone() {
		return true
	}
	m.CloseSubmenu()
	m.ScrollView.SetSelectPos(index)
	item := m.Items[index]
	var childFrame Frame
	if item.SubmenuFrame != nil {
		childFrame = item.SubmenuFrame()
	} else if item.Submenu != nil {
		childFrame = item.Submenu()
	} else if len(item.SubItems) > 0 {
		sub := NewVMenu(item.Text)
		sub.HideShadow = m.HideShadow
		sub.BoxType = m.BoxType
		sub.ColorTextIdx = m.ColorTextIdx
		sub.ColorSelectedTextIdx = m.ColorSelectedTextIdx
		sub.ColorHighlightIdx = m.ColorHighlightIdx
		sub.ColorSelectedHighlightIdx = m.ColorSelectedHighlightIdx
		sub.ColorBoxIdx = m.ColorBoxIdx
		sub.ColorTitleIdx = m.ColorTitleIdx
		if sub.ScrollBar != nil && m.ScrollBar != nil {
			sub.ScrollBar.ColorIdx = m.ScrollBar.ColorIdx
		}
		for _, nested := range item.SubItems {
			sub.AddItem(nested)
		}
		sub.OnAction = func(int) {
			m.CloseChain()
			FrameManager.RemoveFrame(m.menuFrame())
			if m.OnAction != nil {
				m.OnAction(m.SelectPos)
			}
		}
		childFrame = sub
	}
	child := bindMenuFrame(childFrame)
	if child == nil {
		return false
	}
	child.parentMenu = m
	child.parentFrame = m.menuFrame()
	child.parentIndex = index
	child.SetOwner(m)
	child.ClearDone()
	m.positionSubmenu(child, index)
	m.childMenu = child
	m.activeSub = child
	m.childFrame = childFrame
	m.childIndex = index
	FrameManager.PushMenu(childFrame)
	return true
}

func (m *VMenu) notifyClosed() {
	if m.closeNotified {
		return
	}
	m.closeNotified = true
	if m.OnClose != nil {
		m.OnClose()
	}
}

func (m *VMenu) finish(code int, emitClose bool) {
	m.cancelSubmenuHover()
	m.ClearFilter()
	m.CloseSubmenu()
	m.done = true
	m.exitCode = code
	m.notifyClosed()
	if emitClose && FrameManager != nil {
		FrameManager.EmitCommand(CmMenuClose, nil)
	}
}

// CloseSubmenu closes only the descendant branch and restores focus to m.
func (m *VMenu) CloseSubmenu() {
	child := m.childMenu
	if child == nil {
		return
	}
	childFrame := m.childFrame
	if childFrame == nil {
		childFrame = child.menuFrame()
	}
	m.childMenu = nil
	m.activeSub = nil
	m.childFrame = nil
	m.childIndex = -1
	child.finish(-1, false)
	child.parentMenu = nil
	child.parentFrame = nil
	if FrameManager != nil {
		FrameManager.RemoveFrame(childFrame)
		m.declareSemanticMenuState()
	}
}

// CloseChain dismisses the complete nested popup chain.
func (m *VMenu) CloseChain() {
	root := m
	for root.parentMenu != nil {
		root = root.parentMenu
	}
	root.finish(-1, true)
}

func (m *VMenu) declareSemanticMenuState() {
	if FrameManager != nil {
		FrameManager.declareSemanticMenuState()
	}
}

// handleSemanticNavigation delegates to ScrollView and declares a bounded
// semantic update only when no OnSelect callback was invoked. OnSelect is an
// application callback and may change the document or shell behind the menu.
func (m *VMenu) handleSemanticNavigation(e *vtinput.InputEvent) bool {
	oldPos := m.SelectPos
	var handled bool
	if e.VirtualKeyCode == vtinput.VK_UP || e.VirtualKeyCode == vtinput.VK_DOWN {
		handled = m.handleArrowKey(e)
	} else {
		handled = m.HandleKey(e)
	}
	if handled && oldPos != m.SelectPos {
		m.cancelSubmenuHover()
	}
	if handled && oldPos != m.SelectPos && m.childMenu != nil && m.childIndex != m.SelectPos {
		m.CloseSubmenu()
	}
	if handled && (m.OnSelect == nil || m.SelectPos == oldPos) {
		m.declareSemanticMenuState()
	}
	return handled
}

// menuItemHint returns the text drawn right-aligned on a menu row: the
// shortcut, or the marker that says the row opens a nested menu.
func menuItemHint(item MenuItem) string {
	if item.Shortcut != "" {
		return item.Shortcut
	}
	if len(item.SubItems) > 0 {
		return SubMenuMarker
	}
	return ""
}

// menuItemsWidth returns the box width the items need: the widest row plus
// its hint column, never below minWidth.
func menuItemsWidth(items []MenuItem, minWidth int) int {
	width := minWidth
	for _, item := range items {
		if item.Separator {
			continue
		}
		clean, _, _ := ParseAmpersandString(" " + item.Text)
		w := StringWidth(clean)
		if hint := menuItemHint(item); hint != "" {
			w += StringWidth(hint + " ")
		}
		w += 4 // Minimum visual padding between text and shortcut/border
		if w > width {
			width = w
		}
	}
	return width
}

// HasSubMenu reports whether the item at index opens a nested menu.
func (m *VMenu) HasSubMenu(index int) bool { return m.HasSubmenu(index) }

func (m *VMenu) OpenSubMenu(index int) bool { return m.OpenSubmenu(index) }

func (m *VMenu) CloseSubMenu() { m.CloseSubmenu() }

// closeAncestors dismisses the menus this one was opened from. A nested
// menu that finishes -- an item chosen, F10, a click outside -- ends the
// whole construction, and the parents are not on top to notice it
// themselves.
func (m *VMenu) closeAncestors() {
	for parent := m.parentMenu; parent != nil; parent = parent.parentMenu {
		parent.activeSub = nil
		parent.done = true
		parent.exitCode = -1
		if FrameManager != nil {
			FrameManager.RemoveFrame(parent)
		}
	}
}

// ProcessKey processes navigation keys.
func (m *VMenu) ProcessKey(e *vtinput.InputEvent) bool {
	if m.IsDisabled() || !e.KeyDown {
		return false
	}

	if m.filtering() {
		// A consumer may have changed Items since the last key.
		m.steerSelection(m.visibleRows())
	}
	if m.processFilterKey(e) {
		return true
	}
	if m.filterBlocksKey(e) {
		return true
	}

	if m.OnKeyDown != nil && m.OnKeyDown(e) {
		return true
	}

	isSubMenu := false
	if m.owner != nil {
		_, isSubMenu = m.owner.(*MenuBar)
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_LEFT:
		if m.parentMenu != nil {
			m.parentMenu.CloseSubmenu()
			return true
		}
		if isSubMenu {
			FrameManager.EmitCommand(CmMenuLeft, nil)
			return true
		}
		return false // Boundary exit
	case vtinput.VK_RIGHT:
		if m.OpenSubmenu(m.SelectPos) {
			return true
		}
		if isSubMenu {
			FrameManager.EmitCommand(CmMenuRight, nil)
			return true
		}
		if m.parentMenu != nil {
			// Inside a nested menu Right is the open gesture; with nothing
			// to open it must not walk the selection sideways.
			return true
		}
		// If last item in standalone menu, let focus cycle (unless wrapping is on)
		if m.atLastRow() && !m.Wrap {
			return false
		}
		return m.handleSemanticNavigation(e)
	case vtinput.VK_UP:
		if m.atFirstRow() && !isSubMenu && !m.Wrap {
			return false
		}
		return m.handleSemanticNavigation(e)
	case vtinput.VK_DOWN:
		if m.atLastRow() && !isSubMenu && !m.Wrap {
			return false
		}
		return m.handleSemanticNavigation(e)
	// PgUp/PgDn fall through to HandleKey like Home/End do: HandleNavKey
	// pages via PageBy, which clamps at the list ends even though Wrap is on.
	case vtinput.VK_ESCAPE, vtinput.VK_F10:
		if m.parentMenu != nil && e.VirtualKeyCode == vtinput.VK_ESCAPE {
			m.parentMenu.CloseSubmenu()
			return true
		}
		m.SetExitCode(-1)
		m.declareSemanticMenuState()
		return FrameManager.GetTopFrame() == Frame(m)
	case vtinput.VK_RETURN:
		if m.HasSubMenu(m.SelectPos) {
			m.OpenSubMenu(m.SelectPos)
			return true
		}
		if m.SelectPos >= 0 && m.SelectPos < m.ItemCount {
			keepOpen := false
			// Virtual consumers size the menu via ItemCount without backing
			// Items; such rows carry no command to fire, but the selection is
			// still confirmed through OnAction and the exit code.
			if m.SelectPos < len(m.Items) {
				item := m.Items[m.SelectPos]
				if !m.itemSelectable(m.SelectPos) {
					return true
				}
				if m.OpenSubmenu(m.SelectPos) {
					return true
				}
				if menuItemDisabled(item) {
					return true
				}

				// 1. Fire the actual action (bubbles through owner)
				oldCmd := m.Command
				m.Command = item.Command
				m.FireAction(item.OnClick, item.UserData)
				m.Command = oldCmd
				keepOpen = item.KeepOpen
			}

			// 2. Notify listener (may close the menu)
			if m.OnAction != nil {
				m.OnAction(m.SelectPos)
			}
			if keepOpen {
				m.declareSemanticMenuState()
				return true
			}

			if m.parentMenu != nil {
				root := m
				for root.parentMenu != nil {
					root = root.parentMenu
				}
				root.finish(m.SelectPos, false)
				m.finish(m.SelectPos, false)
			} else {
				m.SetExitCode(m.SelectPos)
			}
			return true
		}
		return true
	}

	if e.Char != 0 {
		charLower := unicode.ToLower(e.Char)
		xlatLower := unicode.ToLower(GlobalXlator.Translate(e.Char))
		var shown []int
		if m.filtering() {
			shown = m.visibleRows()
		}
		for i, item := range m.Items {
			if !m.itemSelectable(i) {
				continue
			}
			// A locked filter hands letters back to the hotkeys, but only
			// for the items it shows.
			if shown != nil && rowOfItem(shown, i) < 0 {
				continue
			}
			hk := ExtractHotkey(item.Text)
			if hk != 0 && (hk == charLower || hk == xlatLower) {
				if menuItemDisabled(item) {
					return true
				}
				m.SetSelectPos(i)
				if m.OpenSubmenu(i) {
					return true
				}

				oldCmd := m.Command
				m.Command = item.Command
				m.FireAction(item.OnClick, item.UserData)
				m.Command = oldCmd

				if m.OnAction != nil {
					m.OnAction(i)
				}
				if item.KeepOpen {
					m.declareSemanticMenuState()
					return true
				}

				if m.parentMenu != nil {
					root := m
					for root.parentMenu != nil {
						root = root.parentMenu
					}
					root.finish(i, false)
					m.finish(i, false)
				} else {
					m.SetExitCode(i)
				}
				return true
			}
		}
	}

	return m.handleSemanticNavigation(e)
}

// atFirstRow and atLastRow report whether the selection is on the first or
// the last row shown.
func (m *VMenu) atFirstRow() bool {
	if !m.filtering() {
		return m.SelectPos == 0
	}
	rows := m.visibleRows()
	return len(rows) == 0 || m.SelectPos == rows[0]
}

func (m *VMenu) atLastRow() bool {
	if !m.filtering() {
		return m.SelectPos == m.ItemCount-1
	}
	rows := m.visibleRows()
	return len(rows) == 0 || m.SelectPos == rows[len(rows)-1]
}

// handleArrowKey moves the selection for Up and Down. far2l's VMenu passes
// stop_on_edge = IsRepeatedKey() && !Opt.VMenu.MenuLoopScroll for these keys
// (Far Manager 3 passes IsRepeatedKey() alone): a wrapping menu stops at its
// first or last item while the arrow is held, and keeps the key, so focus
// does not leave the menu either. Wrap is lifted for that one move only, as
// Far 3 clears VMENU_WRAPMODE around a single step.
func (m *VMenu) handleArrowKey(e *vtinput.InputEvent) bool {
	if m.Wrap && !MenuLoopScroll() && FrameManager != nil && FrameManager.IsRepeatedKey() {
		m.Wrap = false
		defer func() { m.Wrap = true }()
	}
	return m.HandleKey(e)
}

func (m *VMenu) ResizeConsole(w, h int) {
	// For standalone VMenus, we might want to keep them centered
}
func (m *VMenu) GetTitle() string {
	return m.title
}

// SetTitle replaces the title, dropping ampersands the way NewVMenu does.
// far2l retitles a menu in place to show a mode, such as far:about's
// hidden-rows marker.
func (m *VMenu) SetTitle(title string) {
	m.title, _, _ = ParseAmpersandString(title)
}

// GetBottomTitle returns the text drawn on the lower border.
func (m *VMenu) GetBottomTitle() string {
	return m.bottomTitle
}

// SetBottomTitle sets the text drawn centred on the lower border, typically
// the keys a menu understands beyond the usual ones. An empty string removes
// it.
func (m *VMenu) SetBottomTitle(title string) {
	m.bottomTitle = title
}

// SetBottomTextLines is far2l's VMenu::SetBottomTextLines: it reserves n
// rows at the foot of the box, under a separator, where the Description of
// the selected item is shown word-wrapped, the way far2l's far:config
// explains the option under the cursor. The list keeps the rows above the
// separator, so its scrollbar, paging and mouse hits stop there too. Zero,
// the default, removes the area.
//
// far2l grows the area while a long description is selected; here it keeps
// the size it is given, so that the list does not jump as the selection
// moves. The caller sizes it for the longest description it will show.
func (m *VMenu) SetBottomTextLines(n int) {
	if n < 0 {
		n = 0
	}
	m.bottomTextLines = n
	m.MarginBottom = 1 + m.bottomAreaHeight()
	if m.Y2 > m.Y1 {
		m.SetPosition(m.X1, m.Y1, m.X2, m.Y2)
	}
}

// GetBottomTextLines returns the value last given to SetBottomTextLines.
func (m *VMenu) GetBottomTextLines() int {
	return m.bottomTextLines
}

// bottomAreaHeight is the bottom text area with its separator row, or zero
// when the menu has none.
func (m *VMenu) bottomAreaHeight() int {
	if m.bottomTextLines <= 0 {
		return 0
	}
	return m.bottomTextLines + 1
}

// BottomTextWidth is the width the bottom text area wraps descriptions to,
// as far2l's VMenu::GetBottomTextWidth: the box less its borders and a
// column of padding on each side.
func (m *VMenu) BottomTextWidth() int {
	return max(0, m.X2-m.X1-3)
}

func (m *VMenu) GetProgress() int {
	return -1
}

func (m *VMenu) GetType() FrameType {
	return TypeMenu
}

func (m *VMenu) SetExitCode(code int) {
	m.mouseSelecting = false
	// far2l drops the filter whenever the menu goes away; the item indices
	// the menu hands back never depended on it.
	m.setFilter(false)
	m.CloseSubMenu()
	m.closeAncestors()
	m.done = true
	m.exitCode = code
	if code == -1 {
		// Cancelled: undo the browsing highlight (see selectAtOpen).
		m.SetSelectPos(m.selectAtOpen)
		FrameManager.EmitCommand(CmMenuClose, nil)
	}
}

func (m *VMenu) IsDone() bool {
	return m.done
}
func (m *VMenu) IsBusy() bool          { return false }
func (m *VMenu) IsModal() bool         { return true }
func (m *VMenu) GetWindowNumber() int  { return 0 }
func (m *VMenu) SetWindowNumber(n int) {}
func (m *VMenu) RequestFocus() bool    { return true }
func (m *VMenu) Close()                { m.CloseChain() }
func (m *VMenu) HasShadow() bool       { return !m.HideShadow }

// ClearDone resets the menu state, allowing it to be shown again.
func (m *VMenu) ClearDone() {
	m.mouseSelecting = false
	m.setFilter(false)
	m.done = false
	m.exitCode = -1
	m.selectAtOpen = m.SelectPos
}

// BeginMouseSelection transfers the opening press to the popup.
func (m *VMenu) BeginMouseSelection() { m.mouseSelecting = true }

// ProcessMouse handles mouse wheel scrolling, menu item hover, and clicks.
func (m *VMenu) ProcessMouse(e *vtinput.InputEvent) bool {
	if m.IsDisabled() || e.Type != vtinput.MouseEventType {
		return false
	}
	if m.processWindowMouse(e) {
		return true
	}
	if m.filtering() {
		m.steerSelection(m.visibleRows())
	}
	if m.mouseSelecting {
		index := m.GetClickIndex(int(e.MouseY))
		inside := int(e.MouseX) > m.X1 && int(e.MouseX) < m.X2 && index >= 0 && index < len(m.Items) && !m.Items[index].Separator
		if inside {
			m.SetSelectPos(index)
		}
		if IsMouseRelease(e) {
			m.mouseSelecting = false
			if inside {
				click := *e
				click.KeyDown = true
				click.ButtonState = vtinput.FromLeft1stButtonPressed
				click.MouseEventFlags = 0
				return m.ProcessMouse(&click)
			}
		}
		return true
	}
	if m.filtering() && e.WheelDirection != 0 && (m.ScrollBar == nil || !m.ScrollBar.IsMouseCaptured()) {
		lines := wheelLinesFor(m.WheelArea, e.WheelDirection)
		if e.WheelDirection > 0 {
			lines = -lines
		}
		m.scrollFilteredBy(m.visibleRows(), lines)
		return true
	}
	oldPos := m.SelectPos
	if m.HandleMouseScroll(e) {
		if m.SelectPos != oldPos && m.OnSelect != nil {
			m.OnSelect(m.SelectPos)
		}
		m.declareSemanticMenuState()
		return true
	}

	if (e.MouseEventFlags & vtinput.MouseMoved) != 0 {
		mx := int(e.MouseX)
		if mx <= m.X1 || mx >= m.X2 {
			m.cancelSubmenuHover()
			return false
		}

		hoverIdx := m.GetClickIndex(int(e.MouseY))
		if hoverIdx == -1 {
			m.cancelSubmenuHover()
			return false
		}
		// Rows past len(Items) belong to virtual consumers that only set
		// ItemCount; they are plain selectable rows, not separators.
		if hoverIdx >= len(m.Items) || m.itemSelectable(hoverIdx) {
			m.SetSelectPos(hoverIdx)
			if m.HasSubmenu(hoverIdx) {
				m.scheduleSubmenuHover(hoverIdx)
			} else {
				m.cancelSubmenuHover()
			}
		} else {
			m.cancelSubmenuHover()
		}
		m.declareSemanticMenuState()
		return true
	}

	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		clickIdx := m.GetClickIndex(int(e.MouseY))
		if clickIdx != -1 && (clickIdx >= len(m.Items) || m.itemSelectable(clickIdx)) {
			m.SetSelectPos(clickIdx)
			keepOpen := false
			// Virtual rows (ItemCount beyond len(Items)) have no command to
			// fire; the click still selects and confirms them.
			if clickIdx < len(m.Items) {
				item := m.Items[clickIdx]
				if m.OpenSubmenu(clickIdx) {
					return true
				}
				if m.IgnoreSingleClick && e.MouseEventFlags&vtinput.DoubleClick == 0 {
					return true
				}
				if menuItemDisabled(item) {
					return true
				}

				// Fire Action BEFORE calling OnAction/SetExitCode
				oldCmd := m.Command
				m.Command = item.Command
				m.FireAction(item.OnClick, item.UserData)
				m.Command = oldCmd
				keepOpen = item.KeepOpen
			}

			if m.OnAction != nil {
				m.OnAction(clickIdx)
			}
			if keepOpen {
				m.declareSemanticMenuState()
				return true
			}
			if m.parentMenu != nil {
				root := m
				for root.parentMenu != nil {
					root = root.parentMenu
				}
				root.finish(clickIdx, false)
				m.finish(clickIdx, false)
			} else {
				m.SetExitCode(clickIdx)
			}
			return true
		}
	}
	return false
}

const submenuHoverDelay = 180 * time.Millisecond

func (m *VMenu) cancelSubmenuHover() {
	m.hoverMu.Lock()
	m.hoverGen++
	if m.hoverTimer != nil {
		m.hoverTimer.Stop()
		m.hoverTimer = nil
	}
	m.hoverMu.Unlock()
}

func (m *VMenu) scheduleSubmenuHover(index int) {
	if !m.itemSelectable(index) || !m.HasSubmenu(index) || FrameManager == nil ||
		(m.childMenu != nil && m.childIndex == index) {
		m.cancelSubmenuHover()
		return
	}
	m.hoverMu.Lock()
	m.hoverGen++
	generation := m.hoverGen
	if m.hoverTimer != nil {
		m.hoverTimer.Stop()
	}
	m.hoverTimer = time.AfterFunc(submenuHoverDelay, func() {
		if FrameManager == nil {
			return
		}
		FrameManager.PostTask(func() {
			m.hoverMu.Lock()
			current := m.hoverGen == generation
			if current {
				m.hoverTimer = nil
			}
			m.hoverMu.Unlock()
			if !current || m.done || m.SelectPos != index {
				return
			}
			if m.OpenSubmenu(index) {
				m.declareSemanticMenuState()
			}
		})
	})
	m.hoverMu.Unlock()
}

// Show prepares the background and calls the render method.
func (m *VMenu) Show(scr *ScreenBuf) {
	m.ScreenObject.Show(scr)
	m.DisplayObject(scr)
	m.DrawWindowControls(scr)
}

// DisplayObject renders the frame and menu items.
func (m *VMenu) DisplayObject(scr *ScreenBuf) {
	if !m.IsVisible() {
		return
	}
	p := NewPainter(scr)

	// 1. Frame and Background
	p.Fill(m.X1, m.Y1, m.X2, m.Y2, ' ', Palette[m.ColorTextIdx])
	p.DrawBox(m.X1, m.Y1, m.X2, m.Y2, Palette[m.ColorBoxIdx], m.BoxType)

	// far2l paints a menu title with Menu.Title whether the menu holds focus
	// or not, so there is no separate focused variant here.
	p.DrawTitle(m.X1, m.Y1, m.X2, m.displayTitle(), Palette[m.ColorTitleIdx])
	p.DrawTitle(m.X1, m.Y2, m.X2, m.bottomTitle, Palette[m.ColorTitleIdx])

	colText := Palette[m.ColorTextIdx]
	colSel := Palette[m.ColorSelectedTextIdx]
	colBox := Palette[m.ColorBoxIdx]
	listBottom := m.Y2 - m.bottomAreaHeight()
	height := listBottom - m.Y1 - 1
	if height < 0 {
		height = 0
	}

	colHigh := Palette[m.ColorHighlightIdx]
	colSelHigh := Palette[m.ColorSelectedHighlightIdx]

	// 3. Rendering items. While the filter hides items, rows map to the
	// items it shows.
	top := m.TopPos
	var shown []int
	if m.filtering() {
		shown = m.visibleRows()
		m.steerSelection(shown)
		top = m.filterTop
	}
	for i := 0; i < height; i++ {
		itemIdx := i + top
		if shown == nil {
			itemIdx = m.ItemAtRow(i)
		}
		currY := m.Y1 + 1 + i
		if currY >= listBottom {
			break
		}
		if shown != nil {
			if itemIdx >= len(shown) {
				continue
			}
			itemIdx = shown[itemIdx]
		}
		if itemIdx >= len(m.Items) {
			continue
		}

		item := m.Items[itemIdx]
		isDisabled := !item.Separator && menuItemDisabled(item)

		attr := colText
		if isDisabled {
			attr = DimColor(attr)
		} else if itemIdx == m.SelectPos {
			attr = colSel
		}

		if item.Separator {
			m.drawSeparator(p, scr, currY, colBox)
			// A separator may carry a heading. It is drawn here, on the row the
			// separator really has, so that it follows scrolling and the filter;
			// a caller painting headings over the menu by row number had them
			// left on rows that hold other things (f4 #263).
			if item.Text != "" {
				p.DrawTitle(m.X1, currY, m.X2, " "+item.Text+" ", Palette[m.ColorTitleIdx])
			}
			continue
		}

		// Resolve item colors
		isSel := itemIdx == m.SelectPos
		isDisabled = menuItemDisabled(item)

		itemAttr := colText
		hiAttr := colHigh
		if isSel {
			itemAttr, hiAttr = colSel, colSelHigh
		}
		if isDisabled {
			itemAttr, hiAttr = DimColor(itemAttr), DimColor(hiAttr)
		}

		// Calculate layout
		//clean, _, _ := ParseAmpersandString(item.Text)
		//vLenText := StringWidth(clean) + 1 // +1 for leading space
		hintText := ""
		vLenHint := 0
		if hint := menuItemHint(item); hint != "" {
			hintText = hint + " "
			vLenHint = StringWidth(hintText)
		}

		// Draw background and text
		p.Fill(m.X1+1, currY, m.X2-1, currY, ' ', itemAttr)
		textX := m.X1 + 1
		p.DrawString(textX, currY, " ", itemAttr)
		textX++
		if item.AccentPrefix != "" {
			p.DrawString(textX, currY, item.AccentPrefix, hiAttr)
			textX += runewidth.StringWidth(item.AccentPrefix)
		}
		if item.Icon == "tag-dot" {
			p.DrawString(textX, currY, "● ", hiAttr)
			textX += 2
		}
		if item.Header {
			itemAttr, hiAttr = DimColor(itemAttr), DimColor(hiAttr)
		}
		// The text must not run past the box: a name longer than the menu (a
		// combo box drop-down of long font names on a narrow console, f4 #1706)
		// would otherwise be painted over the border, the scrollbar and
		// whatever is beside the menu. What does not fit is cut with an
		// ellipsis; the accent letter of a cut item is not drawn.
		avail := m.X2 - textX - vLenHint
		if clean, _, _ := ParseAmpersandString(item.Text); StringWidth(clean) > avail {
			mark := m.TruncateMark
			if mark == "" {
				mark = "…"
			}
			p.DrawString(textX, currY, TruncateString(clean, avail, mark), itemAttr)
		} else {
			p.DrawControlText(textX, currY, item.Text, itemAttr, hiAttr)
		}
		if hintText != "" {
			p.DrawString(m.X2-vLenHint, currY, hintText, itemAttr)
		}
		if m.HasSubmenu(itemIdx) {
			p.DrawString(m.X2-2, currY, "▶", itemAttr)
		}
	}

	// 4. The selected item's description, under the list.
	m.drawBottomText(p, scr, listBottom, colText, colBox)

	// 5. Scrollbar
	m.DrawScrollBar(scr)
}

// drawSeparator draws a horizontal rule across the box on row y, joined to
// its borders.
func (m *VMenu) drawSeparator(p *Painter, scr *ScreenBuf, y int, colBox uint64) {
	if m.BoxType == SingleBox {
		symbols := getBoxSymbols(SingleBox)
		p.DrawLine(m.X1, y, m.X2, y, symbols[bsH], colBox, false, false)
		scr.Write(m.X1, y, []CharInfo{{Char: uint64(symbols[bsHCrossLeft]), Attributes: colBox}})  // #nosec G115 -- box-drawing rune, always small and non-negative
		scr.Write(m.X2, y, []CharInfo{{Char: uint64(symbols[bsHCrossRight]), Attributes: colBox}}) // #nosec G115 -- box-drawing rune, always small and non-negative
	} else {
		p.DrawLine(m.X1, y, m.X2, y, boxSymbols[bsH], colBox, true, true)
	}
}

// drawBottomText is far2l's VMenu::DrawBottomText: a separator on row
// separatorY, then the selected item's Description wrapped to the box,
// as many lines as the area has. What does not fit is cut off.
func (m *VMenu) drawBottomText(p *Painter, scr *ScreenBuf, separatorY int, colText, colBox uint64) {
	if m.bottomTextLines <= 0 || separatorY <= m.Y1 || separatorY >= m.Y2 {
		return
	}
	m.drawSeparator(p, scr, separatorY, colBox)
	width := m.BottomTextWidth()
	if width <= 0 || m.SelectPos < 0 || m.SelectPos >= len(m.Items) {
		return
	}
	item := m.Items[m.SelectPos]
	if item.Separator || item.Description == "" {
		return
	}
	y := separatorY + 1
	for _, line := range WrapText(item.Description, width) {
		if y >= m.Y2 {
			break
		}
		p.DrawString(m.X1+2, y, TruncateString(line, width, ""), colText)
		y++
	}
}
