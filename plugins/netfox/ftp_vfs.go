package netfox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/unxed/f4/internal/netproxy"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

import "golang.org/x/text/encoding"

type FTPVFS struct {
	mu      sync.Mutex
	parent  vfs.VFS
	conn    *ftp.ServerConn
	cwd     string
	title   string
	decoder *encoding.Decoder
	encoder *encoding.Encoder
}

type timeoutConn struct {
	net.Conn
	timeout time.Duration
}

func (c *timeoutConn) Read(b []byte) (int, error) {
	c.Conn.SetReadDeadline(time.Now().Add(c.timeout))
	return c.Conn.Read(b)
}

func (c *timeoutConn) Write(b []byte) (int, error) {
	c.Conn.SetWriteDeadline(time.Now().Add(c.timeout))
	return c.Conn.Write(b)
}

func (v *FTPVFS) encodePath(p string) string {
	if v.encoder == nil {
		return p
	}
	encoded, err := v.encoder.Bytes([]byte(p))
	if err == nil {
		return string(encoded)
	}
	return p
}

func NewFTPVFS(parent vfs.VFS, host, port, user, pass string, timeout int, options map[string]string, cp string, px netproxy.Settings) (*FTPVFS, error) {
	addr := host + ":" + port

	timeoutDur := time.Duration(timeout) * time.Second
	if timeoutDur <= 0 {
		timeoutDur = 15 * time.Second
	}

	dialOpts := []ftp.DialOption{
		ftp.DialWithTimeout(timeoutDur),
		ftp.DialWithDialFunc(func(network, address string) (net.Conn, error) {
			// Both the control connection and every passive data
			// connection come through here, so a proxied site stays
			// proxied for its transfers too.
			ctx, cancel := context.WithTimeout(context.Background(), timeoutDur)
			defer cancel()
			conn, err := px.DialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			return &timeoutConn{Conn: conn, timeout: timeoutDur}, nil
		}),
	}
	if val, ok := options["Passive"]; ok && val == "false" {
		dialOpts = append(dialOpts, ftp.DialWithDisabledEPSV(true))
	}

	c, err := ftp.Dial(addr, dialOpts...)
	if err != nil {
		return nil, err
	}

	err = c.Login(user, pass)
	if err != nil {
		c.Quit()
		return nil, err
	}
	vtui.DebugLog("NET: FTP logged in successfully")

	pwd, err := c.CurrentDir()
	if err != nil {
		pwd = "/"
	}

	title := host
	if user != "" && user != "anonymous" {
		title = user + "@" + host
	}

	dec, enc := vfs.GetCodepageDecoderEncoder(cp)
	return &FTPVFS{
		parent:  parent,
		conn:    c,
		cwd:     pwd,
		title:   title,
		decoder: dec,
		encoder: enc,
	}, nil
}

func (v *FTPVFS) GetTitle() string { return v.title }
func (v *FTPVFS) SessionKey() any  { return v.conn }

func (v *FTPVFS) IsAtRoot() bool      { return v.cwd == "/" || v.cwd == "" || v.cwd == "." }
func (v *FTPVFS) GetPath() string     { return v.cwd }
func (v *FTPVFS) IsAbs(p string) bool { return path.IsAbs(p) }
func (v *FTPVFS) SetPath(p string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	target := p
	if !path.IsAbs(p) {
		target = path.Join(v.cwd, p)
	}
	if err := v.conn.ChangeDir(v.encodePath(target)); err != nil {
		return err
	}
	v.cwd = target
	return nil
}

func (v *FTPVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	target := p
	if target == "/" || target == "." {
		target = ""
	}
	vtui.DebugLog("FTP: ReadDir(%q) starting...", target)
	entries, err := v.conn.List(v.encodePath(target))
	if err != nil {
		vtui.DebugLog("FTP: ReadDir(%q) failed: %v", target, err)
		return err
	}
	var items []vfs.VFSItem
	for i, e := range entries {
		if ctx.Err() != nil {
			vtui.DebugLog("FTP: ReadDir(%q) aborted by context cancellation after %d items", target, i)
			return ctx.Err()
		}
		if e.Name == "." || e.Name == ".." {
			continue
		}

		name := e.Name
		if v.decoder != nil {
			if decoded, err := v.decoder.Bytes([]byte(name)); err == nil {
				name = string(decoded)
			}
		}

		items = append(items, vfs.VFSItem{
			Name: name, Size: int64(e.Size),
			IsDir: e.Type == ftp.EntryTypeFolder, MTime: e.Time,
			IsHidden: strings.HasPrefix(name, "."),
		})
		if len(items) >= 500 || i == len(entries)-1 {
			onChunk(items)
			items = make([]vfs.VFSItem, 0, 500)
		}
	}
	vtui.DebugLog("FTP: ReadDir(%q) finished, total: %d", target, len(entries))
	return nil
}

func (v *FTPVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	dir, base := path.Dir(p), path.Base(p)
	entries, err := v.conn.List(v.encodePath(dir))
	if err != nil {
		return vfs.VFSItem{}, err
	}
	for _, e := range entries {
		if e.Name == base {
			return vfs.VFSItem{
				Name: e.Name, Size: int64(e.Size),
				IsDir: e.Type == ftp.EntryTypeFolder, MTime: e.Time,
				IsHidden: strings.HasPrefix(e.Name, "."),
			}, nil
		}
	}
	return vfs.VFSItem{}, os.ErrNotExist
}

