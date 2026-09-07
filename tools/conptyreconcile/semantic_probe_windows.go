//go:build windows

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type semanticProbeReport struct {
	Mode          string             `json:"mode"`
	Host          pinnedHostIdentity `json:"host"`
	Kind          string             `json:"kind"`
	Expected      string             `json:"expected"`
	Observed      string             `json:"observed"`
	Exact         bool               `json:"exact"`
	RawSHA256     string             `json:"raw_sha256"`
	ChildExited   bool               `json:"child_exited"`
	HostExited    bool               `json:"host_exited"`
	HandlesClosed bool               `json:"handles_closed"`
	CompletedAt   time.Time          `json:"completed_at"`
}

func runNativeSemanticProbe(hostPath, reportPath, kind string) error {
	if reportPath == "" {
		reportPath = filepath.Join("artifacts", "pinned-conpty-"+kind+".json")
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
	expected := "link"
	if kind == "tabs" {
		expected = "tabs:   X       Y"
	} else if kind == "progress" {
		expected = "progress: 100%"
	} else if kind == "unicode" {
		expected = "unicode: 漢字 e\u0301 ☕️ 😀 👩‍💻 אבג العربية"
	}
	workload := []byte(semanticProbeWorkload(kind, begin, end))
	command := fmt.Sprintf(`"%s" -emit-semantic -emit-probe-width 512 -emit-semantic-kind %s`, executable, kind)
	session, runErr := runNativeProbeSessionWithWorkload(resolved, executable, 512, 25, false, workload, command, []string{begin, end})
	lines := parseRenderedHistoryAtWidth(session.RawOutput, 512).Lines()
	observed := ""
	if segment, ok := renderedMarkerSegment(lines, begin, end); ok {
		for _, line := range segment {
			trimmed := bytes.TrimRight(line.Bytes, " ")
			if bytes.HasPrefix(trimmed, []byte("tabs:")) || bytes.Equal(trimmed, []byte("link")) || bytes.HasPrefix(trimmed, []byte("progress:")) || bytes.HasPrefix(trimmed, []byte("unicode:")) {
				observed = string(trimmed)
				break
			}
		}
	}
	report := semanticProbeReport{Mode: "pinned-conpty-semantic", Host: identity, Kind: kind, Expected: expected, Observed: observed, Exact: observed == expected, RawSHA256: session.RawSHA256, ChildExited: session.ChildExited, HostExited: session.HostExited, HandlesClosed: session.HandlesClosed, CompletedAt: time.Now().UTC()}
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
	if !report.Exact || !report.ChildExited || !report.HostExited || !report.HandlesClosed {
		return fmt.Errorf("semantic probe failed kind=%s exact=%t child=%t host=%t handles=%t", kind, report.Exact, report.ChildExited, report.HostExited, report.HandlesClosed)
	}
	fmt.Printf("native semantic probe complete: %s kind=%s exact=%t\n", reportPath, kind, report.Exact)
	return nil
}
