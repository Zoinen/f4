//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type quirkProbeReport struct {
	Mode            string             `json:"mode"`
	Host            pinnedHostIdentity `json:"host"`
	WithoutQuirk    nativeProbeSession `json:"without_quirk"`
	WithQuirk       nativeProbeSession `json:"with_quirk"`
	WithoutRepaints int                `json:"without_quirk_repaints"`
	WithRepaints    int                `json:"with_quirk_repaints"`
	CompletedAt     time.Time          `json:"completed_at"`
}

func runNativeQuirkProbe(hostPath, reportPath string) error {
	if reportPath == "" {
		reportPath = filepath.Join("artifacts", "pinned-conpty-quirk.json")
	}
	resolved, err := ensureProbeHost(hostPath)
	if err != nil {
		return err
	}
	identity, err := verifyPinnedHost(resolved)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	workload := quirkProbeWorkload()
	command := fmt.Sprintf(`"%s" -emit-quirk`, executable)
	without, withoutErr := runNativeProbeSessionWithWorkloadQuirkDelay(resolved, executable, 80, 25, true, workload, command, []string{quirkBeginMarker, quirkEndMarker}, false, 100*time.Millisecond)
	if withoutErr != nil {
		return fmt.Errorf("resizeQuirk-disabled session: %w", withoutErr)
	}
	with, withErr := runNativeProbeSessionWithWorkloadQuirkDelay(resolved, executable, 80, 25, true, workload, command, []string{quirkBeginMarker, quirkEndMarker}, true, 100*time.Millisecond)
	if withErr != nil {
		return fmt.Errorf("resizeQuirk-enabled session: %w", withErr)
	}
	report := quirkProbeReport{
		Mode: "pinned-conpty-resize-quirk", Host: identity,
		WithoutQuirk: without, WithQuirk: with,
		WithoutRepaints: len(without.RepaintFrames), WithRepaints: len(with.RepaintFrames),
		CompletedAt: time.Now().UTC(),
	}
	if err := writeJSON(reportPath, report); err != nil {
		return err
	}
	artifactDir := reportPath + ".sessions"
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return err
	}
	if err := writeAndVerifyRawArtifact(filepath.Join(artifactDir, "without-quirk-80x25.raw"), without.RawOutput, without.RawSHA256); err != nil {
		return err
	}
	if err := writeAndVerifyRawArtifact(filepath.Join(artifactDir, "with-quirk-80x25.raw"), with.RawOutput, with.RawSHA256); err != nil {
		return err
	}
	fmt.Printf("native resizeQuirk probe complete: %s without_repaints=%d with_repaints=%d\n", reportPath, report.WithoutRepaints, report.WithRepaints)
	return nil
}
