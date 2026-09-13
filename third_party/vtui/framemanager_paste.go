package vtui

import (
	"time"

	"github.com/unxed/vtinput"
)

// QueuePaste places one complete clipboard transaction in the same FIFO as
// physical keys. The private payload is owned until consumption or queue failure;
// unrelated input fields and the priority task queue are deliberately unused.
func (fm *frameManager) QueuePaste(queue chan<- *vtinput.InputEvent, text string) bool {
	if queue == nil || fm.IsShutdown() {
		return false
	}
	event := &vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: true, InputSource: "extui"}
	fm.pendingPastes.Store(event, text)
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	select {
	case queue <- event:
		if fm.IsShutdown() {
			fm.pendingPastes.Delete(event)
		}
		return true
	case <-timer.C:
		fm.pendingPastes.Delete(event)
		DebugLog("clipboard paste queue remained blocked")
		return false
	}
}

// dispatchFramePaste retains each control's existing bracketed-paste behavior,
// but runs it within one semantic input update. Pasting literal characters must
// not trigger hundreds of global dispatches, menu snapshots, or scene commits.
func dispatchFramePaste(frame Frame, text string) bool {
	frame.ProcessKey(&vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: true, InputSource: "extui"})
	for _, char := range text {
		frame.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: char, InputSource: "extui_paste"})
	}
	frame.ProcessKey(&vtinput.InputEvent{Type: vtinput.PasteEventType, InputSource: "extui"})
	return true
}
