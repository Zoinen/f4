package vfs

type SudoCommand byte

const (
	CmdPing SudoCommand = iota
	CmdOpen
	CmdCreate
	CmdStat
	CmdReadDir
	CmdMkDir
	CmdRemove
	CmdRename
	CmdSetAttributes
	// CmdSymlink makes Path a symbolic link that points at Path2.
	CmdSymlink
	// CmdHardlink makes Path a hard link to the existing Path2.
	CmdHardlink
	// CmdUnmount unmounts the filesystem mounted at Path (f4#415). It is
	// the fallback UnmountDevice (mount_linux.go) reaches for only when
	// udisksctl is unavailable or refuses: unmounting something the current
	// user did not mount typically needs root, unlike the rest of this
	// dispatcher's calls, which exist for permission-denied filesystem
	// operations in general.
	CmdUnmount
	// CmdHello opens a session with an elevated dispatcher that cannot lean on
	// the file system to say who is calling (Windows, f4#1768): Path carries the
	// one-time token the dispatcher was started with. Nothing else is answered
	// before it has been accepted.
	CmdHello
)

// SudoRenameNoReplace, in Flags of a CmdRename, makes the dispatcher refuse to
// replace what is already at the destination, atomically, as a plain
// RenameNoReplace does.
const SudoRenameNoReplace = 1

type SudoRequest struct {
	Cmd   SudoCommand
	Path  string
	Path2 string  // Used for rename
	Flags int     // OS flags (e.g. O_RDONLY)
	Mode  uint32  // File permissions
	Item  VFSItem // Used for SetAttributes
}

type SudoResponse struct {
	Error string
	Item  VFSItem
	Items []VFSItem
	IsEOF bool
}
