package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/f4/vtvibe"
	"github.com/unxed/vtui"
)

// Applying an ap patch that the model wrote (github.com/unxed/ap).
//
// f4 does not implement the format: it shells out to the reference patcher,
// ap.py, downloaded once from the ap repository and cached in the config
// directory. That keeps the two projects in step while ap is still growing:
// a format that f4 half-implements would be worse than no button at all.
// A Go port lives in vtvibe.md as a future idea, with its trade-offs.

const (
	vtvibeAPScriptURL = "https://raw.githubusercontent.com/unxed/ap/main/implementation/ap.py"
	vtvibeAPSpecURL   = "https://raw.githubusercontent.com/unxed/ap/main/ap.md"
	// vtvibeAPMaxDownload caps both downloads: they are a script and a
	// specification, neither is anywhere near a megabyte.
	vtvibeAPMaxDownload = 4 << 20
)

// aiPatcherPath is where the cached copy of ap.py lives.
func aiPatcherPath() string {
	return filepath.Join(GetF4ConfigDir(), "vtvibe", "ap.py")
}

// aiPatchTargetDir picks the folder the patch applies to: the other panel,
// because the AI panel itself holds the dialog, not the project. Paths inside
// an ap patch are relative to that folder.
func aiPatchTargetDir(pf *PanelsFrame) (string, bool) {
	if pf == nil {
		return "", false
	}
	for _, p := range pf.panels {
		fsp, ok := p.(*FileSystemPanel)
		if !ok || fsp == nil || fsp.vfs == nil {
			continue
		}
		if _, isAI := fsp.vfs.(*aiVFSWrapper); isAI {
			continue
		}
		if _, isOS := fsp.vfs.(*vfs.OSVFS); isOS {
			return fsp.vfs.GetPath(), true
		}
	}
	return "", false
}

// aiApplyPatch is the whole feature from the human side: confirm what will be
// touched, then run the patcher.
func aiApplyPatch(pf *PanelsFrame) {
	if pf == nil {
		return
	}
	patch := aiSession().LastPatch()
	if patch == nil {
		vtui.ShowMessage(Msg("AI.Title"), Msg("AI.NoPatch"), []string{Msg("vtui.Ok")})
		return
	}
	root, ok := aiPatchTargetDir(pf)
	if !ok {
		vtui.ShowMessage(Msg("AI.ErrorTitle"), Msg("AI.PatchNoLocalDir"), []string{Msg("vtui.Ok")})
		return
	}

	body := fmt.Sprintf(Msg("AI.PatchConfirm"), root, len(patch.Files))
	shown := patch.Files
	if len(shown) > 12 {
		shown = shown[:12]
	}
	for _, f := range shown {
		body += "\n  " + f
	}
	if len(patch.Files) > len(shown) {
		body += "\n  " + fmt.Sprintf(Msg("AI.PatchMoreFiles"), len(patch.Files)-len(shown))
	}
	if patch.Ignored > 0 {
		body += "\n\n" + fmt.Sprintf(Msg("AI.PatchIgnored"), patch.Ignored)
	}

	dlg := vtui.ShowMessage(Msg("AI.PatchTitle"), body,
		[]string{Msg("AI.BtnApplyPatch"), Msg("AI.BtnDryRun"), Msg("vtui.Cancel")})
	dlg.OnResult = func(code int) {
		switch code {
		case 0:
			aiRunPatcher(pf, patch, root, false)
		case 1:
			aiRunPatcher(pf, patch, root, true)
		}
	}
}

