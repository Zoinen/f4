//go:build !linux && !windows

package main

// fsInfo stub for non-Linux unices. Statfs is available on the BSDs
// and macOS but the field types (`Bavail`, `Bsize`, `Namelen`) differ
// between them; the info panel silently omits the section rather than
// carry per-BSD ports for now. Fixable in a follow-up if anyone cares.
func fsInfo(path string) (FSInfo, bool) {
	return FSInfo{}, false
}
