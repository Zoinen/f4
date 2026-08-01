package netfox

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

type ioCtxReader struct {
	r   io.Reader
	ctx context.Context
}

func (cr *ioCtxReader) Read(p []byte) (int, error) {
	if cr.ctx.Err() != nil {
		return 0, cr.ctx.Err()
	}
	return cr.r.Read(p)
}

type NetFoxPlugin struct{}

type netFoxVFSWrapper struct {
	*NetFoxVFS
}

func (w *netFoxVFSWrapper) Clone() vfs.VFS {
	return &netFoxVFSWrapper{w.NetFoxVFS.Clone().(*NetFoxVFS)}
}

// ctxReader wraps vfs.ReadAtCloser to implement standard io.Reader
type ctxReader struct {
	r   vfs.ReadAtCloser
	ctx context.Context
}

func (cr ctxReader) Read(p []byte) (int, error) {
	return cr.r.Read(cr.ctx, p)
}

func (w *netFoxVFSWrapper) ProcessPanelKey(app vfs.App, e *vtinput.InputEvent) bool {
	if !e.KeyDown {
		return false
	}

	shift := (e.ControlKeyState & vtinput.ShiftPressed) != 0
	ctrl := (e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
	alt := (e.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0
	noMods := !shift && !ctrl && !alt

	// F4 -> Edit existing connection
	if e.VirtualKeyCode == vtinput.VK_F4 && noMods {
		name := app.GetSelectedName()
		if name != "" && name != ".." && name != "<Add connection>" {
			showConnectionDialog(app, w.NetFoxVFS, name)
		}
		return true
	}

	// Shift+F4 -> Add new connection
	if e.VirtualKeyCode == vtinput.VK_F4 && shift && !ctrl && !alt {
		showConnectionDialog(app, w.NetFoxVFS, "")
		return true
	}

	// Enter on <Add connection>
	if e.VirtualKeyCode == vtinput.VK_RETURN && noMods {
		if app.GetSelectedName() == "<Add connection>" {
			showConnectionDialog(app, w.NetFoxVFS, "")
			return true
		}
	}

	// Protect <Add connection> from deletion via F8
	if e.VirtualKeyCode == vtinput.VK_F8 && noMods {
		names := app.GetSelectedNames()
		if len(names) == 1 && names[0] == "<Add connection>" {
			return true // Swallow to prevent core from deleting it
		}
	}

	return false
}

func (p *NetFoxPlugin) Init(api vfs.HostAPI) error {
	api.RegisterDrive("NetFox", func() vfs.VFS {
		cfgDir := vfs.CustomConfigDir
		if cfgDir == "" {
			sysDir, _ := os.UserConfigDir()
			cfgDir = filepath.Join(sysDir, "f4")
		}
		return &netFoxVFSWrapper{NewNetFoxVFS(filepath.Join(cfgDir, "NetFox.json"))}
	})
	return nil
}

func (p *NetFoxPlugin) Close() error    { return nil }
func (p *NetFoxPlugin) GetName() string { return "NetFox" }
