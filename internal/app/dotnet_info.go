package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/unxed/f4/internal/dotnet"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// vfsReaderAt lets a vfs file serve as the io.ReaderAt the metadata reader
// wants; nothing is copied to disk and nothing of the file is executed.
type vfsReaderAt struct {
	ctx  context.Context
	file vfs.ReadAtCloser
}

func (r vfsReaderAt) ReadAt(p []byte, off int64) (int, error) {
	return r.file.ReadAt(r.ctx, p, off)
}

// dotnetReport reads the .NET metadata of the file at path on v and renders
// it as Markdown; dotnet.ErrNotAssembly says the file holds none.
func dotnetReport(ctx context.Context, v vfs.VFS, path, name string) (string, error) {
	file, err := v.Open(ctx, path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	info, err := dotnet.Read(vfsReaderAt{ctx: ctx, file: file}, file.Size())
	if err != nil {
		return "", err
	}
	return dotnet.Report(info, name), nil
}

// actionAssemblyInfo shows the assembly identity, references, types and
// resources of the .NET file under the cursor (f4#1666) in the Markdown
// viewer. The file is only read, never loaded.
func actionAssemblyInfo(pf *panel.PanelsFrame) {
	fsp := pf.GetActivePanel()
	if fsp == nil {
		return
	}
	idx := fsp.GetCursorIndex()
	if idx < 0 || idx >= len(fsp.Entries) || fsp.Entries[idx].IsDir {
		return
	}
	name := fsp.GetSelectedName()
	path := fsp.Vfs.Join(fsp.Vfs.GetPath(), name)
	text, err := dotnetReport(context.Background(), fsp.Vfs, path, name)
	switch {
	case errors.Is(err, dotnet.ErrNotAssembly):
		toast.Show(fmt.Sprintf(i18n.Msg("DotNet.NotAssembly"), name), 3*time.Second)
	case err != nil:
		toast.Show(fmt.Sprintf(i18n.Msg("DotNet.ReadFailed"), name, err), 3*time.Second)
	default:
		vfs.ShowPanelHelp(name, text)
		vtui.FrameManager.Redraw()
	}
}
