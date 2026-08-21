package gitplugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/unxed/f4/vfs"
)

// GitLogPageSize is the intentionally bounded first page exposed by LogVFS.
// A history panel must remain quick even for repositories with a very large
// graph; a later pagination command can create another view from a cursor
// without changing this VFS contract.
const GitLogPageSize = 200

// LogTraversalMode controls the graph used for the root listing of LogVFS.
// It contains no UI types so a key binding, a command, or a future GUI can
// choose the presentation independently.
type LogTraversalMode uint8

const (
	// LogTraversalHeadDAG walks every commit reachable from HEAD.
	LogTraversalHeadDAG LogTraversalMode = iota
	// LogTraversalAllLocalRefs walks refs/heads and refs/tags, never remote
	// tracking refs. Annotated tags are resolved to the commit they name.
	LogTraversalAllLocalRefs
	// LogTraversalFirstParent walks only the first-parent chain from HEAD.
	LogTraversalFirstParent
)

// CommitTreeMode selects the listing shown after entering a commit row.
type CommitTreeMode uint8

const (
	// CommitTreeChangedFiles exposes the after-side of the first-parent diff,
	// plus read-only deleted entries for an honest change summary.
	CommitTreeChangedFiles CommitTreeMode = iota
	// CommitTreeFullSnapshot exposes the complete after-tree of the commit.
	CommitTreeFullSnapshot
)

// LogVFSOptions only changes the size of the initial lazy page. Zero retains
// the production limit. Values above GitLogPageSize are clamped so callers
// cannot accidentally turn opening a panel into an unbounded graph walk.
// A smaller value is useful to constrained hosts and focused tests.
type LogVFSOptions struct {
	CommitLimit int
}

// LogVFS is a read-only virtual filesystem over Git history. Its root is a
// lazy commit listing. A commit is a virtual directory; below it, paths name
// the after-tree either as a changed-files tree or as a full snapshot.
//
// The VFS does not call git.exe. Every repository operation goes through
// go-git and all loops check their context between object operations.
//
// A LogVFS clone shares one session. That makes historical editor overlays
// visible to F5/copy operations from every clone while still keeping them in
// memory only and never mutating the repository.
type LogVFS struct {
	session *logSession

	mu          sync.RWMutex
	currentPath string
	closed      bool
}

type logSession struct {
	mu sync.RWMutex

	repository Repository
	limit      int
	logMode    LogTraversalMode
	treeMode   CommitTreeMode

	// Commit rows are immutable values. A cache keeps revisiting the history
	// root free of object I/O, but callers may explicitly invalidate it when
	// refs have changed.
	commitCache map[LogTraversalMode][]logCommit
	overlays    map[historyOverlayKey][]byte
}

type logCommit struct {
	hash       plumbing.Hash
	subject    string
	author     string
	committed  time.Time
	parentText string
}

type historyOverlayKey struct {
	commit string
	path   string
}

type historyFile struct {
	path       string
	mode       filemode.FileMode
	blob       plumbing.Hash
	after      bool
	action     string
	previous   string
	commitHash plumbing.Hash
}

type historyRow struct {
	item vfs.VFSItem
	file *historyFile
}

// NewLogVFS creates a session-only history VFS for repository. Repository
// comes from the asynchronous discovery cache, so construction itself does no
// filesystem I/O. The zero Repository is allowed for host wiring but ReadDir
// will report that no repository root is available.
func NewLogVFS(repository Repository, options ...LogVFSOptions) *LogVFS {
	limit := GitLogPageSize
	if len(options) != 0 && options[0].CommitLimit > 0 {
		limit = options[0].CommitLimit
		if limit > GitLogPageSize {
			limit = GitLogPageSize
		}
	}
	return &LogVFS{
		session: &logSession{
			repository:  repository,
			limit:       limit,
			commitCache: make(map[LogTraversalMode][]logCommit),
			overlays:    make(map[historyOverlayKey][]byte),
		},
		currentPath: "/",
	}
}

func (view *LogVFS) IsAtRoot() bool { return view.pathSnapshot() == "/" }

func (view *LogVFS) GetPath() string { return view.pathSnapshot() }

func (*LogVFS) IsAbs(value string) bool { return path.IsAbs(value) }

// SetPath only changes virtual navigation state. Validation deliberately
// happens in ReadDir/Stat on a worker rather than resolving Git objects on the
// UI navigation path.
func (view *LogVFS) SetPath(value string) error {
	view.mu.Lock()
	defer view.mu.Unlock()
	if view.closed {
		return os.ErrClosed
	}
	view.currentPath = cleanHistoryPath(value, view.currentPath)
	return nil
}

