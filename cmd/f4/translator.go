package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// translatorElement is a short-lived UIElement used for controls that are
// rendered by f4 but are not represented as a vtui child element. It gives
// the shared translator the same text/owner/help information as a regular
// ScreenObject without changing the live widget tree.
// zoin-bot: f4's panels and menu rows need this adapter because their visible
// contents are backed by application data rather than standalone widgets.
type translatorElement struct {
	vtui.ScreenObject
}

func newTranslatorElement(text string, owner vtui.CommandHandler, help string, x1, y1, x2, y2 int) vtui.UIElement {
	e := &translatorElement{}
	e.SetText(text)
	e.SetHelp(help)
	e.SetPosition(x1, y1, x2, y2)
	e.SetVisible(true)
	if owner != nil {
		e.SetOwner(owner)
	}
	return e
}

func isTranslatorMouseEvent(e *vtinput.InputEvent) bool {
	if e == nil || e.Type != vtinput.MouseEventType || !e.KeyDown {
		return false
	}
	if e.ButtonState&vtinput.RightmostButtonPressed == 0 ||
		e.MouseEventFlags&vtinput.MouseMoved != 0 {
		return false
	}
	ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
	alt := e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
	return ctrl && alt
}

// handleTranslatorMouseEvent is installed as an f4 event-filter fallback.
// vtui handles ordinary Window children before invoking EventFilter; this
// path fills the gap for f4's custom PanelsFrame, VMenu rows, and global menu
// bar, whose visible targets are not exposed as UIElement children.
func handleTranslatorMouseEvent(e *vtinput.InputEvent) bool {
	if !isTranslatorMouseEvent(e) {
		return false
	}
	target := translatorTargetAt(int(e.MouseX), int(e.MouseY))
	if target == nil {
		return false
	}

	vtui.SetClipboard(formatTranslatorReport(target))
	vtui.ShowToast("Translator info copied to clipboard", 3*time.Second)
	return true
}

func formatTranslatorReport(target vtui.UIElement) string {
	text := ""
	if txtObj, ok := target.(interface{ GetText() string }); ok {
		text = txtObj.GetText()
	}

	key := ""
	if text != "" {
		key = vtui.ReverseLookup(text)
	}

	var contexts []string
	addContext := func(help string, prepend bool) {
		if help == "" {
			return
		}
		for _, existing := range contexts {
			if existing == help {
				return
			}
		}
		if prepend {
			contexts = append([]string{help}, contexts...)
		} else {
			contexts = append(contexts, help)
		}
	}
	addContext(target.GetHelp(), false)
	owner := target.GetOwner()
	for owner != nil {
		addContext(owner.GetHelp(), true)
		if obj, ok := owner.(interface{ GetOwner() vtui.CommandHandler }); ok {
			owner = obj.GetOwner()
		} else {
			break
		}
	}

	report := "--- f4 Translator Tool ---\n"
	if key != "" {
		report += fmt.Sprintf("Key:  %s\nText: %s\n", key, text)
	} else if text != "" {
		report += fmt.Sprintf("Key:  <HARDCODED>\nText: %s\n", text)
	} else {
		report += "Key:  <NO TEXT>\n"
	}

	context := "None"
	if len(contexts) > 0 {
		context = strings.Join(contexts, " -> ")
	}
	return report + fmt.Sprintf("Help Context: %s\n", context)
}

func translatorTargetAt(x, y int) vtui.UIElement {
	fm := vtui.FrameManager
	if fm == nil {
		return nil
	}

	frames := fm.GetActiveFrames(fm.ActiveIdx)
	if len(frames) == 0 {
		return nil
	}
	top := frames[len(frames)-1]

	// zoin-bot: match FrameManager's normal global-component priority. A
	// visible menu bar is painted over the frame below it and must win there.
	menuVisible := func(menu *vtui.MenuBar) bool {
		if menu == nil || !menu.IsVisible() {
			return false
		}
		return menu.Active || AppConfig.AlwaysShowMenuBar
	}
	if menu := fm.GetActiveMenuBar(); menuVisible(menu) && menu.HitTest(x, y) {
		canUseMenu := !top.IsModal() || top.GetType() == vtui.TypeMenu || top.GetMenuBar() == menu
		if canUseMenu {
			if target := translatorMenuBarTarget(menu, x, y); target != nil {
				return target
			}
		}
	}

	for i := len(frames) - 1; i >= 0; i-- {
		frame := frames[i]
		if !frame.HitTest(x, y) {
			continue
		}
		if target := translatorFrameTarget(frame, x, y); target != nil {
			return target
		}
		// The regular dispatcher stops at the first modal or hit frame even
		// when that frame declines the event. Do not inspect frames behind it.
		return nil
	}
	return nil
}

