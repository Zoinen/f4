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

// registeredIOSPluginMenuDriveFactory returns the Factory registered under
// iosDriveName, or nil if none was.
func registeredIOSPluginMenuDriveFactory() func() vfs.VFS {
	for _, d := range sysinfo.DriveRegistrySnapshot() {
		if d.Name == iosDriveName {
			return d.Factory
		}
	}
	return nil
}

// TestIOSPluginMenuDriveRegisteredUnconditionally pins the property that
// makes this file different from internal/app/cloud_storage_lite.go:
// ios_plugin_menu.go's init has no liteBuild guard, so the "iOS" entry must
// already be in the drive registry from this package's ordinary package
// init -- in every build go test itself ever compiles, liteBuild included --
// without any test having to call registerIOSPluginMenuDrive itself first.
// TestCloudStorageLiteDriveAbsentOutsideLiteBuild pins the opposite property
// for cloudfox, which is deliberately lite-only.
func TestIOSPluginMenuDriveRegisteredUnconditionally(t *testing.T) {
	if registeredIOSPluginMenuDriveFactory() == nil {
		t.Fatal("the \"iOS\" drive is not registered; ios_plugin_menu.go's init must run unconditionally in every build, not just a lite one")
	}
}

// TestIOSPluginMenuDriveOpensPlugRingFocusedOnIOS is f4#1178, part 4 of 4: it
// pins down what every build now shows where "iOS" used to be a real
// device-browsing VFS (see ios_plugin_menu.go's own comments for the
// history). It mirrors
// TestCloudStorageLiteDriveOpensPlugRingFocusedOnCloudfox
// (cloud_storage_lite_test.go) closely, calling registerIOSPluginMenuDrive
// directly after clearing the registry so the test does not depend on
// package init ordering relative to sysinfo.SetDrives(nil) below.
func TestIOSPluginMenuDriveOpensPlugRingFocusedOnIOS(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	// Stand in for the network catalog fetch with a fixed list that puts
	// "ios" between two other entries, the same way
	// TestCloudStorageLiteDriveOpensPlugRingFocusedOnCloudfox replaces
	// plugRingCatalog rather than touching the real community catalog or
	// plughost's first-party list.
	previousCatalog := plugRingCatalog
	plugRingCatalog = func(context.Context) ([]plughost.PlugRingItem, error) {
		return []plughost.PlugRingItem{
			{ID: "aaa-before", Name: "Alphabetically first", Entrypoint: "aaa.lua", Category: "tools"},
			{ID: "ios", Name: "Apple mobile devices (iOS)", Entrypoint: "ios-plugin", Category: "filesystem", FirstParty: true},
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
	registerIOSPluginMenuDrive()

	factory := registeredIOSPluginMenuDriveFactory()
	if factory == nil {
		t.Fatal("did not register an iOS entry in the drive registry (drive menu / command palette)")
	}

	// Selecting the entry must not switch into a real VFS: both the drive
	// menu (PanelsFrame.SwitchToVFS) and the command palette
	// (switchCommandPaletteDriveVFS) already treat a nil VFS as "do
	// nothing", so this is what keeps the panel exactly where it was instead
	// of leading nowhere or panicking.
	if got := factory(); got != nil {
		t.Fatalf("the iOS drive factory returned a real VFS %#v, want nil", got)
	}

	dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("selecting iOS did not open a dialog, top frame = %T", vtui.FrameManager.GetTopFrame())
	}
	if got, want := dlg.GetTitle(), i18n.Msg("PlugRing.Title"); got != want {
		t.Fatalf("dialog title = %q, want the PlugRing dialog %q", got, want)
	}

	// The dialog fetches its catalog on the background task pump
	// (vtui.RunAsync); drain it the same way
	// TestCloudStorageLiteDriveOpensPlugRingFocusedOnCloudfox does.
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
	if !ok || row.item.ID != "ios" {
		t.Fatalf("opening iOS did not land on the ios row, selected row = %#v", table.Rows[table.SelectPos])
	}
}
