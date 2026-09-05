package main

// FSInfo describes the filesystem holding a given path. Fields default
// to their zero value if the platform can't supply them. Per-OS
// implementations of fsInfo live in fs_info_{linux,windows,other}.go.
type FSInfo struct {
	Total, Free uint64
	Type        string // e.g. "ext4" / "NTFS"; "" when the OS doesn't tell us
	Label       string // populated on Windows
	Serial      string // populated on Windows
	Mount       string // mount point / drive root
	MaxFilename int    // Statfs.Namelen on Linux, MAX_PATH-derived on Windows
	Flags       string // "rw,relatime" (Linux) / decoded volume flags (Windows)
	ClusterSize uint64 // fs cluster / block size in bytes; 0 = unknown
}
