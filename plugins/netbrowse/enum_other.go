//go:build !windows

package netbrowse

import (
	"strings"

	"github.com/unxed/f4/vfs"
)

// Off Windows the network is what SMB shows: the servers known to this session
// and their shares (smbnet.go). Without an SMB client (the lite build) there is
// nothing to ask.
func enumerateNetwork(parent *resource) ([]resource, error) {
	if !smbBuilt {
		return nil, errUnsupported
	}
	return enumerateSMB(parent)
}

func openShare(unc string) vfs.VFS { return openSMBShare(unc) }

// guessResource is a server named by hand: a name typed in the Network drive
// at its top that the session does not know yet is taken as a host to try.
func guessResource(parent *resource, name string) *resource {
	if !smbBuilt || parent != nil || name == "" || strings.ContainsAny(name, `\/ `) {
		return nil
	}
	return &resource{Remote: `\\` + name, Provider: "SMB", Display: displayServer, Container: true}
}
