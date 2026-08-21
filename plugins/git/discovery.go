// Package gitplugin contains the built-in Git integration for f4.
package gitplugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// DefaultUpwardSearchDelay is deliberately long enough that ordinary folder
// navigation is never held up by a walk towards a repository root.
const DefaultUpwardSearchDelay = 300 * time.Millisecond

const maxGitDirFileSize = 64 << 10

// LookupState describes how much the session cache knows about a directory.
// Unknown never means that the directory is not a repository; it only means
// that no completed observation has cached an answer yet.
type LookupState uint8

const (
	LookupUnknown LookupState = iota
	LookupNotRepository
	LookupRepository
)

// Branch is a cached HEAD presentation value. Name is a branch name for a
// symbolic HEAD and the full object ID for a detached HEAD.
type Branch struct {
	Name     string
	Detached bool
	// Unborn is true for a symbolic branch name whose reference does not yet
	// resolve to an object. This is the state immediately after git init (or
	// after checking out an orphan branch), and it is useful prompt context
	// without treating the repository as broken.
	Unborn bool
}

// Prompt returns the compact form suitable for a command prompt. It performs
// no repository access and is safe to call from UI rendering code.
func (branch Branch) Prompt() string {
	if branch.Name == "" {
		return ""
	}
	if !branch.Detached {
		return branch.Name
	}
	short := branch.Name
	if len(short) > 7 {
		short = short[:7]
	}
	return "@" + short
}

// Repository identifies one working tree and its Git administrative paths.
// All paths are absolute and cleaned, but retain their display casing.
type Repository struct {
	Root      string
	GitDir    string
	CommonDir string
	Branch    Branch
}

// LookupResult is a value snapshot from the session-only discovery cache.
type LookupResult struct {
	State      LookupState
	Repository Repository
}

// Found reports whether the cache contains a repository for the queried
// directory.
func (result LookupResult) Found() bool {
	return result.State == LookupRepository
}

// DiscoveryOptions configures a RepositoryDiscovery. A zero UpwardDelay is
// valid and useful for deterministic tests.
type DiscoveryOptions struct {
	UpwardDelay time.Duration
	// BranchDelay holds branch presentation back after an immediate local .git
	// hit. Repository identity is still cached and delivered right away, while
	// prompt rendering gets its branch value only after this debounce. A zero
	// value is useful for deterministic callers and tests.
	BranchDelay time.Duration
}

// DiscoveryUpdate is delivered from a worker goroutine after a complete
// observation. UI callers must marshal it onto their own UI task queue.
type DiscoveryUpdate struct {
	ObserverID string
	Generation uint64
	Directory  string
	Result     LookupResult
	Err        error
}

// RepositoryDiscovery keeps only in-memory answers for the running f4
// session. It intentionally has no dependency on a particular UI framework
// or on git.exe.
//
// An observer ID normally denotes a panel. Starting a new observation with
// the same ID cancels the prior one and increments its generation, while
// observations for other IDs continue independently.
type RepositoryDiscovery struct {
	mu          sync.RWMutex
	cache       map[string]LookupResult
	observers   map[string]observation
	nextGen     uint64
	upwardDelay time.Duration
	branchDelay time.Duration
	closed      bool
}

type observation struct {
	generation uint64
	cancel     context.CancelFunc
}

// NewRepositoryDiscovery creates a session-only repository cache.
func NewRepositoryDiscovery(options ...DiscoveryOptions) *RepositoryDiscovery {
	delay := DefaultUpwardSearchDelay
	branchDelay := DefaultUpwardSearchDelay
	if len(options) > 0 {
		delay = options[0].UpwardDelay
		branchDelay = options[0].BranchDelay
	}
	if delay < 0 {
		delay = 0
	}
	if branchDelay < 0 {
		branchDelay = 0
	}
	return &RepositoryDiscovery{
		cache:       make(map[string]LookupResult),
		observers:   make(map[string]observation),
		upwardDelay: delay,
		branchDelay: branchDelay,
	}
}

