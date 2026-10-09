//go:build vtuiframewatch

package app

import (
	"github.com/unxed/f4/internal/stallwatch"
	"github.com/unxed/vtui"
)

// bindFrameWatch is the build-tagged half described in framewatch.go: with a
// vtui that has the hook, every piece of work the UI goroutine does is timed,
// so a stall is caught wherever in the loop it happens.
func bindFrameWatch() bool {
	vtui.FrameWatch = stallwatch.Frame
	return true
}
