//go:build !windows && !darwin && !linux

package filemenu

const Platform = "other"

func showNative(Request) Result { return Result{Outcome: Unavailable} }
