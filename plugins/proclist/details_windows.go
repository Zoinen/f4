//go:build windows

package proclist

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// errDetailUnsupportedWindows explains why the open-files section of f4#312
// part 4's F3 view is empty on Windows: it needs a handle enumeration
// (NtQuerySystemInformation, DuplicateHandle, NtQueryObject) that can hang on
// pipes and lives behind undocumented NT APIs -- the same class this plugin
// already refuses to use for WMI performance counters and the handle viewer
// (plugin.go's own package doc) and for Ctrl+F8 suspend/resume
// (actions_windows.go).
var errDetailUnsupportedWindows = errors.New("proclist: not available on Windows without an undocumented API")

// collectProcDetails on Windows reads the command line and the environment
// out of the target's PEB (f4#312), the way Process Explorer and every "show
// environment" tool does: NtQueryInformationProcess finds the PEB, and
// ReadProcessMemory follows it to the process parameters. The layout of those
// structures is not documented but has been stable since Windows 7, and only a
// process of the same bitness is read; anything that fails is reported as the
// error of its section, and the command line falls back to the executable's
// own path.
func collectProcDetails(pid int) procDetails {
	details := procDetails{openFiles: procDetailsSection{err: errDetailUnsupportedWindows}}
	cmdline, environ, err := readWindowsProcessParameters(pid)
	if err != nil {
		details.cmdline = readWindowsExecutablePath(pid)
		details.environ = procDetailsSection{err: err}
		return details
	}
	details.cmdline = procDetailsSection{lines: []string{cmdline}}
	details.environ = procDetailsSection{lines: environ}
	return details
}

// Offsets inside the 64-bit PEB and RTL_USER_PROCESS_PARAMETERS.
const (
	pebProcessParametersOffset = 0x20
	paramsCommandLineOffset    = 0x70 // UNICODE_STRING: Length, MaximumLength, Buffer
	paramsEnvironmentOffset    = 0x80
	paramsEnvironmentSizeOff   = 0x3F0
	maxEnvironmentBytes        = 4 << 20
)

func readWindowsProcessParameters(pid int) (cmdline string, environ []string, err error) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		return "", nil, errors.New("proclist: reading another process' parameters is supported on 64-bit f4 only")
	}
	// #nosec G115 -- pid is a real OS process id.
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, uint32(pid))
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	var wow64 bool
	if err := windows.IsWow64Process(handle, &wow64); err != nil {
		return "", nil, err
	}
	if wow64 {
		return "", nil, errors.New("proclist: not available for a 32-bit process")
	}

	var pbi windows.PROCESS_BASIC_INFORMATION
	var returned uint32
	if err := windows.NtQueryInformationProcess(handle, windows.ProcessBasicInformation,
		unsafe.Pointer(&pbi), uint32(unsafe.Sizeof(pbi)), &returned); err != nil {
		return "", nil, err
	}
	if pbi.PebBaseAddress == nil {
		return "", nil, errors.New("proclist: the process has no PEB")
	}

	params, err := readRemotePointer(handle, uintptr(unsafe.Pointer(pbi.PebBaseAddress))+pebProcessParametersOffset)
	if err != nil {
		return "", nil, err
	}

	// The command line is a UNICODE_STRING.
	var us [16]byte
	if err := readRemote(handle, params+paramsCommandLineOffset, us[:]); err != nil {
		return "", nil, err
	}
	length := int(*(*uint16)(unsafe.Pointer(&us[0])))
	bufferAddr := *(*uintptr)(unsafe.Pointer(&us[8]))
	if length > 0 && bufferAddr != 0 {
		raw := make([]uint16, length/2)
		if err := readRemote(handle, bufferAddr, unsafe.Slice((*byte)(unsafe.Pointer(&raw[0])), length)); err != nil {
			return "", nil, err
		}
		cmdline = windows.UTF16ToString(raw)
	}

	envAddr, err := readRemotePointer(handle, params+paramsEnvironmentOffset)
	if err != nil {
		return cmdline, nil, err
	}
	var sizeBuf [8]byte
	if err := readRemote(handle, params+paramsEnvironmentSizeOff, sizeBuf[:]); err != nil {
		return cmdline, nil, err
	}
	size := int(*(*uint64)(unsafe.Pointer(&sizeBuf[0])))
	if envAddr == 0 || size <= 0 {
		return cmdline, nil, nil
	}
	if size > maxEnvironmentBytes {
		return cmdline, nil, fmt.Errorf("proclist: environment block of %d bytes is not read", size)
	}
	units := make([]uint16, size/2)
	if err := readRemote(handle, envAddr, unsafe.Slice((*byte)(unsafe.Pointer(&units[0])), len(units)*2)); err != nil {
		return cmdline, nil, err
	}
	return cmdline, splitWindowsEnvironmentBlock(units), nil
}

func readRemote(handle windows.Handle, addr uintptr, buf []byte) error {
	if len(buf) == 0 {
		return nil
	}
	var read uintptr
	if err := windows.ReadProcessMemory(handle, addr, &buf[0], uintptr(len(buf)), &read); err != nil {
		return err
	}
	if read != uintptr(len(buf)) {
		return fmt.Errorf("proclist: read %d of %d bytes of the process' memory", read, len(buf))
	}
	return nil
}

func readRemotePointer(handle windows.Handle, addr uintptr) (uintptr, error) {
	var b [8]byte
	if err := readRemote(handle, addr, b[:]); err != nil {
		return 0, err
	}
	return *(*uintptr)(unsafe.Pointer(&b[0])), nil
}

func readWindowsExecutablePath(pid int) procDetailsSection {
	// #nosec G115 -- pid is a real OS process id, the same conversion
	// collector_windows.go's own readWindowsProcess already makes for
	// OpenProcess.
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return procDetailsSection{err: err}
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	// windows.MAX_PATH (260) is the historical limit; a few multiples of it
	// leaves headroom for a long-path-aware target without needing to retry
	// on ERROR_INSUFFICIENT_BUFFER.
	buf := make([]uint16, windows.MAX_PATH*4)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil {
		return procDetailsSection{err: err}
	}
	return procDetailsSection{lines: []string{windows.UTF16ToString(buf[:size])}}
}
