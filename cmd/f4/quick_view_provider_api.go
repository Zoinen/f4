package main

import "github.com/unxed/f4/vfs"

// RegisterQuickViewProvider implements the optional vfs.ContributionHost
// surface without widening the legacy HostAPI used by RPC/Lua/Wasm plugins.
func (c *coreAPI) RegisterQuickViewProvider(provider vfs.QuickViewProvider) (vfs.Registration, error) {
	return vfs.RegisterQuickViewProvider(provider)
}
