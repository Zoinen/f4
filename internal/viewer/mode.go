package viewer

// CycleViewMode advances text, hex and disassembly while invalidating unfinished layout work.
func (vv *ViewerView) CycleViewMode() {
	vv.SemanticLayoutRevision++
	vv.SemanticNeedsReflow = true
	vv.SemanticPendingScroll = false
	vv.SemanticProjection, vv.ConsoleProjection = nil, nil
	vv.SemanticWrapSeek = SemanticWrapSeekState{}
	// Whichever way this goes, the view mode is now the user's and
	// a later codepage switch must not second-guess it.
	vv.HexAuto = false
	if !vv.HexMode && !vv.DecodeMode {
		vv.HexMode = true
		vv.TopOffset &= ^int64(0xF)
	} else if vv.HexMode {
		vv.HexMode = false
		vv.DecodeMode = true
	} else {
		vv.DecodeMode = false
	}
}
