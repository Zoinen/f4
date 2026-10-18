//go:build !linux

package sysinfo

// Device is not implemented on this platform yet: callers get ok=false and
// must treat the storage as unknown.
func Device(path string) (DeviceInfo, bool) {
	return DeviceInfo{}, false
}
