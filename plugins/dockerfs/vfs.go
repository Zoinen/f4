package dockerfs

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
)

const (
	// maxScanEntries bounds how much of a directory's archive is read while
	// listing it. The archive endpoint has no "one level only" mode: a
	// directory comes as a tar of everything under it, so listing / walks the
	// whole image. The first-level entries stream out in order, and a listing
	// that has to stop early says so instead of pretending to be complete.
	maxScanEntries = 400000
	// listChunk is how many rows go to the panel at once while a listing runs.
	listChunk = 256
	// maxLinkHops bounds symlink chasing (/bin -> usr/bin and the like).
	maxLinkHops = 16
	// maxLinkResolves bounds the extra requests made to learn whether the
	// symlinks of one folder point at folders.
	maxLinkResolves = 256
	// statTimeout bounds the quick calls made from SetPath and Stat.
	statTimeout = 15 * time.Second
)

var (
	errNotADirectory = errors.New("not a directory")
	errIsADirectory  = errors.New("is a directory")
	errLinkLoop      = errors.New("too many levels of symbolic links")
)

// unsupportedError is what an operation the Docker panel cannot do gets; it
// is os.ErrPermission to the file operations that ask.
type unsupportedError struct{}

func (unsupportedError) Error() string {
	return dockerText("Docker.NotSupported",
		"The Docker panel cannot do this: containers themselves are not managed here, and file attributes cannot be changed",
		"Панель Docker этого не умеет: самими контейнерами здесь не управляют, а атрибуты файлов изменить нельзя")
}

func (unsupportedError) Is(target error) bool { return target == os.ErrPermission }

// listingTruncatedError ends a listing that hit maxScanEntries.
type listingTruncatedError struct{}

func (listingTruncatedError) Error() string {
	return dockerText("Docker.ListingTruncated",
		"The folder is too large to list completely through the Docker API; the list is partial",
		"Папка слишком велика, чтобы получить её список целиком через API Docker; список неполный")
}

// dockerVFS is the Docker panel. Paths are POSIX: "/" lists the containers,
// "/<container>" is the root of one container's file system, and the rest is
// the path inside it.
type dockerVFS struct {
	open func() (*client, error)
	// prefix is the URI head of this panel's paths: uriPrefix, or
	// uriPrefix plus the escaped name of a docker context.
	prefix string

	mu     sync.Mutex
	cli    *client
	ids    map[string]string // container name -> id, from the last listing
	cwd    string
	closed bool
}

func newDockerVFS(open func() (*client, error)) *dockerVFS {
	return &dockerVFS{open: open, prefix: uriPrefix, cwd: "/", ids: map[string]string{}}
}

// clientFor connects on first use and keeps the connection; a failed attempt
// is not remembered, so starting the daemon and pressing Ctrl+R is enough.
func (v *dockerVFS) clientFor() (*client, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return nil, errors.New("Docker: the panel is closed")
	}
	if v.cli != nil {
		return v.cli, nil
	}
	cli, err := v.open()
	if err != nil {
		return nil, err
	}
	v.cli = cli
	return cli, nil
}

// Panel paths are written as docker:///<path> (uriPrefix and the POSIX path),
// so that a bookmark, a folder history entry or a restored session can open the
// panel again through the URI provider (f4#1669); the methods accept both that
// form and the plain path, and keep the form they were given.
const uriPrefix = "docker://"

// A panel opened for one docker context (see contexts.go) writes
// docker://<context>/<path> instead, the context name escaped as a URL path
// segment; its prefix is what the methods below strip and put back.
func (v *dockerVFS) stripURI(p string) (plain string, wasURI bool) {
	if v.prefix == uriPrefix {
		if rest, ok := strings.CutPrefix(p, uriPrefix); ok {
			return rest, true
		}
		return p, false
	}
	if rest, ok := strings.CutPrefix(p, v.prefix); ok && (rest == "" || rest[0] == '/') {
		if rest == "" {
			rest = "/"
		}
		return rest, true
	}
	return p, false
}

func (v *dockerVFS) withURI(p string, uri bool) string {
	if uri {
		return v.prefix + p
	}
	return p
}

func (v *dockerVFS) IsAtRoot() bool { return v.plainPath() == "/" }

func (v *dockerVFS) IsAbs(p string) bool {
	return strings.HasPrefix(p, "/") || strings.HasPrefix(p, v.prefix)
}

