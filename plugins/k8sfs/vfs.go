package k8sfs

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
)

const (
	// statBatch is how many paths one stat command gets, to keep the command
	// line short.
	statBatch = 100
	// statTimeout bounds the quick calls made from SetPath.
	statTimeout = 30 * time.Second
)

var (
	errNotADirectory = errors.New("not a directory")
	errIsADirectory  = errors.New("is a directory")
)

// unsupportedError is what every change gets; it is os.ErrPermission to the
// file operations that ask.
type unsupportedError struct{}

func (unsupportedError) Error() string {
	return k8sText("K8s.NotSupported",
		"The Kubernetes panel cannot do this: namespaces, pods and containers are not managed here, and file attributes cannot be changed",
		"Панель Kubernetes этого не умеет: пространствами имён, подами и контейнерами здесь не управляют, а атрибуты файлов изменить нельзя")
}

func (unsupportedError) Is(target error) bool { return target == os.ErrPermission }

// k8sVFS is the Kubernetes panel. Paths are POSIX: "/" lists namespaces,
// "/<ns>" its pods, "/<ns>/<pod>" the pod's containers and
// "/<ns>/<pod>/<container>/..." the container's file system.
type k8sVFS struct {
	open func() (*restClient, error)
	// prefix is the URI head of this panel's paths: uriPrefix, or uriPrefix
	// plus the escaped name of a kubeconfig context.
	prefix string

	mu     sync.Mutex
	cli    *restClient
	cwd    string
	closed bool
}

func newK8sVFS(open func() (*restClient, error)) *k8sVFS {
	return &k8sVFS{open: open, prefix: uriPrefix, cwd: "/"}
}

