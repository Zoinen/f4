package netfox

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

import "golang.org/x/text/encoding"

type SFTPVFS struct {
	parent  vfs.VFS
	client  *sftp.Client
	ssh     *ssh.Client
	path    string
	title   string
	decoder *encoding.Decoder
	encoder *encoding.Encoder
}

func (v *SFTPVFS) encodePath(p string) string {
	if v.encoder == nil {
		return p
	}
	encoded, err := v.encoder.Bytes([]byte(p))
	if err == nil {
		return string(encoded)
	}
	return p
}

func NewSFTPVFS(parent vfs.VFS, host, port, user, pass string, timeout int, cp string) (*SFTPVFS, error) {
	vtui.DebugLog("NET: Initiating SFTP connection to %s:%s (user: %s)", host, port, user)
	sshClient, err := DialSSH(host, port, user, pass, timeout)
	if err != nil {
		return nil, err
	}

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		sshClient.Close()
		return nil, err
	}
	vtui.DebugLog("NET: SFTP session established successfully")

	pwd, err := sftpClient.Getwd()
	if err != nil || pwd == "" {
		pwd = "/"
	}

	title := host
	if user != "" {
		title = user + "@" + host
	}

	dec, enc := vfs.GetCodepageDecoderEncoder(cp)
	return &SFTPVFS{
		parent:  parent,
		client:  sftpClient,
		ssh:     sshClient,
		path:    pwd,
		title:   title,
		decoder: dec,
		encoder: enc,
	}, nil
}

func (v *SFTPVFS) GetTitle() string { return v.title }

func (v *SFTPVFS) IsAtRoot() bool      { return v.path == "/" || v.path == "" }
func (v *SFTPVFS) GetPath() string     { return v.path }
func (v *SFTPVFS) IsAbs(p string) bool { return path.IsAbs(p) }
func (v *SFTPVFS) SetPath(p string) error {
	var target string
	if path.IsAbs(p) {
		target = p
	} else {
		target = v.Join(v.path, p)
	}
	target = path.Clean(target)
	info, err := v.client.Stat(v.encodePath(target))
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return os.ErrInvalid
	}
	v.path = target
	return nil
}

func (v *SFTPVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	vtui.DebugLog("SFTP: ReadDir(%q) starting...", p)
	entries, err := v.client.ReadDir(v.encodePath(p))
	if err != nil {
		vtui.DebugLog("SFTP: ReadDir(%q) failed: %v", p, err)
		return err
	}
	var items []vfs.VFSItem
	for i, e := range entries {
		if ctx.Err() != nil {
			vtui.DebugLog("SFTP: ReadDir(%q) aborted by context cancellation after %d items", p, i)
			return ctx.Err()
		}

		isDir := e.IsDir()
		if !isDir && (e.Mode()&os.ModeSymlink != 0) {
			if target, err := v.client.Stat(v.encodePath(v.Join(p, e.Name()))); err == nil {
				isDir = target.IsDir()
			}
		}

		var unixMode uint32
		var uid, gid int
		var aTime time.Time
		if stat, ok := e.Sys().(*sftp.FileStat); ok {
			unixMode = stat.Mode
			uid = int(stat.UID)
			gid = int(stat.GID)
			aTime = time.Unix(int64(stat.Atime), 0)
		} else {
			unixMode = uint32(e.Mode().Perm())
			aTime = e.ModTime()
		}

		name := e.Name()
		if v.decoder != nil {
			if decoded, err := v.decoder.Bytes([]byte(name)); err == nil {
				name = string(decoded)
			}
		}

		items = append(items, vfs.VFSItem{
			Name: name, Size: e.Size(), IsDir: isDir,
			MTime: e.ModTime(), IsExecutable: e.Mode().Perm()&0111 != 0,
			IsHidden: strings.HasPrefix(name, "."),
			UnixMode: unixMode, Uid: uid, Gid: gid, ATime: aTime,
		})

		if len(items) >= 500 || i == len(entries)-1 {
			onChunk(items)
			items = make([]vfs.VFSItem, 0, 500)
		}
	}
	vtui.DebugLog("SFTP: ReadDir(%q) finished, total: %d", p, len(entries))
	return nil
}

func (v *SFTPVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	info, err := v.client.Stat(v.encodePath(p))
	if err != nil {
		return vfs.VFSItem{}, err
	}

	var unixMode uint32
	var uid, gid int
	var aTime time.Time

	if stat, ok := info.Sys().(*sftp.FileStat); ok {
		unixMode = stat.Mode
		uid = int(stat.UID)
		gid = int(stat.GID)
		aTime = time.Unix(int64(stat.Atime), 0)
	} else {
		unixMode = uint32(info.Mode().Perm())
		aTime = info.ModTime()
	}

	return vfs.VFSItem{
		Name: info.Name(), Size: info.Size(), IsDir: info.IsDir(),
		MTime: info.ModTime(), IsExecutable: info.Mode().Perm()&0111 != 0,
		IsHidden: strings.HasPrefix(info.Name(), "."),
		UnixMode: unixMode, Uid: uid, Gid: gid, ATime: aTime,
	}, nil
}

