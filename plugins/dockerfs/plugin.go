// Package dockerfs shows Docker containers in a file panel: the containers are
// the folders at the top, and inside one is its own file system, read through
// the Engine API's archive endpoint (the one `docker cp` uses).
//
// It needs a reachable daemon (unix socket or
// tcp:// DOCKER_HOST) and nothing else: no Docker CLI, no SDK, no CGO.
package dockerfs

import (
	"errors"
	"os"

	"github.com/unxed/f4/vfs"
)

// driveName is what the drive menu (Alt+F1) calls the panel.
const driveName = "Docker"

// Plugin registers the Docker drive.
type Plugin struct{}

// NewPlugin constructs the built-in Docker plugin.
func NewPlugin() *Plugin { return &Plugin{} }

func (*Plugin) GetName() string { return "Docker" }

// Init only registers the drive: no connection is made until the panel is
// opened, so a machine without Docker pays nothing.
func (*Plugin) Init(api vfs.HostAPI) error {
	if api == nil {
		return errors.New("Docker: nil host API")
	}
	api.RegisterDrive(driveName, func() vfs.VFS { return newDockerVFS(clientFromEnv) })
	// docker:///<path> reopens the panel from a bookmark, history or a saved session.
	// One more drive per docker context of the CLI's store (docker context create).
	for _, name := range listContexts(dockerConfigDir(os.Getenv)) {
		api.RegisterDrive(contextDriveName(name), func() vfs.VFS { return newContextVFS(name) })
	}
	return api.RegisterURIProvider(uriProvider{open: clientFromEnv, openContext: contextOpener})
}

func (*Plugin) Close() error {
	vfs.UnregisterURIProvider("docker")
	return nil
}