func (v *k8sVFS) clientFor() (*restClient, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return nil, errors.New("Kubernetes: the panel is closed")
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

// Panel paths are written as k8s:///<path> (uriPrefix and the POSIX path),
// so that a bookmark, a folder history entry or a restored session can open the
// panel again through the URI provider (f4#1669); the methods accept both that
// form and the plain path, and keep the form they were given.
const uriPrefix = "k8s://"

// A panel opened for one kubeconfig context (see contexts.go) writes
// k8s://<context>/<path> instead, the context name escaped as a URL path
// segment; its prefix is what the methods below strip and put back.
func (v *k8sVFS) stripURI(p string) (plain string, wasURI bool) {
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

func (v *k8sVFS) withURI(p string, uri bool) string {
	if uri {
		return v.prefix + p
	}
	return p
}

func (v *k8sVFS) IsAtRoot() bool { return v.plainPath() == "/" }

func (v *k8sVFS) IsAbs(p string) bool {
	return strings.HasPrefix(p, "/") || strings.HasPrefix(p, v.prefix)
}

// plainPath is the current folder as a POSIX path.
func (v *k8sVFS) plainPath() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.cwd
}

// GetPath is the current folder as a URI.
func (v *k8sVFS) GetPath() string { return v.prefix + v.plainPath() }

func (v *k8sVFS) Join(elem ...string) string {
	if len(elem) == 0 {
		return ""
	}
	first, uri := v.stripURI(elem[0])
	return v.withURI(path.Join(append([]string{first}, elem[1:]...)...), uri)
}

func (v *k8sVFS) Base(p string) string {
	plain, _ := v.stripURI(p)
	return path.Base(path.Clean(plain))
}

func (v *k8sVFS) Dir(p string) string {
	plain, uri := v.stripURI(p)
	return v.withURI(path.Dir(path.Clean(plain)), uri)
}

// Abs is always the plain POSIX path: it is what the rest of the panel works with.
func (v *k8sVFS) Abs(p string) (string, error) {
	if p == "" {
		return v.plainPath(), nil
	}
	plain, _ := v.stripURI(p)
	if strings.HasPrefix(plain, "/") {
		return path.Clean(plain), nil
	}
	return path.Join(v.plainPath(), plain), nil
}

// location is a panel path taken apart. depth counts how many of namespace,
// pod and container it names (0 to 3); inner is the path inside the container
// ("/" at its root).
type location struct {
	ns, pod, container, inner string
	depth                     int
}

func parseLocation(abs string) location {
	rest := strings.Trim(path.Clean(abs), "/")
	if rest == "" {
		return location{inner: "/"}
	}
	parts := strings.SplitN(rest, "/", 4)
	loc := location{ns: parts[0], depth: 1, inner: "/"}
	if len(parts) > 1 {
		loc.pod, loc.depth = parts[1], 2
	}
	if len(parts) > 2 {
		loc.container, loc.depth = parts[2], 3
	}
	if len(parts) > 3 {
		loc.inner = "/" + parts[3]
	}
	return loc
}

func (v *k8sVFS) SetPath(p string) error {
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

func dirItem(name string, mtime time.Time) vfs.VFSItem {
	return vfs.VFSItem{
		KnownMetadata: vfs.MetadataExplicit | vfs.MetadataMTime,
		Name:          name,
		IsDir:         true,
		NoExtension:   true, // a pod called web.1 is not a file of type "1"
		MTime:         mtime,
	}
}

func (v *k8sVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	abs, err := v.Abs(p)
	if err != nil {
		return err
	}
	cli, err := v.clientFor()
	if err != nil {
		return err
	}
	loc := parseLocation(abs)
	send := func(items []vfs.VFSItem) {
		if len(items) > 0 && onChunk != nil {
			onChunk(items)
		}
	}
	switch loc.depth {
	case 0:
		names, err := cli.listNamespaces(ctx)
		if err != nil {
			return err
		}
		items := make([]vfs.VFSItem, 0, len(names))
		for _, n := range names {
			items = append(items, dirItem(n, time.Time{}))
		}
		send(items)
		return nil
	case 1:
		pods, err := cli.listPods(ctx, loc.ns)
		if err != nil {
			return err
		}
		items := make([]vfs.VFSItem, 0, len(pods))
		for _, pod := range pods {
			items = append(items, dirItem(pod.name, pod.created))
		}
		send(items)
		return nil
	case 2:
		pod, err := findPod(ctx, cli, loc.ns, loc.pod)
		if err != nil {
			return err
		}
		items := make([]vfs.VFSItem, 0, len(pod.containers))
		for _, c := range pod.containers {
			items = append(items, dirItem(c, pod.created))
		}
		send(items)
		return nil
	}
	return v.listInside(ctx, cli, loc, send)
}

func findPod(ctx context.Context, cli *restClient, ns, name string) (podInfo, error) {
	pods, err := cli.listPods(ctx, ns)
	if err != nil {
		return podInfo{}, err
	}
	for _, p := range pods {
		if p.name == name {
			return p, nil
		}
	}
	return podInfo{}, fmt.Errorf("%s/%s: %w", ns, name, os.ErrNotExist)
}

func run(ctx context.Context, cli *restClient, loc location, cmd ...string) (string, error) {
	var out bytes.Buffer
	err := cli.exec(ctx, loc.ns, loc.pod, loc.container, cmd, &out)
	return out.String(), err
}

// listInside lists a folder of a container: ls gives the names and which are
// folders (links to folders included), stat then adds sizes, times and modes
// where the container has stat.
func (v *k8sVFS) listInside(ctx context.Context, cli *restClient, loc location, send func([]vfs.VFSItem)) error {
	out, err := run(ctx, cli, loc, "ls", "-1ApL", "--", loc.inner)
	if err != nil && (out == "" || !errors.Is(err, errExecFailed)) {
		return err
	}
	var names []string
	isDir := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		name := strings.TrimSuffix(line, "/")
		if name == "" {
			continue
		}
		names = append(names, name)
		isDir[name] = strings.HasSuffix(line, "/")
	}
	details := map[string]vfs.VFSItem{}
	for start := 0; start < len(names); start += statBatch {
		end := min(start+statBatch, len(names))
		args := []string{"stat", "-c", "%F|%s|%Y|%a|%n", "--"}
		for _, n := range names[start:end] {
			args = append(args, path.Join(loc.inner, n))
		}
		text, statErr := run(ctx, cli, loc, args...)
		if statErr != nil && text == "" {
			break // no stat here: names and folder flags are all there is
		}
		for _, line := range strings.Split(text, "\n") {
			if item, ok := parseStatLine(line); ok {
				details[item.Name] = item
			}
		}
	}
	items := make([]vfs.VFSItem, 0, len(names))
	for _, n := range names {
		item, ok := details[n]
		if !ok {
			item = vfs.VFSItem{KnownMetadata: vfs.MetadataExplicit | vfs.MetadataHidden, Name: n, IsHidden: strings.HasPrefix(n, ".")}
		}
		item.IsDir = isDir[n]
		items = append(items, item)
	}
	send(items)
	return ctx.Err()
}

// parseStatLine reads one line of `stat -c '%F|%s|%Y|%a|%n'`. Name is the base
// name of the path stat was given.
func parseStatLine(line string) (vfs.VFSItem, bool) {
	parts := strings.SplitN(strings.TrimRight(line, "\r"), "|", 5)
	if len(parts) != 5 || parts[4] == "" {
		return vfs.VFSItem{}, false
	}
	size, _ := strconv.ParseInt(parts[1], 10, 64)
	unix, _ := strconv.ParseInt(parts[2], 10, 64)
	perm, _ := strconv.ParseUint(parts[3], 8, 32)
	name := path.Base(parts[4])
	kind := parts[0]
	item := vfs.VFSItem{
		KnownMetadata: vfs.MetadataExplicit | vfs.MetadataPermissions | vfs.MetadataExecutable | vfs.MetadataHidden | vfs.MetadataMTime,
		Name:          name,
		IsDir:         kind == "directory",
		IsSymlink:     kind == "symbolic link",
		MTime:         time.Unix(unix, 0),
		UnixMode:      uint32(perm),
		IsExecutable:  perm&0o111 != 0,
		IsHidden:      strings.HasPrefix(name, "."),
	}
	if strings.HasPrefix(kind, "regular") {
		item.Size, item.SizeKnown = size, true
	}
	return item, true
}

func (v *k8sVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	abs, err := v.Abs(p)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	loc := parseLocation(abs)
	if loc.depth == 0 {
		return dirItem("Kubernetes", time.Time{}), nil
	}
	cli, err := v.clientFor()
	if err != nil {
		return vfs.VFSItem{}, err
	}
	switch loc.depth {
	case 1:
		names, err := cli.listNamespaces(ctx)
		if err != nil {
			return vfs.VFSItem{}, err
		}
		for _, n := range names {
			if n == loc.ns {
				return dirItem(n, time.Time{}), nil
			}
		}
		return vfs.VFSItem{}, fmt.Errorf("%s: %w", loc.ns, os.ErrNotExist)
	case 2:
		pod, err := findPod(ctx, cli, loc.ns, loc.pod)
		if err != nil {
			return vfs.VFSItem{}, err
		}
		return dirItem(pod.name, pod.created), nil
	}
	if loc.inner == "/" {
		pod, err := findPod(ctx, cli, loc.ns, loc.pod)
		if err != nil {
			return vfs.VFSItem{}, err
		}
		for _, c := range pod.containers {
			if c == loc.container {
				return dirItem(c, pod.created), nil
			}
		}
		return vfs.VFSItem{}, fmt.Errorf("%s: %w", abs, os.ErrNotExist)
	}
	out, err := run(ctx, cli, loc, "stat", "-c", "%F|%s|%Y|%a|%n", "--", loc.inner)
	if err != nil {
		if errors.Is(err, errExecFailed) {
			return vfs.VFSItem{}, fmt.Errorf("%s: %w", abs, os.ErrNotExist)
		}
		return vfs.VFSItem{}, err
	}
	item, ok := parseStatLine(strings.TrimSpace(out))
	if !ok {
		return vfs.VFSItem{}, fmt.Errorf("%s: unreadable stat output", abs)
	}
	if item.IsSymlink {
		if _, err := run(ctx, cli, loc, "test", "-d", loc.inner); err == nil {
			item.IsDir = true
		}
	}
	return item, nil
}

// Open copies a file out of the container with cat into a temporary file,
// which is what gives F3 and F5 random access.
func (v *k8sVFS) Open(ctx context.Context, p string) (vfs.ReadAtCloser, error) {
	abs, err := v.Abs(p)
	if err != nil {
		return nil, err
	}
	loc := parseLocation(abs)
	if loc.depth < 3 || loc.inner == "/" {
		return nil, fmt.Errorf("%s: %w", abs, errIsADirectory)
	}
	cli, err := v.clientFor()
	if err != nil {
		return nil, err
	}
	if item, err := v.Stat(ctx, abs); err != nil {
		return nil, err
	} else if item.IsDir {
		return nil, fmt.Errorf("%s: %w", abs, errIsADirectory)
	}
	file, err := os.CreateTemp("", "f4-k8s-*")
	if err != nil {
		return nil, err
	}
	tempPath := file.Name()
	fail := func(err error) (vfs.ReadAtCloser, error) {
		_ = file.Close()
		_ = os.Remove(tempPath)
		return nil, err
	}
	if err := cli.exec(ctx, loc.ns, loc.pod, loc.container, []string{"cat", "--", loc.inner}, file); err != nil {
		return fail(err)
	}
	size, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return fail(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fail(err)
	}
	return &vfs.TempFileWrapper{File: file, SizeVal: size, TempPath: tempPath}, nil
}

// Writing goes through exec too. The exec protocol available over a plain
// WebSocket (v4.channel.k8s.io) cannot close a command's stdin, so a file is
// not streamed in: it is sent as base64 in the arguments of a few short
// commands, one chunk each (`sh -c 'printf %s "$1" | base64 -d >> "$2"'`), and
// the size is capped. Folders, deletion and renaming are single commands. The
// container needs sh, base64, mkdir, rm and mv (busybox has them all).

// maxUploadSize is the largest file the panel uploads; every 12 KiB of it is
// one exec round trip.
const maxUploadSize = 8 << 20

// uploadChunk is how much of a file one command carries (before base64).
const uploadChunk = 12 << 10

// inside resolves a path that must lie inside a container, below its root.
func (v *k8sVFS) inside(p string) (cli *restClient, loc location, err error) {
	abs, err := v.Abs(p)
	if err != nil {
		return nil, location{}, err
	}
	loc = parseLocation(abs)
	if loc.depth < 3 || loc.inner == "/" {
		return nil, location{}, unsupportedError{}
	}
	cli, err = v.clientFor()
	return cli, loc, err
}

func (v *k8sVFS) MkDir(ctx context.Context, p string) error {
	cli, loc, err := v.inside(p)
	if err != nil {
		return err
	}
	_, err = run(ctx, cli, loc, "mkdir", "--", loc.inner)
	return err
}

func (v *k8sVFS) Remove(ctx context.Context, p string) error {
	cli, loc, err := v.inside(p)
	if err != nil {
		return err
	}
	_, err = run(ctx, cli, loc, "rm", "-rf", "--", loc.inner)
	return err
}

func (v *k8sVFS) Rename(ctx context.Context, oldpath, newpath string) error {
	cli, from, err := v.inside(oldpath)
	if err != nil {
		return err
	}
	_, to, err := v.inside(newpath)
	if err != nil {
		return err
	}
	if from.ns != to.ns || from.pod != to.pod || from.container != to.container {
		return unsupportedError{}
	}
	_, err = run(ctx, cli, from, "mv", "--", from.inner, to.inner)
	return err
}

func (v *k8sVFS) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return unsupportedError{}
}

