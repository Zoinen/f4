package main

import (
	"fmt"
	"path/filepath"
)

// runNativeGate is deliberately a composition of native runs. There is no
// simulated terminal path: each stage must pass through the pinned host.
func runNativeGate(hostPath, reportPath string) error {
	if reportPath == "" {
		reportPath = filepath.Join("artifacts", "pinned-conpty-gate.json")
	}
	if err := runNativeProbe(hostPath, reportPath+".static", false); err != nil {
		return fmt.Errorf("static pinned-host stage: %w", err)
	}
	if err := runNativeReflowProbe(hostPath, reportPath+".reflow"); err != nil {
		return fmt.Errorf("consumer reflow stage: %w", err)
	}
	stages := []struct {
		name string
		run  func(string, string) error
	}{
		{"command-suite", runNativeCommandSuite},
		{"command-compare", func(host, report string) error {
			return runNativeCommandCompareAtWidth(host, report, 512)
		}},
		{"clear", runNativeClearProbe},
		{"scroll", runNativeScrollProbe},
		{"empty", runNativeEmptyProbe},
	}
	for _, stage := range stages {
		if err := stage.run(hostPath, reportPath+"."+stage.name); err != nil {
			return fmt.Errorf("%s stage: %w", stage.name, err)
		}
	}
	if err := runNativeLifecycleProbe(hostPath, reportPath+".lifecycle"); err != nil {
		return fmt.Errorf("lifecycle stage: %w", err)
	}
	if err := runNativeEdgeProbe(hostPath, reportPath+".edge"); err != nil {
		return fmt.Errorf("edge payload stage: %w", err)
	}
	if err := runNativeQuirkProbe(hostPath, reportPath+".quirk"); err != nil {
		return fmt.Errorf("resizeQuirk stage: %w", err)
	}
	for _, kind := range []string{"tabs", "link", "progress", "unicode"} {
		if err := runNativeSemanticProbe(hostPath, reportPath+"."+kind, kind); err != nil {
			return fmt.Errorf("%s semantic stage: %w", kind, err)
		}
	}
	if err := runNativeSeedGate(hostPath, reportPath+".seeds"); err != nil {
		return fmt.Errorf("300-seed native stage: %w", err)
	}
	return nil
}
