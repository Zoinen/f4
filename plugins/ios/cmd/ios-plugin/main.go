// Command ios-plugin is the standalone subprocess entry point for the iOS
// drive (f4#1178, part 1 of 4 of the plan posted at
// https://github.com/unxed/f4/issues/1178#issuecomment-5851326218, per the
// decision at
// https://github.com/unxed/f4/issues/1178#issuecomment-5851392645 that iOS
// gets the same downloadable-plugin treatment as cloud storage).
//
// It links the same Apple mobile-device transport that used to be
// registered in-process by internal/plughost's loadInternal in every build
// (plugins/ios), now built into its own binary via plugins/ios's own
// go.mod so go-ios and its fully separate dependency chain --
// gvisor.dev/gvisor (a full userspace TCP/IP stack), quic-go,
// vishvananda/netlink+netns, songgao/water, miekg/dns, grandcat/zeroconf,
// howett.net/plist, go.mozilla.org/pkcs7, software.sslmate.com/src/go-pkcs12,
// golang.zx2c4.com/wintun (~49 MB together) -- never touch f4's own module
// graph or binary, in either the full or the lite build. f4 launches this
// binary as a native OS subprocess and talks to it over the standard
// F4-RPC transport (docs/PLUGINS.md, sdk/f4plugin), exactly like any other
// subprocess plugin (see plugins/dummy_rpc/main.go for the minimal
// reference example this mirrors, and plugins/cloudfox/cmd/cloudfox-plugin
// for the same pattern applied first).
//
// See plugring-manifest.json in this directory for the declarative
// manifest PlugRing needs to offer this as an installable plugin. Building
// and publishing release binaries per platform, wiring a first-party
// PlugRing entry, and putting an entry point back in a lite build's menu
// are parts 2-4 of the plan above -- none of that is done here.
package main

import (
	iosfs "github.com/unxed/f4/plugins/ios"
	"github.com/unxed/f4/sdk/f4plugin"
)

func main() {
	f4plugin.Run(iosfs.NewRPCPlugin())
}
