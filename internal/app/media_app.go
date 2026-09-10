package app

import (
	"github.com/unxed/f4/internal/media"
)

type mediaApplication struct{}

// HandleCommand: the only frame command the media views hand upward is the
// workspace fork, which every full-screen view forwards the same way.
func (mediaApplication) HandleCommand(cmd int, args any) bool {
	return HandleWorkspaceForkCommand(cmd, args)
}

var _ media.Application = mediaApplication{}

func init() { media.App = mediaApplication{} }
