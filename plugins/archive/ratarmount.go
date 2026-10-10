package archive

import (
	"os/exec"
	"sync"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

// ratarmountBinary is the external tool f4#251 asks to support as an
// optional, faster tar-index backend for users who already have it
// installed for other purposes (https://github.com/mxmlnkn/ratarmount).
const ratarmountBinary = "ratarmount"

var (
	ratarmountOnce      sync.Once
	ratarmountAvailable bool
)

// RatarmountAvailable reports whether the external ratarmount binary is on
// PATH. The owner's request (f4#251) is not "replace internal/tarindexcache
// with ratarmount": it is an opt-in, config.App.ArchiveUseRatarmountIfAvailable,
// for the people who already run ratarmount for other tools and would
// rather f4 reused it than kept its own separate index. Detection is cached
// for the process's lifetime -- PATH does not change while f4 runs, and
// tarIndexPath calls this on every tar archive open.
func RatarmountAvailable() bool {
	ratarmountOnce.Do(func() {
		_, err := exec.LookPath(ratarmountBinary)
		ratarmountAvailable = err == nil
	})
	return ratarmountAvailable
}

// maybeLogRatarmountPreference is this first part's only externally visible
// effect of config.App.ArchiveUseRatarmountIfAvailable: a debug breadcrumb
// confirming that the setting and the PATH detection both work. Actually
// using ratarmount as the index backend (spawning it, or reading the index
// format it produces, instead of internal/tarindexcache) is follow-up work
// -- every tar open still goes through the internal cache regardless of this
// setting until then. Splitting it this way lets the setting ship and be
// tested on its own before the larger, riskier integration piece.
func maybeLogRatarmountPreference() {
	if !config.App.ArchiveUseRatarmountIfAvailable {
		return
	}
	if RatarmountAvailable() {
		vtui.DebugLog("archive: ratarmount backend requested and found on PATH, but not implemented yet (f4#251); using the internal tar index cache")
		return
	}
	vtui.DebugLog("archive: ratarmount backend requested but ratarmount was not found on PATH (f4#251); using the internal tar index cache")
}
