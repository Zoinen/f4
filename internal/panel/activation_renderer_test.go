package panel

import (
	"github.com/unxed/vtui"
)

type searchFirstActivationRenderer struct {
	calls           int
	side            int
	invalidations   int
	activationMenus int
}

func (*searchFirstActivationRenderer) Render([]vtui.CharInfo, []vtui.CharInfo, int, int, bool) {
}

func (*searchFirstActivationRenderer) SetCursor(int, int, bool, vtui.CursorShape) {}

func (*searchFirstActivationRenderer) SetPalette(*[256]uint32) {}

func (*searchFirstActivationRenderer) SetWindowTitle(string) {}

func (*searchFirstActivationRenderer) Flush() {}

func (r *searchFirstActivationRenderer) QueuePanelActivationState(side int, _ string, _ map[string]any) {
	r.calls++
	r.side = side
}

func (r *searchFirstActivationRenderer) InvalidateSemanticSceneUpdate() {
	r.invalidations++
}

func (r *searchFirstActivationRenderer) AllowSemanticMenuAfterPanelActivation() {
	r.activationMenus++
}