// plainPath is the current folder as a POSIX path.
func (v *dockerVFS) plainPath() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.cwd
}

// GetPath is the current folder as a URI.
func (v *dockerVFS) GetPath() string { return v.prefix + v.plainPath() }

func (v *dockerVFS) Join(elem ...string) string {
	if len(elem) == 0 {
		return ""
	}
	first, uri := v.stripURI(elem[0])
	return v.withURI(path.Join(append([]string{first}, elem[1:]...)...), uri)
}

func (v *dockerVFS) Base(p string) string {
	plain, _ := v.stripURI(p)
	return path.Base(path.Clean(plain))
}

func (v *dockerVFS) Dir(p string) string {
	plain, uri := v.stripURI(p)
	return v.withURI(path.Dir(path.Clean(plain)), uri)
}

// Abs is always the plain POSIX path: it is what the rest of the panel works with.
func (v *dockerVFS) Abs(p string) (string, error) {
	if p == "" {
		return v.plainPath(), nil
	}
	plain, _ := v.stripURI(p)
	if strings.HasPrefix(plain, "/") {
		return path.Clean(plain), nil
	}
	return path.Join(v.plainPath(), plain), nil
}

// split cuts an absolute path into the container name and the path inside it
// ("/" for the container's root). The root of the panel has no container.
func split(abs string) (name, inner string) {
	rest := strings.TrimPrefix(path.Clean(abs), "/")
	if rest == "" {
		return "", "/"
	}
	name, tail, found := strings.Cut(rest, "/")
	if !found {
		return name, "/"
	}
	return name, "/" + tail
}

func (v *dockerVFS) SetPath(p string) error {
	abs, err := v.Abs(p)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), statTimeout)
	defer cancel()
	item, err := v.Stat(ctx, abs)
	if err != nil {
		return err
	}
	if !item.IsDir {
		return fmt.Errorf("%s: %w", abs, errNotADirectory)
	}
	v.mu.Lock()
	v.cwd = abs
	v.mu.Unlock()
	return nil
}

// containerID maps a panel name to a container id, listing the containers
// again when the name is not one it has seen.
func (v *dockerVFS) containerID(ctx context.Context, cli *client, name string) (string, error) {
	v.mu.Lock()
	id, ok := v.ids[name]
	v.mu.Unlock()
	if ok {
		return id, nil
	}
	list, err := cli.listContainers(ctx)
	if err != nil {
		return "", err
	}
	v.rememberContainers(list)
	v.mu.Lock()
	defer v.mu.Unlock()
	if id, ok := v.ids[name]; ok {
		return id, nil
	}
	return "", fmt.Errorf("%s: %w", name, os.ErrNotExist)
}

func (v *dockerVFS) rememberContainers(list []containerInfo) {
	ids := make(map[string]string, len(list))
	for _, c := range list {
		ids[c.name()] = c.ID
	}
	v.mu.Lock()
	v.ids = ids
	v.mu.Unlock()
}

// follow chases symlinks from p until it reaches something that is not one, so
// that /bin -> usr/bin can be entered like the folder it stands for.
func follow(ctx context.Context, cli *client, id, p string) (string, pathStat, error) {
	cur := path.Clean(p)
	for hop := 0; hop < maxLinkHops; hop++ {
		st, err := cli.statPath(ctx, id, cur)
		if err != nil {
			return "", pathStat{}, err
		}
		if !st.isSymlink() {
			return cur, st, nil
		}
		target := st.LinkTarget
		if !path.IsAbs(target) {
			target = path.Join(path.Dir(cur), target)
		}
		cur = path.Clean(target)
	}
	return "", pathStat{}, fmt.Errorf("%s: %w", p, errLinkLoop)
}

func containerItem(c containerInfo) vfs.VFSItem {
	return vfs.VFSItem{
		KnownMetadata: vfs.MetadataExplicit | vfs.MetadataMTime,
		Name:          c.name(),
		IsDir:         true,
		NoExtension:   true, // web.1 is a container, not a file of type "1"
		MTime:         time.Unix(c.Created, 0),
	}
}