func (v *FTPVFS) Join(e ...string) string { return path.Join(e...) }
func (v *FTPVFS) Abs(p string) (string, error) {
	if path.IsAbs(p) {
		return path.Clean(p), nil
	}
	return path.Join(v.cwd, p), nil
}
func (v *FTPVFS) Base(p string) string                      { return path.Base(p) }
func (v *FTPVFS) Dir(p string) string                       { return path.Dir(p) }
func (v *FTPVFS) MkDir(ctx context.Context, p string) error { return v.conn.MakeDir(v.encodePath(p)) }
func (v *FTPVFS) Remove(ctx context.Context, p string) error {
	return v.removeRecursive(ctx, p)
}

func (v *FTPVFS) removeRecursive(ctx context.Context, p string) error {
	enc := v.encodePath(p)
	err := v.conn.Delete(enc)
	if err == nil {
		return nil
	}

	entries, err := v.conn.List(enc)
	if err != nil {
		return v.conn.RemoveDir(enc)
	}

	for _, e := range entries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e.Name == "." || e.Name == ".." {
			continue
		}
		full := path.Join(p, e.Name)
		if e.Type == ftp.EntryTypeFolder {
			if err := v.removeRecursive(ctx, full); err != nil {
				return err
			}
		} else {
			if err := v.conn.Delete(v.encodePath(full)); err != nil {
				return err
			}
		}
	}

	return v.conn.RemoveDir(enc)
}
func (v *FTPVFS) Rename(ctx context.Context, o, n string) error {
	return v.conn.Rename(v.encodePath(o), v.encodePath(n))
}

func (v *FTPVFS) SetAttributes(ctx context.Context, path string, item vfs.VFSItem) error {
	return fmt.Errorf("SetAttributes not supported for FTP")
}

func (v *FTPVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasUnixPermissions: true}
}
func (v *FTPVFS) Search(ctx context.Context, p, pat string) (chan int64, error) {
	return nil, nil
}

func (v *FTPVFS) Open(ctx context.Context, p string) (vfs.ReadAtCloser, error) {
	vtui.DebugLog("FTP: Opening file %q for reading...", p)
	resp, err := v.conn.Retr(v.encodePath(p))
	if err != nil {
		return nil, err
	}
	tmp, _ := os.CreateTemp("", "f4ftp-*")
	io.Copy(tmp, &ioCtxReader{r: resp, ctx: ctx})
	resp.Close()
	tmp.Seek(0, 0)
	stat, _ := tmp.Stat()
	return &ftpFileWrapper{File: tmp, size: stat.Size(), path: tmp.Name()}, nil
}

func (v *FTPVFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	pr, pw := io.Pipe()
	go func() {
		err := v.conn.Stor(v.encodePath(p), pr)
		pr.CloseWithError(err)
	}()
	return pw, nil
}

func (v *FTPVFS) ParentVFS() vfs.VFS { return v.parent }
func (v *FTPVFS) Close() error       { return v.conn.Quit() }
func (v *FTPVFS) Clone() vfs.VFS {
	return v
}

type ftpProvider struct{}

func (p *ftpProvider) Name() string  { return "NetFox-FTP" }
func (p *ftpProvider) Priority() int { return 100 }
func (p *ftpProvider) CanOpen(ctx context.Context, parent vfs.VFS, pth string) bool {
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
	return cfg.Type == "ftp"
}
func (p *ftpProvider) Open(ctx context.Context, parent vfs.VFS, pth string) (vfs.VFS, error) {
	w := parent.(*netFoxVFSWrapper)
	f, _ := w.Open(ctx, pth)
	defer f.Close()
	var cfg NetFoxConfig
	json.NewDecoder(ctxReader{f, ctx}).Decode(&cfg)
	port := cfg.Port
	if port == "" {
		port = "21"
	}
	timeout := 15
	if cfg.Timeout != "" {
		if t, err := strconv.Atoi(cfg.Timeout); err == nil && t > 0 {
			timeout = t
		}
	}
	return NewFTPVFS(parent, cfg.Host, port, cfg.User, cfg.Pass, timeout, cfg.Options, cfg.Codepage, cfg.Proxy())
}

type ftpProtocolHandler struct{}

func (ph *ftpProtocolHandler) Prefix() string      { return "ftp" }
func (ph *ftpProtocolHandler) DefaultPort() string { return "21" }
func (ph *ftpProtocolHandler) BuildExtraUI(cfg *NetFoxConfig, x, y, w, h int) (vtui.UIElement, func()) {
	passive := true
	if val, ok := cfg.Options["Passive"]; ok {
		passive = (val == "true")
	}

	chk := vtui.NewCheckbox(x, y, vtui.Msg("NetFox.PassiveMode"), false)
	if passive {
		chk.State = 1
	} else {
		chk.State = 0
	}

	save := func() {
		if cfg.Options == nil {
			cfg.Options = make(map[string]string)
		}
		if chk.State == 1 {
			cfg.Options["Passive"] = "true"
		} else {
			cfg.Options["Passive"] = "false"
		}
	}
	return chk, save
}

func init() {
	vfs.RegisterProvider(&ftpProvider{})
	RegisterProtocol(&ftpProtocolHandler{})
}

type ftpFileWrapper struct {
	*os.File
	size int64
	path string
}

func (w *ftpFileWrapper) Size() int64 { return w.size }
func (w *ftpFileWrapper) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	return w.File.ReadAt(p, off)
}
func (w *ftpFileWrapper) Read(ctx context.Context, p []byte) (int, error) { return w.File.Read(p) }
func (w *ftpFileWrapper) Close() error                                    { w.File.Close(); return os.Remove(w.path) }
