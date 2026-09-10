package dialog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func portableSettingsMouseCoordinate(value int) int16 {
	return int16(value) // #nosec G115 -- the test dialog is inside the test screen.
}

// portableSettingsDialogButtonBounds is the horizontal extent of the dialog's
// two buttons, which the resize has to keep centred on the dialog.
func portableSettingsDialogButtonBounds(t *testing.T, dlg *portableSettingsDialog) (int, int) {
	t.Helper()
	minX, maxX := 0, 0
	buttonCount := 0
	for _, child := range dlg.GetChildren() {
		button, ok := child.(*vtui.Button)
		if !ok {
			continue
		}
		x1, _, x2, _ := button.GetPosition()
		if buttonCount == 0 || x1 < minX {
			minX = x1
		}
		if buttonCount == 0 || x2 > maxX {
			maxX = x2
		}
		buttonCount++
	}
	if buttonCount != 2 {
		t.Fatalf("portable settings has %d buttons, want 2", buttonCount)
	}
	return minX, maxX
}

// The portable settings dialog is dragged by its resize corner: the width
// grows symmetrically about the centre so the dialog stays centred, and the
// height stays where the layout put it.
func TestPortableSettingsDialogResizesHorizontallyOnly(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(120, 30)
	vtui.FrameManager.Init(screen)
	theme.SetDefaultF4Palette()

	tmpDir := t.TempDir()
	exe := filepath.Join(tmpDir, "f4")
	if err := os.WriteFile(exe, nil, 0600); err != nil {
		t.Fatal(err)
	}
	oldExecutable := config.Executable
	oldUserConfigDir := config.UserConfigDir
	config.Executable = func() (string, error) { return exe, nil }
	config.UserConfigDir = func() (string, error) { return tmpDir, nil }
	config.ResetConfigDirForTest()
	t.Cleanup(func() {
		config.Executable = oldExecutable
		config.UserConfigDir = oldUserConfigDir
		config.ResetConfigDirForTest()
	})

	ShowPortableSettings()
	top := vtui.FrameManager.GetTopFrame()
	if top == nil {
		t.Fatal("portable settings did not open a dialog")
	}
	dlg, ok := top.(*portableSettingsDialog)
	if !ok {
		t.Fatalf("portable settings frame has type %T, want *portableSettingsDialog", top)
	}

	startX1, startX2, startY2 := dlg.X1, dlg.X2, dlg.Y2
	if !dlg.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      portableSettingsMouseCoordinate(startX2),
		MouseY:      portableSettingsMouseCoordinate(startY2),
	}) {
		t.Fatal("portable settings resize corner was not handled")
	}
	dlg.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      portableSettingsMouseCoordinate(startX2 + 8),
		MouseY:      portableSettingsMouseCoordinate(startY2 + 4),
	})
	dlg.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType})
	if dlg.X1 != startX1-8 {
		t.Errorf("portable settings left edge = %d, want %d", dlg.X1, startX1-8)
	}
	if dlg.X2 != startX2+8 {
		t.Errorf("portable settings right edge = %d, want %d", dlg.X2, startX2+8)
	}
	if dlg.X1+dlg.X2 != startX1+startX2 {
		t.Errorf("portable settings center moved from %d to %d", startX1+startX2, dlg.X1+dlg.X2)
	}
	if dlg.Y2 != startY2 {
		t.Errorf("portable settings bottom edge = %d, want fixed %d", dlg.Y2, startY2)
	}
	buttonX1, buttonX2 := portableSettingsDialogButtonBounds(t, dlg)
	if buttonX1+buttonX2 != dlg.X1+dlg.X2 {
		t.Fatalf("portable settings buttons center = %d, dialog center = %d before screen resize", buttonX1+buttonX2, dlg.X1+dlg.X2)
	}

	width := dlg.X2 - dlg.X1 + 1
	vtui.FrameManager.Resize(120, 30)
	if got := dlg.X2 - dlg.X1 + 1; got != width {
		t.Errorf("portable settings width after screen resize = %d, want %d", got, width)
	}
	if got := dlg.X1 + dlg.X2; got != 119 {
		t.Errorf("portable settings center after screen resize = %d, want 119", got)
	}
	buttonX1, buttonX2 = portableSettingsDialogButtonBounds(t, dlg)
	if got := buttonX1 + buttonX2; got != dlg.X1+dlg.X2 {
		t.Errorf("portable settings buttons center after screen resize = %d, dialog center = %d", got, dlg.X1+dlg.X2)
	}
}