func itemFromHeader(name string, hdr *tar.Header) vfs.VFSItem {
	mode := hdr.FileInfo().Mode()
	item := vfs.VFSItem{
		KnownMetadata: vfs.MetadataExplicit | vfs.MetadataPermissions | vfs.MetadataExecutable | vfs.MetadataHidden | vfs.MetadataMTime,
		Name:          name,
		IsDir:         hdr.Typeflag == tar.TypeDir,
		IsSymlink:     hdr.Typeflag == tar.TypeSymlink,
		MTime:         hdr.ModTime,
		UnixMode:      uint32(mode.Perm()),
		IsExecutable:  mode.Perm()&0o111 != 0,
		IsHidden:      strings.HasPrefix(name, "."),
		Uid:           hdr.Uid,
		Gid:           hdr.Gid,
	}
	if mode.IsRegular() {
		item.Size = hdr.Size
		item.SizeKnown = true
	}
	return item
}

func itemFromStat(name string, st pathStat, isDir bool) vfs.VFSItem {
	perm := st.fileMode().Perm()
	item := vfs.VFSItem{
		KnownMetadata: vfs.MetadataExplicit | vfs.MetadataPermissions | vfs.MetadataExecutable | vfs.MetadataHidden | vfs.MetadataMTime,
		Name:          name,
		IsDir:         isDir,
		IsSymlink:     st.isSymlink(),
		MTime:         st.MTime,
		UnixMode:      uint32(perm),
		IsExecutable:  perm&0o111 != 0,
		IsHidden:      strings.HasPrefix(name, "."),
	}
	if st.fileMode().IsRegular() {
		item.Size = st.Size
		item.SizeKnown = true
	}
	return item
}

// tarSegments turns an archive entry name into its path segments, whatever
// prefix the daemon gave it ("dir/", "./dir", "/dir", "" for the root).
func tarSegments(name string) []string {
	clean := strings.Trim(path.Clean("/"+name), "/")
	if clean == "" {
		return nil
	}
	return strings.Split(clean, "/")
}

func hasPrefix(segs, prefix []string) bool {
	if len(segs) < len(prefix) {
		return false
	}
	for i := range prefix {
		if segs[i] != prefix[i] {
			return false
		}
	}
	return true
}

func (v *dockerVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	abs, err := v.Abs(p)
	if err != nil {
		return err
	}
	cli, err := v.clientFor()
	if err != nil {
		return err
	}
	name, inner := split(abs)
	if name == "" {
		list, err := cli.listContainers(ctx)
		if err != nil {
			return err
		}
		v.rememberContainers(list)
		items := make([]vfs.VFSItem, 0, len(list))
		for _, c := range list {
			items = append(items, containerItem(c))
		}
		if len(items) > 0 && onChunk != nil {
			onChunk(items)
		}
		return nil
	}
	id, err := v.containerID(ctx, cli, name)
	if err != nil {
		return err
	}
	dir, st, err := follow(ctx, cli, id, inner)
	if err != nil {
		return err
	}
	if !st.isDir() {
		return fmt.Errorf("%s: %w", abs, errNotADirectory)
	}
	return listArchive(ctx, cli, id, dir, onChunk)
}

// listArchive reads the archive of a folder and hands over its first-level
// entries as they pass. The first entry of the tar is the folder itself; its
// name (whatever the daemon spelled it as) is the prefix to strip.
func listArchive(ctx context.Context, cli *client, id, dir string, onChunk func([]vfs.VFSItem)) error {
	body, err := cli.archive(ctx, id, dir)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()

	tr := tar.NewReader(body)
	var (
		prefix   []string
		first    = true
		chunk    []vfs.VFSItem
		scanned  int
		resolved int
	)
	flush := func() {
		if len(chunk) > 0 && onChunk != nil {
			onChunk(chunk)
		}
		chunk = nil
	}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			flush()
			return err
		}
		if scanned++; scanned > maxScanEntries {
			flush()
			return listingTruncatedError{}
		}
		segs := tarSegments(hdr.Name)
		if first {
			first = false
			if hdr.Typeflag != tar.TypeDir {
				return fmt.Errorf("%s: %w", dir, errNotADirectory)
			}
			prefix = segs
			continue
		}
		if len(segs) != len(prefix)+1 || !hasPrefix(segs, prefix) {
			continue
		}
		item := itemFromHeader(segs[len(prefix)], hdr)
		if item.IsSymlink && resolved < maxLinkResolves {
			resolved++
			target := hdr.Linkname
			if !path.IsAbs(target) {
				target = path.Join(dir, target) // #nosec G305 -- a path inside the container, only ever sent to the daemon
			}
			if _, st, err := follow(ctx, cli, id, target); err == nil && st.isDir() {
				item.IsDir = true
			}
		}
		chunk = append(chunk, item)
		if len(chunk) >= listChunk {
			flush()
		}
	}
	flush()
	return ctx.Err()
}

