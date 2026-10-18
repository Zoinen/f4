package androidfs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"time"

	"github.com/unxed/f4/plugins/netfox"
	"github.com/unxed/f4/vfs"
)

const (
	remoteModeFile = 0100000
	remoteModePerm = 07777

	// newUploadMode is what `adb push` gives a file that did not exist.
	newUploadMode = 0644
)

// syncUploadVFS is a FISH+ device view whose file transfers travel over ADB Sync.
// The FISH+ helper has to push every byte through the device shell, and the
// portable base64 write path makes that shell read the payload one byte at a
// time: about 200 KB/s on a real phone (unxed/f4#1761), where SEND, the path
// `adb push` uses, runs at wire speed. Reading is the same story the other
// way (1.6 MB/s against 33 MB/s of `adb pull`), so Open uses RECV. Listing,
// mutations and commands stay on the FISH+ session; only Create and Open are
// redirected.
type syncUploadVFS struct {
	*netfox.FishVFS
	client syncFS
}

var _ vfs.VFS = (*syncUploadVFS)(nil)

// withSyncUploads wraps a FISH+ view so that Create uses client. Anything that
// is not a FISH+ view, or has no client, is returned as it is.
func withSyncUploads(mounted vfs.VFS, client syncFS) vfs.VFS {
	fish, ok := mounted.(*netfox.FishVFS)
	if !ok || client == nil {
		return mounted
	}
	return &syncUploadVFS{FishVFS: fish, client: client}
}

// Create streams the file with Sync SEND, and falls back to the FISH+ writer
// whenever the shell is the one that should decide (see openSyncUpload).
func (v *syncUploadVFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	if abs, err := v.Abs(p); err == nil {
		if w := openSyncUpload(ctx, v.client, abs); w != nil {
			return w, nil
		}
	}
	return v.FishVFS.Create(ctx, p)
}

// Open streams a regular file with Sync RECV, and falls back to the FISH+
// reader for everything else (see openSyncRead).
func (v *syncUploadVFS) Open(ctx context.Context, p string) (vfs.ReadAtCloser, error) {
	// A configured hybrid lane keeps thumbnail prefix probes bounded and
	// materializes random-access reads only once. Do not bypass that lane
	// when adding the upstream Sync upload/sequential-download wrapper.
	if v.FishVFS.GetCapabilities().ReadAccess == vfs.ReadAccessHybridRange {
		return v.FishVFS.Open(ctx, p)
	}
	if abs, err := v.Abs(p); err == nil {
		if r := openSyncRead(ctx, v.client, abs); r != nil {
			return r, nil
		}
	}
	return v.FishVFS.Open(ctx, p)
}

// Clone keeps the redirect: the embedded Clone would hand back a bare FISH+
// view and silently bring the slow path back for every second panel.
func (v *syncUploadVFS) Clone() vfs.VFS {
	return &syncUploadVFS{FishVFS: v.FishVFS.CloneForParent(v.ParentVFS()), client: v.client}
}

// openSyncUpload starts a Sync SEND of target, or returns nil when the upload
// must go through the FISH+ shell instead. SEND replaces the file, so it is
// used only for a path that is free or already a regular file (whose
// permission bits it keeps): a symlink, which the shell would follow, a
// directory and anything odd are left to the helper, as are an unsafe path and
// a Sync service that does not answer.
func openSyncUpload(ctx context.Context, client syncFS, target string) io.WriteCloser {
	target, err := mutationPath(target)
	if err != nil {
		return nil
	}
	mode := uint32(newUploadMode)
	entry, err := client.Lstat(ctx, target)
	switch {
	case err == nil && entry.Mode&remoteModeType == remoteModeFile:
		mode = entry.Mode & remoteModePerm
	case err == nil, !errors.Is(err, fs.ErrNotExist):
		return nil
	}
	w, err := client.Send(ctx, target, mode, time.Now())
	if err != nil {
		return nil
	}
	return w
}

// openSyncRead returns a Sync RECV reader for target, or nil when the FISH+
// shell must read it: a symlink (the shell follows it, Lstat does not), a
// directory or anything odd, an unsafe path, and a Sync service that does not
// answer. The stream starts on the first read, so Open itself is one Lstat.
func openSyncRead(ctx context.Context, client syncFS, target string) vfs.ReadAtCloser {
	target, err := mutationPath(target)
	if err != nil {
		return nil
	}
	entry, err := client.Lstat(ctx, target)
	if err != nil || entry.Mode&remoteModeType != remoteModeFile {
		return nil
	}
	return &syncReadFile{client: client, path: target, size: syncEntrySize(entry.Size)}
}