// aiRunPatcher writes the patch to a temporary file and hands it to ap.py.
// The patch never touches the target folder itself: --dir is what decides
// where the changes land.
func aiRunPatcher(pf *PanelsFrame, patch *vtvibe.Patch, root string, dry bool) {
	var output string
	exitCode := 0

	title := Msg("AI.PatchTitle")
	pf.RunProgressTask(title, Msg("AI.PatchRunning"), false,
		func(ctx context.Context, update func(msg string, percent int)) error {
			script, err := aiEnsurePatcher(ctx, update)
			if err != nil {
				return err
			}
			python, err := aiPythonPath()
			if err != nil {
				return err
			}

			dir, err := os.MkdirTemp("", "vtvibe-ap-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(dir)

			patchPath := filepath.Join(dir, "afix.ap")
			if err := os.WriteFile(patchPath, []byte(patch.Text), 0600); err != nil {
				return err
			}

			update(Msg("AI.PatchRunning"), -1)
			args := []string{script, patchPath, "--dir", root}
			if dry {
				args = append(args, "--dry-run")
			}
			cmd := exec.CommandContext(ctx, python, args...)
			cmd.Dir = root
			out, runErr := cmd.CombinedOutput()
			output = string(out)
			if ee, ok := runErr.(*exec.ExitError); ok {
				exitCode = ee.ExitCode()
				return nil
			}
			return runErr
		},
		func(err error) {
			if err != nil {
				if err != context.Canceled {
					aiShowError(err)
				}
				return
			}
			pf.RefreshAll()
			aiShowPatchResult(pf, root, dry, exitCode, output)
		})
}

// aiShowPatchResult reports what the patcher said. Exit codes come from ap.py:
// 0 applied, 2 applied in part, anything else nothing was written.
func aiShowPatchResult(pf *PanelsFrame, root string, dry bool, exitCode int, output string) {
	var head string
	switch {
	case exitCode == 0 && dry:
		head = Msg("AI.PatchDryOk")
	case exitCode == 0:
		head = Msg("AI.PatchOk")
	case exitCode == 2:
		head = Msg("AI.PatchPartial")
	default:
		head = Msg("AI.PatchFailed")
	}

	output = strings.TrimSpace(output)
	preview := output
	if len(preview) > 500 {
		preview = preview[:500] + "\n..."
	}

	body := head
	if preview != "" {
		body += "\n\n" + preview
	}

	buttons := []string{Msg("vtui.Ok")}
	hasOutput := output != ""
	if hasOutput {
		buttons = append(buttons, Msg("AI.BtnViewLog"))
	}

	reportPath := filepath.Join(root, "afailed.md")
	hasReport := false
	if exitCode != 0 {
		if st, err := os.Stat(reportPath); err == nil && !st.IsDir() {
			buttons = append(buttons, Msg("AI.BtnAttachReport"))
			hasReport = true
		}
	}

	dlg := vtui.ShowMessage(Msg("AI.PatchTitle"), body, buttons)
	dlg.OnResult = func(code int) {
		var viewLogIdx = -1
		var attachReportIdx = -1

		currIdx := 1
		if hasOutput {
			viewLogIdx = currIdx
			currIdx++
		}
		if hasReport {
			attachReportIdx = currIdx
		}

		switch code {
		case viewLogIdx:
			dir, err := os.MkdirTemp("", "vtvibe-ap-log-")
			if err == nil {
				logPath := filepath.Join(dir, "ap_output.log")
				if os.WriteFile(logPath, []byte(output), 0600) == nil {
					tempVfs := vfs.NewOSVFS(dir)
					actionOpenViewer(pf, tempVfs, "ap_output.log")
				}
			}
		case attachReportIdx:
			aiAttachFailureReport(reportPath)
		}
	}
}

// aiAttachFailureReport puts afailed.md into the dialog context. ap.py writes
// that file precisely so a model can be told what went wrong without the
// human retyping it.
func aiAttachFailureReport(reportPath string) {
	data, err := os.ReadFile(reportPath)
	if err != nil {
		aiShowError(err)
		return
	}
	if err := aiWriteContextFile("afailed.md", data); err != nil {
		aiShowError(err)
		return
	}
	showToast(Msg("AI.PatchReportAttached"), 3*time.Second)
	if pf := findPanelsFrameAnyScreen(); pf != nil {
		pf.RefreshAll()
	}
}

// aiWriteContextFile drops a file into ai://ctx through the normal VFS, so it
// obeys the same size limits as a file copied there with F5.
func aiWriteContextFile(name string, data []byte) error {
	v := vtvibe.NewVFS(aiSession())
	w, err := v.Create(context.Background(), "/ctx/"+name)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

// aiPythonPath finds an interpreter for ap.py.
func aiPythonPath() (string, error) {
	candidates := []string{"python3", "python"}
	if runtime.GOOS == "windows" {
		candidates = []string{"python", "python3", "py"}
	}
	if ini := LoadIni(vtvibeIniPath()); ini != nil {
		if custom := strings.TrimSpace(ini.GetString("general", "python", "")); custom != "" {
			candidates = append([]string{custom}, candidates...)
		}
	}
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s", Msg("AI.NoPython"))
}

// aiEnsurePatcher returns a path to ap.py, downloading it once if needed.
// vtvibe.ini may point at a local copy with ap_patcher, or at another build
// of the script with ap_url.
func aiEnsurePatcher(ctx context.Context, update func(msg string, percent int)) (string, error) {
	ini := LoadIni(vtvibeIniPath())
	url := vtvibeAPScriptURL
	if ini != nil {
		if custom := strings.TrimSpace(ini.GetString("general", "ap_patcher", "")); custom != "" {
			if _, err := os.Stat(custom); err != nil {
				return "", err
			}
			return custom, nil
		}
		url = ini.GetString("general", "ap_url", vtvibeAPScriptURL)
	}

	path := aiPatcherPath()
	if st, err := os.Stat(path); err == nil && st.Size() > 0 {
		return path, nil
	}

	update(Msg("AI.PatchDownloading"), -1)
	data, err := aiDownload(ctx, url)
	if err != nil {
		return "", err
	}
	// A proxy login page is also a 200 with a body. Refuse anything that is
	// not recognizably the patcher rather than feeding it to an interpreter.
	if !strings.Contains(string(data), "def apply_patch(") {
		return "", fmt.Errorf("%s", Msg("AI.PatchBadDownload"))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", err
	}
	return path, nil
}

// aiAttachAPSpec teaches the model the format: the specification goes into the
// context and the dialog switches to asking for patches.
func aiAttachAPSpec(pf *PanelsFrame) {
	if pf == nil {
		return
	}
	url := vtvibeAPSpecURL
	if ini := LoadIni(vtvibeIniPath()); ini != nil {
		url = ini.GetString("general", "ap_spec_url", vtvibeAPSpecURL)
	}
	var spec []byte
	pf.RunProgressTask(Msg("AI.PatchTitle"), Msg("AI.SpecDownloading"), false,
		func(ctx context.Context, update func(msg string, percent int)) error {
			data, err := aiDownload(ctx, url)
			if err != nil {
				return err
			}
			spec = data
			return nil
		},
		func(err error) {
			if err != nil {
				if err != context.Canceled {
					aiShowError(err)
				}
				return
			}
			if err := aiWriteContextFile("ap.md", spec); err != nil {
				aiShowError(err)
				return
			}
			aiSession().SetPatchMode(true)
			pf.RefreshAll()
			vtui.ShowMessage(Msg("AI.Title"), Msg("AI.SpecAttached"), []string{Msg("vtui.Ok")})
		})
}

// aiDownload is one small HTTP GET with the context of the progress task, so
// Cancel really cancels it.
func aiDownload(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "f4-vtvibe")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, vtvibeAPMaxDownload))
}