func (view *LogVFS) ReadDir(ctx context.Context, value string, onChunk func([]vfs.VFSItem)) error {
	if err := historyContextError(ctx); err != nil {
		return err
	}
	if onChunk == nil {
		return nil
	}
	if view.isClosed() {
		return os.ErrClosed
	}

	virtualPath := view.resolvePath(value)
	if virtualPath == "/" {
		commits, err := view.commits(ctx)
		if err != nil {
			return err
		}
		items := make([]vfs.VFSItem, 0, len(commits))
		for _, commit := range commits {
			if err := historyContextError(ctx); err != nil {
				return err
			}
			items = append(items, commit.item(view.LogMode()))
		}
		return emitHistoryItems(ctx, items, onChunk)
	}

	commitHash, directory, err := parseHistoryPath(virtualPath)
	if err != nil {
		return err
	}
	rows, err := view.treeRows(ctx, commitHash, directory)
	if err != nil {
		return err
	}
	items := make([]vfs.VFSItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.item)
	}
	return emitHistoryItems(ctx, items, onChunk)
}

func (view *LogVFS) Stat(ctx context.Context, value string) (vfs.VFSItem, error) {
	if err := historyContextError(ctx); err != nil {
		return vfs.VFSItem{}, err
	}
	if view.isClosed() {
		return vfs.VFSItem{}, os.ErrClosed
	}
	virtualPath := view.resolvePath(value)
	if virtualPath == "/" {
		return vfs.VFSItem{Name: "/", IsDir: true}, nil
	}
	commitHash, directory, err := parseHistoryPath(virtualPath)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	if directory == "" {
		commit, err := view.commit(ctx, commitHash)
		if err != nil {
			return vfs.VFSItem{}, err
		}
		return commit.item(view.LogMode()), nil
	}
	parent := path.Dir(directory)
	if parent == "." {
		parent = ""
	}
	rows, err := view.treeRows(ctx, commitHash, parent)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	base := path.Base(directory)
	for _, row := range rows {
		if row.item.Name == base {
			return row.item, nil
		}
	}
	return vfs.VFSItem{}, os.ErrNotExist
}

func (*LogVFS) Join(parts ...string) string { return path.Join(parts...) }

func (view *LogVFS) Abs(value string) (string, error) {
	return view.resolvePath(value), nil
}

func (*LogVFS) Base(value string) string { return path.Base(value) }

func (*LogVFS) Dir(value string) string { return path.Dir(value) }

func (*LogVFS) MkDir(context.Context, string) error { return os.ErrPermission }

func (*LogVFS) Remove(context.Context, string) error { return os.ErrPermission }

func (*LogVFS) Rename(context.Context, string, string) error { return os.ErrPermission }

func (*LogVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true}
}

func (*LogVFS) Search(context.Context, string, string) (chan int64, error) {
	return nil, errors.New("Git log search is not supported")
}

// Open returns the after-blob for a historical regular file. A session overlay
// always wins, which makes an F5 copy automatically use a historical edit
// without publishing it to the worktree or index.
func (view *LogVFS) Open(ctx context.Context, value string) (vfs.ReadAtCloser, error) {
	if err := historyContextError(ctx); err != nil {
		return nil, err
	}
	if view.isClosed() {
		return nil, os.ErrClosed
	}
	file, err := view.fileAt(ctx, view.resolvePath(value))
	if err != nil {
		return nil, err
	}
	if !file.after || !file.mode.IsFile() {
		return nil, os.ErrPermission
	}
	key := overlayKey(file)
	if data, ok := view.overlay(key); ok {
		return &historyReadAtCloser{data: data}, nil
	}

	repository, err := view.openRepository(ctx)
	if err != nil {
		return nil, err
	}
	defer repository.Close()
	if err := historyContextError(ctx); err != nil {
		return nil, err
	}
	blob, err := repository.BlobObject(file.blob)
	if err != nil {
		return nil, err
	}
	reader, err := blob.Reader()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := readHistoryBlob(ctx, reader)
	if err != nil {
		return nil, err
	}
	return &historyReadAtCloser{data: data}, nil
}

func (*LogVFS) Create(context.Context, string) (io.WriteCloser, error) {
	return nil, os.ErrPermission
}

func (*LogVFS) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return os.ErrPermission
}

func (*LogVFS) ParentVFS() vfs.VFS { return nil }

func (view *LogVFS) Clone() vfs.VFS {
	view.mu.RLock()
	clone := &LogVFS{session: view.session, currentPath: view.currentPath, closed: view.closed}
	view.mu.RUnlock()
	return clone
}

