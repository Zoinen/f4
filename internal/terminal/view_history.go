package terminal

import (
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/textlayout"
	"github.com/unxed/vtui"
)

// CloneHistoryFrom copies primary-screen output into a fresh terminal before
// its shell starts. The source shell's screen and protocol state stay separate.
func (tv *TerminalView) CloneHistoryFrom(other *TerminalView, omitPrompt bool) {
	other.mu.Lock()
	defer other.mu.Unlock()
	tv.ResetBuffer(other.Width, other.Height)
	tv.mu.Lock()
	defer tv.mu.Unlock()

	tv.GridHistory = make([][]vtui.CharInfo, len(other.GridHistory))
	for i, row := range other.GridHistory {
		tv.GridHistory[i] = append([]vtui.CharInfo(nil), row...)
	}
	tv.GridHistoryWrap = append([]bool(nil), other.GridHistoryWrap...)
	data, _ := other.Pt.Bytes()
	tv.Pt = piecetable.New(data)
	tv.Li = piecetable.NewLineIndex()
	tv.Li.Rebuild(tv.Pt)
	tv.Engine = textlayout.NewWrapEngine(tv.Pt, tv.Li)
	tv.Engine.SetWidth(tv.Width)
	tv.styles = append([]StyleChange(nil), other.styles...)
	tv.lastAttr = other.lastAttr
	tv.Palette = other.Palette

	last := min(other.Height, len(other.Lines))
	if omitPrompt && !other.UseAltScreen {
		last = min(last, max(0, other.CursorY))
	} else {
		for last > 0 && !other.rowHasText(last-1) {
			last--
		}
	}
	first := 0
	if len(tv.GridHistory) == 0 && tv.Pt.Size() == 0 {
		for first < last && !other.rowHasText(first) {
			first++
		}
	}
	for row := first; row < last; row++ {
		tv.GridHistory = append(tv.GridHistory, append([]vtui.CharInfo(nil), other.Lines[row]...))
		tv.GridHistoryWrap = append(tv.GridHistoryWrap, other.WrapFlags[row])
	}
	tv.trimGridHistoryLocked()
	tv.CursorY = 0
	tv.semanticFollowTail = true
}
