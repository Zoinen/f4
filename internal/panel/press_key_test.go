package panel

import (
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// pressKey is testutil.PressKey with this package's key filter. A key that
// reaches a frame in production has already passed the application's filter —
// the key remap, macro recording, the configurable hotkeys — so a test that
// hands the event straight to the frame is testing a path the user never
// takes. KeyFilter is what the composition root installs; here it is whatever
// this package's own tests set, which is nothing by default.
func pressKey(f vtui.Frame, e *vtinput.InputEvent) bool {
	return testutil.PressKey(f, e, KeyFilter)
}