func translatorFrameTarget(frame vtui.Frame, x, y int) vtui.UIElement {
	if provider, ok := frame.(interface{ GetElementAt(x, y int) vtui.UIElement }); ok {
		if target := provider.GetElementAt(x, y); target != nil {
			return target
		}
	}

	switch frame := frame.(type) {
	case *vtui.VMenu:
		return translatorVMenuTarget(frame, x, y)
	case *PanelsFrame:
		return frame.translatorElementAt(x, y)
	default:
		return nil
	}
}

func translatorVMenuTarget(menu *vtui.VMenu, x, y int) vtui.UIElement {
	idx := menu.GetClickIndex(y)
	if idx < 0 || idx >= len(menu.Items) || menu.Items[idx].Separator {
		return nil
	}
	item := menu.Items[idx]
	x1, y1, x2, y2 := menu.GetPosition()
	rowY := menu.Y1 + menu.MarginTop + idx - menu.TopPos
	if rowY < y1 || rowY > y2 {
		return nil
	}
	return newTranslatorElement(item.Text, menu, "", x1+1, rowY, x2-1, rowY)
}

func translatorMenuBarTarget(menu *vtui.MenuBar, x, y int) vtui.UIElement {
	for i, item := range menu.Items {
		x1 := menu.GetItemX(i)
		x2 := x1
		if i < len(menu.Items)-1 {
			x2 = menu.GetItemX(i+1) - 1
		} else {
			clean, _, _ := vtui.ParseAmpersandString(item.Label)
			x2 += runewidth.StringWidth("  "+clean+"  ") - 1
		}
		if x >= x1 && x <= x2 && y >= menu.Y1 && y <= menu.Y2 {
			return newTranslatorElement(item.Label, menu, "", x1, menu.Y1, x2, menu.Y2)
		}
	}
	return nil
}

func (pf *PanelsFrame) translatorElementAt(x, y int) vtui.UIElement {
	if pf == nil {
		return nil
	}

	if pf.cmdLine != nil && pf.cmdLine.IsVisible() && pf.cmdLine.Edit != nil && pf.cmdLine.Edit.HitTest(x, y) {
		return newTranslatorElement(pf.cmdLine.Edit.GetText(), pf, "", pf.cmdLine.Edit.X1, pf.cmdLine.Edit.Y1, pf.cmdLine.Edit.X2, pf.cmdLine.Edit.Y2)
	}
	if !pf.showPanels {
		return nil
	}

	if i := pf.hitAltPanel(x, y); i >= 0 {
		x1, y1, x2, y2 := pf.altPanels[i].GetPosition()
		return newTranslatorElement("", pf, "", x1, y1, x2, y2)
	}

	for i, panel := range pf.panels {
		if panel == nil || (pf.wide && i != pf.widePanel) ||
			(!pf.wide && i == 0 && !pf.showLeftPanel) ||
			(!pf.wide && i == 1 && !pf.showRightPanel) {
			continue
		}
		x1, y1, x2, y2 := panel.GetPosition()
		if x < x1 || x > x2 || y < y1 || y > y2 {
			continue
		}
		if fsp, ok := panel.(*FileSystemPanel); ok {
			return fsp.translatorElementAt(x, y, pf)
		}
		return newTranslatorElement("", pf, "", x1, y1, x2, y2)
	}
	return nil
}

func (fp *FileSystemPanel) translatorElementAt(x, y int, owner vtui.CommandHandler) vtui.UIElement {
	if fp == nil {
		return nil
	}
	if fp.pathTitleHitTest(x, y) {
		return newTranslatorElement(fp.currentTitle, owner, "", fp.X1+1, fp.Y1, fp.X2-1, fp.Y1)
	}
	if fp.table == nil || !fp.table.HitTest(x, y) {
		return nil
	}

	if fp.table.ShowHeader && y == fp.table.Y1 {
		if column := panelTranslatorColumn(fp.table, x); column >= 0 && column < len(fp.table.Columns) {
			c := fp.table.Columns[column]
			return newTranslatorElement(c.Title, owner, "", fp.table.X1, y, fp.table.X2, y)
		}
		return nil
	}

	rowOffset := y - (fp.table.Y1 + fp.table.MarginTop)
	if rowOffset < 0 || rowOffset >= fp.table.ViewHeight {
		return nil
	}
	column := panelTranslatorColumn(fp.table, x)
	if column < 0 {
		return nil
	}
	row := fp.table.TopPos + rowOffset
	if fp.entryIndex(row, column) < 0 || fp.entryIndex(row, column) >= len(fp.entries) {
		return nil
	}
	return newTranslatorElement(fp.GetCellText(row, column), owner, "", fp.table.X1, y, fp.table.X2, y)
}

func panelTranslatorColumn(table *vtui.Table, x int) int {
	if table == nil {
		return -1
	}
	columnX := table.X1
	for i, column := range table.Columns {
		width := column.Width
		if width <= 0 {
			return -1
		}
		if x >= columnX && x < columnX+width {
			return i
		}
		columnX += width
		if i < len(table.Columns)-1 {
			columnX++
		}
	}
	return -1
}
