package app

import (
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/vfs"
)

// The environment manager itself is in internal/terminal, next to the pty backends
// that hand it to a child. What stays here is coreAPI's implementation of the
// plugin-facing contract, and the runtime and broadcast it needs from the
// panel side.

var _ vfs.ProcessEnvironmentHost = (*CoreAPI)(nil)

func (c *CoreAPI) SnapshotProcessEnvironment() vfs.ProcessEnvironmentSnapshot {
	// EnvMan calls Snapshot during plugin initialization, making this the
	// earliest reliable point to establish this process's isolated runtime.
	_ = panel.InitializeProcessEnvironmentRuntime()
	snapshot, _ := terminal.GlobalProcessEnvironment.Snapshot()
	return snapshot
}

func (c *CoreAPI) ApplyProcessEnvironment(changes []vfs.ProcessEnvironmentChange) (vfs.ProcessEnvironmentSnapshot, error) {
	snapshot, generations, err := terminal.ApplyProcessEnvironmentWithRuntime(terminal.GlobalProcessEnvironment, panel.InitializeProcessEnvironmentRuntime, changes)
	panel.BroadcastProcessEnvironmentGenerations(generations)
	return snapshot, err
}
