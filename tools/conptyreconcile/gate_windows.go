//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type seedGateReport struct {
	Mode        string               `json:"mode"`
	Host        pinnedHostIdentity   `json:"host"`
	SeedCount   int                  `json:"seed_count"`
	Failures    []string             `json:"failures,omitempty"`
	Sessions    []nativeProbeSession `json:"sessions"`
	CompletedAt time.Time            `json:"completed_at"`
}

// runNativeSingleSeed is the reproducible D3 diagnostic path.  It uses the
// same fresh pinned host session and artifact verification as the 300-seed
// gate, but never re-downloads a verified package.
func runNativeSingleSeed(hostPath, reportPath string, seed uint64) error {
	if seed == 0 {
		return fmt.Errorf("seed must be non-zero")
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
	widths := []int{1, 79, 80, 81, 121}
	width := widths[(seed-1)%uint64(len(widths))]
	hostWidth := 512
	workload := seedWorkload(seed, width)
	begin := fmt.Sprintf("__PINNED_CONPTY_PROBE_SEED_%016x_BEGIN__", seed)
	end := fmt.Sprintf("__PINNED_CONPTY_PROBE_SEED_%016x_END__", seed)
	command := fmt.Sprintf(`"%s" -emit-seed %016x -emit-probe-width %d`, executable, seed, width)
	// B1-0 keeps the host size fixed. The old host-resize interleaving remains
	// available only through the diagnostic -probe path, not this gate.
	session, runErr := runNativeProbeSessionWithWorkload(resolved, executable, hostWidth, 25, false, workload, command, []string{begin, end})
	report := seedGateReport{Mode: "pinned-conpty-seed", Host: identity, SeedCount: 1, Sessions: []nativeProbeSession{session}, CompletedAt: time.Now().UTC()}
	if reportPath == "" {
		reportPath = filepath.Join("artifacts", fmt.Sprintf("pinned-conpty-seed-%d.json", seed))
	}
	artifactDir := reportPath + ".sessions"
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return err
	}
	if session.RawSHA256 != "" {
		if err := writeAndVerifyRawArtifact(filepath.Join(artifactDir, fmt.Sprintf("%dx25.raw", hostWidth)), session.RawOutput, session.RawSHA256); err != nil {
			return err
		}
	}
	if runErr != nil {
		report.Failures = []string{fmt.Sprintf("seed %d width %d: %v", seed, width, runErr)}
	}
	if failures := assertionFailures(session.Assertions); len(failures) != 0 {
		report.Failures = append(report.Failures, fmt.Sprintf("seed %d width %d assertions: %s", seed, width, strings.Join(failures, "; ")))
	}
	consumerChecks, consumerErr := verifySeedConsumerChecks(session, begin, end)
	session.ConsumerChecks = consumerChecks
	report.Sessions[0] = session
	if consumerErr != nil {
		report.Failures = append(report.Failures, fmt.Sprintf("seed %d consumer checks: %v", seed, consumerErr))
	}
	if err := writeJSON(reportPath, report); err != nil {
		return err
	}
	if len(report.Failures) != 0 {
		return fmt.Errorf("native seed failed: %s", strings.Join(report.Failures, "; "))
	}
	fmt.Printf("native seed complete: seed=%d width=%d report=%s\n", seed, width, reportPath)
	return nil
}

func runNativeSeedGate(hostPath, reportPath string) error {
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
	report := seedGateReport{Mode: "pinned-conpty-seeds", Host: identity}
	widths := []int{1, 79, 80, 81, 121}
	for i := 0; i < 300; i++ {
		seed := uint64(i + 1)
		width := widths[i%len(widths)]
		hostWidth := 512
		workload := seedWorkload(seed, width)
		begin := fmt.Sprintf("__PINNED_CONPTY_PROBE_SEED_%016x_BEGIN__", seed)
		end := fmt.Sprintf("__PINNED_CONPTY_PROBE_SEED_%016x_END__", seed)
		command := fmt.Sprintf(`"%s" -emit-seed %016x -emit-probe-width %d`, executable, seed, width)
		// The gate changes display width in its consumer, never the ConPTY host.
		// The resizeDuringOutput=true alternative is retained in -probe solely
		// for historical diagnostics and is inactive for this requirement.
		session, runErr := runNativeProbeSessionWithWorkload(resolved, executable, hostWidth, 25, false, workload, command, []string{begin, end})
		report.Sessions = append(report.Sessions, session)
		fmt.Printf("native seed gate: %d/300 (%d%%) width=%d\n", i+1, (i+1)*100/300, width)
		artifactDir := reportPath + ".sessions"
		if err := os.MkdirAll(artifactDir, 0o755); err != nil {
			return err
		}
		artifact := filepath.Join(artifactDir, fmt.Sprintf("%03d-%dx25.raw", i+1, hostWidth))
		if err := writeAndVerifyRawArtifact(artifact, session.RawOutput, session.RawSHA256); err != nil {
			return err
		}
		if runErr != nil {
			report.Failures = append(report.Failures, fmt.Sprintf("seed %d width %d: %v", seed, width, runErr))
			fmt.Printf("native seed gate: seed %d recorded failure: %v\n", seed, runErr)
		}
		if failures := assertionFailures(session.Assertions); len(failures) != 0 {
			report.Failures = append(report.Failures, fmt.Sprintf("seed %d width %d assertions: %s", seed, width, strings.Join(failures, "; ")))
		}
		consumerChecks, consumerErr := verifySeedConsumerChecks(session, begin, end)
		session.ConsumerChecks = consumerChecks
		report.Sessions[len(report.Sessions)-1] = session
		if consumerErr != nil {
			report.Failures = append(report.Failures, fmt.Sprintf("seed %d consumer checks: %v", seed, consumerErr))
		}
	}
	report.SeedCount = len(report.Sessions)
	report.CompletedAt = time.Now().UTC()
	if err := writeJSON(reportPath, report); err != nil {
		return err
	}
	if len(report.Failures) != 0 {
		return fmt.Errorf("300-seed native stage recorded %d failures", len(report.Failures))
	}
	return nil
}