func (view *LogVFS) Close() error {
	view.mu.Lock()
	view.closed = true
	view.mu.Unlock()
	return nil
}

func (*LogVFS) GetTitle() string { return "Git: Log" }

func (view *LogVFS) PanelTitle(value string) string {
	virtualPath := view.resolvePath(value)
	if virtualPath == "/" {
		return "Git: Log — " + view.LogMode().String()
	}
	commitHash, _, err := parseHistoryPath(virtualPath)
	if err != nil {
		return "Git: Log"
	}
	return "Git: Commit " + shortHistoryHash(commitHash.String()) + " — " + view.CommitTreeMode().String()
}

// LogMode returns the current root traversal mode without repository I/O.
func (view *LogVFS) LogMode() LogTraversalMode {
	if view == nil || view.session == nil {
		return LogTraversalHeadDAG
	}
	view.session.mu.RLock()
	mode := view.session.logMode
	view.session.mu.RUnlock()
	return mode
}

// SetLogMode changes root traversal. It invalidates the lazy page so an
// explicit mode selection also observes current local refs on the next read.
func (view *LogVFS) SetLogMode(mode LogTraversalMode) error {
	if !mode.valid() {
		return fmt.Errorf("Git log: unknown traversal mode %d", mode)
	}
	if view == nil || view.session == nil {
		return os.ErrClosed
	}
	view.session.mu.Lock()
	view.session.logMode = mode
	delete(view.session.commitCache, mode)
	view.session.mu.Unlock()
	return nil
}

// ToggleLogMode cycles HEAD DAG, all local refs, then first parent.
func (view *LogVFS) ToggleLogMode() LogTraversalMode {
	if view == nil || view.session == nil {
		return LogTraversalHeadDAG
	}
	view.session.mu.Lock()
	mode := view.session.logMode.next()
	view.session.logMode = mode
	delete(view.session.commitCache, mode)
	view.session.mu.Unlock()
	return mode
}

// CommitTreeMode returns the current view below a commit without I/O.
func (view *LogVFS) CommitTreeMode() CommitTreeMode {
	if view == nil || view.session == nil {
		return CommitTreeChangedFiles
	}
	view.session.mu.RLock()
	mode := view.session.treeMode
	view.session.mu.RUnlock()
	return mode
}

func (view *LogVFS) SetCommitTreeMode(mode CommitTreeMode) error {
	if !mode.valid() {
		return fmt.Errorf("Git log: unknown commit tree mode %d", mode)
	}
	if view == nil || view.session == nil {
		return os.ErrClosed
	}
	view.session.mu.Lock()
	view.session.treeMode = mode
	view.session.mu.Unlock()
	return nil
}

// ToggleCommitTreeMode switches changed files and the full after-tree.
func (view *LogVFS) ToggleCommitTreeMode() CommitTreeMode {
	if view == nil || view.session == nil {
		return CommitTreeChangedFiles
	}
	view.session.mu.Lock()
	mode := view.session.treeMode.next()
	view.session.treeMode = mode
	view.session.mu.Unlock()
	return mode
}

// ToggleModeForPath is the F2-oriented API: at the history root it switches
// graph traversal; at a commit (or below it) it switches the tree listing.
func (view *LogVFS) ToggleModeForPath(value string) (LogTraversalMode, CommitTreeMode) {
	if view.resolvePath(value) == "/" {
		return view.ToggleLogMode(), view.CommitTreeMode()
	}
	return view.LogMode(), view.ToggleCommitTreeMode()
}

// Invalidate discards the cached root page. It is safe to call after a commit,
// checkout, or ref change; no session state is persisted to disk.
func (view *LogVFS) Invalidate() {
	if view == nil || view.session == nil {
		return
	}
	view.session.mu.Lock()
	view.session.commitCache = make(map[LogTraversalMode][]logCommit)
	view.session.mu.Unlock()
}

// SetOverlay records a session-only edit for a historical after-blob. It
// refuses deleted paths, gitlinks, and directories, leaving the repository
// untouched. The bytes are copied so caller buffers can be reused safely.
func (view *LogVFS) SetOverlay(ctx context.Context, value string, content []byte) error {
	if err := historyContextError(ctx); err != nil {
		return err
	}
	if view.isClosed() {
		return os.ErrClosed
	}
	file, err := view.fileAt(ctx, view.resolvePath(value))
	if err != nil {
		return err
	}
	if !file.after || !file.mode.IsFile() {
		return os.ErrPermission
	}
	copyContent := append([]byte(nil), content...)
	view.session.mu.Lock()
	view.session.overlays[overlayKey(file)] = copyContent
	view.session.mu.Unlock()
	return nil
}

