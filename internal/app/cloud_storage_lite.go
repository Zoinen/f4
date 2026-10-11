package app

import (
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
)

// cloudStorageDriveName mirrors plugins/cloudfox.DriveName, which registered
// "CloudFox" as a drive (api.RegisterDrive) before f4#1178 part 1 pulled
// cloudfox out of the build entirely -- in both the full and the lite build,
// see internal/plughost/plugins_full.go's own comment on that. The name is
// duplicated here by hand rather than imported: importing plugins/cloudfox
// from this module would drag its ~30 MB of cloud SDKs straight back into
// f4's own build graph, exactly what cmd/f4/cloudfox_deps_test.go polices
// (it forbids importing github.com/unxed/f4/plugins/cloudfox at all).
const cloudStorageDriveName = "CloudFox"

// cloudStoragePlugRingID is plugins/cloudfox/cmd/cloudfox-plugin's PlugRing
// catalog id, duplicated from internal/plughost/plugring_firstparty.go's
// FirstPartyPlugRingItems by hand for the same reason: there is no shared
// constant for it, only the literal in that one first-party list entry.
const cloudStoragePlugRingID = "cloudfox"

// This is f4#1178, part 4 of 4 of the plan at
// https://github.com/unxed/f4/issues/1178#issuecomment-5851218447: a lite
// build's user used to see "CloudFox" right here -- the drive menu's
// "Plugins & custom drives" section (internal/panel/frame.go) and the
// command palette's drive list (command_palette_drives.go), both of which
// read sysinfo.DriveRegistry -- before part 1 removed the in-process
// registration from every build. A full build lost the exact same
// registration in the exact same commit, so there is nothing to restore a
// fallback for there; this stays lite-only on purpose.
func init() {
	if liteBuild {
		registerCloudStorageLiteDrive()
	}
}

// registerCloudStorageLiteDrive is split out of init so a test can call it
// directly. liteBuild is false in every build this repository's own test
// suite ever compiles under -- go test never runs with -tags lite; see
// cmd/f4/lite_deps_test.go's comment on why the lite build is instead
// checked mechanically, by shelling out to `go list`/`go build` with the tag
// -- so gating this call on the constant would leave the registration
// itself untested.
func registerCloudStorageLiteDrive() {
	sysinfo.RegisterDrive(cloudStorageDriveName, cloudStorageLiteDriveFactory)
}

// cloudStorageLiteDriveFactory is what selecting "CloudFox" runs in a lite
// build instead of opening a real cloud VFS: f4#1178 part 3 landed a
// first-party PlugRing catalog entry for cloudfox-plugin
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
func cloudStorageLiteDriveFactory() vfs.VFS {
	if pf := panel.FindPanelsFrameAnyScreen(); pf != nil {
		actionPlugRingFocused(pf, cloudStoragePlugRingID)
	}
	return nil
}