// Create collects what is written in a temporary file and sends it when the
// writer is closed.
func (v *k8sVFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	cli, loc, err := v.inside(p)
	if err != nil {
		return nil, err
	}
	file, err := os.CreateTemp("", "f4-k8s-up-*")
	if err != nil {
		return nil, err
	}
	return &uploadWriter{ctx: ctx, cli: cli, loc: loc, file: file}, nil
}

type uploadWriter struct {
	ctx    context.Context
	cli    *restClient
	loc    location
	file   *os.File
	size   int64
	closed bool
}

func (w *uploadWriter) Write(p []byte) (int, error) {
	if w.size+int64(len(p)) > maxUploadSize {
		return 0, fmt.Errorf("kubernetes: %s", k8sText("K8s.UploadTooLarge",
			"a file larger than 8 MiB cannot be uploaded through exec; copy it with kubectl cp",
			"файл больше 8 МиБ нельзя загрузить через exec; скопируйте его командой kubectl cp"))
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

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
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	buf := make([]byte, uploadChunk)
	redirect := ">" // the first chunk truncates, the rest append
	for first := true; ; first = false {
		n, err := io.ReadFull(w.file, buf)
		if n > 0 || first { // an empty file still gets created
			script := `printf %s "$1" | base64 -d ` + redirect + ` "$2"`
			enc := base64.StdEncoding.EncodeToString(buf[:n])
			if _, runErr := run(w.ctx, w.cli, w.loc, "sh", "-c", script, "sh", enc, w.loc.inner); runErr != nil {
				return runErr
			}
			redirect = ">>"
		}
		if err != nil { // io.EOF or io.ErrUnexpectedEOF: that was the last chunk
			return nil
		}
	}
}

func (v *k8sVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true, HasUnixPermissions: true, HasWrite: true}
}

func (v *k8sVFS) Search(context.Context, string, string) (chan int64, error) { return nil, nil }

func (v *k8sVFS) ParentVFS() vfs.VFS { return nil }

// PanelTitle names the panel by where it is, "Kubernetes:default/web/app/etc"
// (for a kubeconfig context, "Kubernetes(name):default/...").
func (v *k8sVFS) PanelTitle(p string) string {
	head := "Kubernetes"
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

// Clone opens its own connection when first used.
func (v *k8sVFS) Clone() vfs.VFS {
	clone := newK8sVFS(v.open)
	clone.prefix = v.prefix
	clone.cwd = v.plainPath()
	return clone
}

func (v *k8sVFS) Close() error {
	v.mu.Lock()
	cli := v.cli
	v.cli = nil
	v.closed = true
	v.mu.Unlock()
	cli.close()
	return nil
}

var (
	_ vfs.VFS                = (*k8sVFS)(nil)
	_ vfs.PanelTitleProvider = (*k8sVFS)(nil)
)
