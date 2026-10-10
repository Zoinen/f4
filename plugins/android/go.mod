module github.com/unxed/f4/plugins/android

go 1.26.6

// f4#1178 Android plugin, part 1 of 4 (mirrors plugins/cloudfox's own part 1
// of 4; plan and this ticket's Android-specific decision:
// https://github.com/unxed/f4/issues/1178#issuecomment-5851392645).
//
// plugins/android (ADB device browsing over shell-v2/FISH+ or ADB Sync) is
// its own module for the same reason cloudfox and (eventually) iOS are: a
// single, uniform "download this plugin on demand" mechanism across all
// three, per the owner's explicit request in the comment above -- NOT
// because this package carries unique heavy dependencies of its own. It
// does not: plugins/android's own files import nothing beyond the standard
// library (net, os/exec, encoding/*, ...) and this repo's own vfs,
// sdk/f4plugin and netfox packages -- see rpc_plugin.go's package comment
// for the full accounting, including plugins/netfox, which android/device.go
// and fish_pool.go reuse for the FISH+ session/pool machinery (the same
// pattern plugins/multiarc uses for archivers: wrapping an existing in-repo
// package rather than vendoring its own copy).
//
// That reuse is also why -tags lite matters for this module specifically,
// unlike plugins/cloudfox (which does not depend on netfox at all):
// plugins/netfox itself is a shared package, not its own module, and its
// non-FISH+ files (FTP, SFTP, the SSH dialer, Pageant) are gated
// //go:build lite/!lite rather than split out. Building this module's own
// binary WITHOUT -tags lite would silently link golang.org/x/crypto/ssh,
// github.com/pkg/sftp, github.com/jlaffaye/ftp and github.com/kbolino/pageant
// into android-plugin even though plugins/android's own code never
// references any of them (see device.go/fish_pool.go: only
// netfox.FishVFS/fishplus, never netfox's FTP/SFTP/SSH-dialer types). The
// build-android-plugin CI job in .github/workflows/build.yml therefore
// passes -tags lite, which keeps this module's actual external dependency
// surface down to go-runewidth, vtinput and vtui (plus vtui's own ffi/goffi
// forks below) -- see cmd/f4/lite_deps_test.go's
// TestLiteBuildExcludesHeavyNetworkDependencies for the same mechanical
// check applied to the main lite build.
//
// This go.mod is intentionally not hand-tidied: per this repo's own policy
// (no local `go build`/`go mod tidy` -- see the build-android-plugin CI job
// in .github/workflows/build.yml, mirroring build-cloudfox-plugin), the
// require block below was written by reading plugins/android's (and, via
// plugins/netfox, its transitive) imports rather than by running the Go
// toolchain. That CI job runs `go mod tidy` before building, which is the
// actual source of truth for the resulting go.mod/go.sum; committing its
// output back (or correcting anything it changes here) is expected
// follow-up, not a sign this file is wrong on arrival.
require (
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/unxed/f4 v0.0.0
	github.com/unxed/vtinput v0.1.11
	github.com/unxed/vtui v0.1.399-0.20261009152503-8b34a0d98bcb
)

require (
	github.com/abadojack/whatlanggo v1.0.1 // indirect
	github.com/charlievieth/strcase v0.0.6 // indirect
	github.com/cloudsoda/go-smb2 v0.0.0-20260918041005-0c5d69b69701 // indirect
	github.com/cloudsoda/sddl v0.0.0-20250224235906-926454e91efc // indirect
	github.com/coregx/ahocorasick v0.2.1 // indirect
	github.com/coregx/coregex v0.12.19 // indirect
	github.com/ebitengine/gomobile v0.0.0-20260211053922-3d992dae95d1 // indirect
	github.com/ebitengine/hideconsole v1.0.0 // indirect
	github.com/ebitengine/purego v0.11.0-alpha.8 // indirect
	github.com/emmansun/base64 v0.9.0 // indirect
	github.com/fogleman/gg v1.3.0 // indirect
	github.com/geoffgarside/ber v1.1.0 // indirect
	github.com/go-webgpu/goffi v0.6.3 // indirect
	github.com/go-webgpu/webgpu v0.5.5 // indirect
	github.com/gogpu/gg v0.52.5 // indirect
	github.com/gogpu/gogpu v0.53.0 // indirect
	github.com/gogpu/gpucontext v0.28.0 // indirect
	github.com/gogpu/gputypes v0.5.2 // indirect
	github.com/gogpu/naga v0.18.0 // indirect
	github.com/gogpu/wgpu v0.31.6 // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/hajimehoshi/ebiten/v2 v2.10.0-alpha.13.0.20260811162617-464c2ddfc34c // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.7.6 // indirect
	github.com/jcmturner/goidentity/v6 v6.0.1 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.4 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/jezek/xgb v1.3.1 // indirect
	github.com/jlaffaye/ftp v0.2.0 // indirect
	github.com/kbolino/pageant v0.0.0-20180919004629-179b60797d9f // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/neurlang/wayland v0.4.5-0.20261007184820-37f4fac9bad0 // indirect
	github.com/neurlang/winc v0.1.2 // indirect
	github.com/pkg/sftp v1.13.6 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/soniakeys/quant v1.0.0 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/unxed/goclip v0.1.2 // indirect
	github.com/unxed/keytrans v0.1.35 // indirect
	github.com/unxed/kiwi-go v0.1.0 // indirect
	github.com/unxed/libwinescape v0.2.1 // indirect
	github.com/unxed/localecp v0.1.7 // indirect
	github.com/unxed/winkeys v0.1.1 // indirect
	github.com/unxed/xkb-go v0.1.9 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/yalue/native_endian v1.0.2 // indirect
	github.com/yuin/goldmark v1.8.6 // indirect
	github.com/zzl/go-win32api/v2 v2.1.0 // indirect
	golang.design/x/clipboard v0.7.0 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/exp/shiny v0.0.0-20260727155853-b88d891fe743 // indirect
	golang.org/x/image v0.45.0 // indirect
	golang.org/x/mobile v0.0.0-20260611195102-4dd8f1dbf5d2 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/term v0.46.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

replace github.com/unxed/f4 => ../..

// Use the same Qt-aware vtui fork as the host. Dependency replaces from the
// root module are not inherited when building this standalone module.
replace github.com/unxed/vtui => ../../third_party/vtui

replace github.com/unxed/goclip => github.com/Zoinen/goclip v0.1.3-clipboard.1

// Same forks the root module uses (see ../../go.mod) -- vtui transitively
// needs ffi.Available, which only exists in unxed/pureffi, not upstream
// ebitengine/purego. Without these, `go mod tidy` here resolves the
// vanilla upstream modules instead and the build fails with
// "undefined: ffi.Available".
replace github.com/ebitengine/purego => github.com/unxed/pureffi v0.1.21

replace github.com/ebitengine/hideconsole => ../../internal/hideconsole

replace github.com/go-webgpu/goffi => github.com/unxed/goffi v0.1.11

replace github.com/neurlang/wayland => github.com/unxed/wayland v0.4.5-0.20260924170549-04f3e691fadc
