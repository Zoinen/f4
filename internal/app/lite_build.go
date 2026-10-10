//go:build !lite

package app

// liteBuild is false here and true in lite_build_lite.go (built only with
// -tags lite). It gates the few things a lite build offers differently,
// such as the cloud storage drive entry that points to PlugRing.
const liteBuild = false
