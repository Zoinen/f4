package filemenu

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/zzl/go-win32api/v2/win32"
	"golang.org/x/sys/windows"
)

type menuTheme struct {
	module             windows.Handle
	prefer             func(uintptr)
	allow              func(win32.HWND, bool)
	refresh            func()
	shouldDark         func() bool
	flush              func()
	contrast           func() bool
	initialized        bool
	dark, highContrast bool
	hwnd               win32.HWND
}

func newMenuTheme() (*menuTheme, error) {
	// The v6 manifest alone is insufficient: user32 keeps drawing classic menus
	// until comctl32 has initialized the process's visual-style activation.
	controls := win32.INITCOMMONCONTROLSEX{DwICC: win32.ICC_WIN95_CLASSES}
	controls.DwSize = uint32(unsafe.Sizeof(controls))
	if win32.InitCommonControlsEx(&controls) == 0 {
		return nil, fmt.Errorf("Initialize visual styles failed")
	}
	m := &menuTheme{contrast: windowsHighContrast}
	version := windows.RtlGetVersion()
	if !supportsDarkMenus(version.MajorVersion, version.BuildNumber) {
		return m, nil
	}
	module, err := windows.LoadLibraryEx("uxtheme.dll", 0, windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return m, nil
	}
	// These are private exports, also used by Microsoft PowerToys. Ordinal 135
	// had a different signature before Windows 10 1903, hence the version guard.
	prefer, _ := windows.GetProcAddressByOrdinal(module, 135)
	allow, _ := windows.GetProcAddressByOrdinal(module, 133)
	shouldDark, _ := windows.GetProcAddressByOrdinal(module, 132)
	flush, _ := windows.GetProcAddressByOrdinal(module, 136)
	refresh, _ := windows.GetProcAddressByOrdinal(module, 104)
	if prefer == 0 || allow == 0 || shouldDark == 0 || flush == 0 {
		_ = windows.FreeLibrary(module)
		return m, nil
	}
	m.module = module
	m.prefer = func(mode uintptr) { syscall.SyscallN(prefer, mode) }
	m.allow = func(hwnd win32.HWND, dark bool) {
		var enabled uintptr
		if dark {
			enabled = 1
		}
		syscall.SyscallN(allow, uintptr(hwnd), enabled)
	}
	m.shouldDark = func() bool { value, _, _ := syscall.SyscallN(shouldDark); return value&0xff != 0 }
	m.flush = func() { syscall.SyscallN(flush) }
	m.refresh = func() {
		if refresh != 0 {
			syscall.SyscallN(refresh)
		}
	}
	m.update(0)
	return m, nil
}

func supportsDarkMenus(major, build uint32) bool { return major >= 10 && build >= 18362 }

func windowsHighContrast() bool {
	hc := win32.HIGHCONTRASTW{}
	hc.CbSize = uint32(unsafe.Sizeof(hc))
	ok, _ := win32.SystemParametersInfoW(win32.SPI_GETHIGHCONTRAST, hc.CbSize, unsafe.Pointer(&hc), 0)
	return ok != 0 && hc.DwFlags&win32.HCF_HIGHCONTRASTON != 0
}

func (m *menuTheme) update(hwnd win32.HWND) bool {
	if m.prefer == nil {
		return false
	}
	hc := m.contrast()
	if !m.initialized || hc != m.highContrast {
		mode := uintptr(1) // AllowDark: follow the Windows application color preference.
		if hc {
			mode = 0
		} // Let Windows render its high-contrast scheme unchanged.
		m.prefer(mode)
	}
	m.refresh()
	dark := m.shouldDark() && !hc
	changed := !m.initialized || dark != m.dark || hc != m.highContrast
	if changed || hwnd != m.hwnd {
		if hwnd != 0 {
			m.allow(hwnd, dark)
		}
		m.flush()
	}
	m.initialized = true
	m.dark, m.highContrast, m.hwnd = dark, hc, hwnd
	return changed
}

func (m *menuTheme) close() {
	if m.module != 0 {
		_ = windows.FreeLibrary(m.module)
	}
}
