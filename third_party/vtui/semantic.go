package vtui

import (
	"fmt"
	"strings"

	"github.com/unxed/vtinput"
)

const SemanticSceneVersion = 1

type SemanticContext struct {
	Width        int
	Height       int
	ActiveScreen int
}

type SemanticProvider interface {
	SemanticNode(ctx *SemanticContext) map[string]any
}

type SemanticActionHandler interface {
	HandleSemanticAction(action map[string]any) bool
}

type SemanticSceneRenderer interface {
	SetSemanticScene(scene map[string]any)
}

func SemanticID(v any) string {
	if v == nil {
		return ""
	}
	if el, ok := v.(UIElement); ok {
		if id := el.GetId(); id != "" {
			return "id:" + id
		}
	}
	return fmt.Sprintf("%T:%p", v, v)
}

func (fm *frameManager) ExportSemanticScene() map[string]any {
	if fm == nil || fm.scr == nil {
		return nil
	}

	ctx := &SemanticContext{
		Width:        fm.scr.width,
		Height:       fm.scr.height,
		ActiveScreen: fm.ActiveIdx,
	}

	fm.SyncCurrentScreen()
	screens := make([]map[string]any, 0, len(fm.Screens))
	for i, screen := range fm.Screens {
		frames := make([]map[string]any, 0, len(screen.Frames))
		for _, frame := range screen.Frames {
			if node := semanticFrame(ctx, frame); node != nil {
				frames = append(frames, node)
			}
		}
		screens = append(screens, map[string]any{
			"index":       i,
			"active":      i == fm.ActiveIdx,
			"title":       screen.GetTitle(),
			"progress":    screen.GetProgress(),
			"attention":   screen.NeedsAttention(),
			"transparent": screen.Transparent,
			"frames":      frames,
		})
	}

	scene := map[string]any{
		"type":         "scene",
		"version":      SemanticSceneVersion,
		"width":        fm.scr.width,
		"height":       fm.scr.height,
		"activeScreen": fm.ActiveIdx,
		"screens":      screens,
	}
	if fm.ActiveIdx >= 0 && fm.ActiveIdx < len(screens) {
		scene["frames"] = screens[fm.ActiveIdx]["frames"]
	}
	if mb := fm.GetActiveMenuBar(); mb != nil {
		scene["menuBar"] = semanticMenuBar(mb)
	}
	if fm.KeyBar != nil {
		scene["keyBar"] = semanticKeyBar(fm.KeyBar)
	}
	if fm.currentToast != nil {
		scene["toast"] = map[string]any{"message": fm.currentToast.Message}
	}
	if len(fm.Screens) > 1 {
		scene["workspaceCount"] = len(fm.Screens)
	}
	return scene
}

func semanticFrame(ctx *SemanticContext, frame Frame) map[string]any {
	if frame == nil {
		return nil
	}
	if sp, ok := frame.(SemanticProvider); ok {
		if node := sp.SemanticNode(ctx); node != nil {
			return node
		}
	}
	base := semanticFrameBase(frame)
	switch f := frame.(type) {
	case *Window:
		base["kind"] = "window"
		if f.Modal {
			base["kind"] = "dialog"
		}
		base["children"] = semanticChildren(ctx, f.rootGroup.GetChildren())
		base["showClose"] = f.ShowClose
		base["showZoom"] = f.ShowZoom
		return base
	case *VMenu:
		return semanticVMenu(f)
	default:
		base["kind"] = "fallback"
		base["fallback"] = true
		base["reason"] = fmt.Sprintf("unsupported frame %T", frame)
		return base
	}
}

func semanticFrameBase(frame Frame) map[string]any {
	x1, y1, x2, y2 := frame.GetPosition()
	return map[string]any{
		"id":       SemanticID(frame),
		"title":    strings.TrimSpace(frame.GetTitle()),
		"type":     int(frame.GetType()),
		"x":        x1,
		"y":        y1,
		"w":        x2 - x1 + 1,
		"h":        y2 - y1 + 1,
		"modal":    frame.IsModal(),
		"busy":     frame.IsBusy(),
		"progress": frame.GetProgress(),
		"shadow":   frame.HasShadow(),
	}
}

