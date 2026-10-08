package panel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// OpenRemoteAssociatedFile downloads a snapshot and opens it on this desktop.
// Desktop launchers may return before their player opens the path, so successful
// snapshots stay in the system temporary directory for its normal cleanup.
func OpenRemoteAssociatedFile(pf *PanelsFrame, fs vfs.VFS, path string) {
	if pf == nil || fs == nil || pf.Closed {
		return
	}
	if _, _, supported := AssociatedFileCommand(path); !supported {
		vtui.ShowMessage(" Error ", "Opening files with desktop associations is unavailable.", []string{"&Ok"})
		return
	}
	pf.RunProgressTask(" Opening remote file ", "Downloading for local playback...", false,
		func(ctx context.Context, update func(string, int)) error {
			local, cleanup, err := downloadAssociatedFile(ctx, fs, path, update)
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				cleanup()
				return err
			}
			command, args, _ := AssociatedFileCommand(local)
			vtui.DebugLog("[FIX:remote-open] downloaded snapshot; launching local desktop association")
			if err := pf.RunExternalUICommand(command, args, filepath.Dir(local)); err != nil {
				cleanup()
				return fmt.Errorf("opening downloaded file: %w", err)
			}
			return nil
		}, func(err error) {
			if err != nil {
				vtui.DebugLog("[FIX:remote-open] failed: %v", err)
				if !errors.Is(err, context.Canceled) && !pf.Closed {
					vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to open remote file locally:\n%v", err), []string{"&Ok"})
				}
			}
		})
}

func downloadAssociatedFile(ctx context.Context, fs vfs.VFS, path string, update func(string, int)) (local string, cleanup func(), err error) {
	item, err := fs.Stat(ctx, path)
	if err != nil {
		return "", nil, err
	}
	if item.IsDir {
		return "", nil, fmt.Errorf("cannot open a directory as a file")
	}
	source, err := fs.Open(ctx, path)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = source.Close() }()
	dir, err := os.MkdirTemp("", "f4-open-*")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(dir) }
	defer func() {
		if err != nil {
			cleanup()
		}
	}()
	name := filepath.Base(strings.ReplaceAll(fs.Base(path), `\`, "/"))
	if name == "" || name == "." || name == ".." || name == string(os.PathSeparator) {
		return "", cleanup, fmt.Errorf("invalid file name")
	}
	local = filepath.Join(dir, name)
	dest, err := os.OpenFile(local, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", cleanup, err
	}
	defer func() { _ = dest.Close() }()
	buffer := make([]byte, 128*1024)
	var downloaded int64
	for {
		if err = ctx.Err(); err != nil {
			return "", cleanup, err
		}
		var n int
		n, err = source.Read(ctx, buffer)
		if n > 0 {
			if _, writeErr := dest.Write(buffer[:n]); writeErr != nil {
				return "", cleanup, writeErr
			}
			downloaded += int64(n)
			percent := -1
			if item.Size > 0 {
				percent = min(100, int(float64(downloaded)/float64(item.Size)*100))
			}
			update("Downloading for local playback...", percent)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return "", cleanup, err
			}
			break
		}
		if n == 0 {
			return "", cleanup, io.ErrNoProgress
		}
	}
	if err = ctx.Err(); err != nil {
		return "", cleanup, err
	}
	if err = dest.Close(); err != nil {
		return "", cleanup, err
	}
	return local, cleanup, nil
}
