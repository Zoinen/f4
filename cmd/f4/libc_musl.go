//go:build linux && goffi_musl

package main

// buildLibc names the C library this binary was linked against.
//
// The goffi_musl tag points goffi's dynamic imports at musl's libc and bakes
// the musl loader path into PT_INTERP, so a binary built with it runs on
// Alpine and not on a glibc system. The release publishes those artifacts
// under their own names (f4-linux-musl-<arch>.tar.gz), and the updater has to
// know which flavor is running to pick the right one -- matching on GOOS and
// GOARCH alone cannot tell them apart. This constant is that knowledge.
const buildLibc = "musl"
