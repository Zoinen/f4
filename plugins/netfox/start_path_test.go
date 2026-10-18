package netfox

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// startPathProbe is a connection that knows one folder, /srv/data.
type startPathProbe struct {
	*vfs.NullVFS
	path string
}

func (p *startPathProbe) GetPath() string { return p.path }

func (p *startPathProbe) SetPath(s string) error {
	if s != "/srv/data" {
		return errors.New("no such folder")
	}
	p.path = s
	return nil
}

// nz256, f4#1894: a saved connection opens in its folder, as a Far connection
// with a path does.
func TestEnterStartPathOpensTheSavedFolder(t *testing.T) {
	var toasts []string
	old := startPathToast
	startPathToast = func(m string) { toasts = append(toasts, m) }
	t.Cleanup(func() { startPathToast = old })
	for _, tc := range []struct {
		name, saved, want string
		toast             bool
	}{
		{"saved folder", "/srv/data", "/srv/data", false},
		{"spaces around it", "  /srv/data ", "/srv/data", false},
		{"no folder saved", "", "/home/u", false},
		{"a folder that is not there", "/srv/gone", "/home/u", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			toasts = nil
			v := &startPathProbe{NullVFS: vfs.NewNullVFS(0), path: "/home/u"}
			enterStartPath(v, NetFoxConfig{Host: "h", Path: tc.saved})
			if v.path != tc.want {
				t.Errorf("the connection is in %q, want %q", v.path, tc.want)
			}
			if got := len(toasts) == 1 && strings.Contains(toasts[0], "/srv/gone") && strings.Contains(toasts[0], "/home/u"); got != tc.toast {
				t.Errorf("toasts %q, want one naming /srv/gone and /home/u: %v", toasts, tc.toast)
			}
		})
	}
}

func TestNetFoxConfigKeepsThePath(t *testing.T) {
	manager := NewNetFoxVFS(filepath.Join(t.TempDir(), "NetFox.json"))
	if err := manager.SaveConfig("site", NetFoxConfig{Type: "sftp", Host: "h", Path: "/srv/data"}); err != nil {
		t.Fatal(err)
	}
	cfg, ok := netFoxConfigAt(t.Context(), &netFoxVFSWrapper{NetFoxVFS: manager}, "site")
	if !ok || cfg.Path != "/srv/data" {
		t.Fatalf("the saved connection reads back as %+v (%v), want Path /srv/data", cfg, ok)
	}
}

// The folder is set in the connection dialog too, and saved without the
// spaces around it.
func TestConnectionDialogEditsTheStartFolder(t *testing.T) {
	vtui.SetDefaultPalette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	nf := NewNetFoxVFS(filepath.Join(t.TempDir(), "NetFox.json"))
	if err := nf.SaveConfig("site", NetFoxConfig{Type: "sftp", Host: "h", Path: "/srv/data"}); err != nil {
		t.Fatal(err)
	}
	showConnectionDialog(&mockApp{}, nf, "site")
	dlg, ok := vtui.FrameManager.GetTopFrame().(interface{ GetChildren() []vtui.UIElement })
	if !ok {
		t.Fatalf("the top frame %T is not the dialog", vtui.FrameManager.GetTopFrame())
	}
	var folder *vtui.Edit
	var save *vtui.Button
	for _, c := range dlg.GetChildren() {
		switch c := c.(type) {
		case *vtui.Edit:
			if c.GetText() == "/srv/data" {
				folder = c
			}
		case *vtui.Button:
			if c.IsDefault {
				save = c
			}
		}
	}
	if folder == nil || save == nil {
		t.Fatalf("the dialog does not show the saved folder (edit %v, Save %v)", folder != nil, save != nil)
	}
	folder.SetText("  /var/log ")
	save.OnClick()
	cfg, ok := netFoxConfigAt(t.Context(), &netFoxVFSWrapper{NetFoxVFS: nf}, "site")
	if !ok || cfg.Path != "/var/log" {
		t.Fatalf("saved %+v (%v), want Path /var/log", cfg, ok)
	}
}