// ClearOverlay removes an edited historical after-blob from this in-memory
// session. It verifies the path is a real, readable after-file first.
func (view *LogVFS) ClearOverlay(ctx context.Context, value string) error {
	if err := historyContextError(ctx); err != nil {
		return err
	}
	if view.isClosed() {
		return os.ErrClosed
	}
	file, err := view.fileAt(ctx, view.resolvePath(value))
	if err != nil {
		return err
	}
	if !file.after || !file.mode.IsFile() {
		return os.ErrPermission
	}
	view.session.mu.Lock()
	delete(view.session.overlays, overlayKey(file))
	view.session.mu.Unlock()
	return nil
}

// Overlay reports a copy of the session overlay for an already resolved
// history path. It intentionally performs no repository I/O, making it safe
// for UI state inspection. Open remains the authoritative path validator.
func (view *LogVFS) Overlay(value string) ([]byte, bool) {
	if view == nil || view.session == nil {
		return nil, false
	}
	commitHash, relative, err := parseHistoryPath(view.resolvePath(value))
	if err != nil || relative == "" {
		return nil, false
	}
	key := historyOverlayKey{commit: commitHash.String(), path: relative}
	return view.overlay(key)
}

func (view *LogVFS) commits(ctx context.Context) ([]logCommit, error) {
	if err := historyContextError(ctx); err != nil {
		return nil, err
	}
	if view == nil || view.session == nil {
		return nil, os.ErrClosed
	}
	view.session.mu.RLock()
	mode := view.session.logMode
	if cached, ok := view.session.commitCache[mode]; ok {
		copyCache := append([]logCommit(nil), cached...)
		view.session.mu.RUnlock()
		return copyCache, nil
	}
	limit := view.session.limit
	view.session.mu.RUnlock()

	repository, err := view.openRepository(ctx)
	if err != nil {
		return nil, err
	}
	defer repository.Close()

	var commits []logCommit
	switch mode {
	case LogTraversalHeadDAG:
		commits, err = collectHeadDAG(ctx, repository, limit)
	case LogTraversalAllLocalRefs:
		commits, err = collectAllLocalRefs(ctx, repository, limit)
	case LogTraversalFirstParent:
		commits, err = collectFirstParent(ctx, repository, limit)
	default:
		return nil, fmt.Errorf("Git log: unknown traversal mode %d", mode)
	}
	if err != nil {
		return nil, err
	}
	if err := historyContextError(ctx); err != nil {
		return nil, err
	}

	view.session.mu.Lock()
	if view.session.logMode == mode {
		view.session.commitCache[mode] = append([]logCommit(nil), commits...)
	}
	view.session.mu.Unlock()
	return commits, nil
}

func (view *LogVFS) commit(ctx context.Context, hash plumbing.Hash) (logCommit, error) {
	if err := historyContextError(ctx); err != nil {
		return logCommit{}, err
	}
	// Prefer the small root cache if the entry is already visible.
	view.session.mu.RLock()
	for _, cached := range view.session.commitCache[view.session.logMode] {
		if cached.hash.Equal(hash) {
			view.session.mu.RUnlock()
			return cached, nil
		}
	}
	view.session.mu.RUnlock()
	repository, err := view.openRepository(ctx)
	if err != nil {
		return logCommit{}, err
	}
	defer repository.Close()
	commit, err := repository.CommitObject(hash)
	if err != nil {
		return logCommit{}, err
	}
	if err := historyContextError(ctx); err != nil {
		return logCommit{}, err
	}
	return makeLogCommit(commit), nil
}

func (view *LogVFS) treeRows(ctx context.Context, hash plumbing.Hash, directory string) ([]historyRow, error) {
	if err := historyContextError(ctx); err != nil {
		return nil, err
	}
	repository, err := view.openRepository(ctx)
	if err != nil {
		return nil, err
	}
	defer repository.Close()
	commit, err := repository.CommitObject(hash)
	if err != nil {
		return nil, err
	}
	if err := historyContextError(ctx); err != nil {
		return nil, err
	}
	var files []historyFile
	switch view.CommitTreeMode() {
	case CommitTreeChangedFiles:
		files, err = changedHistoryFiles(ctx, commit)
	case CommitTreeFullSnapshot:
		files, err = snapshotHistoryFiles(ctx, commit)
	default:
		return nil, fmt.Errorf("Git log: unknown commit tree mode")
	}
	if err != nil {
		return nil, err
	}
	return directHistoryRows(files, directory, commit.Committer.When), nil
}

