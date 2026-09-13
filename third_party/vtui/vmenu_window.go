package vtui

import "github.com/unxed/vtinput"

// windowControl reuses Window's moving, resizing and maximize/restore behavior
// while the menu keeps its identity, keyboard handling and virtualized rows.
func (m *VMenu) windowControl() *Window {
	if m.SemanticPresentation != "window" && m.SemanticPresentation != "fullWidth" {
		return nil
	}
	if m.windowFrame == nil {
		m.windowFrame = NewWindow(m.X1, m.Y1, m.X2, m.Y2, m.title)
		m.windowFrame.MinW, m.windowFrame.MinH = 20, 5
		m.windowFrame.OnResult = func(int) { m.SetExitCode(-1) }
	}
	w := m.windowFrame
	if w.X1 != m.X1 || w.Y1 != m.Y1 || w.X2 != m.X2 || w.Y2 != m.Y2 {
		w.SetPosition(m.X1, m.Y1, m.X2, m.Y2)
	}
	return w
}

func (m *VMenu) processWindowMouse(e *vtinput.InputEvent) bool {
	w := m.windowControl()
	if w == nil {
		return false
	}
	// Only window borders own window operations; interior events belong to rows.
	x, y := int(e.MouseX), int(e.MouseY)
	border := x == m.X1 || x == m.X2 || y == m.Y1 || y == m.Y2
	if !w.isDragging && !w.isResizing && !border {
		return false
	}
	if !w.ProcessMouse(e) {
		return false
	}
	m.SetPosition(w.X1, w.Y1, w.X2, w.Y2)
	m.SetSelectPos(m.SelectPos)
	return true
}

// DrawWindowControls restores standard title controls after a custom painter.
func (m *VMenu) DrawWindowControls(scr *ScreenBuf) {
	w := m.windowControl()
	if w == nil || m.X2-m.X1 < 10 {
		return
	}
	offset := w.frame.getControlOffset()
	attr := withForeground(Palette[m.ColorBoxIdx], Palette[m.ColorTitleIdx])
	NewPainter(scr).DrawCloseButton(m.X2, m.Y1, offset, attr)
	zoom := string(UIStrings.CloseBrackets[0]) + string(UIStrings.ZoomSymbol) + string(UIStrings.CloseBrackets[1])
	scr.Write(m.X2-offset-3, m.Y1, StringToCharInfo(zoom, attr))
}
