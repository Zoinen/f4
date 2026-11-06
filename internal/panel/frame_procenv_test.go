package panel

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/vfs"
)

func TestPOSIXEnvironmentHandoffFitsInteractiveInput(t *testing.T) {
	payload, err := preparePOSIXProcessEnvironmentShellPayload([]vfs.ProcessEnvironmentChange{
		{Name: "F4_HANDOFF_TEST", Value: "fixture"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(payload.cleanup)
	if len(payload.wire) > 512 {
		t.Fatalf("environment handoff sends %d bytes through interactive input", len(payload.wire))
	}
}

func TestEnvironmentAcknowledgementKeepsEchoHidden(t *testing.T) {
	previous := processEnvironmentRunOnUI
	processEnvironmentRunOnUI = func(task func()) { task() }
	t.Cleanup(func() { processEnvironmentRunOnUI = previous })
	for _, name := range []string{"single chunk", "split acknowledgement"} {
		t.Run(name, func(t *testing.T) {
			pf := newPanelsFrame(false)
			t.Cleanup(pf.closeProcessEnvironmentShell)
			pty := &processEnvironmentPTY{}
			pf.Pty = pty
			pf.ShowPanels = false
			pf.Parser = terminal.NewAnsiParser(pf.TermView, pty)
			pf.processEnvironmentGeneration = terminal.GlobalProcessEnvironment.CurrentGeneration()
			pf.processEnvironmentInFlight = &processEnvironmentShellInFlight{token: "test-token", muted: true}
			pf.TermView.SetMuted(true)
			ack := "\x1b]133;E;test-token;0\a"
			chunks := []string{"PRIVATE WRAPPER ECHO\r\n" + ack + "user prompt> "}
			if name == "split acknowledgement" {
				chunks = []string{"PRIVATE WRAPPER ECHO\r\n" + ack[:13], ack[13:] + "user prompt> "}
			}
			for _, chunk := range chunks {
				pf.consumeLocalOutput(pty, []byte(chunk))
			}
			var text strings.Builder
			for _, row := range pf.TermView.Lines {
				text.WriteString(terminal.CellsText(row))
			}
			if strings.Contains(text.String(), "PRIVATE WRAPPER") {
				t.Fatal("environment wrapper echo reached the visible terminal")
			}
			if !strings.Contains(text.String(), "user prompt>") {
				t.Fatal("visible prompt after the acknowledgement was lost")
			}
		})
	}
}
