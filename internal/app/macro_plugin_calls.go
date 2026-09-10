package app

import (
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/vfs"
)

// The registry this writes to lives in internal/macro, next to the engine that
// dispatches the calls.
func (c *CoreAPI) RegisterMacroCallProvider(provider vfs.MacroCallProvider) (vfs.Registration, error) {
	return macro.RegisterMacroCallProvider(provider)
}
