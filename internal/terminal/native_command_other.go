//go:build !windows

package terminal

import "context"

// The native host-process transport is only meaningful for a Windows binary
// running under Wine. Native Unix builds already use os/exec directly.
func runNativeLocalCommand(context.Context, string, string, func([]byte)) (bool, int, error) {
	return false, 0, nil
}

func runNativeLocalCommandCapture(context.Context, string, string) (bool, []byte, error) {
	return false, nil, nil
}