func (view *LogVFS) fileAt(ctx context.Context, virtualPath string) (historyFile, error) {
	commitHash, relative, err := parseHistoryPath(virtualPath)
	if err != nil {
		return historyFile{}, err
	}
	if relative == "" {
		return historyFile{}, os.ErrNotExist
	}
	directory := path.Dir(relative)
	if directory == "." {
		directory = ""
	}
	rows, err := view.treeRows(ctx, commitHash, directory)
	if err != nil {
		return historyFile{}, err
	}
	base := path.Base(relative)
	for _, row := range rows {
		if row.item.Name == base && row.file != nil {
			return *row.file, nil
		}
	}
	return historyFile{}, os.ErrNotExist
}

func (view *LogVFS) openRepository(ctx context.Context) (*gogit.Repository, error) {
	if err := historyContextError(ctx); err != nil {
		return nil, err
	}
	if view == nil || view.session == nil {
		return nil, os.ErrClosed
	}
	view.session.mu.RLock()
	root := view.session.repository.Root
	view.session.mu.RUnlock()
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("Git log: repository root is empty")
	}
	repository, err := gogit.PlainOpen(root)
	if err != nil {
		return nil, err
	}
	if err := historyContextError(ctx); err != nil {
		_ = repository.Close()
		return nil, err
	}
	return repository, nil
}

func (view *LogVFS) pathSnapshot() string {
	if view == nil {
		return "/"
	}
	view.mu.RLock()
	current := view.currentPath
	view.mu.RUnlock()
	if current == "" {
		return "/"
	}
	return current
}

func (view *LogVFS) resolvePath(value string) string {
	return cleanHistoryPath(value, view.pathSnapshot())
}

func (view *LogVFS) isClosed() bool {
	if view == nil {
		return true
	}
	view.mu.RLock()
	closed := view.closed
	view.mu.RUnlock()
	return closed
}

func (view *LogVFS) overlay(key historyOverlayKey) ([]byte, bool) {
	view.session.mu.RLock()
	data, ok := view.session.overlays[key]
	if ok {
		data = append([]byte(nil), data...)
	}
	view.session.mu.RUnlock()
	return data, ok
}

func (mode LogTraversalMode) valid() bool {
	return mode == LogTraversalHeadDAG || mode == LogTraversalAllLocalRefs || mode == LogTraversalFirstParent
}

func (mode LogTraversalMode) next() LogTraversalMode {
	switch mode {
	case LogTraversalHeadDAG:
		return LogTraversalAllLocalRefs
	case LogTraversalAllLocalRefs:
		return LogTraversalFirstParent
	default:
		return LogTraversalHeadDAG
	}
}

func (mode LogTraversalMode) String() string {
	switch mode {
	case LogTraversalHeadDAG:
		return "HEAD DAG"
	case LogTraversalAllLocalRefs:
		return "all local refs"
	case LogTraversalFirstParent:
		return "first parent"
	default:
		return "unknown"
	}
}

func (mode CommitTreeMode) valid() bool {
	return mode == CommitTreeChangedFiles || mode == CommitTreeFullSnapshot
}

func (mode CommitTreeMode) next() CommitTreeMode {
	if mode == CommitTreeChangedFiles {
		return CommitTreeFullSnapshot
	}
	return CommitTreeChangedFiles
}

func (mode CommitTreeMode) String() string {
	if mode == CommitTreeFullSnapshot {
		return "snapshot"
	}
	return "changed files"
}

func (commit logCommit) item(mode LogTraversalMode) vfs.VFSItem {
	attributes := map[string]string{
		"git.commit":       commit.hash.String(),
		"git.subject":      commit.subject,
		"git.author":       commit.author,
		"git.parents":      commit.parentText,
		"git.logTraversal": mode.String(),
	}
	return vfs.VFSItem{
		Name:               commit.hash.String(),
		DisplayName:        shortHistoryHash(commit.hash.String()) + "  " + commit.subject,
		ExtendedAttributes: attributes,
		IsDir:              true,
		MTime:              commit.committed,
		NoExtension:        true,
		Revision:           commit.hash.String(),
	}
}

func makeLogCommit(commit *object.Commit) logCommit {
	parents := make([]string, 0, len(commit.ParentHashes))
	for _, parent := range commit.ParentHashes {
		parents = append(parents, parent.String())
	}
	author := strings.TrimSpace(commit.Author.Name)
	if author == "" {
		author = strings.TrimSpace(commit.Author.Email)
	}
	return logCommit{
		hash:       commit.Hash,
		subject:    commitSubject(commit.Message),
		author:     author,
		committed:  commit.Committer.When,
		parentText: strings.Join(parents, " "),
	}
}

