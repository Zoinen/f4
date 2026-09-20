//go:build !vtuiframewatch

package app

// bindFrameWatch puts the stall watchdog on vtui's frame loop, so that a
// freeze anywhere in it — a render, an input event, a posted task — is caught,
// and not only the editor's own frames. It reports whether it could.
//
// The hook it needs is newer than the vtui release f4 builds against, so the
// ordinary build does without it and the editor's own marks carry the
// watchdog. A diagnostic binary gets the whole loop:
//
//	go build -tags vtuiframewatch ./cmd/f4
//
// with a replace directive pointing github.com/unxed/vtui at a checkout that
// has vtui.FrameWatch.
func bindFrameWatch() bool { return false }
