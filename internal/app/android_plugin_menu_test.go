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

// registeredAndroidDriveFactory returns the Factory registered under
// androidDriveName, or nil if none was.
func registeredAndroidDriveFactory() func() vfs.VFS {
	for _, d := range sysinfo.DriveRegistrySnapshot() {
		if d.Name == androidDriveName {
			return d.Factory
		}
	}
	return nil
}

// TestAndroidDriveRegisteredInEveryBuild is the one place this file's test
// suite differs in shape from cloud_storage_lite_test.go's own
// TestCloudStorageLiteDriveAbsentOutsideLiteBuild: cloud_storage_lite.go
// only registers its fallback drive when liteBuild is true, because CloudFox
// was already excluded from the full build before f4#1178. plugins/android
// had no such asymmetry -- internal/plughost/manager.go registered it
// unconditionally in loadInternal, in both builds -- so
// android_plugin_menu.go's own init() calls registerAndroidDrive()
// unconditionally too, with no liteBuild guard at all.
//
// This checks that registration directly from package init, without calling
// registerAndroidDrive itself first: go test always compiles this package
// without -tags lite (see cmd/f4/lite_deps_test.go's comment on why the lite
// tag is instead checked mechanically), so if the "Android" entry is present
// here, init ran registerAndroidDrive unconditionally rather than behind a
// liteBuild check that would leave it absent in exactly this build.
func TestAndroidDriveRegisteredInEveryBuild(t *testing.T) {
	if registeredAndroidDriveFactory() == nil {
		t.Fatal("android_plugin_menu.go's init did not register the Android entry in the drive registry (drive menu / command palette); it must run unconditionally, not behind a liteBuild guard")
	}
}

// TestAndroidDriveOpensPlugRingFocusedOnAndroid is f4#1178, part 4 of 4: it
// pins down what selecting "Android" now shows where a real ADB device-list
// VFS used to open (see android_plugin_menu.go's own comments for the
// history). It calls registerAndroidDrive directly, the same way
// cloud_storage_lite_test.go's
// TestCloudStorageLiteDriveOpensPlugRingFocusedOnCloudfox does for cloudfox,
// so the test controls the registry directly rather than depending on
// whatever order package init functions across this package happened to run
// in.
func TestAndroidDriveOpensPlugRingFocusedOnAndroid(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	// Stand in for the network catalog fetch with a fixed list that puts
	// "android" between two other entries, the same way
	// cloud_storage_lite_test.go replaces plugRingCatalog rather than
	// touching the real community catalog or plughost's first-party list.
	previousCatalog := plugRingCatalog
	plugRingCatalog = func(context.Context) ([]plughost.PlugRingItem, error) {
		return []plughost.PlugRingItem{
			{ID: "aaa-before", Name: "Alphabetically first", Entrypoint: "aaa.lua", Category: "tools"},
			{ID: "android", Name: "Android devices (ADB)", Entrypoint: "android-plugin", Category: "filesystem", FirstParty: true},
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
	registerAndroidDrive()

	factory := registeredAndroidDriveFactory()
	if factory == nil {
		t.Fatal("registerAndroidDrive did not register an Android entry in the drive registry (drive menu / command palette)")
	}

	// Selecting the entry must not switch into a real VFS: both the drive
	// menu (PanelsFrame.SwitchToVFS) and the command palette
	// (switchCommandPaletteDriveVFS) already treat a nil VFS as "do
	// nothing", so this is what keeps the panel exactly where it was instead
	// of leading nowhere or panicking.
	if got := factory(); got != nil {
		t.Fatalf("the Android drive factory returned a real VFS %#v, want nil", got)
	}

	dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("selecting Android did not open a dialog, top frame = %T", vtui.FrameManager.GetTopFrame())
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
	if !ok || row.item.ID != "android" {
		t.Fatalf("opening Android did not land on the android row, selected row = %#v", table.Rows[table.SelectPos])
	}
}
