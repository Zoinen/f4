package sysinfo

// HostInfo describes the local computer, independent of the active filesystem.
type HostInfo struct {
	Model, Chip, OS, Architecture string
	Displays                      []DisplayInfo
}

// DisplayInfo contains physical pixel resolution, not scaled desktop points.
// Diagonal is the approximate size reported by macOS, or empty if unavailable.
type DisplayInfo struct {
	Name, Resolution, Diagonal string
	id                         uint32
}
