package vtui

import (
	"github.com/mattn/go-runewidth"
	"sync"
	"time"
	"unicode"

	"github.com/unxed/vtinput"
)

// MenuItem represents a single menu item.
type MenuItem struct {
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
	IconColor string
	Shortcut  string // Optional right-aligned hotkey hint (e.g. "F3")
	Command   int    // TV-style Command ID to emit when selected
	OnClick   func() // Closure called when selected
	UserData  any
	Separator bool
	Header    bool
	Disabled  bool
	// KeepOpen leaves the menu chain active after invoking this leaf. It is
	// useful for Retry/refresh commands that replace rows asynchronously.
	KeepOpen bool
	// Submenu creates the anchored child lazily. A fresh child is requested on
	// every opening so callers can start an asynchronous refresh at that point.
	Submenu func() *VMenu
}

// VMenu implements a vertical menu with navigation support.
type VMenu struct {
	ScrollView
	title    string
	Items    []MenuItem
	done     bool
	exitCode int
	// selectAtOpen is SelectPos as of the last ClearDone. Browsing moves
	// SelectPos live (arrows, mouse hover), so cancelling has to put it
	// back: dialogs read SelectPos as the confirmed choice, and without the
	// restore an Esc'd dropdown silently commits whatever row the user
	// happened to stop on.
	selectAtOpen int
	OnAction     func(int)
	OnKeyDown    func(*vtinput.InputEvent) bool
	HideShadow   bool
	BoxType      int
	// OnClose is invoked exactly once for one shown lifetime. Dynamic menus use
	// it to cancel native requests and live queries when their chain closes.
	OnClose func()

	parentMenu    *VMenu
	parentIndex   int
	childMenu     *VMenu
	childIndex    int
	closeNotified bool
	hoverMu       sync.Mutex
	hoverTimer    *time.Timer
	hoverGen      uint64

	// Palette entries the menu paints with. They default to the Menu.* group;
	// a ComboBox points them at Dialog.Combo.* so its dropdown stands apart
	// from the dialog underneath it.
	ColorTextIdx              int
	ColorSelectedTextIdx      int
	ColorHighlightIdx         int
	ColorSelectedHighlightIdx int
	ColorBoxIdx               int
	ColorTitleIdx             int
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
	if m.childMenu != nil {
		nextChildIndex := -1
		if childID != "" {
			for i := range m.Items {
				if m.Items[i].ID == childID && m.itemSelectable(i) &&
					m.Items[i].Submenu != nil {
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
			m.positionSubmenu(m.childMenu, nextChildIndex)
		}
	}
	if m.parentMenu != nil {
		m.parentMenu.positionSubmenu(m, m.parentIndex)
	}
	m.declareSemanticMenuState()
}

func (m *VMenu) ParentMenu() *VMenu { return m.parentMenu }
func (m *VMenu) ParentIndex() int   { return m.parentIndex }

func (m *VMenu) HasSubmenu(index int) bool {
	return index >= 0 && index < len(m.Items) && m.Items[index].Submenu != nil
}

func (m *VMenu) positionSubmenu(child *VMenu, index int) {
	if child == nil {
		return
	}
	width := 24
	for _, item := range child.Items {
		clean, _, _ := ParseAmpersandString(item.Text)
		itemWidth := runewidth.StringWidth(item.AccentPrefix) +
			runewidth.StringWidth(clean) + runewidth.StringWidth(item.Shortcut) + 6
		if item.Submenu != nil {
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
	y := m.Y1 + m.MarginTop + index - m.TopPos
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
	child := m.Items[index].Submenu()
	if child == nil {
		return false
	}
	child.parentMenu = m
	child.parentIndex = index
	child.SetOwner(m)
	child.ClearDone()
	m.positionSubmenu(child, index)
	m.childMenu = child
	m.childIndex = index
	FrameManager.PushMenu(child)
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
	m.childMenu = nil
	m.childIndex = -1
	child.finish(-1, false)
	if FrameManager != nil {
		FrameManager.RemoveFrame(child)
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
	handled := m.HandleKey(e)
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

// ProcessKey processes navigation keys.
func (m *VMenu) ProcessKey(e *vtinput.InputEvent) bool {
	if m.IsDisabled() || !e.KeyDown {
		return false
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
			m.finish(-1, false)
			m.parentMenu.childMenu = nil
			m.parentMenu.childIndex = -1
			m.declareSemanticMenuState()
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
		// If last item in standalone menu, let focus cycle (unless wrapping is on)
		if m.SelectPos == m.ItemCount-1 && !m.Wrap {
			return false
		}
		return m.handleSemanticNavigation(e)
	case vtinput.VK_UP:
		if m.SelectPos == 0 && !isSubMenu && !m.Wrap {
			return false
		}
		return m.handleSemanticNavigation(e)
	case vtinput.VK_DOWN:
		if m.SelectPos == m.ItemCount-1 && !isSubMenu && !m.Wrap {
			return false
		}
		return m.handleSemanticNavigation(e)
	// PgUp/PgDn fall through to HandleKey like Home/End do: HandleNavKey
	// pages via PageBy, which clamps at the list ends even though Wrap is on.
	case vtinput.VK_ESCAPE, vtinput.VK_F10:
		if m.parentMenu != nil && e.VirtualKeyCode == vtinput.VK_ESCAPE {
			m.finish(-1, false)
			m.parentMenu.childMenu = nil
			m.parentMenu.childIndex = -1
			m.declareSemanticMenuState()
			return true
		}
		m.SetExitCode(-1)
		m.declareSemanticMenuState()
		return FrameManager.GetTopFrame() == Frame(m)
	case vtinput.VK_RETURN:
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
				if FrameManager.DisabledCommands.IsDisabled(item.Command) {
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
		for i, item := range m.Items {
			if !m.itemSelectable(i) {
				continue
			}
			hk := ExtractHotkey(item.Text)
			if hk != 0 && (hk == charLower || hk == xlatLower) {
				if FrameManager.DisabledCommands.IsDisabled(item.Command) {
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

func (m *VMenu) ResizeConsole(w, h int) {
	// For standalone VMenus, we might want to keep them centered
}
func (m *VMenu) GetTitle() string {
	return m.title
}
func (m *VMenu) GetProgress() int {
	return -1
}

func (m *VMenu) GetType() FrameType {
	return TypeMenu
}

func (m *VMenu) SetExitCode(code int) {
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
	m.done = false
	m.exitCode = -1
	m.selectAtOpen = m.SelectPos
}

// ProcessMouse handles mouse wheel scrolling, menu item hover, and clicks.
func (m *VMenu) ProcessMouse(e *vtinput.InputEvent) bool {
	if m.IsDisabled() || e.Type != vtinput.MouseEventType {
		return false
	}
	if m.HandleMouseScroll(e) {
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
				if FrameManager.DisabledCommands.IsDisabled(item.Command) {
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
	p.DrawTitle(m.X1, m.Y1, m.X2, m.title, Palette[m.ColorTitleIdx])

	colText := Palette[m.ColorTextIdx]
	colSel := Palette[m.ColorSelectedTextIdx]
	colBox := Palette[m.ColorBoxIdx]
	height := m.Y2 - m.Y1 - 1
	if height < 0 {
		height = 0
	}

	colHigh := Palette[m.ColorHighlightIdx]
	colSelHigh := Palette[m.ColorSelectedHighlightIdx]

	// 3. Rendering items
	for i := 0; i < height; i++ {
		itemIdx := i + m.TopPos
		currY := m.Y1 + 1 + i
		if currY >= m.Y2 {
			break
		}
		if itemIdx >= len(m.Items) {
			continue
		}

		item := m.Items[itemIdx]
		isDisabled := !item.Separator && (item.Disabled || FrameManager.DisabledCommands.IsDisabled(item.Command))

		attr := colText
		if isDisabled {
			attr = DimColor(attr)
		} else if itemIdx == m.SelectPos {
			attr = colSel
		}

		if item.Separator {
			if m.BoxType == SingleBox {
				symbols := getBoxSymbols(SingleBox)
				p.DrawLine(m.X1, currY, m.X2, currY, symbols[bsH], colBox, false, false)
				scr.Write(m.X1, currY, []CharInfo{{Char: uint64(symbols[bsHCrossLeft]), Attributes: colBox}})
				scr.Write(m.X2, currY, []CharInfo{{Char: uint64(symbols[bsHCrossRight]), Attributes: colBox}})
			} else {
				p.DrawLine(m.X1, currY, m.X2, currY, boxSymbols[bsH], colBox, true, true)
			}
			continue
		}

		// Resolve item colors
		isSel := itemIdx == m.SelectPos
		isDisabled = item.Disabled || FrameManager.DisabledCommands.IsDisabled(item.Command)

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
		shortcutText := ""
		vLenShortcut := 0
		if item.Shortcut != "" {
			shortcutText = item.Shortcut + " "
			vLenShortcut = StringWidth(shortcutText)
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
		p.DrawControlText(textX, currY, item.Text, itemAttr, hiAttr)
		if shortcutText != "" {
			p.DrawString(m.X2-vLenShortcut, currY, shortcutText, itemAttr)
		}
		if item.Submenu != nil {
			p.DrawString(m.X2-2, currY, "▶", itemAttr)
		}
	}

	// 4. Scrollbar
	m.DrawScrollBar(scr)
}