func semanticChildren(ctx *SemanticContext, children []UIElement) []map[string]any {
	nodes := make([]map[string]any, 0, len(children))
	for _, child := range children {
		if node := semanticElement(ctx, child); node != nil {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func semanticElement(ctx *SemanticContext, el UIElement) map[string]any {
	if el == nil {
		return nil
	}
	if sp, ok := el.(SemanticProvider); ok {
		if node := sp.SemanticNode(ctx); node != nil {
			return node
		}
	}
	base := semanticElementBase(el)
	switch e := el.(type) {
	case *Group:
		base["kind"] = "group"
		base["children"] = semanticChildren(ctx, e.GetChildren())
	case *Text:
		base["kind"] = "text"
		base["text"] = e.cleanText
		base["hotkey"] = stringOrEmpty(e.hotkey)
	case *Button:
		base["kind"] = "button"
		base["text"] = e.cleanText
		base["hotkey"] = stringOrEmpty(e.hotkey)
		base["default"] = e.IsDefault
	case *Edit:
		base["kind"] = "edit"
		base["text"] = e.GetText()
		base["cursor"] = e.curPos
		base["left"] = e.leftPos
		base["password"] = e.PasswordMode
		base["selectionStart"] = e.selStart
		base["selectionEnd"] = e.selEnd
		base["history"] = e.ShowHistoryButton
	case *Checkbox:
		base["kind"] = "checkbox"
		base["text"] = e.cleanText
		base["hotkey"] = stringOrEmpty(e.hotkey)
		base["state"] = e.State
		base["threeState"] = e.ThreeState
	case *RadioGroup:
		base["kind"] = "radioGroup"
		base["items"] = e.Items
		base["selected"] = e.Selected
		base["focusIndex"] = e.focusIdx
		base["columns"] = e.Columns
	case *CheckGroup:
		base["kind"] = "checkGroup"
		base["items"] = e.Items
		base["states"] = e.States
		base["focusIndex"] = e.focusIdx
		base["columns"] = e.Columns
	case *ComboBox:
		base["kind"] = "comboBox"
		base["text"] = e.Edit.GetText()
		base["dropdownOnly"] = e.DropdownOnly
		base["items"] = menuItemsForSemantic(e.Menu)
		base["selected"] = e.Menu.SelectPos
	case *ListBox:
		base["kind"] = "listBox"
		base["items"] = e.Items
		base["selected"] = selectedIndices(e.SelectedMap)
		base["cursor"] = e.SelectPos
		base["top"] = e.TopPos
		base["multiSelect"] = e.MultiSelect
	case *Table:
		base["kind"] = "table"
		base["columns"] = tableColumns(e.Columns)
		base["rows"] = tableRows(e)
		base["cursor"] = e.SelectPos
		base["top"] = e.TopPos
		base["showHeader"] = e.ShowHeader
	case *ScrollBar:
		base["kind"] = "scrollBar"
		base["value"] = e.Value
		base["min"] = e.Min
		base["max"] = e.Max
	case *ProgressBar:
		base["kind"] = "progressBar"
		base["percent"] = e.Percent
	default:
		base["kind"] = "fallbackWidget"
		base["fallback"] = true
		base["typeName"] = fmt.Sprintf("%T", el)
	}
	return base
}

func semanticElementBase(el UIElement) map[string]any {
	x1, y1, x2, y2 := el.GetPosition()
	return map[string]any{
		"id":       SemanticID(el),
		"x":        x1,
		"y":        y1,
		"w":        x2 - x1 + 1,
		"h":        y2 - y1 + 1,
		"visible":  el.IsVisible(),
		"focused":  el.IsFocused(),
		"disabled": el.IsDisabled(),
		"canFocus": el.CanFocus(),
	}
}

func semanticMenuBar(mb *MenuBar) map[string]any {
	if mb == nil {
		return nil
	}
	items := make([]map[string]any, 0, len(mb.Items))
	for i, item := range mb.Items {
		clean, hotkey, _ := ParseAmpersandString(item.Label)
		items = append(items, map[string]any{
			"index":    i,
			"text":     clean,
			"rawText":  item.Label,
			"hotkey":   stringOrEmpty(hotkey),
			"command":  item.Command,
			"disabled": menuBarItemDisabled(item),
			"items":    menuItemsForSemanticItems(item.SubItems),
		})
	}
	x1, y1, x2, y2 := mb.GetPosition()
	return map[string]any{
		"id":       SemanticID(mb),
		"kind":     "menuBar",
		"x":        x1,
		"y":        y1,
		"w":        x2 - x1 + 1,
		"h":        y2 - y1 + 1,
		"active":   mb.Active,
		"selected": mb.SelectPos,
		"items":    items,
	}
}

func semanticVMenu(menu *VMenu) map[string]any {
	base := semanticFrameBase(menu)
	base["kind"] = "menu"
	base["selected"] = menu.SelectPos
	base["top"] = menu.TopPos
	base["items"] = menuItemsForSemantic(menu)
	return base
}

func menuItemsForSemantic(menu *VMenu) []map[string]any {
	if menu == nil {
		return nil
	}
	return menuItemsForSemanticItems(menu.Items)
}

func menuItemsForSemanticItems(items []MenuItem) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for i, item := range items {
		clean, hotkey, _ := ParseAmpersandString(item.Text)
		disabled := false
		if FrameManager != nil && !item.Separator {
			disabled = FrameManager.DisabledCommands.IsDisabled(item.Command)
		}
		result = append(result, map[string]any{
			"index":     i,
			"text":      clean,
			"rawText":   item.Text,
			"hotkey":    stringOrEmpty(hotkey),
			"shortcut":  item.Shortcut,
			"command":   item.Command,
			"separator": item.Separator,
			"disabled":  disabled,
		})
	}
	return result
}

func menuBarItemDisabled(item MenuBarItem) bool {
	if len(item.SubItems) == 0 {
		return FrameManager != nil && FrameManager.DisabledCommands.IsDisabled(item.Command)
	}
	for _, sub := range item.SubItems {
		if sub.Separator {
			continue
		}
		if FrameManager == nil || !FrameManager.DisabledCommands.IsDisabled(sub.Command) {
			return false
		}
	}
	return true
}

func semanticKeyBar(kb *KeyBar) map[string]any {
	labels := kb.Normal
	modifier := "normal"
	if kb.shiftState {
		labels = kb.Shift
		modifier = "shift"
	} else if kb.ctrlState {
		labels = kb.Ctrl
		modifier = "ctrl"
	} else if kb.altState {
		labels = kb.Alt
		modifier = "alt"
	}
	items := make([]map[string]any, 0, len(labels))
	for i, label := range labels {
		items = append(items, map[string]any{
			"index": i,
			"key":   fmt.Sprintf("F%d", i+1),
			"text":  label,
		})
	}
	x1, y1, x2, y2 := kb.GetPosition()
	return map[string]any{
		"id":       SemanticID(kb),
		"kind":     "keyBar",
		"x":        x1,
		"y":        y1,
		"w":        x2 - x1 + 1,
		"h":        y2 - y1 + 1,
		"visible":  kb.IsVisible(),
		"modifier": modifier,
		"items":    items,
	}
}

func tableColumns(columns []TableColumn) []map[string]any {
	result := make([]map[string]any, 0, len(columns))
	for i, col := range columns {
		result = append(result, map[string]any{
			"index": i,
			"title": col.Title,
			"width": col.Width,
			"align": int(col.Alignment),
		})
	}
	return result
}

func tableRows(t *Table) []map[string]any {
	rows := make([]map[string]any, 0, len(t.Rows))
	for rowIdx, row := range t.Rows {
		cells := make([]string, 0, len(t.Columns))
		for colIdx := range t.Columns {
			cells = append(cells, row.GetCellText(colIdx))
		}
		selected := false
		if sr, ok := row.(SelectableRow); ok {
			selected = sr.IsSelected()
		}
		rows = append(rows, map[string]any{
			"index":    rowIdx,
			"cells":    cells,
			"selected": selected,
			"cursor":   rowIdx == t.SelectPos,
		})
	}
	return rows
}

func selectedIndices(selected map[int]bool) []int {
	result := make([]int, 0, len(selected))
	for idx, ok := range selected {
		if ok {
			result = append(result, idx)
		}
	}
	return result
}

func stringOrEmpty(r rune) string {
	if r == 0 {
		return ""
	}
	return string(r)
}

func (fm *frameManager) HandleSemanticAction(action map[string]any) bool {
	if fm == nil || action == nil {
		return false
	}
	if kind, _ := action["kind"].(string); kind == "command" {
		return fm.EmitCommand(semanticInt(action["command"]), action["args"])
	}
	for i := len(fm.frames) - 1; i >= 0; i-- {
		if h, ok := fm.frames[i].(SemanticActionHandler); ok && h.HandleSemanticAction(action) {
			fm.Redraw()
			return true
		}
	}
	target := semanticString(action["target"])
	if target == "" {
		return false
	}
	for i := len(fm.frames) - 1; i >= 0; i-- {
		if handleSemanticFrameAction(fm.frames[i], target, action) {
			fm.Redraw()
			return true
		}
	}
	return false
}

func handleSemanticFrameAction(frame Frame, target string, action map[string]any) bool {
	if SemanticID(frame) == target {
		switch semanticString(action["action"]) {
		case "close":
			frame.Close()
			return true
		case "menu_activate":
			if menu, ok := frame.(*VMenu); ok {
				idx := semanticInt(action["index"])
				if idx >= 0 && idx < len(menu.Items) && !menu.Items[idx].Separator {
					menu.SetSelectPos(idx)
					return menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
				}
			}
		}
	}
	if c, ok := frame.(Container); ok {
		return handleSemanticChildrenAction(c.GetChildren(), target, action)
	}
	return false
}

func handleSemanticChildrenAction(children []UIElement, target string, action map[string]any) bool {
	for _, child := range children {
		if SemanticID(child) == target {
			return handleSemanticElementAction(child, action)
		}
		if c, ok := child.(Container); ok {
			if handleSemanticChildrenAction(c.GetChildren(), target, action) {
				return true
			}
		}
	}
	return false
}

func handleSemanticElementAction(el UIElement, action map[string]any) bool {
	switch semanticString(action["action"]) {
	case "focus":
		el.SetFocus(true)
		return true
	case "activate":
		return el.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
	case "toggle":
		return el.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE, Char: ' ', InputSource: "qt_semantic"})
	case "set_text":
		if edit, ok := el.(*Edit); ok {
			edit.SetText(semanticString(action["text"]))
			if edit.OnTextChange != nil {
				edit.OnTextChange(edit.GetText())
			}
			return true
		}
	case "insert_text":
		if edit, ok := el.(*Edit); ok {
			edit.InsertString(semanticString(action["text"]))
			return true
		}
	case "select":
		idx := semanticInt(action["index"])
		switch w := el.(type) {
		case *RadioGroup:
			if idx >= 0 && idx < len(w.Items) {
				w.focusIdx = idx
				w.Selected = idx
				if w.OnChange != nil {
					w.OnChange(idx)
				}
				w.FireAction(nil, idx)
				return true
			}
		case *ListBox:
			if idx >= 0 && idx < len(w.Items) {
				w.SetSelectPos(idx)
				return true
			}
		case *ComboBox:
			if idx >= 0 && idx < len(w.Menu.Items) {
				w.Menu.SetSelectPos(idx)
				w.Edit.SetText(w.Menu.Items[idx].Text)
				return true
			}
		}
	}
	return false
}

func semanticString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func semanticInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint:
		return int(n)
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}
