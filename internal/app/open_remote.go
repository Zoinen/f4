package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// openedCopies are the temporary directories holding files from archives and
// other non-local file systems that Enter handed to their associated program
// (f4#1999). That program usually returns at once (xdg-open, open), so f4
// cannot tell when the file is closed; the copies are removed when f4 exits.
var (
	openedCopiesMu sync.Mutex
	openedCopies   []string
)

func rememberOpenedCopy(dir string) {
	openedCopiesMu.Lock()
	openedCopies = append(openedCopies, dir)
	openedCopiesMu.Unlock()
}

// removeOpenedCopies deletes what openRemoteWithAssociation left behind.
func removeOpenedCopies() {
	openedCopiesMu.Lock()
	dirs := openedCopies
	openedCopies = nil
	openedCopiesMu.Unlock()
	for _, dir := range dirs {
		_ = os.RemoveAll(dir) // Best effort at exit: a file still open elsewhere may stay.
	}
}

// copyVFSFileToLocal copies an opened file of a VFS to the local path dst,
// reporting progress against size when it is known. It closes src.
func copyVFSFileToLocal(ctx context.Context, src vfs.ReadAtCloser, dst string, size int64, update func(msg string, percent int)) error {
	defer func() { _ = src.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	closeOut := fileops.CloseOnce(out)
	defer func() { _ = closeOut() }()

	buf := make([]byte, 128*1024)
	var copied int64
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, err := src.Read(ctx, buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
			copied += int64(n)
			pct := 0
			if size > 0 {
				pct = int((copied * 100) / size)
			}
			update("Downloading...", pct)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	return closeOut()
}

// openRemoteWithAssociation is Enter on a file that is not on a local disk,
// such as a file inside an archive (f4#1999): the file is copied to a
// temporary directory under its own name and opened there with the associated
// program, as Enter does for a local file. Changes made to the copy do not go
// back; F4 is the way to edit such a file.
func openRemoteWithAssociation(pf *panel.PanelsFrame, v vfs.VFS, path string) {
	name := filepath.Base(v.Base(path))
	if name == "" || name == "." || name == ".." || name == string(filepath.Separator) {
		name = "file"
	}
	dir, err := os.MkdirTemp("", "f4-open-*")
	if err != nil {
		vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to open file:\n%v", err), []string{"&Ok"})
		return
	}
	local := filepath.Join(dir, name)

	pf.RunProgressTask(" Downloading... ", "Preparing to download...", false, func(ctx context.Context, update func(msg string, percent int)) error {
		var size int64
		if item, err := v.Stat(ctx, path); err == nil {
			size = item.Size
		}
		src, err := v.Open(ctx, path)
		if err != nil {
			return err
		}
		return copyVFSFileToLocal(ctx, src, local, size, update)
	}, func(err error) {
		if err != nil {
			_ = os.RemoveAll(dir)
			if err != context.Canceled {
				vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to download file:\n%v", err), []string{"&Ok"})
			}
			return
		}
		command, args, ok := panel.AssociatedFileCommand(local)
		if !ok {
			_ = os.RemoveAll(dir)
			return
		}
		rememberOpenedCopy(dir)
		vtui.RunAsync(func(ctx *vtui.TaskContext) {
			vtui.DebugLog("ACTIONS: Opening a copy with its associated program: %s %q", command, args)
			if err := pf.RunExternalUICommand(command, args, dir); err != nil {
				vtui.DebugLog("ACTIONS: External command failed: %v", err)
				ctx.RunOnUI(func() {
					vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to open file:\n%v", err), []string{"&Ok"})
				})
			}
		})
	})
}
