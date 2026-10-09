package panel

import (
	"fmt"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func newWheelTestPanel(t *testing.T, entries int) (*PanelsFrame, *FileSystemPanel) {
	t.Helper()

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()

	oldCfg := config.App
	t.Cleanup(func() { config.App = oldCfg })
	config.App.WheelPanelUp = 3
	config.App.WheelPanelDown = 3

	pf := NewPanelsFrame()
	t.Cleanup(pf.Close)
	pf.ResizeConsole(80, 25)

	p := pf.Panels[0].(*FileSystemPanel)
	if p.CancelLoad != nil {
		p.CancelLoad()
	}
	p.IsLoading = false
	for i := 0; i < entries; i++ {
		p.Entries = append(p.Entries, &FileEntry{VFSItem: vfs.VFSItem{Name: fmt.Sprintf("f%03d", i)}})
	}
	p.Refresh()
	p.SetCursorIndex(0)
	return pf, p
}

// wheelNotch sends one wheel notch over the panel the way the terminal does.
func wheelNotch(t *testing.T, p *FileSystemPanel, dir int) {
	t.Helper()
	x1, y1, _, _ := p.GetPosition()
	if !p.ProcessMouse(&vtinput.InputEvent{
		Type:           vtinput.MouseEventType,
		MouseX:         testutil.Int16(x1 + 2),
		MouseY:         testutil.Int16(y1 + 2),
		WheelDirection: dir,
	}) {
		t.Fatalf("wheel notch %d was not handled", dir)
	}
}

func TestFilePanel_WheelLoneNotchDoesNotQueue(t *testing.T) {
	_, p := newWheelTestPanel(t, 60)

	wheelNotch(t, p, -1)

	if got := p.GetCursorIndex(); got != 3 {
		t.Errorf("cursor after one notch = %d, want the configured 3 lines", got)
	}
	if p.wheel.Direction() != 1 {
		t.Errorf("the notch did not reach the ramp: direction = %d, want 1", p.wheel.Direction())
	}
	if p.wheel.Pending() != 0 {
		t.Errorf("a lone notch queued %d lines, want none", p.wheel.Pending())
	}
	if p.wheel.IsCoasting() {
		t.Error("a lone notch started the coast timer")
	}
}

func TestFilePanel_WheelSpinCoastsAhead(t *testing.T) {
	oldCfg := config.App
	t.Cleanup(func() { config.App = oldCfg })
	config.App.WheelAcceleration = config.WheelAccelerationMax

	_, p := newWheelTestPanel(t, 60)

	wheelNotch(t, p, -1)
	if got := p.GetCursorIndex(); got != 3 {
		t.Fatalf("cursor after one notch = %d, want 3", got)
	}

	// Three more notches ten milliseconds apart: the ramp doubles each time
	// (2, 4, 8) and queues 1 + 3 + 7 extra lines.
	base := time.Now()
	p.wheel.NotchAt(1, base.Add(10*time.Millisecond), p.wheelScrollBy)
	p.wheel.NotchAt(1, base.Add(20*time.Millisecond), p.wheelScrollBy)
	p.wheel.NotchAt(1, base.Add(30*time.Millisecond), p.wheelScrollBy)
	if p.wheel.Pending() != 11 {
		t.Fatalf("queued lines = %d, want 11", p.wheel.Pending())
	}

	steps := 0
	for p.wheel.Tick() {
		steps++
		if steps > 1000 {
			t.Fatal("the coast never stopped")
		}
	}
	if got := p.GetCursorIndex(); got != 3+11 {
		t.Errorf("cursor after the coast = %d, want %d", got, 3+11)
	}
	if p.wheel.Pending() != 0 {
		t.Errorf("queued lines left after the coast = %d, want 0", p.wheel.Pending())
	}
}

func TestFilePanel_WheelWeakestRampQueuesNothing(t *testing.T) {
	_, p := newWheelTestPanel(t, 60)

	oldCfg := config.App
	t.Cleanup(func() { config.App = oldCfg })
	config.App.WheelAcceleration = config.WheelAccelerationMin

	wheelNotch(t, p, -1)
	base := time.Now()
	p.wheel.NotchAt(1, base.Add(10*time.Millisecond), p.wheelScrollBy)
	p.wheel.NotchAt(1, base.Add(20*time.Millisecond), p.wheelScrollBy)

	if p.wheel.Pending() != 0 {
		t.Errorf("queued %d lines on the weakest ramp, want none", p.wheel.Pending())
	}
	if p.wheel.IsCoasting() {
		t.Error("the weakest ramp still started the coast timer")
	}
}

func TestFilePanel_WheelDirectionChangeDropsQueue(t *testing.T) {
	oldCfg := config.App
	t.Cleanup(func() { config.App = oldCfg })
	config.App.WheelAcceleration = config.WheelAccelerationMax

	_, p := newWheelTestPanel(t, 60)

	base := time.Now()
	p.wheel.NotchAt(1, base, p.wheelScrollBy)
	p.wheel.NotchAt(1, base.Add(10*time.Millisecond), p.wheelScrollBy)
	if p.wheel.Pending() == 0 {
		t.Fatal("test setup: the spin queued nothing")
	}

	p.wheel.NotchAt(-1, base.Add(20*time.Millisecond), p.wheelScrollBy)

	if p.wheel.Pending() != 0 {
		t.Errorf("queued lines after the reversal = %d, want 0", p.wheel.Pending())
	}
	if p.wheel.Direction() != -1 {
		t.Errorf("direction after the reversal = %d, want -1", p.wheel.Direction())
	}
	if p.wheel.IsCoasting() {
		t.Error("the reversal left the coast timer running")
	}
}

func TestFilePanel_PointerGestureStopsTheCoast(t *testing.T) {
	oldCfg := config.App
	t.Cleanup(func() { config.App = oldCfg })
	config.App.WheelAcceleration = config.WheelAccelerationMax

	_, p := newWheelTestPanel(t, 60)

	base := time.Now()
	p.wheel.NotchAt(1, base, p.wheelScrollBy)
	p.wheel.NotchAt(1, base.Add(10*time.Millisecond), p.wheelScrollBy)
	if p.wheel.Pending() == 0 {
		t.Fatal("test setup: the spin queued nothing")
	}

	x1, y1, _, _ := p.GetPosition()
	p.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseX:      testutil.Int16(x1 + 2),
		MouseY:      testutil.Int16(y1 + 2),
		KeyDown:     true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
	})

	if p.wheel.Pending() != 0 {
		t.Errorf("queued lines after a click = %d, want 0", p.wheel.Pending())
	}
	if p.wheel.IsCoasting() {
		t.Error("a click left the coast timer running")
	}
}

func TestFilePanel_WheelScrollByStopsAtTheEndOfTheList(t *testing.T) {
	_, p := newWheelTestPanel(t, 10)

	// Walk to the end of the list: the first jump moves it, the next one
	// runs out of rows and reports no movement, which is what tells the
	// coast to stop instead of spinning in place.
	if !p.wheelScrollBy(500) {
		t.Fatal("the first jump did not move the panel")
	}
	if got := p.GetCursorIndex(); got != len(p.Entries)-1 {
		t.Errorf("cursor = %d, want the last entry %d", got, len(p.Entries)-1)
	}
	if p.wheelScrollBy(500) {
		t.Error("a further jump past the end of the list reported movement")
	}

	// And the same the other way round, back to the first entry.
	if !p.wheelScrollBy(-500) {
		t.Fatal("the jump back did not move the panel")
	}
	if got := p.GetCursorIndex(); got != 0 {
		t.Errorf("cursor = %d, want the first entry 0", got)
	}
	if p.wheelScrollBy(-500) {
		t.Error("a jump before the start of the list reported movement")
	}
}