func commitSubject(message string) string {
	if line, _, found := strings.Cut(message, "\n"); found {
		message = line
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return "(no subject)"
	}
	return message
}

func collectHeadDAG(ctx context.Context, repository *gogit.Repository, limit int) ([]logCommit, error) {
	if err := historyContextError(ctx); err != nil {
		return nil, err
	}
	iterator, err := repository.Log(&gogit.LogOptions{Order: gogit.LogOrderCommitterTime})
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil, nil // unborn repository
	}
	if err != nil {
		return nil, err
	}
	defer iterator.Close()
	return collectCommitIterator(ctx, iterator, limit)
}

func collectFirstParent(ctx context.Context, repository *gogit.Repository, limit int) ([]logCommit, error) {
	head, err := repository.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil, nil // unborn repository
	}
	if err != nil {
		return nil, err
	}
	hash := head.Hash()
	commits := make([]logCommit, 0, limit)
	for len(commits) < limit && !hash.IsZero() {
		if err := historyContextError(ctx); err != nil {
			return nil, err
		}
		commit, err := repository.CommitObject(hash)
		if err != nil {
			return nil, err
		}
		commits = append(commits, makeLogCommit(commit))
		if len(commit.ParentHashes) == 0 {
			break
		}
		hash = commit.ParentHashes[0]
	}
	return commits, historyContextError(ctx)
}

// collectAllLocalRefs builds a bounded, timestamp-prioritised graph from
// local branches and tags. This intentionally avoids LogOptions.All because
// that option includes refs/remotes, which is surprising in a local-only mode.
func collectAllLocalRefs(ctx context.Context, repository *gogit.Repository, limit int) ([]logCommit, error) {
	if err := historyContextError(ctx); err != nil {
		return nil, err
	}
	tips := make(map[string]plumbing.Hash)
	refs, err := repository.References()
	if err != nil {
		return nil, err
	}
	defer refs.Close()
	for {
		if err := historyContextError(ctx); err != nil {
			return nil, err
		}
		ref, nextErr := refs.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return nil, nextErr
		}
		if !ref.Name().IsBranch() && !ref.Name().IsTag() {
			continue
		}
		hash, resolved, resolveErr := resolveCommitTip(ctx, repository, ref.Hash())
		if resolveErr != nil {
			return nil, resolveErr
		}
		if resolved {
			tips[hash.String()] = hash
		}
	}
	// A detached HEAD has no refs/heads name and still needs representation in
	// a local history panel.
	if head, headErr := repository.Head(); headErr == nil && !head.Hash().IsZero() {
		tips[head.Hash().String()] = head.Hash()
	} else if headErr != nil && !errors.Is(headErr, plumbing.ErrReferenceNotFound) {
		return nil, headErr
	}

	queue := make([]*object.Commit, 0, len(tips))
	queued := make(map[string]struct{})
	for _, hash := range tips {
		if err := historyContextError(ctx); err != nil {
			return nil, err
		}
		commit, err := repository.CommitObject(hash)
		if err != nil {
			return nil, err
		}
		queue = append(queue, commit)
		queued[hash.String()] = struct{}{}
	}

	commits := make([]logCommit, 0, limit)
	for len(queue) != 0 && len(commits) < limit {
		if err := historyContextError(ctx); err != nil {
			return nil, err
		}
		sort.SliceStable(queue, func(left, right int) bool {
			if queue[left].Committer.When.Equal(queue[right].Committer.When) {
				return queue[left].Hash.String() > queue[right].Hash.String()
			}
			return queue[left].Committer.When.After(queue[right].Committer.When)
		})
		commit := queue[0]
		queue = queue[1:]
		commits = append(commits, makeLogCommit(commit))
		for _, parentHash := range commit.ParentHashes {
			if _, seen := queued[parentHash.String()]; seen {
				continue
			}
			if err := historyContextError(ctx); err != nil {
				return nil, err
			}
			parent, err := repository.CommitObject(parentHash)
			if err != nil {
				return nil, err
			}
			queued[parentHash.String()] = struct{}{}
			queue = append(queue, parent)
		}
	}
	return commits, historyContextError(ctx)
}

func resolveCommitTip(ctx context.Context, repository *gogit.Repository, hash plumbing.Hash) (plumbing.Hash, bool, error) {
	for depth := 0; depth != 8; depth++ {
		if err := historyContextError(ctx); err != nil {
			return plumbing.ZeroHash, false, err
		}
		if _, err := repository.CommitObject(hash); err == nil {
			return hash, true, nil
		}
		tag, err := repository.TagObject(hash)
		if err != nil {
			// A local tag can name a tree or blob. It is not a commit history
			// tip, so omit it rather than making the entire log unusable.
			return plumbing.ZeroHash, false, nil
		}
		hash = tag.Target
	}
	return plumbing.ZeroHash, false, errors.New("Git log: annotated tag chain is too deep")
}

