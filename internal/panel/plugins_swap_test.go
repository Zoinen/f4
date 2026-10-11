package panel

import (
	"testing"

	"github.com/unxed/f4/internal/appcmd"
)

// Ctrl+U swaps the file panels, and a panel plugin shown over one of them
// (ProcList) has to stay with it instead of being left on its old side
// (f4#312).
func TestSwapPanelsMovesPluginPanelWithItsSide(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()

	plug := &PluginPanelInstance{host: pf, slot: 1, controller: &greedyController{}}
	pf.AltPanels[1] = plug
	pf.ActiveIdx = 1

	pf.HandleCommand(appcmd.CmSwapPanels, nil)
	if pf.AltPanels[0] != plug || pf.AltPanels[1] != nil || plug.slot != 0 {
		t.Fatalf("after one swap: left=%v right=%v slot=%d, want the plugin panel on the left", pf.AltPanels[0], pf.AltPanels[1], plug.slot)
	}

	pf.HandleCommand(appcmd.CmSwapPanels, nil)
	if pf.AltPanels[1] != plug || pf.AltPanels[0] != nil || plug.slot != 1 {
		t.Fatalf("after two swaps: left=%v right=%v slot=%d, want the plugin panel back on the right", pf.AltPanels[0], pf.AltPanels[1], plug.slot)
	}
}
