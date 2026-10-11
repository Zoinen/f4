package netbrowse

import (
	"errors"
	"runtime"
)

// resource is one entry of the network as the system enumerates it.
type resource struct {
	Remote    string // the entry's name: a provider, a domain, "\\server" or "\\server\share"
	Comment   string
	Provider  string
	Display   uint32 // RESOURCEDISPLAYTYPE_*, see kindName
	Container bool   // it can be enumerated further (a domain, a server, a provider)
}

// The RESOURCEDISPLAYTYPE_* values of winnetwk.h.
const (
	displayGeneric   = 0
	displayDomain    = 1
	displayServer    = 2
	displayShare     = 3
	displayNetwork   = 6
	displayRoot      = 7
	displayShareAdmn = 8
	displayDirectory = 9
	displayTree      = 10
	displayNDSContnr = 11
)

// kindName is the entry's kind as text, for the type column.
func kindName(display uint32) string {
	switch display {
	case displayDomain:
		return "Domain"
	case displayServer:
		return "Server"
	case displayShare, displayShareAdmn:
		return "Share"
	case displayNetwork:
		return "Network"
	case displayRoot:
		return "Root"
	case displayDirectory:
		return "Directory"
	case displayTree:
		return "Tree"
	case displayNDSContnr:
		return "Container"
	}
	return ""
}

// errUnsupported is what the enumerator reports off Windows.
var errUnsupported = errors.New("the network browser needs Windows or a build with SMB support")

// supported reports whether this OS has the WNet API; a variable so tests can
// pretend either way.
var supported = runtime.GOOS == "windows" || smbBuilt

// Supported reports whether the plugin can browse the network here.
func Supported() bool { return supported }

// enumerator lists the entries under a container of the network; parent nil
// is the top of the network (its providers).
type enumerator func(parent *resource) ([]resource, error)