// Observe schedules a fresh check for directory. The current directory is
// probed immediately in a worker; only an unsuccessful probe waits for the
// configured delay before walking towards its parents. No filesystem access
// occurs before this method returns.
//
// notify, when non-nil, is never invoked for a superseded generation.
func (discovery *RepositoryDiscovery) Observe(ctx context.Context, observerID, directory string, notify func(DiscoveryUpdate)) (uint64, error) {
	if discovery == nil {
		return 0, errors.New("git discovery: nil service")
	}
	if strings.TrimSpace(observerID) == "" {
		return 0, errors.New("git discovery: observer ID is required")
	}
	normalized, err := normalizeDirectory(directory)
	if err != nil {
		return 0, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	childContext, cancel := context.WithCancel(ctx)
	discovery.mu.Lock()
	if discovery.closed {
		discovery.mu.Unlock()
		cancel()
		return 0, errors.New("git discovery: closed")
	}
	previous, hadPrevious := discovery.observers[observerID]
	discovery.nextGen++
	generation := discovery.nextGen
	discovery.observers[observerID] = observation{generation: generation, cancel: cancel}
	discovery.mu.Unlock()
	if hadPrevious {
		previous.cancel()
	}

	go discovery.observe(childContext, observerID, generation, normalized, notify)
	return generation, nil
}

// Cancel stops the active observation for observerID. Cached completed
// answers are deliberately retained for the rest of the session.
func (discovery *RepositoryDiscovery) Cancel(observerID string) {
	if discovery == nil {
		return
	}
	discovery.mu.Lock()
	entry, ok := discovery.observers[observerID]
	if ok {
		delete(discovery.observers, observerID)
	}
	discovery.mu.Unlock()
	if ok {
		entry.cancel()
	}
}

// Lookup reads the completed session cache only. In particular, it never
// stats .git, reads HEAD, or otherwise performs filesystem I/O.
func (discovery *RepositoryDiscovery) Lookup(directory string) LookupResult {
	if discovery == nil {
		return LookupResult{State: LookupUnknown}
	}
	normalized, err := normalizeDirectory(directory)
	if err != nil {
		return LookupResult{State: LookupUnknown}
	}
	discovery.mu.RLock()
	result, ok := discovery.cache[normalized.key]
	discovery.mu.RUnlock()
	if !ok {
		return LookupResult{State: LookupUnknown}
	}
	return result
}

// CachedBranch returns the cached branch state for directory. Like Lookup it
// does not perform I/O and is suitable for a command-prompt render path.
func (discovery *RepositoryDiscovery) CachedBranch(directory string) (Branch, bool) {
	result := discovery.Lookup(directory)
	if !result.Found() {
		return Branch{}, false
	}
	return result.Repository.Branch, true
}

// Close cancels unfinished observations and releases the active observer
// registry. The completed cache is intentionally not persisted anywhere.
func (discovery *RepositoryDiscovery) Close() {
	if discovery == nil {
		return
	}
	discovery.mu.Lock()
	if discovery.closed {
		discovery.mu.Unlock()
		return
	}
	discovery.closed = true
	observers := discovery.observers
	discovery.observers = make(map[string]observation)
	discovery.mu.Unlock()
	for _, entry := range observers {
		entry.cancel()
	}
}

func (discovery *RepositoryDiscovery) observe(ctx context.Context, observerID string, generation uint64, start normalizedDirectory, notify func(DiscoveryUpdate)) {
	repository, found, err := inspectRepository(ctx, start.path)
	if err != nil {
		discovery.emit(ctx, observerID, generation, start.path, LookupResult{State: LookupUnknown}, err, notify)
		return
	}
	if found {
		discovery.publishFound(ctx, observerID, generation, start, []normalizedDirectory{start}, repository, true, notify)
		return
	}

	if discovery.upwardDelay > 0 {
		timer := time.NewTimer(discovery.upwardDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}

	walked := []normalizedDirectory{start}
	for candidate := filepath.Dir(start.path); ; candidate = filepath.Dir(candidate) {
		if err := ctx.Err(); err != nil {
			return
		}
		normalized, normalizeErr := normalizeDirectory(candidate)
		if normalizeErr != nil {
			discovery.emit(ctx, observerID, generation, start.path, LookupResult{State: LookupUnknown}, normalizeErr, notify)
			return
		}
		walked = append(walked, normalized)
		repository, found, err = inspectRepository(ctx, normalized.path)
		if err != nil {
			discovery.emit(ctx, observerID, generation, start.path, LookupResult{State: LookupUnknown}, err, notify)
			return
		}
		if found {
			// The upward debounce already elapsed while discovering this root,
			// so exposing its branch now does not add a second needless delay.
			discovery.publishFound(ctx, observerID, generation, start, walked, repository, false, notify)
			return
		}
		parent := filepath.Dir(normalized.path)
		if parent == normalized.path {
			break
		}
	}

	result := LookupResult{State: LookupNotRepository}
	if discovery.store(observerID, generation, walked, result) {
		discovery.emit(ctx, observerID, generation, start.path, result, nil, notify)
	}
}

func (discovery *RepositoryDiscovery) publishFound(
	ctx context.Context,
	observerID string,
	generation uint64,
	start normalizedDirectory,
	paths []normalizedDirectory,
	repository Repository,
	delayBranch bool,
	notify func(DiscoveryUpdate),
) {
	full := LookupResult{State: LookupRepository, Repository: repository}
	initial := full
	if delayBranch && discovery.branchDelay > 0 && repository.Branch.Name != "" {
		// The local repository result is enough for status/decorations. Keep
		// prompt state empty until the debounce expires so rapid folder
		// navigation does not cause branch labels to flicker.
		initial.Repository.Branch = Branch{}
	}
	if discovery.store(observerID, generation, paths, initial) {
		discovery.emit(ctx, observerID, generation, start.path, initial, nil, notify)
	}
	if initial.Repository.Branch == full.Repository.Branch || ctx.Err() != nil {
		return
	}
	timer := time.NewTimer(discovery.branchDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	if discovery.store(observerID, generation, paths, full) {
		discovery.emit(ctx, observerID, generation, start.path, full, nil, notify)
	}
}

func (discovery *RepositoryDiscovery) store(observerID string, generation uint64, paths []normalizedDirectory, result LookupResult) bool {
	discovery.mu.Lock()
	defer discovery.mu.Unlock()
	if discovery.closed || !discovery.currentLocked(observerID, generation) {
		return false
	}
	for _, path := range paths {
		discovery.cache[path.key] = result
	}
	return true
}

func (discovery *RepositoryDiscovery) emit(ctx context.Context, observerID string, generation uint64, directory string, result LookupResult, err error, notify func(DiscoveryUpdate)) {
	if notify == nil || ctx.Err() != nil {
		return
	}
	discovery.mu.RLock()
	current := !discovery.closed && discovery.currentLocked(observerID, generation)
	discovery.mu.RUnlock()
	if current && ctx.Err() == nil {
		notify(DiscoveryUpdate{
			ObserverID: observerID,
			Generation: generation,
			Directory:  directory,
			Result:     result,
			Err:        err,
		})
	}
}

func (discovery *RepositoryDiscovery) currentLocked(observerID string, generation uint64) bool {
	entry, ok := discovery.observers[observerID]
	return ok && entry.generation == generation
}

type normalizedDirectory struct {
	path string
	key  string
}

func normalizeDirectory(directory string) (normalizedDirectory, error) {
	return normalizeDirectoryWithAbs(directory, filepath.Abs)
}

// normalizeDirectoryWithAbs keeps the cache lookup path free of Win32 path
// resolution. A panel's OSVFS already supplies an absolute path, and calling
// filepath.Abs for it on every redraw invokes GetFullPathNameW on Windows.
// The injectable resolver makes that no-syscall guarantee regression-testable
// without a mutable global test hook.
func normalizeDirectoryWithAbs(directory string, absolutePath func(string) (string, error)) (normalizedDirectory, error) {
	if strings.TrimSpace(directory) == "" {
		return normalizedDirectory{}, errors.New("git discovery: directory is required")
	}
	absolute := directory
	if !filepath.IsAbs(absolute) {
		var err error
		absolute, err = absolutePath(directory)
		if err != nil {
			return normalizedDirectory{}, fmt.Errorf("git discovery: resolve %q: %w", directory, err)
		}
	}
	absolute = filepath.Clean(absolute)
	key := absolute
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	return normalizedDirectory{path: absolute, key: key}, nil
}

func inspectRepository(ctx context.Context, root string) (Repository, bool, error) {
	if err := ctx.Err(); err != nil {
		return Repository{}, false, err
	}
	marker := filepath.Join(root, ".git")
	info, err := os.Stat(marker)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Repository{}, false, nil
		}
		return Repository{}, false, fmt.Errorf("git discovery: inspect %q: %w", marker, err)
	}

	var gitDir string
	switch {
	case info.IsDir():
		gitDir = marker
	case info.Mode().IsRegular():
		resolved, resolvedOK, resolveErr := resolveGitDirFile(ctx, marker)
		if resolveErr != nil {
			return Repository{}, false, resolveErr
		}
		if !resolvedOK {
			return Repository{}, false, nil
		}
		gitDir = resolved
	default:
		return Repository{}, false, nil
	}

	gitDir, err = absoluteClean(gitDir)
	if err != nil {
		return Repository{}, false, err
	}
	repository := Repository{Root: root, GitDir: gitDir, CommonDir: gitDir}
	if commonDir, ok := readCommonDir(ctx, gitDir); ok {
		repository.CommonDir = commonDir
	}
	if branch, branchErr := readHead(ctx, gitDir, repository.CommonDir); branchErr == nil {
		repository.Branch = branch
	}
	return repository, true, nil
}

func resolveGitDirFile(ctx context.Context, marker string) (string, bool, error) {
	contents, err := readSmallFile(ctx, marker)
	if err != nil {
		return "", false, fmt.Errorf("git discovery: read %q: %w", marker, err)
	}
	line := strings.TrimSpace(strings.SplitN(string(contents), "\n", 2)[0])
	if !strings.HasPrefix(line, "gitdir:") {
		return "", false, nil
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
	if gitDir == "" {
		return "", false, nil
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(filepath.Dir(marker), gitDir)
	}
	gitDir, err = absoluteClean(gitDir)
	if err != nil {
		return "", false, err
	}
	info, err := os.Stat(gitDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("git discovery: inspect gitdir %q: %w", gitDir, err)
	}
	if !info.IsDir() {
		return "", false, nil
	}
	return gitDir, true, nil
}

func readCommonDir(ctx context.Context, gitDir string) (string, bool) {
	contents, err := readSmallFile(ctx, filepath.Join(gitDir, "commondir"))
	if err != nil {
		return "", false
	}
	value := strings.TrimSpace(string(contents))
	if value == "" {
		return "", false
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(gitDir, value)
	}
	value, err = absoluteClean(value)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(value)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return value, true
}

func readHead(ctx context.Context, gitDir, commonDir string) (Branch, error) {
	contents, err := readSmallFile(ctx, filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return Branch{}, err
	}
	head := strings.TrimSpace(string(contents))
	const prefix = "ref:"
	if strings.HasPrefix(head, prefix) {
		ref := strings.TrimSpace(strings.TrimPrefix(head, prefix))
		if ref == "" {
			return Branch{}, nil
		}
		if branch, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
			return Branch{Name: branch, Unborn: !referenceExists(ctx, gitDir, commonDir, ref)}, nil
		}
		return Branch{Name: ref}, nil
	}
	if isObjectID(head) {
		return Branch{Name: head, Detached: true}, nil
	}
	return Branch{}, nil
}

// referenceExists intentionally treats an unreadable packed-refs file as an
// existing reference. A false "unborn" prompt is more confusing than simply
// omitting that extra annotation while the administrative file is unavailable.
func referenceExists(ctx context.Context, gitDir, commonDir, ref string) bool {
	if err := ctx.Err(); err != nil {
		return true
	}
	for _, base := range []string{gitDir, commonDir} {
		if base == "" {
			continue
		}
		if info, err := os.Stat(filepath.Join(base, filepath.FromSlash(ref))); err == nil && !info.IsDir() {
			return true
		}
	}
	for _, base := range []string{gitDir, commonDir} {
		if base == "" {
			continue
		}
		contents, err := readSmallFile(ctx, filepath.Join(base, "packed-refs"))
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return true
			}
			continue
		}
		for _, line := range strings.Split(string(contents), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && fields[1] == ref && isObjectID(fields[0]) {
				return true
			}
		}
	}
	return false
}

func readSmallFile(ctx context.Context, filename string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, maxGitDirFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(contents) > maxGitDirFileSize {
		return nil, fmt.Errorf("file exceeds %d bytes", maxGitDirFileSize)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return contents, nil
}

func absoluteClean(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("git discovery: resolve %q: %w", path, err)
	}
	return filepath.Clean(absolute), nil
}

func isObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')) {
			return false
		}
	}
	return true
}
