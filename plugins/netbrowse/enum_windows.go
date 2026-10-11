//go:build windows

package netbrowse

import (
	"syscall"
	"unsafe"

	"github.com/unxed/f4/vfs"
	"golang.org/x/sys/windows"
)

var (
	mpr          = windows.NewLazySystemDLL("mpr.dll")
	procOpenEnum = mpr.NewProc("WNetOpenEnumW")
	procEnum     = mpr.NewProc("WNetEnumResourceW")
	procCloseEnu = mpr.NewProc("WNetCloseEnum")
)

const (
	resourceGlobalNet  = 2
	resourceTypeAny    = 0
	resourceUsageCont  = 0x2
	errorMoreData      = 234
	errorNoMoreItems   = 259
	enumEverything     = 0xFFFFFFFF
	firstBufferSize    = 16 * 1024
	maxEnumeratedBytes = 4 * 1024 * 1024
)

// netResource is NETRESOURCEW of winnetwk.h.
type netResource struct {
	Scope       uint32
	Type        uint32
	DisplayType uint32
	Usage       uint32
	LocalName   *uint16
	RemoteName  *uint16
	Comment     *uint16
	Provider    *uint16
}

// enumerateNetwork lists the entries under parent (nil: the top of the
// network) with WNetOpenEnum and WNetEnumResource.
func enumerateNetwork(parent *resource) ([]resource, error) {
	var handle uintptr
	var open *netResource
	if parent != nil {
		remote, err := windows.UTF16PtrFromString(parent.Remote)
		if err != nil {
			return nil, err
		}
		var provider *uint16
		if parent.Provider != "" {
			if provider, err = windows.UTF16PtrFromString(parent.Provider); err != nil {
				return nil, err
			}
		}
		open = &netResource{
			Scope: resourceGlobalNet, DisplayType: parent.Display, Usage: resourceUsageCont,
			RemoteName: remote, Provider: provider,
		}
	}
	if code, _, _ := procOpenEnum.Call(resourceGlobalNet, resourceTypeAny, 0,
		uintptr(unsafe.Pointer(open)), uintptr(unsafe.Pointer(&handle))); code != 0 {
		return nil, syscall.Errno(code)
	}
	defer func() { _, _, _ = procCloseEnu.Call(handle) }()

	var out []resource
	buf := make([]byte, firstBufferSize)
	for {
		count := uint32(enumEverything)
		size := uint32(len(buf))
		code, _, _ := procEnum.Call(handle, uintptr(unsafe.Pointer(&count)),
			uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
		switch code {
		case 0:
		case errorNoMoreItems:
			return out, nil
		case errorMoreData:
			if int(size) > maxEnumeratedBytes {
				return out, syscall.Errno(code)
			}
			buf = make([]byte, size)
			continue
		default:
			return out, syscall.Errno(code)
		}
		entries := unsafe.Slice((*netResource)(unsafe.Pointer(&buf[0])), int(count))
		for _, e := range entries {
			out = append(out, resource{
				Remote:    windows.UTF16PtrToString(e.RemoteName),
				Comment:   windows.UTF16PtrToString(e.Comment),
				Provider:  windows.UTF16PtrToString(e.Provider),
				Display:   e.DisplayType,
				Container: e.Usage&resourceUsageCont != 0,
			})
		}
	}
}

// openShare is the file system at a share's UNC name: the ordinary one.
func openShare(unc string) vfs.VFS { return openUNC(unc) }

// guessResource: on Windows every server the network has is enumerated, so
// there is nothing to guess.
func guessResource(*resource, string) *resource { return nil }