func (v *dockerVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	abs, err := v.Abs(p)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	name, inner := split(abs)
	if name == "" {
		return vfs.VFSItem{KnownMetadata: vfs.MetadataExplicit, Name: "Docker", IsDir: true, NoExtension: true}, nil
	}
	cli, err := v.clientFor()
	if err != nil {
		return vfs.VFSItem{}, err
	}
	id, err := v.containerID(ctx, cli, name)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	if inner == "/" {
		return vfs.VFSItem{KnownMetadata: vfs.MetadataExplicit, Name: name, IsDir: true, NoExtension: true}, nil
	}
	st, err := cli.statPath(ctx, id, inner)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	isDir := st.isDir()
	if st.isSymlink() {
		if _, target, err := follow(ctx, cli, id, inner); err == nil {
			isDir = target.isDir()
		}
	}
	return itemFromStat(path.Base(inner), st, isDir), nil
}

// Open copies a file out of the container into a temporary file, which is what
// gives F3 and F5 random access; the archive endpoint only streams.
func (v *dockerVFS) Open(ctx context.Context, p string) (vfs.ReadAtCloser, error) {
	abs, err := v.Abs(p)
	if err != nil {
		return nil, err
	}
	name, inner := split(abs)
	if name == "" || inner == "/" {
		return nil, fmt.Errorf("%s: %w", abs, errIsADirectory)
	}
	cli, err := v.clientFor()
	if err != nil {
		return nil, err
	}
	id, err := v.containerID(ctx, cli, name)
	if err != nil {
		return nil, err
	}
	target, st, err := follow(ctx, cli, id, inner)
	if err != nil {
		return nil, err
	}
	if st.isDir() {
		return nil, fmt.Errorf("%s: %w", abs, errIsADirectory)
	}
	body, err := cli.archive(ctx, id, target)
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()

	tr := tar.NewReader(body)
	hdr, err := tr.Next()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", abs, err)
	}
	if hdr.Typeflag == tar.TypeDir {
		return nil, fmt.Errorf("%s: %w", abs, errIsADirectory)
	}
	file, err := os.CreateTemp("", "f4-docker-*")
	if err != nil {
		return nil, err
	}
	tempPath := file.Name()
	fail := func(err error) (vfs.ReadAtCloser, error) {
		_ = file.Close()
		_ = os.Remove(tempPath)
		return nil, err
	}
	written, err := io.Copy(file, &ctxReader{ctx: ctx, r: tr})
	if err != nil {
		return fail(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fail(err)
	}
	return &vfs.TempFileWrapper{File: file, SizeVal: written, TempPath: tempPath}, nil
}

// ctxReader stops a long copy when the task is cancelled.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// target resolves a path that must lie inside a container (not the container
// list and not a container's own root).
func (v *dockerVFS) target(ctx context.Context, p string) (cli *client, id, inner string, err error) {
	abs, err := v.Abs(p)
	if err != nil {
		return nil, "", "", err
	}
	name, inner := split(abs)
	if name == "" || inner == "/" {
		return nil, "", "", unsupportedError{}
	}
	cli, err = v.clientFor()
	if err != nil {
		return nil, "", "", err
	}
	id, err = v.containerID(ctx, cli, name)
	if err != nil {
		return nil, "", "", err
	}
	return cli, id, inner, nil
}

// parentDir is the folder a new entry goes into, symlinks followed, checked to
// be a folder.
func parentDir(ctx context.Context, cli *client, id, inner string) (string, error) {
	dir, st, err := follow(ctx, cli, id, path.Dir(inner))
	if err != nil {
		return "", err
	}
	if !st.isDir() {
		return "", fmt.Errorf("%s: %w", path.Dir(inner), errNotADirectory)
	}
	return dir, nil
}

