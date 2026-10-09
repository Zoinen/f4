package panel

import (
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// greedyController is a complete panel controller that consumes every key,
// as a list-like panel does with the page keys.
type greedyController struct {
	keys           int
	x1, y1, x2, y2 int
	focused        bool
}

func (g *greedyController) Show(*vtui.ScreenBuf)                  {}
func (g *greedyController) ProcessKey(*vtinput.InputEvent) bool   { g.keys++; return true }
func (g *greedyController) ProcessMouse(*vtinput.InputEvent) bool { return false }
func (g *greedyController) SetFocus(f bool)                       { g.focused = f }
func (g *greedyController) IsFocused() bool                       { return g.focused }
func (g *greedyController) SetPosition(x1, y1, x2, y2 int)        { g.x1, g.y1, g.x2, g.y2 = x1, y1, x2, y2 }
func (g *greedyController) GetPosition() (int, int, int, int)     { return g.x1, g.y1, g.x2, g.y2 }
func (g *greedyController) GetSelectedName() string               { return "" }
func (g *greedyController) SetContext(vfs.PanelContext)           {}
func (g *greedyController) Close() error                          { return nil }

// Ctrl+PgUp has to close a panel plugin even when its controller would take
// the key for itself (ProcList's table reads it as page up; f4#312).
func TestPluginPanelCtrlPgUpClosesEvenWhenTheControllerTakesTheKey(t *testing.T) {
	g := &greedyController{}
	closed := 0
	p := &PluginPanelInstance{controller: g, closeFn: func() { closed++ }}

	pgUp := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_PRIOR}
	if !p.ProcessKey(pgUp) || g.keys != 1 || closed != 0 {
		t.Fatalf("plain PgUp: keys=%d closed=%d, want the controller to take it", g.keys, closed)
	}

	ctrlPgUp := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_PRIOR, ControlKeyState: vtinput.LeftCtrlPressed}
	if !p.ProcessKey(ctrlPgUp) || closed != 1 || g.keys != 1 {
		t.Fatalf("Ctrl+PgUp: keys=%d closed=%d, want the panel closed without asking the controller", g.keys, closed)
	}
}
