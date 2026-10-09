//go:build !windows && !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris)

package dialog

// aboutOSRows has nothing to ask on a platform without uname or RtlGetVersion.
func aboutOSRows() []aboutRow { return nil }
