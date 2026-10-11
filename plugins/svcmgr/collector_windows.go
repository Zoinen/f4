//go:build windows

package svcmgr

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// listServices asks the Service Control Manager of the given computer (empty:
// this one) for every Win32 service in any
// state. The manager is opened with the enumerate right only -- the mgr
// package's Connect asks for all access, which a user without administrator
// rights does not get -- and the answer comes in one buffer that grows while
// the call reports ERROR_MORE_DATA.
func listServices(machine string) ([]service, error) {
	manager, err := openManager(machine, windows.SC_MANAGER_CONNECT|windows.SC_MANAGER_ENUMERATE_SERVICE)
	if err != nil {
		return nil, err
	}
	defer func() { _ = windows.CloseServiceHandle(manager) }()

	var buf []byte
	var needed, returned, resume uint32
	for {
		var first *byte
		if len(buf) > 0 {
			first = &buf[0]
		}
		err := windows.EnumServicesStatusEx(manager, windows.SC_ENUM_PROCESS_INFO,
			windows.SERVICE_WIN32, windows.SERVICE_STATE_ALL,
			first, uint32(len(buf)), &needed, &returned, &resume, nil)
		if err == nil {
			break
		}
		if err != windows.ERROR_MORE_DATA || needed == 0 {
			return nil, err
		}
		buf = make([]byte, len(buf)+int(needed))
		resume = 0
	}
	if returned == 0 {
		return nil, nil
	}
	entries := unsafe.Slice((*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(&buf[0])), int(returned))
	out := make([]service, 0, len(entries))
	for _, e := range entries {
		out = append(out, service{
			Name:    windows.UTF16PtrToString(e.ServiceName),
			Display: windows.UTF16PtrToString(e.DisplayName),
			State:   e.ServiceStatusProcess.CurrentState,
			PID:     e.ServiceStatusProcess.ProcessId,
			// The list shows how each service starts, as FAR's SvcMgr does; a
			// service that refuses the query just has none.
			StartType: startTypeOf(manager, e.ServiceName),
		})
	}
	return out, nil
}

// startTypeOf reads one service's start type with the query-configuration
// right only, or startUnknown when the service cannot be opened or read.
func startTypeOf(manager windows.Handle, name *uint16) uint32 {
	h, err := windows.OpenService(manager, name, windows.SERVICE_QUERY_CONFIG)
	if err != nil {
		return startUnknown
	}
	defer func() { _ = windows.CloseServiceHandle(h) }()
	var needed uint32
	// The first call only reports the size the configuration needs.
	if err := windows.QueryServiceConfig(h, nil, 0, &needed); err != windows.ERROR_INSUFFICIENT_BUFFER || needed == 0 {
		return startUnknown
	}
	buf := make([]byte, needed)
	cfg := (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buf[0]))
	if err := windows.QueryServiceConfig(h, cfg, needed, &needed); err != nil {
		return startUnknown
	}
	return cfg.StartType
}
