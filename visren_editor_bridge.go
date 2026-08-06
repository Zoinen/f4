package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/plugins/visren"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// OpenVisRenEditor opens a small UTF-8 temporary file in F4's built-in editor
// and returns its bytes after the editor closes.
func (pf *PanelsFrame) OpenVisRenEditor(req visren.EditorRequest) error {
	tmp, err := os.CreateTemp("", "f4-visren-*.txt")
	if err != nil {
		return err
	}
	path := tmp.Name()
	if _, err := tmp.Write(req.Content); err != nil {
		tmp.Close()
		os.Remove(path)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(path)
		return err
	}

	local := vfs.NewOSVFS(filepath.Dir(path))
	editor := NewEditorView(piecetable.New(req.Content), local, path)
	editor.DisplayTitle = req.Title
	if req.CursorLine >= 0 {
		editor.CursorLine = req.CursorLine
	}
	if req.CursorCol >= 0 {
		editor.CursorPos = req.CursorCol
	}
	editor.OnClose = func() {
		data, readErr := os.ReadFile(path)
		if removeErr := os.Remove(path); readErr == nil && removeErr != nil {
			readErr = fmt.Errorf("remove temporary list: %w", removeErr)
		}
		if req.OnClose != nil {
			vtui.FrameManager.PostTask(func() { req.OnClose(data, readErr) })
		}
	}
	editor.ResizeConsole(pf.lastW, pf.lastH)
	editor.StartIndexing()
	vtui.FrameManager.AddScreen(editor)
	return nil
}
