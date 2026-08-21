package gitplugin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

// PanelKeybarLabels supplies the history-specific controls while retaining
// ordinary F3 viewing and F5 copying. A copied row is served through Open, so
// any session overlay automatically wins without a special copy path.
func (*LogVFS) PanelKeybarLabels() [12]string {
	return [12]string{
		1: "Mode",
		3: "Overlay",
	}
}

// ProcessPanelKey makes F2 mode-sensitive: it switches the graph at the
// history root and switches changed-files/full-snapshot below a commit. F4
// opens a session-only historical overlay editor; F5 deliberately falls back
// to f4's ordinary copy operation, whose Open call already sees the overlay.
func (view *LogVFS) ProcessPanelKey(app vfs.App, event *vtinput.InputEvent) bool {
	if event == nil || event.Type != vtinput.KeyEventType || !event.KeyDown || event.ControlKeyState != 0 {
		return false
	}
	switch event.VirtualKeyCode {
	case vtinput.VK_F2:
		view.ToggleModeForPath(view.GetPath())
		refreshLogVFS(app, view)
		return true
	case vtinput.VK_F4:
		view.editSelectedOverlay(app)
		return true
	default:
		return false
	}
}

func (view *LogVFS) HandlePanelAction(app vfs.App, action vfs.PanelAction, paths []string) bool {
	switch action {
	case vfs.PanelActionEdit:
		if len(paths) == 0 {
			view.editSelectedOverlay(app)
			return true
		}
		view.editOverlay(app, paths[0])
		return true
	case vfs.PanelActionCreate, vfs.PanelActionDelete:
		app.Message(" Git log ", "Git history is read-only; F4 creates a session-only overlay for a historical file.", []string{"&Ok"})
		return true
	default:
		return false
	}
}

func (view *LogVFS) editSelectedOverlay(app vfs.App) {
	names := app.GetSelectedNames()
	if len(names) == 0 {
		app.Message(" Git log ", "Select a historical text file first.", []string{"&Ok"})
		return
	}
	view.editOverlay(app, view.Join(view.GetPath(), names[0]))
}

func (view *LogVFS) editOverlay(app vfs.App, virtualPath string) {
	editor, supported := app.(vfs.TextEditorHost)
	if !supported {
		app.Message(" Git log ", "This host cannot open a historical overlay editor.", []string{"&Ok"})
		return
	}
	var result struct {
		patch string
		err   error
	}
	app.RunProgressTask(" Git history ", "Loading historical file…", false, func(ctx context.Context, update func(string, int)) error {
		update("Reading historical after-blob…", -1)
		reader, err := view.Open(ctx, virtualPath)
		if err != nil {
			return err
		}
		defer reader.Close()
		data, err := readLogVFSContent(ctx, reader)
		if err != nil {
			return err
		}
		if bytes.IndexByte(data, 0) >= 0 {
			return errors.New("binary historical files are read-only")
		}
		result.patch = string(data)
		update("Historical file ready", 100)
		return nil
	}, func(err error) {
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				app.Message(" Git history ", fmt.Sprintf("Cannot open historical file:\n%v", err), []string{"&Ok"})
			}
			return
		}
		if openErr := editor.OpenTextEditor(vfs.TextEditorRequest{
			Temporary:    true,
			DisplayTitle: "Git overlay — " + strings.TrimPrefix(virtualPath, "/"),
			Content:      []byte(result.patch),
			OnSave: func(ctx context.Context, content []byte) error {
				if bytes.IndexByte(content, 0) >= 0 {
					return errors.New("binary historical overlays are not supported")
				}
				if err := view.SetOverlay(ctx, virtualPath, content); err != nil {
					return err
				}
				refreshLogVFSFromWorker(app, view)
				return nil
			},
		}); openErr != nil {
			app.Message(" Git history ", fmt.Sprintf("Cannot open overlay editor:\n%v", openErr), []string{"&Ok"})
		}
	})
}

func readLogVFSContent(ctx context.Context, reader vfs.ReadAtCloser) ([]byte, error) {
	if reader == nil {
		return nil, osErrInvalidLogReader
	}
	var output bytes.Buffer
	buffer := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		count, err := reader.Read(ctx, buffer)
		if count != 0 {
			output.Write(buffer[:count])
		}
		if err == io.EOF {
			return output.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
	}
}

var osErrInvalidLogReader = errors.New("Git history: invalid file reader")

func refreshLogVFS(app vfs.App, view *LogVFS) {
	if host, ok := app.(vfs.PanelHost); ok && host != nil {
		host.RefreshVFS(view)
		return
	}
	app.RefreshAll()
}

func refreshLogVFSFromWorker(app vfs.App, view *LogVFS) {
	if host, ok := app.(vfs.PanelHost); ok && host != nil {
		host.RefreshVFS(view)
	}
}

var _ vfs.PanelKeybarLabels = (*LogVFS)(nil)
var _ vfs.PanelActionHandler = (*LogVFS)(nil)
