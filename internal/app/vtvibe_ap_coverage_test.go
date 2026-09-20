package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/vtvibe"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestAIPatchTargetDirSkipsNonLocalPanels(t *testing.T) {
	pf := &panel.PanelsFrame{}
	pf.Panels[0] = &panel.FileSystemPanel{Vfs: vfs.NewNullVFS(0)}
	if root, ok := aiPatchTargetDir(pf); ok || root != "" {
		t.Fatalf("non-local panel target = %q, %v; want empty, false", root, ok)
	}
}

func TestAIPythonPathReportsMissingInterpreter(t *testing.T) {
	setupPortableIni(t, "0")
	t.Setenv("PATH", t.TempDir())
	writeVtvibeINI(t, "[general]\n")

	if got, err := aiPythonPath(); err == nil || got != "" {
		t.Fatalf("aiPythonPath() = %q, %v; want no interpreter error", got, err)
	}
}

func TestAIShowPatchResultStatesAndAttachReport(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	for _, tc := range []struct {
		name   string
		dry    bool
		exit   int
		output string
	}{
		{name: "dry success", dry: true},
		{name: "success", exit: 0, output: "applied"},
		{name: "partial", exit: 2, output: "partly applied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			aiShowPatchResult(nil, t.TempDir(), tc.dry, tc.exit, tc.output)
			top := vtui.FrameManager.GetTopFrame()
			if top == nil {
				t.Fatal("patch result did not open a message")
			}
			top.Close()
			vtui.FrameManager.RemoveFrame(top)
		})
	}

	reportDir := t.TempDir()
	reportPath := filepath.Join(reportDir, "afailed.md")
	if err := os.WriteFile(reportPath, []byte("report"), 0600); err != nil {
		t.Fatal(err)
	}
	aiShowPatchResult(nil, reportDir, false, 1, "failed output")
	top := vtui.FrameManager.GetTopFrame()
	dlg, ok := top.(*vtui.Window)
	if !ok {
		t.Fatalf("patch failure frame = %T, want message window", top)
	}
	// Failure output is button 1; the attached report is button 2.
	dlg.OnResult(2)
	top.Close()
	vtui.FrameManager.RemoveFrame(top)

	attached, err := vtvibe.NewVFS(aiSession()).Open(context.Background(), "/ctx/afailed.md")
	if err == nil {
		_ = attached.Close()
	} else {
		t.Fatalf("attached failure report: %v", err)
	}
}
