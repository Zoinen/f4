//go:build !windows && !darwin

package vfs

func applyPlatformAttributes(path string, item VFSItem) error {
	return nil
}

// SupportsSetBTime is false here: Unix has no call that sets a birth time.
func (v *OSVFS) SupportsSetBTime() bool { return false }
