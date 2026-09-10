package plughost_test

import (
	"github.com/unxed/f4/internal/nativeui"
	"github.com/unxed/f4/internal/plughost"
)

// Exercise the real projection adapter through the transport's dependency seam.
func init() {
	plughost.Presentation = nativeui.Adapter{}
	plughost.LegacySceneProjectionForTest = nativeui.BuildAppSceneFromLegacy
}