func collectCommitIterator(ctx context.Context, iterator object.CommitIter, limit int) ([]logCommit, error) {
	commits := make([]logCommit, 0, limit)
	for len(commits) < limit {
		if err := historyContextError(ctx); err != nil {
			return nil, err
		}
		commit, err := iterator.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		commits = append(commits, makeLogCommit(commit))
	}
	return commits, historyContextError(ctx)
}

func changedHistoryFiles(ctx context.Context, commit *object.Commit) ([]historyFile, error) {
	if err := historyContextError(ctx); err != nil {
		return nil, err
	}
	afterTree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	var beforeTree *object.Tree
	if len(commit.ParentHashes) != 0 {
		parent, err := commit.Parent(0)
		if err != nil {
			return nil, err
		}
		beforeTree, err = parent.Tree()
		if err != nil {
			return nil, err
		}
	}
	changes, err := object.DiffTreeContext(ctx, beforeTree, afterTree)
	if err != nil {
		return nil, err
	}
	files := make([]historyFile, 0, len(changes))
	for _, change := range changes {
		if err := historyContextError(ctx); err != nil {
			return nil, err
		}
		action, err := change.Action()
		if err != nil {
			return nil, err
		}
		file := historyFile{commitHash: commit.Hash, action: action.String()}
		if change.To.Name != "" {
			file.path = change.To.Name
			file.mode = change.To.TreeEntry.Mode
			file.blob = change.To.TreeEntry.Hash
			file.after = file.mode.IsFile()
			file.previous = change.From.Name
		} else {
			file.path = change.From.Name
			file.mode = change.From.TreeEntry.Mode
			file.action = "delete"
			file.previous = change.From.Name
		}
		if file.path == "" || file.mode == filemode.Dir || !safeHistoryRelativePath(file.path) {
			continue
		}
		files = append(files, file)
	}
	return files, nil
}

func snapshotHistoryFiles(ctx context.Context, commit *object.Commit) ([]historyFile, error) {
	if err := historyContextError(ctx); err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	walker := object.NewTreeWalker(tree, true, nil)
	defer walker.Close()
	files := make([]historyFile, 0)
	for {
		if err := historyContextError(ctx); err != nil {
			return nil, err
		}
		name, entry, nextErr := walker.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return nil, nextErr
		}
		if entry.Mode == filemode.Dir || !safeHistoryRelativePath(name) {
			continue
		}
		files = append(files, historyFile{
			path:       name,
			mode:       entry.Mode,
			blob:       entry.Hash,
			after:      entry.Mode.IsFile(),
			action:     "snapshot",
			commitHash: commit.Hash,
		})
	}
	return files, nil
}

func directHistoryRows(files []historyFile, directory string, modifiedAt time.Time) []historyRow {
	directory = strings.Trim(directory, "/")
	prefix := ""
	if directory != "" {
		prefix = directory + "/"
	}
	directories := make(map[string]historyRow)
	regular := make(map[string]historyRow)
	for index := range files {
		file := files[index]
		if !strings.HasPrefix(file.path, prefix) {
			continue
		}
		remainder := strings.TrimPrefix(file.path, prefix)
		if remainder == "" {
			continue
		}
		name, nested, isNested := strings.Cut(remainder, "/")
		if isNested {
			fullPath := name
			if directory != "" {
				fullPath = directory + "/" + name
			}
			directories[name] = historyRow{item: vfs.VFSItem{
				Name:        name,
				DisplayName: name,
				IsDir:       true,
				NoExtension: true,
				MTime:       modifiedAt,
				ExtendedAttributes: map[string]string{
					"git.path": fullPath,
					"git.kind": "directory",
				},
			}}
			_ = nested // the first child is all this directory needs.
			continue
		}
		attributes := map[string]string{
			"git.path":   file.path,
			"git.commit": file.commitHash.String(),
			"git.action": file.action,
			"git.mode":   file.mode.String(),
		}
		if file.previous != "" && file.previous != file.path {
			attributes["git.previousPath"] = file.previous
		}
		if !file.after {
			attributes["git.readOnly"] = "deleted"
		}
		if file.mode == filemode.Submodule {
			attributes["git.readOnly"] = "gitlink"
		}
		regular[name] = historyRow{
			item: vfs.VFSItem{
				Name:               name,
				DisplayName:        name,
				ExtendedAttributes: attributes,
				MTime:              modifiedAt,
				Mode:               file.mode.String(),
				IsExecutable:       file.mode == filemode.Executable,
				NoExtension:        true,
				Revision:           file.blob.String(),
			},
			file: &file,
		}
	}
	rows := make([]historyRow, 0, len(directories)+len(regular))
	for _, row := range directories {
		rows = append(rows, row)
	}
	for _, row := range regular {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(left, right int) bool {
		if rows[left].item.IsDir != rows[right].item.IsDir {
			return rows[left].item.IsDir
		}
		return rows[left].item.Name < rows[right].item.Name
	})
	return rows
}

