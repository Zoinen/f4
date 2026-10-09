//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type passthroughProbeReport struct {
	Mode          string             `json:"mode"`
	Host          pinnedHostIdentity `json:"host"`
	Results       map[string]bool    `json:"results"`
	RawSHA256     string             `json:"raw_sha256"`
	ChildExited   bool               `json:"child_exited"`
	HostExited    bool               `json:"host_exited"`
	HandlesClosed bool               `json:"handles_closed"`
	CompletedAt   time.Time          `json:"completed_at"`
}

// runNativePassthroughProbe sends the sequences of passthroughSequences through
// the pinned host and records which of them arrive unchanged. It measures, so a
// sequence that does not pass is a result, not a failure; only a broken session
// fails the probe.
func runNativePassthroughProbe(hostPath, reportPath string) error {
	if reportPath == "" {
		reportPath = filepath.Join("artifacts", "pinned-conpty-passthrough.json")
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
	begin := "__PINNED_CONPTY_PROBE_SEMANTIC_BEGIN__"
	end := "__PINNED_CONPTY_PROBE_SEMANTIC_END__"
	workload := []byte(semanticProbeWorkload("passthrough", begin, end))
	command := fmt.Sprintf(`"%s" -emit-semantic -emit-probe-width 512 -emit-semantic-kind passthrough`, executable)
	session, runErr := runNativeProbeSessionWithWorkload(resolved, executable, 512, 25, false, workload, command, []string{begin, end})
	report := passthroughProbeReport{
		Mode: "pinned-conpty-passthrough", Host: identity, Results: passthroughResults(session.RawOutput),
		RawSHA256: session.RawSHA256, ChildExited: session.ChildExited, HostExited: session.HostExited,
		HandlesClosed: session.HandlesClosed, CompletedAt: time.Now().UTC(),
	}
	artifactDir := reportPath + ".sessions"
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return err
	}
	if err := writeAndVerifyRawArtifact(filepath.Join(artifactDir, "512x25.raw"), session.RawOutput, session.RawSHA256); err != nil {
		return err
	}
	if err := writeJSON(reportPath, report); err != nil {
		return err
	}
	if runErr != nil {
		return runErr
	}
	if !report.ChildExited || !report.HostExited || !report.HandlesClosed {
		return fmt.Errorf("passthrough probe session broken: child=%t host=%t handles=%t", report.ChildExited, report.HostExited, report.HandlesClosed)
	}
	for _, seq := range passthroughSequences {
		fmt.Printf("passthrough %-22s %t\n", seq.Name, report.Results[seq.Name])
	}
	fmt.Printf("native passthrough probe complete: %s\n", reportPath)
	return nil
}