// MkDir uploads a tar holding the one folder, which is what `docker cp` does
// for a copy into a container.
func (v *dockerVFS) MkDir(ctx context.Context, p string) error {
	cli, id, inner, err := v.target(ctx, p)
	if err != nil {
		return err
	}
	dir, err := parentDir(ctx, cli, id, inner)
	if err != nil {
		return err
	}
	return cli.putArchive(ctx, id, dir, func(tw *tar.Writer) error {
		return tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeDir, Name: path.Base(inner) + "/", Mode: 0o755, ModTime: time.Now(),
		})
	})
}

// Remove and Rename: the Engine API cannot delete or move a container's files,
// so they run rm and mv inside the container. That needs a running container
// with those tools in it; when it has none, the error says what failed.
func (v *dockerVFS) Remove(ctx context.Context, p string) error {
	cli, id, inner, err := v.target(ctx, p)
	if err != nil {
		return err
	}
	return cli.exec(ctx, id, []string{"rm", "-rf", "--", inner})
}

func (v *dockerVFS) Rename(ctx context.Context, oldpath, newpath string) error {
	cli, id, from, err := v.target(ctx, oldpath)
	if err != nil {
		return err
	}
	absTo, err := v.Abs(newpath)
	if err != nil {
		return err
	}
	nameTo, to := split(absTo)
	oldAbs, _ := v.Abs(oldpath)
	nameFrom, _ := split(oldAbs)
	if nameTo != nameFrom || to == "/" {
		return unsupportedError{}
	}
	return cli.exec(ctx, id, []string{"mv", "--", from, to})
}

func (v *dockerVFS) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return unsupportedError{}
}

// Create collects what is written in a temporary file and uploads it as a tar
// when the writer is closed: a tar header needs the size up front, which a
// stream of unknown length cannot give.
func (v *dockerVFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	cli, id, inner, err := v.target(ctx, p)
	if err != nil {
		return nil, err
	}
	dir, err := parentDir(ctx, cli, id, inner)
	if err != nil {
		return nil, err
	}
	file, err := os.CreateTemp("", "f4-docker-up-*")
	if err != nil {
		return nil, err
	}
	return &uploadWriter{ctx: ctx, cli: cli, id: id, dir: dir, name: path.Base(inner), file: file}, nil
}

// uploadWriter is the writer Create returns.
type uploadWriter struct {
	ctx    context.Context
	cli    *client
	id     string
	dir    string
	name   string
	file   *os.File
	closed bool
}

func (w *uploadWriter) Write(p []byte) (int, error) { return w.file.Write(p) }

func (w *uploadWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	tempPath := w.file.Name()
	defer func() {
		_ = w.file.Close()
		_ = os.Remove(tempPath)
	}()
	size, err := w.file.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return w.cli.putArchive(w.ctx, w.id, w.dir, func(tw *tar.Writer) error {
		if err := tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeReg, Name: w.name, Mode: 0o644, Size: size, ModTime: time.Now(),
		}); err != nil {
			return err
		}
		_, err := io.Copy(tw, w.file)
		return err
	})
}

func (v *dockerVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true, HasUnixPermissions: true, HasWrite: true}
}

func (v *dockerVFS) Search(context.Context, string, string) (chan int64, error) { return nil, nil }

func (v *dockerVFS) ParentVFS() vfs.VFS { return nil }

// PanelTitle names the panel by where it is, "Docker:web/etc" (for a docker
// context, "Docker(name):web/etc").
func (v *dockerVFS) PanelTitle(p string) string {
	head := "Docker"
	if v.prefix != uriPrefix {
		name, _ := url.PathUnescape(strings.TrimPrefix(v.prefix, uriPrefix))
		head += "(" + name + ")"
	}
	abs, err := v.Abs(p)
	if err != nil || abs == "/" {
		return head
	}
	return head + ":" + strings.TrimPrefix(abs, "/")
}

// Clone opens its own connection when first used, so closing one panel does
// not cut the other's.
func (v *dockerVFS) Clone() vfs.VFS {
	clone := newDockerVFS(v.open)
	clone.prefix = v.prefix
	clone.cwd = v.plainPath()
	return clone
}

func (v *dockerVFS) Close() error {
	v.mu.Lock()
	cli := v.cli
	v.cli = nil
	v.closed = true
	v.mu.Unlock()
	cli.close()
	return nil
}

var (
	_ vfs.VFS                = (*dockerVFS)(nil)
	_ vfs.PanelTitleProvider = (*dockerVFS)(nil)
)
