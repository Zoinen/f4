package app

import (
	"context"
	"testing"
	"time"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// registeredCloudStorageLiteDriveFactory returns the Factory a lite build
// registered under cloudStorageDriveName, or nil if none was.
func registeredCloudStorageLiteDriveFactory() func() vfs.VFS {
	for _, d := range sysinfo.DriveRegistrySnapshot() {
		if d.Name == cloudStorageDriveName {
			return d.Factory
		}
	}
	return nil
}

// TestCloudStorageLiteDriveOpensPlugRingFocusedOnCloudfox is f4#1178, part 4
// of 4: it pins down what a lite build now shows where "CloudFox" used to be
// a real cloud VFS (see cloud_storage_lite.go's own comments for the
// history). liteBuild is always false under `go test` -- see
// cloud_storage_lite.go and cmd/f4/lite_deps_test.go for why the lite tag is
// checked mechanically instead -- so this calls registerCloudStorageLiteDrive
// directly, the same way TestCommandPaletteDriveEntriesExposeRegistryNamesForBothPanels
// (command_palette_drives_test.go) drives sysinfo's drive registry straight
// from a test rather than through a real plugin load.
func TestCloudStorageLiteDriveOpensPlugRingFocusedOnCloudfox(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	// Stand in for the network catalog fetch with a fixed list that puts
	// "cloudfox" between two other entries, the same way
	// TestPlugRingDialog_Layout (plugring_ui_test.go) replaces plugRingCatalog
	// rather than touching the real community catalog or plughost's
	// first-party list.
	previousCatalog := plugRingCatalog
	plugRingCatalog = func(context.Context) ([]plughost.PlugRingItem, error) {
		return []plughost.PlugRingItem{
			{ID: "aaa-before", Name: "Alphabetically first", Entrypoint: "aaa.lua", Category: "tools"},
			{ID: "cloudfox", Name: "Cloud storage (CloudFox)", Entrypoint: "cloudfox-plugin", Category: "filesystem", FirstParty: true},
			{ID: "zzz-after", Name: "Alphabetically last", Entrypoint: "zzz.lua", Category: "tools"},
		}, nil
	}
	t.Cleanup(func() { plugRingCatalog = previousCatalog })

	pf := panel.NewPanelsFrame()
	defer pf.Close()
	vtui.FrameManager.Push(pf)

	restoreDrives := sysinfo.SnapshotDrives()
	defer restoreDrives()
	sysinfo.SetDrives(nil)
	registerCloudStorageLiteDrive()

	factory := registeredCloudStorageLiteDriveFactory()
	if factory == nil {
		t.Fatal("a lite build did not register a CloudFox entry in the drive registry (drive menu / command palette)")
	}

	// Selecting the entry must not switch into a real VFS: both the drive
	// menu (PanelsFrame.SwitchToVFS) and the command palette
	// (switchCommandPaletteDriveVFS) already treat a nil VFS as "do
	// nothing", so this is what keeps the panel exactly where it was instead
	// of leading nowhere or panicking.
	if got := factory(); got != nil {
		t.Fatalf("the lite CloudFox drive factory returned a real VFS %#v, want nil", got)
	}

	dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("selecting CloudFox in a lite build did not open a dialog, top frame = %T", vtui.FrameManager.GetTopFrame())
	}
	if got, want := dlg.GetTitle(), i18n.Msg("PlugRing.Title"); got != want {
		t.Fatalf("dialog title = %q, want the PlugRing dialog %q", got, want)
	}

	// The dialog fetches its catalog on the background task pump
	// (vtui.RunAsync); drain it the same way TestPlugRing_InstallAndRemove_
	// EndToEnd and TestCheckForPluginUpdates (plugring_test.go) do.
	select {
	case task := <-vtui.FrameManager.TaskChan:
		task()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the PlugRing catalog fetch")
	}

	var table *vtui.Table
	for _, child := range dlg.GetChildren() {
		if candidate, ok := child.(*vtui.Table); ok {
			table = candidate
			break
		}
	}
	if table == nil {
		t.Fatal("PlugRing dialog has no table")
	}
	if table.SelectPos < 0 || table.SelectPos >= len(table.Rows) {
		t.Fatalf("SelectPos = %d out of range for %d rows", table.SelectPos, len(table.Rows))
	}
	row, ok := table.Rows[table.SelectPos].(plugRingRow)
	if !ok || row.item.ID != "cloudfox" {
		t.Fatalf("opening CloudFox from a lite build did not land on the cloudfox row, selected row = %#v", table.Rows[table.SelectPos])
	}
}

// TestCloudStorageLiteDriveAbsentOutsideLiteBuild pins the other half of the
// pair: a normal build (which is every build `go test` itself ever compiles,
// liteBuild included) must not pick up the lite-only fallback drive from
// package init.
func TestCloudStorageLiteDriveAbsentOutsideLiteBuild(t *testing.T) {
	if liteBuild {
		t.Skip("this build tag is exercised mechanically (go build/go list -tags lite), not by go test -- see cmd/f4/lite_deps_test.go")
	}
	if registeredCloudStorageLiteDriveFactory() != nil {
		t.Fatal("a non-lite build registered the lite-only CloudFox fallback drive from init")
	}
}
