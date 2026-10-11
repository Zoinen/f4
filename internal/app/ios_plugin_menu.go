package app

import (
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
)

// iosDriveName mirrors plugins/ios.Plugin.Init's own api.RegisterDrive("iOS",
// ...) call, which registered "iOS" as a drive before f4#1178 part 1 pulled
// the whole plugin out of the build entirely. The name is duplicated here by
// hand rather than imported: importing plugins/ios from this module would
// drag its ~49 MB of go-ios/gvisor/quic-go userspace networking dependencies
// straight back into f4's own build graph, exactly what
// cmd/f4/ios_deps_test.go polices (it forbids importing
// github.com/unxed/f4/plugins/ios at all, in either build).
const iosDriveName = "iOS"

// iosPlugRingID is plugins/ios/cmd/ios-plugin's PlugRing catalog id,
// duplicated from internal/plughost/plugring_firstparty.go's
// FirstPartyPlugRingItems by hand for the same reason: there is no shared
// constant for it, only the literal in that one first-party list entry.
const iosPlugRingID = "ios"

// This is f4#1178, part 4 of 4 of the plan at
// https://github.com/unxed/f4/issues/1178#issuecomment-5851218447 --
// applied to iOS instead of cloud storage.
//
// The important difference from internal/app/cloud_storage_lite.go, which
// this file otherwise mirrors closely: cloudfox was already excluded from
// the full build's drive list before that point (it was only ever a lite
// build's fallback there was something to replace), so
// registerCloudStorageLiteDrive only runs when liteBuild is true. iOS is not
// like that: internal/plughost/manager.go's loadInternal registered the
// real "iOS" drive unconditionally, in both the full and the lite build,
// before part 1 removed it. Real functionality disappeared from both
// builds, so the replacement entry point below has to appear in both builds
// too -- hence this file's init has no liteBuild guard, and it is not named
// "..._lite.go" the way cloud_storage_lite.go is.
func init() {
	registerIOSPluginMenuDrive()
}

// registerIOSPluginMenuDrive is split out of init so a test can call it
// directly, and so a re-registration attempt (e.g. from a test that also
// exercises a real plugin load) is idempotent the same way sysinfo's own
// registry is elsewhere.
func registerIOSPluginMenuDrive() {
	sysinfo.RegisterDrive(iosDriveName, iosPluginMenuDriveFactory)
}

// iosPluginMenuDriveFactory is what selecting "iOS" now runs instead of
// opening a real device-browsing VFS: f4#1178 part 3 landed a first-party
// PlugRing catalog entry for ios-plugin
// (internal/plughost/plugring_firstparty.go), so the honest, working thing
// to do here is send the user straight to it, via actionPlugRingFocused
// (plugring_ui.go) rather than a hand-written explanation of where to
// click.
//
// Returning nil is deliberate and safe, not a placeholder for a bug: both
// callers of a drive's Factory -- PanelsFrame.SwitchToVFS via the drive menu
// (internal/panel/frame.go) and switchCommandPaletteDriveVFS via the command
// palette (command_palette_drives.go) -- already treat a nil VFS as "do
// nothing" rather than a crash, so selecting this entry opens PlugRing and
// leaves the panel exactly where it was, instead of leading nowhere or
// panicking.
func iosPluginMenuDriveFactory() vfs.VFS {
	if pf := panel.FindPanelsFrameAnyScreen(); pf != nil {
		actionPlugRingFocused(pf, iosPlugRingID)
	}
	return nil
}