func emitHistoryItems(ctx context.Context, items []vfs.VFSItem, onChunk func([]vfs.VFSItem)) error {
	// A bounded page is still emitted in smaller chunks so hosts can render the
	// first rows before a dense commit/tree page has been copied completely.
	const chunkSize = 64
	for start := 0; start < len(items); start += chunkSize {
		if err := historyContextError(ctx); err != nil {
			return err
		}
		end := start + chunkSize
		if end > len(items) {
			end = len(items)
		}
		onChunk(items[start:end])
	}
	return historyContextError(ctx)
}

func parseHistoryPath(value string) (plumbing.Hash, string, error) {
	cleaned := path.Clean(value)
	if !path.IsAbs(cleaned) {
		return plumbing.ZeroHash, "", os.ErrNotExist
	}
	trimmed := strings.TrimPrefix(cleaned, "/")
	if trimmed == "" || trimmed == "." {
		return plumbing.ZeroHash, "", os.ErrNotExist
	}
	parts := strings.Split(trimmed, "/")
	if !plumbing.IsHash(parts[0]) {
		return plumbing.ZeroHash, "", os.ErrNotExist
	}
	relative := strings.Join(parts[1:], "/")
	if relative != "" && !safeHistoryRelativePath(relative) {
		return plumbing.ZeroHash, "", os.ErrNotExist
	}
	return plumbing.NewHash(parts[0]), relative, nil
}

func cleanHistoryPath(value, current string) string {
	if strings.TrimSpace(value) == "" {
		return "/"
	}
	if path.IsAbs(value) {
		return path.Clean(value)
	}
	if current == "" {
		current = "/"
	}
	return path.Join(current, value)
}

func safeHistoryRelativePath(value string) bool {
	if value == "" || path.IsAbs(value) || path.Clean(value) != value {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func shortHistoryHash(value string) string {
	if len(value) > 10 {
		return value[:10]
	}
	return value
}

func overlayKey(file historyFile) historyOverlayKey {
	return historyOverlayKey{commit: file.commitHash.String(), path: file.path}
}

func readHistoryBlob(ctx context.Context, reader io.Reader) ([]byte, error) {
	buffer := make([]byte, 0, 32*1024)
	chunk := make([]byte, 32*1024)
	for {
		if err := historyContextError(ctx); err != nil {
			return nil, err
		}
		count, err := reader.Read(chunk)
		if count != 0 {
			buffer = append(buffer, chunk[:count]...)
		}
		if err == io.EOF {
			return buffer, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func historyContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

type historyReadAtCloser struct {
	mu     sync.Mutex
	data   []byte
	offset int64
}

func (reader *historyReadAtCloser) Size() int64 { return int64(len(reader.data)) }

func (reader *historyReadAtCloser) Read(ctx context.Context, target []byte) (int, error) {
	if err := historyContextError(ctx); err != nil {
		return 0, err
	}
	reader.mu.Lock()
	defer reader.mu.Unlock()
	if reader.offset >= int64(len(reader.data)) {
		return 0, io.EOF
	}
	count := copy(target, reader.data[reader.offset:])
	reader.offset += int64(count)
	if count < len(target) {
		return count, io.EOF
	}
	return count, nil
}

func (reader *historyReadAtCloser) ReadAt(ctx context.Context, target []byte, offset int64) (int, error) {
	if err := historyContextError(ctx); err != nil {
		return 0, err
	}
	if offset < 0 || offset >= int64(len(reader.data)) {
		return 0, io.EOF
	}
	count := copy(target, reader.data[offset:])
	if count < len(target) {
		return count, io.EOF
	}
	return count, nil
}

func (*historyReadAtCloser) Close() error { return nil }

var _ vfs.VFS = (*LogVFS)(nil)
var _ vfs.TitleProvider = (*LogVFS)(nil)
var _ vfs.PanelTitleProvider = (*LogVFS)(nil)
