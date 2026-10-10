// Command android-plugin is the standalone subprocess entry point for the
// Android drive (f4#1178: the owner's decision at
// https://github.com/unxed/f4/issues/1178#issuecomment-5851392645 to give
// Android the same "download this plugin on demand" treatment as cloud
// storage/iOS, part 1 of 4 of that same plan).
//
// It links the same ADB device-discovery and FISH+/ADB-Sync provider code
// that used to be registered in-process by internal/plughost's build
// (plugins/android), now built into its own binary via plugins/android's
// own go.mod. Unlike plugins/cloudfox's extraction, this one is not about
// shedding a large unique dependency -- plugins/android has none of its
// own, being a thin wrapper around the local `adb` server/executable, the
// same pattern as plugins/multiarc's CLI-archiver wrapper -- it exists so
// all three plugins the owner named (cloud storage, iOS, Android) install
// through one uniform mechanism. See plugins/android/rpc_plugin.go's
// package comment for the one dependency this extraction still had to
// account for (plugins/netfox's FISH+ reuse, and why this module builds
// with -tags lite). f4 launches this binary as a native OS subprocess and
// talks to it over the standard F4-RPC transport (docs/PLUGINS.md,
// sdk/f4plugin), exactly like any other subprocess plugin (see
// plugins/dummy_rpc/main.go for the minimal reference example this
// mirrors, and plugins/cloudfox/cmd/cloudfox-plugin/main.go for the sibling
// extraction this one repeats).
//
// See plugring-manifest.json in this directory for the declarative
// manifest PlugRing needs to offer this as an installable plugin. Building
// and publishing release binaries per platform is part 2 of the plan above
// (done: build-android-plugin in .github/workflows/build.yml archives and
// publishes them); internal/plughost/plugring_firstparty.go's
// FirstPartyPlugRingItems (part 3) mirrors this same manifest by hand into a
// first-party PlugRing entry, so the PlugRing UI's "download and install in
// one click" offers android without a community plugring/index.yaml entry,
// which PLUGRING.md's distribution policy would refuse for a native,
// per-platform binary like this one. Keep the two in sync when either
// changes; they cannot share code across the module boundary
// plugins/android's own go.mod draws (part 1). A menu entry that points a
// lite/full build's "Android" drive at this install path is part 4.
package main

import (
	androidfs "github.com/unxed/f4/plugins/android"
	"github.com/unxed/f4/sdk/f4plugin"
)

func main() {
	f4plugin.Run(androidfs.NewRPCPlugin())
}
