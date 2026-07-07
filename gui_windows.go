//go:build windows

package main

import (
	"strings"

	"github.com/unxed/vtui"
)

func RunGui(backend string) error {
	if backend == "qt" || strings.HasPrefix(backend, "ext:") {
		return RunExternalUIWithMapping(backend)
	}
	return vtui.RunInGUIWindow(100, 30, backend, func() {
		SetupUI()
	})
}
