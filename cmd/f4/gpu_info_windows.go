//go:build windows

package main

import (
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	gpuOnce   sync.Once
	cachedGPU []GPUInfo
)

// DISPLAY_DEVICEW as defined in wingdi.h. Fixed-size UTF-16 buffers
// per the SDK; we let Windows write in and then decode. StateFlags
// is captured but no longer used as a filter — the user asked to
// see both the iGPU and a spun-down dGPU on Optimus laptops, and
// EnumDisplayDevices is the only API that lists them without WMI.
type displayDeviceW struct {
	cb           uint32
	DeviceName   [32]uint16
	DeviceString [128]uint16
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

var (
	modUser32              = windows.NewLazySystemDLL("user32.dll")
	procEnumDisplayDevices = modUser32.NewProc("EnumDisplayDevicesW")
)

// gpuInfo enumerates adapters via EnumDisplayDevices. On multi-GPU
// laptops both the iGPU and dGPU show up; duplicates by DeviceString
// are collapsed. Driver name comes from the DeviceKey → registry
// hop (\Registry\Machine\... → HKLM\...\DriverDesc).
func gpuInfo() ([]GPUInfo, bool) {
	gpuOnce.Do(func() {
		cachedGPU = enumerateWindowsGPUs()
	})
	return cachedGPU, len(cachedGPU) > 0
}

func enumerateWindowsGPUs() []GPUInfo {
	seen := map[string]struct{}{}
	var out []GPUInfo
	for i := uint32(0); ; i++ {
		var d displayDeviceW
		d.cb = uint32(unsafe.Sizeof(d))
		r, _, _ := procEnumDisplayDevices.Call(
			0,
			uintptr(i),
			uintptr(unsafe.Pointer(&d)),
			0,
		)
		if r == 0 {
			break
		}
		name := utf16ToStringGPU(d.DeviceString[:])
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		driver := readGPUDriverName(utf16ToStringGPU(d.DeviceKey[:]))
		out = append(out, GPUInfo{Model: name, Driver: driver})
	}
	return out
}

// readGPUDriverName resolves DeviceKey (`\Registry\Machine\...`) to
// the DriverDesc value it stores. Falls back to ProviderName if
// DriverDesc is empty (some Basic Display driver installs). Empty
// on any failure — caller renders the row without a Driver line.
func readGPUDriverName(deviceKey string) string {
	// EnumDisplayDevices returns the key as a fully-qualified NT
	// path (\Registry\Machine\...); RegOpenKeyEx expects the tail
	// under HKLM. Trim both accepted prefixes.
	sub := deviceKey
	for _, prefix := range []string{
		`\Registry\Machine\`,
		`\REGISTRY\MACHINE\`,
	} {
		if strings.HasPrefix(sub, prefix) {
			sub = sub[len(prefix):]
			break
		}
	}
	if sub == "" || sub == deviceKey {
		// Prefix wasn't the shape we expected — give up rather
		// than open a random registry path.
		return ""
	}
	keyPath, err := windows.UTF16PtrFromString(sub)
	if err != nil {
		return ""
	}
	var h windows.Handle
	if err := windows.RegOpenKeyEx(windows.HKEY_LOCAL_MACHINE, keyPath, 0, windows.KEY_READ, &h); err != nil {
		return ""
	}
	defer windows.RegCloseKey(h)

	for _, valName := range []string{"DriverDesc", "ProviderName"} {
		if s := readRegString(h, valName); s != "" {
			return s
		}
	}
	return ""
}

func readRegString(h windows.Handle, name string) string {
	nPtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return ""
	}
	var typ, size uint32
	if err := windows.RegQueryValueEx(h, nPtr, nil, &typ, nil, &size); err != nil {
		return ""
	}
	if size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if err := windows.RegQueryValueEx(h, nPtr, nil, &typ, &buf[0], &size); err != nil {
		return ""
	}
	u16 := unsafe.Slice((*uint16)(unsafe.Pointer(&buf[0])), size/2)
	return strings.TrimSpace(windows.UTF16ToString(u16))
}

// utf16ToStringGPU handles a fixed-size buffer that may or may not
// be NUL-terminated. syscall.UTF16ToString stops at the first NUL;
// if there isn't one we still get the right result.
func utf16ToStringGPU(u []uint16) string {
	for i, c := range u {
		if c == 0 {
			return syscall.UTF16ToString(u[:i])
		}
	}
	return syscall.UTF16ToString(u)
}
