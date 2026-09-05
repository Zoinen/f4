//go:build !windows

package main

import "fmt"

func runNativeSeedGate(hostPath, reportPath string) error {
	_ = hostPath
	_ = reportPath
	return fmt.Errorf("native seed gate requires Windows")
}

func runNativeSingleSeed(hostPath, reportPath string, seed uint64) error {
	_ = hostPath
	_ = reportPath
	_ = seed
	return fmt.Errorf("native single-seed probe requires Windows")
}

func runNativePartialProbe(hostPath, reportPath string) error {
	_ = hostPath
	_ = reportPath
	return fmt.Errorf("native partial probe requires Windows")
}

func runNativeCommandProbe(hostPath, reportPath string) error {
	_ = hostPath
	_ = reportPath
	return fmt.Errorf("native command probe requires Windows")
}

func runNativeCommandCompare(hostPath, reportPath string) error {
	_ = hostPath
	_ = reportPath
	return fmt.Errorf("native command comparison requires Windows")
}

func runNativeCommandCompareAtWidth(hostPath, reportPath string, width int) error {
	_ = hostPath
	_ = reportPath
	_ = width
	return fmt.Errorf("native command comparison requires Windows")
}

func runNativeCommandSuite(hostPath, reportPath string) error {
	_ = hostPath
	_ = reportPath
	return fmt.Errorf("native command suite requires Windows")
}

func runNativeSemanticProbe(hostPath, reportPath, kind string) error {
	_ = hostPath
	_ = reportPath
	_ = kind
	return fmt.Errorf("native semantic probe requires Windows")
}

func runNativeClearProbe(hostPath, reportPath string) error {
	_ = hostPath
	_ = reportPath
	return fmt.Errorf("native clear probe requires Windows")
}

func runNativeScrollProbe(hostPath, reportPath string) error {
	_ = hostPath
	_ = reportPath
	return fmt.Errorf("native scroll probe requires Windows")
}

func runNativeEmptyProbe(hostPath, reportPath string) error {
	_ = hostPath
	_ = reportPath
	return fmt.Errorf("native empty probe requires Windows")
}

func runNativeReflowProbe(hostPath, reportPath string) error {
	_ = hostPath
	_ = reportPath
	return fmt.Errorf("native reflow probe requires Windows")
}

func runNativeLifecycleProbe(hostPath, reportPath string) error {
	_ = hostPath
	_ = reportPath
	return fmt.Errorf("native lifecycle probe requires Windows")
}

func runNativeEdgeProbe(hostPath, reportPath string) error {
	_ = hostPath
	_ = reportPath
	return fmt.Errorf("native edge probe requires Windows")
}

func runNativeQuirkProbe(hostPath, reportPath string) error {
	_ = hostPath
	_ = reportPath
	return fmt.Errorf("native resizeQuirk probe requires Windows")
}
