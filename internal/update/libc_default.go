//go:build !linux || !goffi_musl

package update

// buildLibc names the C library this binary was linked against, and is empty
// for every build that does not target a specific one. See libc_musl.go.
const buildLibc = ""
