package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

var _ vfs.TextEditorHost = (*PanelsFrame)(nil)

const textEditorTargetCheckTimeout = 2 * time.Second

type textEditorStatResult struct {
	item vfs.VFSItem
	err  error
}

// statTextEditorTarget keeps a plugin-provided or remote VFS implementation
// from holding the UI goroutine indefinitely. The result channel is buffered
// so a slow implementation can finish after the caller has timed out.
func statTextEditorTarget(filesystem vfs.VFS, path string, timeout time.Duration) (vfs.VFSItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	result := make(chan textEditorStatResult, 1)
	go func() {
		item, err := filesystem.Stat(ctx, path)
		result <- textEditorStatResult{item: item, err: err}
	}()
	select {
	case value := <-result:
		return value.item, value.err
	case <-ctx.Done():
		return vfs.VFSItem{}, ctx.Err()
	}
}

// OpenTextEditor opens supplied UTF-8 content in f4's non-modal editor. It is
// an optional plugin UI capability rather than part of the baseline App
// interface, so existing hosts remain source compatible.
func (pf *PanelsFrame) OpenTextEditor(request vfs.TextEditorRequest) error {
	if pf == nil {
		return errors.New("text editor host is unavailable")
	}
	content := append([]byte(nil), request.Content...)
	filesystem := request.VFS
	path := request.Path
	temporaryPath := ""

	if request.Temporary {
		tmp, err := os.CreateTemp("", "f4-plugin-editor-*.txt")
		if err != nil {
			return err
		}
		temporaryPath = tmp.Name()
		if _, err := tmp.Write(content); err != nil {
			tmp.Close()
			os.Remove(temporaryPath)
			return err
		}
		if err := tmp.Close(); err != nil {
			os.Remove(temporaryPath)
			return err
		}
		filesystem = vfs.NewOSVFS(filepath.Dir(temporaryPath))
		path = temporaryPath
	} else {
		if filesystem == nil {
			return errors.New("text editor request has no VFS")
		}
		if path == "" {
			return errors.New("text editor request has no path")
		}
		if !request.TargetKnownAbsent {
			if _, err := statTextEditorTarget(filesystem, path, textEditorTargetCheckTimeout); err == nil {
				return fmt.Errorf("text editor target already exists: %s", path)
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("check text editor target: %w", err)
			}
		}
	}

	// Generated plugin reports do not need a remote .editorconfig lookup. The
	// create-new buffer must remain dirty until it is actually saved.
	editor := newEditorView(piecetable.New(content), filesystem, path, false)
	editor.DisplayTitle = request.DisplayTitle
	editor.modified = request.Modified
	editor.unsavedBaseline = request.Modified
	editor.createNewTarget = !request.Temporary
	if request.CursorLine >= 0 {
		editor.CursorLine = request.CursorLine
	}
	if request.CursorCol >= 0 {
		editor.CursorPos = request.CursorCol
	}
	editor.OnClose = func() {
		data, readErr := editor.pt.GetRange(0, editor.pt.Size())
		data = append([]byte(nil), data...)
		if temporaryPath != "" {
			if removeErr := os.Remove(temporaryPath); readErr == nil && removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				readErr = fmt.Errorf("remove temporary editor file: %w", removeErr)
			}
		}
		if request.OnClose != nil {
			vtui.FrameManager.PostTask(func() { request.OnClose(data, readErr) })
		}
	}
	editor.ResizeConsole(pf.lastW, pf.lastH)
	editor.StartIndexing()
	vtui.FrameManager.AddScreen(editor)
	return nil
}
