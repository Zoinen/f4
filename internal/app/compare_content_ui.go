package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/internal/diffview"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/textdiff"
	"github.com/unxed/vtui"
)

// "Compare files by content" (f4#613), Total Commander's "Compare by
// content" for f4: a read-only, side-by-side, line-level diff of the file
// under the cursor in each panel. See internal/diffview for the view itself
// and internal/textdiff for the comparison algorithm; this file only wires
// panel selection to them.
//
// This first version deliberately covers the narrowest useful shape: one
// regular file per panel, both on a local OS filesystem, small enough to
// read into memory whole. Remote/archive VFS, directories, and syntax
// highlighting inside the diff are natural follow-ups, not this step.

// compareContentMaxFileSize caps how large a file this version reads into
// memory for a comparison. 8 MiB comfortably covers source files and
// configs -- the normal target for a content diff -- without needing a
// progress dialog or streaming for a first version.
const compareContentMaxFileSize = 8 << 20

// panelCanCompareFilesByContent reports whether both panels have exactly one
// regular, local file selected -- the only shape "Compare files by content"
// supports right now. It stays out of the menu entirely otherwise, the same
// way panelCanCompareFolders does for Compare Folders.
func panelCanCompareFilesByContent() bool {
	pf := panel.FindPanelsFrameAnyScreen()
	if pf == nil {
		return false
	}
	_, _, ok1 := singleLocalFileTarget(pf.GetActivePanel())
	if !ok1 {
		return false
	}
	_, _, ok2 := singleLocalFileTarget(pf.GetInactivePanel())
	return ok2
}

// singleLocalFileTarget resolves a panel's current selection (marked item, or
// the item under the cursor when nothing is marked -- GetSelectedNames'
// usual "selection or cursor" contract) to a full path and a display title,
// but only when it names exactly one regular file on a local OS VFS.
func singleLocalFileTarget(fsp *panel.FileSystemPanel) (path, title string, ok bool) {
	if fsp == nil || fsp.Vfs == nil || !fileops.IsLocalOSVFS(fsp.Vfs) {
		return "", "", false
	}
	names := fsp.GetSelectedNames()
	if len(names) != 1 {
		return "", "", false
	}
	name := names[0]
	var entry *panel.FileEntry
	for _, e := range fsp.Entries {
		if e.Name == name {
			entry = e
			break
		}
	}
	if entry == nil || entry.IsDir {
		return "", "", false
	}
	dir := fsp.Vfs.GetPath()
	return fsp.Vfs.Join(dir, name), name, true
}

func actionCompareFilesByContent(pf *panel.PanelsFrame) {
	leftPath, leftTitle, ok := singleLocalFileTarget(pf.GetActivePanel())
	if !ok {
		return
	}
	rightPath, rightTitle, ok := singleLocalFileTarget(pf.GetInactivePanel())
	if !ok {
		return
	}

	vtui.RunAsync(func(ctx *vtui.TaskContext) {
		leftLines, leftErr := readTextFileForCompare(leftPath)
		rightLines, rightErr := readTextFileForCompare(rightPath)
		ctx.RunOnUI(func() {
			showCompareFilesByContentResult(pf, leftTitle, rightTitle, leftPath, rightPath, leftLines, rightLines, leftErr, rightErr)
		})
	})
}

func showCompareFilesByContentResult(pf *panel.PanelsFrame, leftTitle, rightTitle, leftPath, rightPath string, leftLines, rightLines []string, leftErr, rightErr error) {
	if leftErr != nil {
		vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to read %s:\n%v", filepath.Base(leftPath), leftErr), []string{"&Ok"})
		return
	}
	if rightErr != nil {
		vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to read %s:\n%v", filepath.Base(rightPath), rightErr), []string{"&Ok"})
		return
	}
	dv, err := diffview.NewDiffView(leftTitle, rightTitle, leftLines, rightLines)
	if err != nil {
		if err == textdiff.ErrTooLarge {
			vtui.ShowMessage(" Compare ", "These files are too large or too different for a content comparison in this version.", []string{"&Ok"})
			return
		}
		vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to compare files:\n%v", err), []string{"&Ok"})
		return
	}
	dv.ResizeConsole(pf.LastW, pf.LastH)
	vtui.FrameManager.AddScreen(dv)
}

// readTextFileForCompare reads a whole file for a content comparison. It
// refuses anything over compareContentMaxFileSize or containing a NUL byte:
// a line-by-line diff over binary data is meaningless, and a NUL byte is the
// same "probably binary" signal f4 already uses elsewhere.
func readTextFileForCompare(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("is a directory")
	}
	if info.Size() > compareContentMaxFileSize {
		return nil, fmt.Errorf("file is larger than %d MiB", compareContentMaxFileSize>>20)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, fmt.Errorf("file looks like a binary file")
	}
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}
