//go:build !amd64 || android || ios || !(linux || darwin || freebsd || netbsd || windows || dragonfly || solaris || illumos)

package editor

func colorerRuntimeCheck() error {
	return nil
}
