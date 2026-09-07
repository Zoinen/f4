//go:build windows

package main

import (
	"encoding/binary"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/windows"
)

var (
	cpuStaticOnce sync.Once
	cachedCPU     CPUInfo
)

// pdhInit runs once and, on success, populates the four function
// pointers used by sampleCPULoad. On any failure we leave them nil
// and the CPU section still renders Model + Cores.
var (
	pdhOnce sync.Once
	pdhOK   bool

	pdhOpenQueryW               func(dataSrc *uint16, userData uintptr, hQuery *uintptr) uint32
	pdhAddEnglishCounterW       func(hQuery uintptr, path *uint16, userData uintptr, hCounter *uintptr) uint32
	pdhCollectQueryData         func(hQuery uintptr) uint32
	pdhGetFormattedCounterValue func(hCounter uintptr, format uint32, counterType *uint32, val *pdhFmtCounterValueDouble) uint32
)

// PDH counter names are English-only when queried through
// PdhAddEnglishCounterW, which sidesteps localization. Utility is
// the Task-Manager metric (Win8+); Time is the classic fallback.
const (
	pdhCounterUtility = `\Processor Information(_Total)\% Processor Utility`
	pdhCounterTime    = `\Processor Information(_Total)\% Processor Time`
	pdhFmtDouble      = 0x00000200
	pdhCstatusValid   = 0
)

// PDH_FMT_COUNTERVALUE with the double member selected. Explicit
// _pad so the layout is unambiguous on x64 regardless of Go's
// struct-packing choices.
type pdhFmtCounterValueDouble struct {
	CStatus   uint32
	_pad      uint32
	DoubleVal float64
}

var (
	cpuLoadMu  sync.Mutex
	hQuery     uintptr
	hCounter   uintptr
	haveSample bool
	lastSample time.Time
	lastPct    int
	hasLast    bool
)

const cpuSampleInterval = 500 * time.Millisecond

func cpuInfo() (CPUInfo, bool) {
	cpuStaticOnce.Do(func() {
		cachedCPU = readStaticWindowsCPU()
	})
	info := cachedCPU
	if pct, ok := sampleCPULoad(); ok {
		info.Load = pct
		info.HasLoadPct = true
	}
	if info.Model == "" && info.LogicalCores == 0 && info.PhysicalCores == 0 &&
		info.FreqMHz == 0 && info.CacheBytes == [4]uint64{} && !info.HasLoadPct {
		return CPUInfo{}, false
	}
	return info, true
}

func readStaticWindowsCPU() CPUInfo {
	info := CPUInfo{LogicalCores: runtime.NumCPU()}
	info.Model = readCPUModelReg()
	info.FreqMHz = readCPUFreqReg()
	readWindowsTopology(&info)
	return info
}

// initPDH loads pdh.dll, opens a query, and adds the counter.
// Called lazily on first sample. Silent failure: pdhOK stays false
// and sampleCPULoad short-circuits. RegisterLibFunc panics on a
// missing symbol, hence the recover — pdh.dll is present on all
// current Windows installs but we tolerate its absence anyway.
func initPDH() {
	pdhOnce.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				pdhOK = false
			}
		}()
		h, err := purego.Dlopen("pdh.dll", 0)
		if err != nil || h == 0 {
			return
		}
		purego.RegisterLibFunc(&pdhOpenQueryW, h, "PdhOpenQueryW")
		purego.RegisterLibFunc(&pdhAddEnglishCounterW, h, "PdhAddEnglishCounterW")
		purego.RegisterLibFunc(&pdhCollectQueryData, h, "PdhCollectQueryData")
		purego.RegisterLibFunc(&pdhGetFormattedCounterValue, h, "PdhGetFormattedCounterValue")

		if pdhOpenQueryW(nil, 0, &hQuery) != 0 {
			return
		}
		util, _ := syscall.UTF16PtrFromString(pdhCounterUtility)
		if pdhAddEnglishCounterW(hQuery, util, 0, &hCounter) != 0 {
			// Older Windows / disabled counter set — fall back
			// to % Processor Time. Same metric the old
			// GetSystemTimes path produced, so we don't regress.
			classic, _ := syscall.UTF16PtrFromString(pdhCounterTime)
			if pdhAddEnglishCounterW(hQuery, classic, 0, &hCounter) != 0 {
				return
			}
		}
		pdhOK = true
	})
}

// sampleCPULoad returns the last computed CPU load %. Throttled to
// once per cpuSampleInterval so the panel can Show() as fast as it
// likes without hammering PDH. Returns ok=false until we have two
// samples (PDH's first collect is unusable).
func sampleCPULoad() (int, bool) {
	initPDH()
	if !pdhOK {
		return 0, false
	}

	cpuLoadMu.Lock()
	defer cpuLoadMu.Unlock()

	now := time.Now()
	if hasLast && now.Sub(lastSample) < cpuSampleInterval {
		return lastPct, true
	}
	if pdhCollectQueryData(hQuery) != 0 {
		return lastPct, hasLast
	}
	lastSample = now
	if !haveSample {
		haveSample = true
		return 0, false
	}
	var v pdhFmtCounterValueDouble
	if pdhGetFormattedCounterValue(hCounter, pdhFmtDouble, nil, &v) != 0 || v.CStatus != pdhCstatusValid {
		return lastPct, hasLast
	}
	pct := int(v.DoubleVal + 0.5)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	lastPct = pct
	hasLast = true
	return pct, true
}

