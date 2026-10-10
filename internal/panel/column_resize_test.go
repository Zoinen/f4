package panel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

// TestFileSystemPanel_ColumnResizeDrag is f4#246's first atomic part: live
// mouse resize of the border between two columns in a single-stripe mode
// (ViewModeDetailed's default "N,SC" -- Name and Size). It follows the same
// mouse-event style as the existing header sort click and right-drag-select
// tests in file_panel_test.go: synthetic vtinput.InputEvent values fed
// straight into FileSystemPanel.ProcessMouse.
func TestFileSystemPanel_ColumnResizeDrag(t *testing.T) {
	resetPanelViewModes(true)
	defer resetPanelViewModes(true)

	// SetPanelViewModeSettings (called when the drag ends) saves to
	// panel_modes.ini beside config.UserConfigDir(); redirect that seam to a
	// scratch directory instead of the real profile, the way
	// panels_frame_test.go's workspace-session tests already do.
	cfgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfgDir, "f4", "settings"), 0o700); err != nil {
		t.Fatal(err)
	}
	oldUserConfigDir := config.UserConfigDir
	config.UserConfigDir = func() (string, error) { return cfgDir, nil }
	t.Cleanup(func() { config.UserConfigDir = oldUserConfigDir })

	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(t.TempDir()))
	waitForLoad(t, fp)
	fp.SetViewMode(ViewModeDetailed)

	if len(fp.Table.Columns) < 2 {
		t.Fatalf("ViewModeDetailed has %d columns, want at least 2", len(fp.Table.Columns))
	}
	headerY := fp.Table.Y1
	borderX := fp.Table.X1 + fp.Table.Columns[0].Width
	startLeft := fp.Table.Columns[0].Width
	startRight := fp.Table.Columns[1].Width

	// Mouse-down exactly on the one-cell separator grabs the border.
	fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		MouseX: testutil.Int16(borderX), MouseY: testutil.Int16(headerY),
		ButtonState: vtinput.FromLeft1stButtonPressed,
	})
	if !fp.columnResizeActive {
		t.Fatal("mouse-down on the column border did not start a resize drag")
	}

	const delta = 3
	fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		MouseX: testutil.Int16(borderX + delta), MouseY: testutil.Int16(headerY),
		ButtonState:     vtinput.FromLeft1stButtonPressed,
		MouseEventFlags: vtinput.MouseMoved,
	})
	if got, want := fp.Table.Columns[0].Width, startLeft+delta; got != want {
		t.Errorf("during drag: column 0 width = %d, want %d", got, want)
	}
	if got, want := fp.Table.Columns[1].Width, startRight-delta; got != want {
		t.Errorf("during drag: column 1 width = %d, want %d", got, want)
	}
	if got := PanelViewModeSettings(ViewModeDetailed).Columns[1].Width; got != startRight {
		t.Errorf("mid-drag: persisted column width already changed to %d, want unchanged %d", got, startRight)
	}

	fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: false,
		MouseX: testutil.Int16(borderX + delta), MouseY: testutil.Int16(headerY),
	})
	if fp.columnResizeActive {
		t.Error("releasing the mouse did not end the resize drag")
	}
	if got, want := PanelViewModeSettings(ViewModeDetailed).Columns[0].Width, startLeft+delta; got != want {
		t.Errorf("after release: persisted column 0 width = %d, want %d", got, want)
	}
	if got, want := PanelViewModeSettings(ViewModeDetailed).Columns[1].Width, startRight-delta; got != want {
		t.Errorf("after release: persisted column 1 width = %d, want %d", got, want)
	}
}

// TestFileSystemPanel_HeaderColumnBorderAt_RequiresSingleStripe checks the
// documented scope limit of this first part: Brief/Medium's multiple
// stripes of repeating columns do not offer a resize border at all yet.
func TestFileSystemPanel_HeaderColumnBorderAt_RequiresSingleStripe(t *testing.T) {
	resetPanelViewModes(true)
	defer resetPanelViewModes(true)

	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(t.TempDir()))
	waitForLoad(t, fp)
	fp.SetViewMode(ViewModeBrief) // "N,N,N": three stripes of one column each.

	if fp.layout.stripes < 2 {
		t.Fatalf("ViewModeBrief has %d stripe(s), want at least 2 for this test", fp.layout.stripes)
	}
	if _, ok := fp.headerColumnBorderAt(fp.Table.X1, fp.Table.Y1); ok {
		t.Error("headerColumnBorderAt found a resize border in a multi-stripe mode")
	}
}
