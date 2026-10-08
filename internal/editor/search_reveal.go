package editor

// A selection's caret lies just after the match. At a wrapped row boundary
// it can be outside the viewport even though every matched character is visible.
func (ev *EditorView) searchMatchVisible(off, length int) bool {
	if length <= 0 || !ev.EnsureEngineWidth() {
		return false
	}
	height := ev.ViewportHeight()
	if ev.HexMode {
		return off >= ev.HexTopOffset && off+length <= ev.HexTopOffset+height*16
	}
	if ev.DecodeMode {
		return false
	}
	firstRow, firstCol := ev.Engine.LogicalToVisual(off)
	lastRow, lastCol := ev.Engine.LogicalToVisual(off + length - 1)
	if firstRow < ev.ScrollTopRow || lastRow >= ev.ScrollTopRow+height {
		return false
	}
	return ev.WordWrap || (firstCol >= ev.ScrollLeft && lastCol < ev.ScrollLeft+ev.viewportWidth())
}
