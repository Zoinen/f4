//go:build amd64 && !android && !ios && (linux || darwin || freebsd || netbsd || windows || dragonfly || solaris || illumos)

package editor

import "golang.org/x/sys/cpu"

func colorerRuntimeCheck() error {
	// These OSes and SSE4.1 match wazero v1.12.0's compilerPlatformSupports.
	// Without SSE4.1, NewRuntimeConfig already chooses the safe interpreter.
	return colorerRuntimeCheckForCPU(cpu.X86.HasSSE41, cpu.X86.HasPOPCNT)
}
