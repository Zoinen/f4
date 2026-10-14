//go:build darwin

package sysinfo

import (
	"fmt"
	"os"
	"sync"

	"github.com/ebitengine/purego"
)

// These read-only libSystem bindings keep native collection cgo-free.
type darwinSystemAPI struct {
	hostSelf   func() uint32
	taskSelf   func() uint32
	hostStats  func(uint32, int32, *uint32, *uint32) int32
	deallocate func(uint32, uint32) int32
	getAttr    func(string, *darwinAttrlist, *byte, uintptr, uint32) int32
}

type darwinAttrlist struct {
	Count, Reserved                       uint16
	Common, Volume, Directory, File, Fork uint32
}

var loadDarwinSystemAPI = sync.OnceValue(func() *darwinSystemAPI {
	handle, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if err != nil {
		darwinInfoDebug("libSystem unavailable: %v", err)
		return nil
	}
	api := &darwinSystemAPI{}
	bindings := []struct {
		name   string
		target any
	}{
		{name: "mach_host_self", target: &api.hostSelf},
		{name: "mach_task_self", target: &api.taskSelf},
		{name: "host_statistics64", target: &api.hostStats},
		{name: "mach_port_deallocate", target: &api.deallocate},
		{name: "getattrlist", target: &api.getAttr},
	}
	for _, binding := range bindings {
		address, err := purego.Dlsym(handle, binding.name)
		if err != nil {
			darwinInfoDebug("%s unavailable: %v", binding.name, err)
			_ = purego.Dlclose(handle)
			return nil
		}
		purego.RegisterFunc(binding.target, address)
	}
	// Keep the handle for the lifetime of its callable functions.
	return api
})

func darwinInfoDebug(format string, args ...any) {
	if os.Getenv("VTUI_DEBUG") != "" {
		_, _ = fmt.Fprintf(os.Stderr, "[FIX:darwin-info] "+format+"\n", args...)
	}
}
