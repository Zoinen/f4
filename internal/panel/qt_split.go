package panel

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtui"
)

// setSemanticSplit records geometry already presented by Qt. The next ordinary
// console layout applies it; resizing the file tables here would refresh their
// rows on every mouse move. No catalog, selection, or cursor changes are needed.
func (pf *PanelsFrame) setSemanticSplit(action map[string]any) bool {
	if vtui.FrameManager != nil {
		vtui.FrameManager.DeclareCurrentInputUnchanged()
	}
	if pf.Closed || semantic.String(action["target"]) != vtui.SemanticID(pf) ||
		!pf.ShowPanels || pf.Wide || pf.LastW < 20 {
		return false
	}
	ratio := semantic.Int(action["ratioMillionths"])
	if ratio <= 0 || ratio >= 1000000 {
		return false
	}
	split := int((int64(pf.LastW)*int64(ratio) + 500000) / 1000000)
	if ratio == 500000 {
		split = pf.LastW / 2
	}
	split = max(10, min(pf.LastW-10, split))
	next := pf.LastW/2 - split
	if next != pf.WidthDecrement {
		pf.WidthDecrement = next
		pf.nativeSplitLayoutPending = true
		config.App.WidthDecrement = next
		config.RequestSaveConfig()
	}
	return true
}
