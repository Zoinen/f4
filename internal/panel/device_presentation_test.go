package panel

import (
	"runtime"
	"testing"

	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type devicePresentationVFS struct {
	*vfs.NullVFS
	path, title string
	root        bool
}

func (v *devicePresentationVFS) GetPath() string  { return v.path }
func (v *devicePresentationVFS) GetTitle() string { return v.title }
func (v *devicePresentationVFS) IsAtRoot() bool   { return v.root }

func TestDevicePublicPathPresentation(t *testing.T) {
	for _, tt := range []struct {
		path, tab string
		root      bool
	}{
		{"ios://Alexander’s iPhone/DCIM", "DCIM", false},
		{"ios://Alexander's iPhone/", "Alexander's iPhone", true},
		{"android://Pixel 3/sdcard", "sdcard", false},
		{"ANDROID://Pixel 3%2FJohn's/", "Pixel 3/John's", true},
	} {
		t.Run(tt.path, func(t *testing.T) {
			filesystem := &devicePresentationVFS{
				NullVFS: vfs.NewNullVFS(0), path: tt.path,
				title: "private-device-id:media", root: tt.root,
			}
			fsp := &FileSystemPanel{Vfs: filesystem}
			pf := &PanelsFrame{Panels: [2]Panel{fsp, nil}, ShowPanels: true, LastW: 400}
			suffix := "$ "
			if runtime.GOOS == "windows" {
				suffix = ">"
			}
			if got := terminal.CellsText(pf.BuildPrompt()); got != tt.path+suffix {
				t.Errorf("prompt = %q, want %q", got, tt.path+suffix)
			}
			if got := pf.GetTitle(); got != "Panels: "+tt.path {
				t.Errorf("title = %q, want public path once", got)
			}
			if got := pf.GetWorkspaceTabTitle(); got != tt.tab+" ─ —" {
				t.Errorf("tab = %q, want %q", got, tt.tab+" ─ —")
			}
			if got := filesystem.GetTitle(); got != "private-device-id:media" {
				t.Errorf("VFS identity changed: %q", got)
			}
			// A pending mount still displays its complete public address once.
			fsp.ProviderOpenTask = &vtui.TaskContext{}
			fsp.ProviderOpenTarget = tt.path
			if got := terminal.CellsText(pf.BuildPrompt()); got != tt.path+suffix {
				t.Errorf("pending prompt = %q, want %q", got, tt.path+suffix)
			}
			if got := pf.GetTitle(); got != "Panels: "+tt.path {
				t.Errorf("pending title = %q, want public path once", got)
			}
		})
	}
}