func (v *SFTPVFS) Join(e ...string) string { return path.Join(e...) }
func (v *SFTPVFS) Abs(p string) (string, error) {
	if path.IsAbs(p) {
		return path.Clean(p), nil
	}
	return v.Join(v.path, p), nil
}
func (v *SFTPVFS) Base(p string) string { return path.Base(p) }
func (v *SFTPVFS) Dir(p string) string  { return path.Dir(p) }
func (v *SFTPVFS) MkDir(ctx context.Context, p string) error {
	return v.client.MkdirAll(v.encodePath(p))
}
func (v *SFTPVFS) Remove(ctx context.Context, p string) error {
	info, err := v.client.Lstat(v.encodePath(p))
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return v.client.Remove(v.encodePath(p))
	}

	walker := v.client.Walk(v.encodePath(p))
	var items []string
	for walker.Step() {
		if err := walker.Err(); err != nil {
			return err
		}
		items = append(items, walker.Path())
	}

	for i := len(items) - 1; i >= 0; i-- {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		itemPath := items[i]
		st, err := v.client.Lstat(itemPath)
		if err != nil {
			continue
		}
		if st.IsDir() {
			if err := v.client.RemoveDirectory(itemPath); err != nil {
				return err
			}
		} else {
			if err := v.client.Remove(itemPath); err != nil {
				return err
			}
		}
	}
	return nil
}
func (v *SFTPVFS) Rename(ctx context.Context, o, n string) error {
	return v.client.Rename(v.encodePath(o), v.encodePath(n))
}

func (v *SFTPVFS) SetAttributes(ctx context.Context, path string, item vfs.VFSItem) error {
	encPath := v.encodePath(path)
	if item.UnixMode != 0 {
		if err := v.client.Chmod(encPath, os.FileMode(item.UnixMode)); err != nil {
			return err
		}
	}
	if item.Uid != -1 && item.Gid != -1 {
		if err := v.client.Chown(encPath, item.Uid, item.Gid); err != nil {
			return err
		}
	}
	atime := item.ATime
	mtime := item.MTime
	if atime.IsZero() {
		atime = mtime
	}
	if mtime.IsZero() {
		mtime = atime
	}
	if !atime.IsZero() && !mtime.IsZero() {
		return v.client.Chtimes(encPath, atime, mtime)
	}
	return nil
}

func (v *SFTPVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true, HasUnixPermissions: true}
}
func (v *SFTPVFS) Search(ctx context.Context, p, pat string) (chan int64, error) { return nil, nil }

func (v *SFTPVFS) Open(ctx context.Context, p string) (vfs.ReadAtCloser, error) {
	vtui.DebugLog("SFTP: Opening file %q for reading...", p)
	f, err := v.client.Open(v.encodePath(p))
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &sftpFileWrapper{File: f, size: info.Size()}, nil
}

func (v *SFTPVFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	return v.client.Create(v.encodePath(p))
}
func (v *SFTPVFS) ParentVFS() vfs.VFS { return v.parent }
func (v *SFTPVFS) Close() error {
	if v.client != nil {
		v.client.Close()
	}
	if v.ssh != nil {
		return v.ssh.Close()
	}
	return nil
}
func (v *SFTPVFS) Clone() vfs.VFS {
	// Re-auth is complex; return self reference.
	return v
}

func (v *SFTPVFS) OpenPty(cols, rows int) (any, error) {
	pty, err := NewSSHPty(v.ssh)
	if err != nil {
		return nil, err
	}
	pty.SetSize(cols, rows)
	pty.Run("")
	return pty, nil
}

type sftpProvider struct{}

func (p *sftpProvider) Name() string  { return "NetFox-SFTP" }
func (p *sftpProvider) Priority() int { return 100 }
func (p *sftpProvider) CanOpen(ctx context.Context, parent vfs.VFS, pth string) bool {
	w, ok := parent.(*netFoxVFSWrapper)
	if !ok {
		return false
	}
	item, err := w.Stat(ctx, pth)
	if err != nil || item.IsDir {
		return false
	}
	f, err := w.Open(ctx, pth)
	if err != nil {
		return false
	}
	defer f.Close()
	var cfg NetFoxConfig
	json.NewDecoder(ctxReader{f, ctx}).Decode(&cfg)
	return cfg.Type == "sftp" || cfg.Type == ""
}
func (p *sftpProvider) Open(ctx context.Context, parent vfs.VFS, pth string) (vfs.VFS, error) {
	w := parent.(*netFoxVFSWrapper)
	f, _ := w.Open(ctx, pth)
	defer f.Close()
	var cfg NetFoxConfig
	json.NewDecoder(ctxReader{f, ctx}).Decode(&cfg)
	port := cfg.Port
	if port == "" {
		port = "22"
	}
	timeout := 15
	if cfg.Timeout != "" {
		if t, err := strconv.Atoi(cfg.Timeout); err == nil && t > 0 {
			timeout = t
		}
	}
	return NewSFTPVFS(parent, cfg.Host, port, cfg.User, cfg.Pass, timeout, cfg.Codepage)
}

type sftpProtocolHandler struct{}

func (ph *sftpProtocolHandler) Prefix() string      { return "sftp" }
func (ph *sftpProtocolHandler) DefaultPort() string { return "22" }
func (ph *sftpProtocolHandler) BuildExtraUI(cfg *NetFoxConfig, x, y, w, h int) (vtui.UIElement, func()) {
	return nil, func() {}
}

func init() {
	vfs.RegisterProvider(&sftpProvider{})
	RegisterProtocol(&sftpProtocolHandler{})
}

type sftpFileWrapper struct {
	*sftp.File
	size int64
}

func (w *sftpFileWrapper) Size() int64 { return w.size }
func (w *sftpFileWrapper) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	return w.File.ReadAt(p, off)
}
func (w *sftpFileWrapper) Read(ctx context.Context, p []byte) (int, error) {
	return w.File.Read(p)
}
