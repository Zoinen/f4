package archive

import (
	"bytes"
	"context"
	"os"
	"strings"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filemask"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/zipper/archive"
)

type ArchiveProvider struct{}

func (p *ArchiveProvider) Name() string  { return "zipper/archive" }
func (p *ArchiveProvider) Priority() int { return 10 }

// PanelEnterAllowed keeps two kinds of entry out of the ordinary Enter and
// double-click path, though their content really is an archive: a document
// the user names in ArchiveEnterExcludeMask, and a self-extracting installer.
// Both stay available through Ctrl+PgDn, which is the deliberate
// archive-entry gesture.
//
// far2l settles the first of those with a list of its own, KnownDocumentTypes
// (multiarc/src/MultiArc.cpp), which it refuses to enter "even while its
// really archive"; Far Manager settles it with arclite's include_masks, which
// is a setting rather than a constant (arclite/options.cpp). f4 takes far2l's
// list and Far's idea of where to keep it: the default names what far2l
// names, and a user who wants Enter to browse their .whl, or to launch their
// .cbz instead of browsing it, writes one line of f4.ini rather than waiting
// for someone to agree with them. A name nobody listed is left to its
// content, which is how both ancestors treat everything not on their list --
// so Enter browses a .jar, as it does in Far and in far2l.
//
// Off the local file system the mask does not apply: there is no association
// and no system opener there for Enter to be held back in favour of.
func (p *ArchiveProvider) PanelEnterAllowed(ctx context.Context, parent vfs.VFS, path string) bool {
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	if _, isLocal := parent.(*vfs.OSVFS); !isLocal {
		// Not the local file system: inside another archive or on a remote
		// one there is no association and no system opener to hand Enter
		// to, so the content stays the only thing to go on.
		return true
	}
	name := path
	if base := parent.Base(path); base != "" {
		name = base
	}
	if enterBarredByMask(name) {
		return false
	}
	embedded, found, err := probeArchive(ctx, parent, path)
	return err != nil || !found || embedded.offset <= 0
}

// enterBarredByMask reports whether the configured mask claims this name for
// its association. An empty mask bars nothing, which is the setting a user
// writes when they want Enter to follow the content and only the content.
//
// The match ignores case, as far2l's own test does: it compares extensions
// with strcasecmp, so a .DOCX is a document there too.
func enterBarredByMask(name string) bool {
	mask := strings.TrimSpace(config.App.ArchiveEnterExcludeMask)
	if mask == "" {
		return false
	}
	return filemask.Match(name, mask, true)
}

func (p *ArchiveProvider) CanOpen(ctx context.Context, parent vfs.VFS, path string) bool {
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	if osvfs, ok := parent.(*vfs.OSVFS); ok {
		localPath, _ := osvfs.Abs(path)
		if fi, err := os.Stat(localPath); err == nil {
			if fi.Mode()&(os.ModeNamedPipe|os.ModeSocket|os.ModeDevice|os.ModeCharDevice) != 0 {
				return false
			}
		}
	}
	name := path
	if parent != nil {
		if base := parent.Base(path); base != "" {
			name = base
		}
	}
	format := archive.DetectFormat(name)
	if format != "" {
		return true
	}
	_, found, err := probeArchive(ctx, parent, path)
	return err == nil && found
}

// headProbeLimit is how far into a file the probe reads when the bytes are
// not on the local disk. It is far2l's PluginMaxReadData, the size of the
// window its plugin manager maps over a file before asking the format modules
// whether they recognize it (far2l/src/plug/plugins.cpp), and it is the same
// bet: an archive announces itself at the start of the file, and what sits in
// front of one is a stub.
const headProbeLimit = 256 << 10

// probeArchive finds the archive in a file by reading its first bytes, and it
// is the only thing in this package that decides what a file is. far2l
// decides the same way: its format modules are handed a window over the file
// and are never told its name, so a .jar is a zip there and opens as one.
//
// One question, one scanner, two budgets. On the local disk the bytes are
// free -- the file is right there and nothing has to be decoded to reach byte
// n -- so the scan runs to sfxProbeLimit, far enough to find the archive
// behind an installer stub of any size anyone ships. Everywhere else each
// byte is decompressed or fetched while the panel waits on the answer, so the
// window is far2l's, and only a file system that offered to produce it
// cheaply and silently is asked at all. The rest report nothing and are left
// to be judged by their names, which is what they were before this existed.
func probeArchive(ctx context.Context, parent vfs.VFS, path string) (embeddedArchive, bool, error) {
	if parent == nil {
		return embeddedArchive{}, false, nil
	}
	if osvfs, ok := parent.(*vfs.OSVFS); ok {
		localPath, err := osvfs.Abs(path)
		if err != nil {
			return embeddedArchive{}, false, err
		}
		return findEmbeddedArchive(localPath)
	}
	head, err := vfs.ReadFileHead(ctx, parent, path, headProbeLimit)
	if err != nil || len(head) == 0 {
		return embeddedArchive{}, false, err
	}
	// One buffer read twice: sequentially for the signature scan, and by
	// offset for the signatures that verify themselves. Both address the
	// same bytes from the start of the file, which is what the scan needs.
	window := bytes.NewReader(head)
	return scanEmbeddedArchive(window, window, int64(len(head)))
}

func (p *ArchiveProvider) Open(ctx context.Context, parent vfs.VFS, path string) (vfs.VFS, error) {
	v, err := NewArchiveVFSContext(ctx, parent, path)
	if err != nil {
		// Return an untyped nil so the interface itself is nil: a typed
		// (*ArchiveVFS)(nil) would still compare != nil to callers, and a
		// later Close() on it panics (nil dereference in v.mu.Lock()).
		return nil, err
	}
	return v, nil
}
