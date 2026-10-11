package app

import (
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
)

// androidDriveName mirrors the literal plugins/android/manager.go used to
// register with api.RegisterDrive("Android", ...) before f4#1178 part 1
// pulled plugins/android out of the build entirely -- in both the full and
// the lite build (unlike CloudFox, plugins/android was never lite-only to
// begin with: internal/plughost/manager.go registered it unconditionally in
// loadInternal, so there is no "full build already had none of this"
// asymmetry to preserve). The name is duplicated here by hand rather than
// imported: importing plugins/android from this module would drag its own
// go.mod (and, transitively, plugins/netfox's FISH+ machinery) straight back
// into f4's own build graph, exactly what cmd/f4/android_deps_test.go
// polices (it forbids importing github.com/unxed/f4/plugins/android at all).
const androidDriveName = "Android"

// androidPlugRingID is plugins/android/cmd/android-plugin's PlugRing catalog
// id, duplicated from internal/plughost/plugring_firstparty.go's
// FirstPartyPlugRingItems by hand for the same reason: there is no shared
// constant for it, only the literal in that one first-party list entry.
const androidPlugRingID = "android"

// This is f4#1178, part 4 of 4: a build's user used to see "Android" right
// here -- the drive menu's "Plugins & custom drives" section
// (internal/panel/frame.go) and the command palette's drive list
// (command_palette_drives.go), both of which read sysinfo.DriveRegistry --
// before part 1 removed the in-process registration from every build.
//
// Unlike cloud_storage_lite.go's registerCloudStorageLiteDrive, this
// registration is NOT gated on liteBuild: plugins/android was never
// excluded from the full build the way plugins/cloudfox already was before
// this ticket (see internal/plughost/manager.go's own comment on
// loadInternal), so real functionality is being removed from both the lite
// and the full build here, and the replacement entry point has to appear in
// both too.
func init() {
	registerAndroidDrive()
}

// registerAndroidDrive is split out of init so a test can call it directly
// without depending on package init ordering.
func registerAndroidDrive() {
	sysinfo.RegisterDrive(androidDriveName, androidDriveFactory)
}

// androidDriveFactory is what selecting "Android" runs now instead of
// opening a real ADB device-list VFS: f4#1178 part 3 landed a first-party
// PlugRing catalog entry for android-plugin
// (internal/plughost/plugring_firstparty.go), so the honest, working thing
// to do here is send the user straight to it, via actionPlugRingFocused
// (plugring_ui.go) rather than a hand-written explanation of where to click.
//
// Returning nil is deliberate and safe, not a placeholder for a bug: both
// callers of a drive's Factory -- PanelsFrame.SwitchToVFS via the drive menu
// (internal/panel/frame.go) and switchCommandPaletteDriveVFS via the command
// palette (command_palette_drives.go) -- already treat a nil VFS as "do
// nothing" rather than a crash, so selecting this entry opens PlugRing and
// leaves the panel exactly where it was, instead of leading nowhere or
// panicking.
func androidDriveFactory() vfs.VFS {
	if pf := panel.FindPanelsFrameAnyScreen(); pf != nil {
		actionPlugRingFocused(pf, androidPlugRingID)
	}
	return nil
}
