package sysinfo

// FSInfo describes the filesystem holding a given path. Fields default
// to their zero value if the platform can't supply them. Per-OS
// implementations of FsInfo live in fs_info_{linux,windows,other}.go.
type FSInfo struct {
	Total, Free uint64
	Type        string // e.g. "ext4" / "NTFS"; "" when the OS doesn't tell us
	Label       string // volume name on Windows and macOS
	Serial      string // volume serial on Windows; volume UUID on macOS
	Mount       string // mount point / drive root
	MaxFilename int    // filesystem filename-component limit
	Flags       string // decoded mount and/or volume capability flags
	ClusterSize uint64 // fs cluster / block size in bytes; 0 = unknown
}