// readCPUModelReg reads the ProcessorNameString registry value the
// kernel populates at boot. Empty on failure — caller skips the row.
func readCPUModelReg() string {
	keyPath, err := windows.UTF16PtrFromString(`HARDWARE\DESCRIPTION\System\CentralProcessor\0`)
	if err != nil {
		return ""
	}
	var h windows.Handle
	if err := windows.RegOpenKeyEx(windows.HKEY_LOCAL_MACHINE, keyPath, 0, windows.KEY_READ, &h); err != nil {
		return ""
	}
	defer windows.RegCloseKey(h)

	name, err := windows.UTF16PtrFromString("ProcessorNameString")
	if err != nil {
		return ""
	}
	var typ, size uint32
	if err := windows.RegQueryValueEx(h, name, nil, &typ, nil, &size); err != nil {
		return ""
	}
	if size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if err := windows.RegQueryValueEx(h, name, nil, &typ, &buf[0], &size); err != nil {
		return ""
	}
	u16 := unsafe.Slice((*uint16)(unsafe.Pointer(&buf[0])), size/2)
	return windows.UTF16ToString(u16)
}

// readCPUFreqReg pulls the ~MHz DWORD from the same registry key.
// It's the nominal clock (base MHz), not the current turbo state —
// good enough for an info-panel row and consistent with what wmic
// / Task Manager list under "Base speed".
func readCPUFreqReg() int {
	keyPath, err := windows.UTF16PtrFromString(`HARDWARE\DESCRIPTION\System\CentralProcessor\0`)
	if err != nil {
		return 0
	}
	var h windows.Handle
	if err := windows.RegOpenKeyEx(windows.HKEY_LOCAL_MACHINE, keyPath, 0, windows.KEY_READ, &h); err != nil {
		return 0
	}
	defer windows.RegCloseKey(h)
	name, err := windows.UTF16PtrFromString("~MHz")
	if err != nil {
		return 0
	}
	var typ, size uint32 = 0, 4
	var val uint32
	if err := windows.RegQueryValueEx(h, name, nil, &typ, (*byte)(unsafe.Pointer(&val)), &size); err != nil {
		return 0
	}
	return int(val)
}

// GetLogicalProcessorInformation gives us physical-core count and
// per-level cache sizes in one call. We loop, sizing the buffer on
// the first ERROR_INSUFFICIENT_BUFFER, then walk 32-byte entries.
var (
	kernel32CPU                        = windows.NewLazySystemDLL("kernel32.dll")
	procGetLogicalProcessorInformation = kernel32CPU.NewProc("GetLogicalProcessorInformation")
)

const (
	relationProcessorCore = 0
	relationCache         = 2
)

// Each SYSTEM_LOGICAL_PROCESSOR_INFORMATION entry is 32 bytes on
// x64 / amd64 build target: ULONG_PTR(8) + Relationship(4) + pad(4)
// + union(16). We keep the layout explicit rather than importing a
// header, because purego doesn't rewrap it and neither does
// golang.org/x/sys/windows.
const slpiSize = 32

func readWindowsTopology(info *CPUInfo) {
	// Two-pass sizing.
	var need uint32
	r1, _, _ := procGetLogicalProcessorInformation.Call(0, uintptr(unsafe.Pointer(&need)))
	if r1 != 0 || need == 0 {
		return
	}
	buf := make([]byte, need)
	r2, _, _ := procGetLogicalProcessorInformation.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&need)),
	)
	if r2 == 0 {
		return
	}
	entries := int(need) / slpiSize
	for i := 0; i < entries; i++ {
		e := buf[i*slpiSize : (i+1)*slpiSize]
		mask := binary.LittleEndian.Uint64(e[0:8])
		rel := binary.LittleEndian.Uint32(e[8:12])
		payload := e[16:32]
		switch rel {
		case relationProcessorCore:
			info.PhysicalCores++
			_ = mask // logical threads = popcount(mask); already have via runtime.NumCPU
		case relationCache:
			// CACHE_DESCRIPTOR: BYTE Level; BYTE Associativity;
			// WORD LineSize; DWORD Size; ProcessorCacheType Type.
			level := int(payload[0])
			size := binary.LittleEndian.Uint32(payload[4:8])
			cType := binary.LittleEndian.Uint32(payload[8:12])
			// PROCESSOR_CACHE_TYPE: 0 Unified, 1 Instruction,
			// 2 Data, 3 Trace. Skip Instruction on L2+ (unified
			// is authoritative); at L1 we count both I+D.
			if level < 1 || level > 4 || size == 0 {
				continue
			}
			if level > 1 && cType == 1 {
				continue
			}
			info.CacheBytes[level-1] += uint64(size)
		}
	}
}
