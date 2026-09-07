//go:build windows

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type edgeProbeReport struct {
	Mode                      string             `json:"mode"`
	Host                      pinnedHostIdentity `json:"host"`
	Session                   nativeProbeSession `json:"session"`
	SpacesEightTopElided      bool               `json:"spaces_eight_top_elided"`
	SpacesNineTopElided       bool               `json:"spaces_nine_top_elided"`
	SpacesEightBottomAdvanced bool               `json:"spaces_eight_bottom_advanced"`
	SpacesNineBottomAdvanced  bool               `json:"spaces_nine_bottom_advanced"`
	BlinkRendered             bool               `json:"blink_rendered"`
	BlinkSequenceInStream     bool               `json:"blink_sequence_in_stream"`
	BlinkInRenderedHistory    bool               `json:"blink_in_rendered_history"`
	AutoWrapHostSequences     int                `json:"auto_wrap_host_sequences"`
	AutoWrapTailLost          bool               `json:"auto_wrap_tail_lost"`
	CompletedAt               time.Time          `json:"completed_at"`
}

func runNativeEdgeProbe(hostPath, reportPath string) error {
	if reportPath == "" {
		reportPath = filepath.Join("artifacts", "pinned-conpty-edge.json")
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
	workload := edgeProbeWorkload()
	command := fmt.Sprintf(`"%s" -emit-edge`, executable)
	// Width 80 is intentional: the auto-wrap-disabled record must exceed the
	// host row. Reflow itself remains consumer-only elsewhere in the gate.
	session, runErr := runNativeProbeSessionWithWorkload(resolved, executable, 80, 25, false, workload, command, []string{edgeBeginMarker, edgeEndMarker})
	if runErr != nil {
		return fmt.Errorf("edge native session: %w", runErr)
	}
	rendered := parseRenderedHistoryAtWidth(session.RawOutput, 80).Lines()
	segment, ok := renderedMarkerSegment(rendered, edgeBeginMarker, edgeEndMarker)
	if !ok {
		return fmt.Errorf("edge markers did not delimit rendered output")
	}
	find := func(value []byte) bool {
		for _, line := range segment {
			if bytes.Equal(line.Bytes, value) {
				return true
			}
		}
		return false
	}
	renderedHas := func(value []byte) bool {
		for _, line := range segment {
			if bytes.Contains(line.Bytes, value) {
				return true
			}
		}
		return false
	}
	report := edgeProbeReport{
		Mode:                      "pinned-conpty-edge",
		Host:                      identity,
		Session:                   session,
		SpacesEightTopElided:      find([]byte("spaces-eight-top:")) && !bytes.Contains(session.RawOutput, []byte("spaces-eight-top:        \r\n")),
		SpacesNineTopElided:       find([]byte("spaces-nine-top:")) && !bytes.Contains(session.RawOutput, []byte("spaces-nine-top:         \r\n")),
		SpacesEightBottomAdvanced: bytes.Contains(session.RawOutput, []byte("spaces-eight-bottom:\x1b[60C")),
		SpacesNineBottomAdvanced:  bytes.Contains(session.RawOutput, []byte("spaces-nine-bottom:\x1b[61C")),
		BlinkRendered:             find([]byte("blink: visible")),
		BlinkSequenceInStream:     bytes.Contains(session.RawOutput, []byte("\x1b[?12")),
		BlinkInRenderedHistory:    renderedHas([]byte("\x1b[?12")),
		AutoWrapHostSequences:     bytes.Count(session.RawOutput, []byte("\x1b[?7")),
		CompletedAt:               time.Now().UTC(),
	}
	// The child explicitly disables DECAWM, so the final portion of the 257-W
	// record is expected not to reach the host stream/history. This is an
	// observed child-controlled limitation, not a history reconstruction rule.
	report.AutoWrapTailLost = !find([]byte("nowrap: " + string(bytes.Repeat([]byte{'W'}, 257))))
	if err := writeJSON(reportPath, report); err != nil {
		return err
	}
	artifactDir := reportPath + ".sessions"
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return err
	}
	if err := writeAndVerifyRawArtifact(filepath.Join(artifactDir, "80x25.raw"), session.RawOutput, session.RawSHA256); err != nil {
		return err
	}
	if !report.SpacesEightTopElided || !report.SpacesNineTopElided || !report.SpacesEightBottomAdvanced || !report.SpacesNineBottomAdvanced || !report.BlinkRendered || report.BlinkInRenderedHistory || report.AutoWrapHostSequences != 0 || !report.AutoWrapTailLost {
		return fmt.Errorf("edge probe assertions failed: %+v", report)
	}
	fmt.Printf("native edge probe complete: %s spaces=4 blink=true nowrap_tail_lost=true\n", reportPath)
	return nil
}
