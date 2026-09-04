package vfs

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"path/filepath"
	"strings"

	"runtime"

	"github.com/unxed/vtui"
)

type OSVFS struct {
	currentPath string
}

var _ OptimisticPathSetter = (*OSVFS)(nil)
var _ PhasedDirectoryReader = (*OSVFS)(nil)
var _ PhasedDirectoryReadProvider = (*OSVFS)(nil)
var _ WindowedDirectoryReader = (*OSVFS)(nil)
var _ WindowedDirectoryReadProvider = (*OSVFS)(nil)

// DirectoryReadPhase identifies which part of a phased directory listing a
// chunk belongs to. The base phase contains only stable row identity and type
// information. The metadata phase enriches those same rows without changing
// their names, directory classification, or order.
type DirectoryReadPhase uint8

const (
	DirectoryReadBase DirectoryReadPhase = iota
	DirectoryReadMetadata
	// DirectoryReadPreview is an optional, provisional leading window. It is
	// never authoritative and is followed by DirectoryReadBase. Readers may use
	// it to overlap rendering a stable name-sorted prefix with the remainder of
	// a very large local directory enumeration.
	DirectoryReadPreview
)

// PhasedDirectoryReader is an optional VFS capability. It lets a panel publish
// an interactive names/types catalog before per-entry FileInfo work completes.
// ReadDir remains the compatibility path for every provider that does not opt
// in. Metadata chunks must refer to rows previously reported in the base phase
// and must preserve Name, IsDir, and NoExtension. Base IsHidden must already be
// authoritative so a panel can apply its visibility policy before enrichment.
type PhasedDirectoryReader interface {
	ReadDirPhased(context.Context, string, func(DirectoryReadPhase, []VFSItem)) error
}

// PhasedDirectoryReadProvider returns the concrete reader that owns the
// capability. Requiring this explicit self-provider avoids accidentally using
// a promoted OSVFS method on a wrapper which overrides ReadDir with different
// semantics.
type PhasedDirectoryReadProvider interface {
	PhasedDirectoryReader() PhasedDirectoryReader
}

// DirectoryWindowRequest describes the only catalog prefix needed before a
// large local directory has been materialized into the panel's complete source
// model. The request is deliberately about source rows, not rendered delegates.
type DirectoryWindowRequest struct {
	Limit         int
	IncludeHidden bool
	Writer        DirectoryItemWriter
}

// DirectoryItemWriter receives the complete authoritative source catalog
// directly after the bounded window has been published. It eliminates an
// otherwise redundant full []VFSItem allocation; implementations own the one
// source store they fill and must not retain the writer after ReadDirWindowed.
type DirectoryItemWriter interface {
	BeginDirectoryItems(total int)
	WriteDirectoryItem(index int, item VFSItem)
	EndDirectoryItems()
}

// DirectoryWindow is an authoritative, name-sorted prefix paired with the
// exact logical source row count. Entries never contains more than the
// requested limit, while TotalCount describes the complete filtered catalog.
type DirectoryWindow struct {
	Entries    []VFSItem
	TotalCount int
}

// WindowedDirectoryReader optionally exposes an authoritative bounded window
// before it materializes the complete []VFSItem result. The ordinary phased
// callback still follows with the complete base, preserving the VFS contract
// for terminal and non-paged consumers.
type WindowedDirectoryReader interface {
	ReadDirWindowed(
		context.Context,
		string,
		DirectoryWindowRequest,
		func(DirectoryWindow),
		func(DirectoryReadPhase, []VFSItem),
	) error
}

// WindowedDirectoryReadProvider follows the same explicit self-provider rule
// as PhasedDirectoryReadProvider so wrappers cannot accidentally advertise a
// promoted OSVFS optimization for a different ReadDir implementation.
type WindowedDirectoryReadProvider interface {
	WindowedDirectoryReader() WindowedDirectoryReader
}

// OSVFSSetPathBenchmarkHook observes the synchronous validation stages in
// SetPath. It is nil during normal operation so the VFS package does not need
// to depend on the application's tracing implementation.
var OSVFSSetPathBenchmarkHook func(event string, fields ...any)

func osVFSSetPathBenchmarkEvent(event string, fields ...any) {
	if OSVFSSetPathBenchmarkHook != nil {
		OSVFSSetPathBenchmarkHook(event, fields...)
	}
}

func NewOSVFS(initialPath string) *OSVFS {
	abs, _ := filepath.Abs(initialPath)
	return &OSVFS{currentPath: abs}
}

func (v *OSVFS) GetPath() string        { return v.currentPath }
func (v *OSVFS) IsAbs(path string) bool { return filepath.IsAbs(path) }

func (v *OSVFS) PhasedDirectoryReader() PhasedDirectoryReader { return v }

func (v *OSVFS) WindowedDirectoryReader() WindowedDirectoryReader { return v }

// SetPathOptimistic changes the local view without probing the filesystem.
// FileSystemPanel only uses this optional fast path after it has obtained a
// directory path from an authoritative panel row (or another already accepted
// navigation target). The following background ReadDir is deliberately the
// validation point: if the row became stale, the panel's normal load-failure
// recovery restores the parent and reports the error.
func (v *OSVFS) SetPathOptimistic(path string) error {
	target := path
	if !filepath.IsAbs(path) && filepath.VolumeName(path) == "" {
		target = filepath.Join(v.currentPath, path)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	v.currentPath = abs
	return nil
}

func (v *OSVFS) IsAtRoot() bool {
	if runtime.GOOS == "windows" {
		vol := filepath.VolumeName(v.currentPath)
		p := filepath.Clean(v.currentPath)
		// Standardize to backslash for comparison on Windows
		p = strings.ReplaceAll(p, "/", "\\")
		vol = strings.ReplaceAll(vol, "/", "\\")
		return p == vol || p == vol+"." || p == vol+"\\" || p == "\\"
	}
	return v.currentPath == "/"
}

func (v *OSVFS) SetPath(path string) error {
	vtui.DebugLog("VFS: SetPath(%q) called", path)
	target := path
	if !filepath.IsAbs(path) && filepath.VolumeName(path) == "" {
		target = filepath.Join(v.currentPath, path)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return err
	}

	// Сначала пробуем проверить оригинальный путь напрямую.
	// Если он существует, доступен и является директорией (симлинком на нее),
	// мы сохраняем оригинальный визуальный путь в панели без принудительного разыменования!
	osVFSSetPathBenchmarkEvent("osvfs.stat_initial.begin", "path", abs)
	initialStat, errStat := os.Stat(prepareOSPath(abs))
	initialFields := []any{"path", abs, "ok", errStat == nil, "isDir", errStat == nil && initialStat.IsDir()}
	if errStat != nil {
		initialFields = append(initialFields, "error", errStat.Error())
	}
	osVFSSetPathBenchmarkEvent("osvfs.stat_initial.end", initialFields...)
	if errStat == nil && initialStat.IsDir() {
		goto verify
	}

	// Если мы получили ошибку (например, Permission Denied на системном джанкшене Windows
	// "Documents and Settings"), то только тогда пытаемся принудительно разыменовать симлинк.
	if resolved, errEval := filepath.EvalSymlinks(prepareOSPath(abs)); errEval == nil {
		resolved = stripExtendedPrefix(resolved)
		if runtime.GOOS == "windows" {
			origVol := filepath.VolumeName(abs)
			resVol := filepath.VolumeName(resolved)
			// Prevent resolving mapped drives (e.g. T:\) into UNC paths (\\server\share)
			if len(origVol) == 2 && origVol[1] == ':' && len(resVol) > 2 && strings.HasPrefix(resVol, `\\`) {
				abs = origVol + strings.TrimPrefix(resolved, resVol)
			} else {
				abs = resolved
			}
		} else {
			abs = resolved
		}
		goto verify
	}

	// Windows fallbacks when EvalSymlinks fails (e.g. protected junctions)
	if runtime.GOOS == "windows" {
		// 1. wellKnownJunction (string comparison, no syscall)
		if link, ok := wellKnownJunction(abs); ok {
			vtui.DebugLog("VFS: SetPath: resolved via wellKnownJunction: %q -> %q", abs, link)
			abs = link
			goto verify
		}
		// 2. os.Readlink
		if link, errRead := os.Readlink(abs); errRead == nil {
			vtui.DebugLog("VFS: SetPath: resolved via Readlink: %q -> %q", abs, link)
			if filepath.IsAbs(link) {
				abs = link
			} else {
				abs = filepath.Join(filepath.Dir(abs), link)
			}
			goto verify
		}
		// 3. Direct syscall (CreateFile + DeviceIoControl)
		if link, errJunc := resolveWindowsJunction(abs); errJunc == nil {
			vtui.DebugLog("VFS: SetPath: resolved via resolveWindowsJunction: %q -> %q", abs, link)
			abs = link
			goto verify
		}
	}

verify:
	osVFSSetPathBenchmarkEvent("osvfs.stat_verify.begin", "path", abs)
	st, err := os.Stat(prepareOSPath(abs))
	verifyFields := []any{"path", abs, "ok", err == nil, "isDir", err == nil && st.IsDir()}
	if err != nil {
		verifyFields = append(verifyFields, "error", err.Error())
	}
	osVFSSetPathBenchmarkEvent("osvfs.stat_verify.end", verifyFields...)
	if err != nil {
		if os.IsPermission(err) && globalSudoClient.IsAvailable() {
			vtui.DebugLog("VFS: SetPath: Permission denied for %q, checking via sudo...", abs)
			item, sudoErr := globalSudoClient.Stat(prepareOSPath(abs))
			if sudoErr == nil {
				if item.IsDir {
					vtui.DebugLog("VFS: Path changed to %q (via sudo Stat)", abs)
					v.currentPath = abs
					return nil
				}
				vtui.DebugLog("VFS: SetPath(%q) FAILED: not a directory (via sudo Stat)", abs)
				return os.ErrInvalid
			}
			return sudoErr
		}
		return err
	}
	if !st.IsDir() {
		vtui.DebugLog("VFS: SetPath(%q) FAILED: not a directory", abs)
		return os.ErrInvalid
	}
	vtui.DebugLog("VFS: Path changed to %q", abs)
	v.currentPath = abs
	return nil
}

func (v *OSVFS) ReadDir(ctx context.Context, path string, onChunk func([]VFSItem)) error {
	// Try to open the directory
	dirPath := path
	f, err := os.Open(prepareOSPath(dirPath))
	if err != nil && os.IsPermission(err) && runtime.GOOS == "windows" {
		// Try to resolve protected junctions (e.g. "Documents and Settings")
		if resolved, ok := wellKnownJunction(dirPath); ok {
			vtui.DebugLog("VFS: ReadDir: resolved junction %q -> %q", dirPath, resolved)
			dirPath = resolved
			f, err = os.Open(prepareOSPath(dirPath))
		}
	}
	if err != nil {
		if os.IsPermission(err) && globalSudoClient.IsAvailable() {
			vtui.DebugLog("VFS: Permission denied for ReadDir(%q), attempting sudo...", dirPath)
			items, sudoErr := globalSudoClient.ReadDir(prepareOSPath(dirPath))
			if sudoErr == nil {
				vtui.DebugLog("VFS: Sudo ReadDir(%q) SUCCESS, items: %d", dirPath, len(items))
				if len(items) > 0 && onChunk != nil {
					onChunk(items)
				}
				return nil
			}
			vtui.DebugLog("VFS: Sudo ReadDir(%q) FAILED: %v", dirPath, sudoErr)
		} else {
			vtui.DebugLog("VFS: ReadDir(%q) FAILED: %v (Permission: %v, SudoAvailable: %v)", dirPath, err, os.IsPermission(err), globalSudoClient.IsAvailable())
		}
		return err
	}
	defer f.Close()

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		entries, err := f.ReadDir(1000)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		items := make([]VFSItem, 0, len(entries))
		for _, e := range entries {
			info, _ := e.Info()
			var size int64
			var mtime time.Time
			var isExec bool
			if info != nil {
				size = info.Size()
				mtime = info.ModTime()
				isExec = info.Mode().Perm()&0111 != 0
			}
			isDir := e.IsDir()
			isSymlink := e.Type()&os.ModeSymlink != 0
			// Windows NTFS junctions are reparse points but Go doesn't
			// always report them via ModeSymlink — the classification
			// has drifted across releases (ModeSymlink / ModeIrregular).
			// Treat the FILE_ATTRIBUTE_REPARSE_POINT bit as authoritative
			// so the scanner's leaf mode actually stops at things like
			// C:\Users\<user>\AppData\Local\Application Data instead of
			// walking into the self-loop.
			if !isSymlink && info != nil && isReparsePoint(info) {
				isSymlink = true
			}
			// If it's not a direct directory, it might be a symlink or a Windows Junction.
			// If it's not a regular file, ask the OS to resolve the final target.
			if !isDir && !e.Type().IsRegular() {
				if target, err := os.Stat(filepath.Join(dirPath, e.Name())); err == nil {
					isDir = target.IsDir()
				}
			}

			entryPath := filepath.Join(dirPath, e.Name())
			item := VFSItem{
				Name:         e.Name(),
				Size:         size,
				SizeKnown:    true,
				IsDir:        isDir,
				IsSymlink:    isSymlink,
				MTime:        mtime,
				IsExecutable: isExec,
				IsHidden:     isHidden(entryPath, e.Name(), info),
			}
			// Cheap variant: on Unix stat.Blocks is already loaded
			// alongside FileInfo, so filling PhysicalSize here is free.
			// On Windows this is a no-op — the scan path pays for
			// GetCompressedFileSize lazily via Stat() when it actually
			// needs the number (see vfs/scanner.go).
			fillPhysicalSizeCheap(&item, info)
			items = append(items, item)
		}

		if len(items) > 0 && onChunk != nil {
			onChunk(items)
		}
	}
	return nil
}

type phasedOSDirEntry struct {
	entry os.DirEntry
	base  VFSItem
	info  os.FileInfo
}

// ReadDirPhased reports a complete names/types catalog first, then enriches
// rows whose metadata was not already available. On Unix, entries with a
// conclusive DirEntry type reach the base callback without DirEntry.Info;
// ambiguous and special entries fall back to Info because their directory
// classification affects navigation. Windows' ReadDir implementation
// already carries WIN32_FIND_DATA in each DirEntry, so Info is a zero-I/O view
// of that enumeration record and is used to make the base row complete. This
// keeps the first UI commit from being followed by a redundant full-catalog
// metadata mutation for large local directories.
func (v *OSVFS) ReadDirPhased(ctx context.Context, path string, onChunk func(DirectoryReadPhase, []VFSItem)) error {
	return v.readDirPhased(ctx, path, DirectoryWindowRequest{}, nil, onChunk)
}

// ReadDirWindowed is the bounded-catalog variant used by native paged panels.
// Unsupported filesystems simply take the existing portable phased path and
// omit the early window callback.
func (v *OSVFS) ReadDirWindowed(
	ctx context.Context,
	path string,
	request DirectoryWindowRequest,
	onWindow func(DirectoryWindow),
	onChunk func(DirectoryReadPhase, []VFSItem),
) error {
	return v.readDirPhased(ctx, path, request, onWindow, onChunk)
}

func (v *OSVFS) readDirPhased(
	ctx context.Context,
	path string,
	windowRequest DirectoryWindowRequest,
	onWindow func(DirectoryWindow),
	onChunk func(DirectoryReadPhase, []VFSItem),
) error {
	dirPath := path
	// Windows can expose the complete WIN32 directory record directly. Avoid
	// wrapping every row in os.DirEntry/os.FileInfo interfaces only to unpack
	// the same record again below; on very large local directories those
	// transient objects dominate the cold navigation path. Other platforms (or
	// unsupported Windows filesystems) keep using the portable implementation.
	if items, handled, streamed, fastErr := readCompleteOSDirectoryBaseWindowed(
		ctx,
		prepareOSPath(dirPath),
		windowRequest,
		func(preview []VFSItem) {
			if onChunk != nil {
				onChunk(DirectoryReadPreview, preview)
			}
		},
		onWindow,
	); handled {
		if fastErr != nil {
			return fastErr
		}
		if onChunk != nil && !streamed {
			onChunk(DirectoryReadBase, items)
		}
		return ctx.Err()
	}
	f, err := os.Open(prepareOSPath(dirPath))
	if err != nil && os.IsPermission(err) && runtime.GOOS == "windows" {
		if resolved, ok := wellKnownJunction(dirPath); ok {
			vtui.DebugLog("VFS: ReadDirPhased: resolved junction %q -> %q", dirPath, resolved)
			dirPath = resolved
			f, err = os.Open(prepareOSPath(dirPath))
		}
	}
	if err != nil {
		if os.IsPermission(err) && globalSudoClient.IsAvailable() {
			vtui.DebugLog("VFS: Permission denied for ReadDirPhased(%q), attempting sudo...", dirPath)
			items, sudoErr := globalSudoClient.ReadDir(prepareOSPath(dirPath))
			if sudoErr == nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				if len(items) > 0 && onChunk != nil {
					base := make([]VFSItem, len(items))
					for i := range items {
						base[i] = directoryBaseItem(items[i])
					}
					onChunk(DirectoryReadBase, base)
					if ctxErr := ctx.Err(); ctxErr != nil {
						return ctxErr
					}
					onChunk(DirectoryReadMetadata, items)
				}
				return nil
			}
			vtui.DebugLog("VFS: Sudo ReadDirPhased(%q) FAILED: %v", dirPath, sudoErr)
		} else {
			vtui.DebugLog("VFS: ReadDirPhased(%q) FAILED: %v (Permission: %v, SudoAvailable: %v)", dirPath, err, os.IsPermission(err), globalSudoClient.IsAvailable())
		}
		return err
	}
	defer f.Close()

	// Keep the DirEntry objects until enumeration is complete. Publishing one
	// complete base catalog avoids revision/order churn while metadata arrives.
	pending := make([]phasedOSDirEntry, 0, 256)
	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		entries, readErr := f.ReadDir(1000)
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return readErr
		}
		for _, entry := range entries {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			base, info := phasedOSDirectoryBase(dirPath, entry)
			pending = append(pending, phasedOSDirEntry{entry: entry, base: base, info: info})
		}
	}

	if onChunk == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if len(pending) == 0 {
		// An empty result is still a complete authoritative base catalog. The
		// panel adds its synthetic ".." row and can publish it immediately,
		// without waiting for the later current/parent Stat decoration.
		onChunk(DirectoryReadBase, []VFSItem{})
		return ctx.Err()
	}
	base := make([]VFSItem, len(pending))
	for i := range pending {
		base[i] = pending[i].base
	}
	onChunk(DirectoryReadBase, base)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}

	// Bounded chunks keep enrichment cancellable for very large directories.
	const metadataChunkSize = 256
	for offset := 0; offset < len(pending); offset += metadataChunkSize {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		end := offset + metadataChunkSize
		if end > len(pending) {
			end = len(pending)
		}
		metadata := make([]VFSItem, 0, end-offset)
		for i := offset; i < end; i++ {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			metadata = append(metadata, phasedOSDirectoryMetadata(pending[i]))
		}
		if len(metadata) > 0 {
			onChunk(DirectoryReadMetadata, metadata)
		}
	}
	return nil
}

func directoryBaseItem(item VFSItem) VFSItem {
	return VFSItem{
		Name:        item.Name,
		IsDir:       item.IsDir,
		IsHidden:    item.IsHidden,
		NoExtension: item.NoExtension,
		IsSymlink:   item.IsSymlink,
	}
}

func phasedOSDirectoryBase(dirPath string, entry os.DirEntry) (VFSItem, os.FileInfo) {
	entryType := entry.Type()
	isDir := entry.IsDir()
	isSymlink := entryType&os.ModeSymlink != 0

	// Windows needs the already-enumerated attribute record for hidden and
	// reparse-point correctness. Elsewhere only ambiguous special entries need
	// an Info fallback before their navigability can be published.
	needsInfo := runtime.GOOS == "windows" || isSymlink ||
		(!isDir && !entryType.IsRegular())
	var info os.FileInfo
	if needsInfo {
		info, _ = entry.Info()
	}
	if info != nil {
		if !isSymlink && (info.Mode()&os.ModeSymlink != 0 || isReparsePoint(info)) {
			isSymlink = true
		}
	}
	if !isDir && (isSymlink || !entryType.IsRegular() || (info != nil && !info.Mode().IsRegular())) {
		if target, statErr := os.Stat(filepath.Join(dirPath, entry.Name())); statErr == nil {
			isDir = target.IsDir()
		}
	}
	entryPath := filepath.Join(dirPath, entry.Name())
	item := VFSItem{
		Name:      entry.Name(),
		IsDir:     isDir,
		IsSymlink: isSymlink,
		IsHidden:  isHidden(entryPath, entry.Name(), info),
	}
	if info != nil && runtime.GOOS == "windows" {
		// On Windows entry.Info() reads the WIN32_FIND_DATA already attached to
		// the directory enumeration. Carry those fields into the base phase so
		// the panel does not need a second O(N) metadata commit after showing the
		// directory.
		item.Size = info.Size()
		item.SizeKnown = true
		item.MTime = info.ModTime()
		item.IsExecutable = info.Mode().Perm()&0111 != 0
		fillPhysicalSizeCheap(&item, info)
	}
	return item, info
}

func phasedOSDirectoryMetadata(pending phasedOSDirEntry) VFSItem {
	info := pending.info
	if info == nil {
		info, _ = pending.entry.Info()
	}
	item := pending.base
	if info == nil {
		return item
	}
	item.Size = info.Size()
	item.SizeKnown = true
	item.MTime = info.ModTime()
	item.IsExecutable = info.Mode().Perm()&0111 != 0
	fillPhysicalSizeCheap(&item, info)
	return item
}

func (v *OSVFS) Stat(ctx context.Context, path string) (VFSItem, error) {
	if ctx.Err() != nil {
		return VFSItem{}, ctx.Err()
	}
	preparedPath := prepareOSPath(path)
	linkInfo, err := os.Lstat(preparedPath)
	if err != nil {
		if os.IsPermission(err) && globalSudoClient.IsAvailable() {
			vtui.DebugLog("VFS: Permission denied for Stat(%q), attempting sudo...", path)
			item, sudoErr := globalSudoClient.Stat(prepareOSPath(path))
			if sudoErr == nil {
				vtui.DebugLog("VFS: Sudo Stat(%q) SUCCESS", path)
				return item, nil
			}
			vtui.DebugLog("VFS: Sudo Stat(%q) FAILED: %v", path, sudoErr)
		}
		return VFSItem{}, err
	}
	isSymlink := linkInfo.Mode()&os.ModeSymlink != 0 || isReparsePoint(linkInfo)
	info := linkInfo
	if isSymlink {
		// Preserve the historical Stat view of the target while exposing that
		// the selected entry itself is a link/junction. Broken links remain
		// addressable as leaf entries.
		if targetInfo, targetErr := os.Stat(preparedPath); targetErr == nil {
			info = targetInfo
		}
	}

	item := VFSItem{
		Name:         info.Name(),
		Size:         info.Size(),
		SizeKnown:    true,
		IsDir:        info.IsDir(),
		IsSymlink:    isSymlink,
		MTime:        info.ModTime(),
		UnixMode:     uint32(info.Mode().Perm()),
		IsExecutable: info.Mode().Perm()&0111 != 0,
		IsHidden:     isHidden(path, info.Name(), info),
	}

	// Platform specific time extraction
	fillPlatformTimes(&item, info)
	fillPhysicalSize(&item, info, preparedPath)

	return item, nil
}

func (v *OSVFS) Join(elem ...string) string { return filepath.Join(elem...) }

func (v *OSVFS) Abs(path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	// Correctly resolve relative to the VFS current path, not process CWD
	return filepath.Join(v.currentPath, path), nil
}

// LocalPath exposes the native path for frontends which can decode local files
// directly. It deliberately mirrors Abs while keeping preview capability an
// optional VFS interface.
func (v *OSVFS) LocalPath(path string) (string, error) {
	return v.Abs(path)
}

func (v *OSVFS) Base(path string) string { return filepath.Base(path) }
func (v *OSVFS) Dir(path string) string  { return filepath.Dir(path) }
func (v *OSVFS) MkDir(ctx context.Context, path string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	err := os.MkdirAll(prepareOSPath(path), 0755)
	if err != nil && os.IsPermission(err) && globalSudoClient.IsAvailable() {
		vtui.DebugLog("VFS: Permission denied for MkDir(%q), attempting sudo...", path)
		return globalSudoClient.MkDir(prepareOSPath(path), 0755)
	}
	return err
}

func (v *OSVFS) Remove(ctx context.Context, path string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	err := os.RemoveAll(prepareOSPath(path))
	if err != nil && os.IsPermission(err) && globalSudoClient.IsAvailable() {
		return globalSudoClient.Remove(prepareOSPath(path))
	}
	return err
}

func (v *OSVFS) Rename(ctx context.Context, old, new string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if overwrite, known := DestinationOverwrite(ctx); known && !overwrite {
		return v.RenameNoReplace(ctx, old, new)
	}
	err := os.Rename(prepareOSPath(old), prepareOSPath(new))
	if err != nil && os.IsPermission(err) && globalSudoClient.IsAvailable() {
		return globalSudoClient.Rename(prepareOSPath(old), prepareOSPath(new))
	}
	return err
}

// RenameNoReplace renames an OS object without ever replacing an unrelated
// destination. It is intentionally an OSVFS capability rather than part of
// the general VFS contract: virtual filesystems may have different atomicity
// guarantees. VisRen uses it to preserve Win32 MoveFile semantics on Unix.
func (v *OSVFS) RenameNoReplace(ctx context.Context, old, new string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return renameNoReplace(prepareOSPath(old), prepareOSPath(new))
}
func (v *OSVFS) SetAttributes(ctx context.Context, path string, item VFSItem) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Try native first
	var errMode error
	if item.UnixMode != 0 {
		errMode = os.Chmod(prepareOSPath(path), os.FileMode(item.UnixMode))
	}

	var errOwn error
	if runtime.GOOS != "windows" {
		if item.Uid != -1 && item.Gid != -1 {
			errOwn = os.Chown(prepareOSPath(path), item.Uid, item.Gid)
		}
	}

	var errTime error
	if !item.ATime.IsZero() || !item.MTime.IsZero() {
		atime := item.ATime
		mtime := item.MTime
		if atime.IsZero() {
			atime = mtime
		}
		if mtime.IsZero() {
			mtime = atime
		}
		errTime = os.Chtimes(prepareOSPath(path), atime, mtime)
	}

	errPlat := applyPlatformAttributes(prepareOSPath(path), item)

	// If any operation failed due to permissions, try sudo
	if (os.IsPermission(errMode) || os.IsPermission(errOwn) || os.IsPermission(errTime) || os.IsPermission(errPlat)) && globalSudoClient.IsAvailable() {
		vtui.DebugLog("VFS: SetAttributes permission denied, trying sudo for %q", path)
		return globalSudoClient.SetAttributes(prepareOSPath(path), item)
	}

	if errMode != nil {
		return errMode
	}
	if errOwn != nil {
		return errOwn
	}
	if errTime != nil {
		return errTime
	}
	return errPlat
}

func (v *OSVFS) PatchInPlace(ctx context.Context, path string, pieces []PatchPiece) error {
	// This path is for devices, which cannot have a temporary sibling. Normal
	// files must use the editor's staged save: a prefix write followed by an
	// unsupported shifted piece otherwise corrupts the source before fallback,
	// and a shortened piece table would leave the old tail on disk.
	info, err := os.Stat(prepareOSPath(path))
	if err != nil {
		return err
	}
	if info.Mode().IsRegular() {
		return fmt.Errorf("in-place patching is only supported for devices")
	}
	f, err := os.OpenFile(prepareOSPath(path), os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	var newOffset int64 = 0
	for _, p := range pieces {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if p.Data != nil {
			if _, err := f.WriteAt(p.Data, newOffset); err != nil {
				return err
			}
		} else {
			if p.Offset != newOffset {
				return fmt.Errorf("in-place patching requires unchanged pieces to remain at their original offsets (no insertions/deletions allowed on raw disks)")
			}
		}
		newOffset += p.Length
	}
	return nil
}
func (v *OSVFS) GetCapabilities() VFSCapabilities {
	return VFSCapabilities{
		HasServerSideCopy:        true,
		HasServerSideMove:        true,
		HasRandomAccess:          true,
		ReadAccess:               ReadAccessDirectLocal,
		StorageClass:             StorageClassLocal,
		HasSearch:                false,
		HasUnixPermissions:       runtime.GOOS != "windows",
		HasAtomicNoReplaceRename: true,
		HasWrite:                 true,
	}
}

func (v *OSVFS) Search(ctx context.Context, path string, pattern string) (chan int64, error) {
	// OSVFS uses local streaming search implemented in actions.go
	return nil, nil
}

type osFileWrapper struct {
	*os.File
	size int64
}

func (f *osFileWrapper) Size() int64                          { return f.size }
func (f *osFileWrapper) LocalPath() (string, bool)            { return f.Name(), f.Name() != "" }
func (f *osFileWrapper) ReadAccessProfile() ReadAccessProfile { return ReadAccessDirectLocal }
func (f *osFileWrapper) Read(ctx context.Context, p []byte) (n int, err error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	return f.File.Read(p)
}

func (f *osFileWrapper) ReadAt(ctx context.Context, p []byte, off int64) (n int, err error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	return f.File.ReadAt(p, off)
}

func (v *OSVFS) Open(ctx context.Context, path string) (ReadAtCloser, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	fi, err := os.Stat(prepareOSPath(path))
	if err == nil && (fi.Mode()&(os.ModeNamedPipe|os.ModeSocket) != 0) {
		return nil, os.ErrInvalid
	}
	f, err := os.OpenFile(prepareOSPath(path), os.O_RDWR, 0)
	if err != nil {
		f, err = os.Open(prepareOSPath(path))
	}
	if err != nil {
		if os.IsPermission(err) && globalSudoClient.IsAvailable() {
			vtui.DebugLog("VFS: Permission denied for Open(%q), attempting sudo...", path)
			sudoF, sudoErr := globalSudoClient.Open(prepareOSPath(path), os.O_RDONLY, 0)
			if sudoErr == nil {
				info, _ := sudoF.Stat()
				size := info.Size()
				if info.Mode()&(os.ModeDevice|os.ModeCharDevice) != 0 {
					if pos, err := sudoF.Seek(0, io.SeekEnd); err == nil && pos > 0 {
						size = pos
						sudoF.Seek(0, io.SeekStart)
					}
				}
				vtui.DebugLog("VFS: Sudo Open(%q) SUCCESS, size: %d", path, size)
				return &osFileWrapper{File: sudoF, size: size}, nil
			}
			vtui.DebugLog("VFS: Sudo Open(%q) FAILED: %v", path, sudoErr)
		}
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	size := info.Size()
	if info.Mode()&(os.ModeDevice|os.ModeCharDevice) != 0 {
		if pos, err := f.Seek(0, io.SeekEnd); err == nil && pos > 0 {
			size = pos
			f.Seek(0, io.SeekStart)
		}
	}
	return &osFileWrapper{File: f, size: size}, nil
}

func (v *OSVFS) Create(ctx context.Context, path string) (io.WriteCloser, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	prepared := prepareOSPath(path)
	fi, err := os.Stat(prepared)
	if err == nil && (fi.Mode()&(os.ModeNamedPipe|os.ModeSocket) != 0) {
		return nil, os.ErrInvalid
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if err == nil && (fi.Mode()&(os.ModeDevice|os.ModeCharDevice) != 0) {
		flags = os.O_WRONLY // Do not truncate devices
	}
	createMode := os.FileMode(0o666)
	if overwrite, known := DestinationOverwrite(ctx); known && !overwrite {
		// O_EXCL makes the editor's unique sibling creation collision-safe and
		// closes the Stat/Create race. Do not include O_TRUNC in this mode: an
		// existing path must remain byte-for-byte untouched.
		flags = os.O_CREATE | os.O_WRONLY | os.O_EXCL
		// Newly staged/copied data must not be exposed through a permissive
		// umask window before the caller restores its final metadata.
		createMode = 0o600
	}
	f, err := os.OpenFile(prepared, flags, createMode)
	if err != nil && os.IsPermission(err) && globalSudoClient.IsAvailable() {
		vtui.DebugLog("VFS: Permission denied for Create(%q), attempting sudo...", path)
		return globalSudoClient.Open(prepared, flags, uint32(createMode))
	}
	if err != nil {
		// Converting a nil *os.File directly to io.WriteCloser creates a
		// non-nil interface. Return a literal nil so callers cannot accidentally
		// use a writer after O_EXCL or another open failure.
		return nil, err
	}
	return f, nil
}

func (v *OSVFS) ParentVFS() VFS {
	return nil // OSVFS is the root
}
func (v *OSVFS) Clone() VFS {
	return NewOSVFS(v.currentPath)
}
func (v *OSVFS) Close() error { return nil }

// wellKnownJunction checks if the given path on Windows is a well-known junction
// and returns its known target. This is a last-resort fallback when all other
// reparse point resolution methods fail or are blocked by permissions.
func wellKnownJunction(path string) (string, bool) {
	if runtime.GOOS != "windows" {
		return "", false
	}
	parent := filepath.Dir(path)
	name := strings.ToLower(filepath.Base(path))
	parentBase := strings.ToLower(filepath.Base(parent))

	// Documents and Settings at drive root -> Users
	if len(path) >= 3 && path[1] == ':' && path[2] == '\\' && name == "documents and settings" {
		return filepath.Join(parent, "Users"), true
	}

	// C:\Users\All Users -> C:\ProgramData
	if name == "all users" {
		return filepath.Join(filepath.Dir(parent), "ProgramData"), true
	}

	// C:\Users\Default User -> C:\Users\Default
	if name == "default user" && parentBase == "users" {
		return filepath.Join(parent, "Default"), true
	}

	return "", false
}

// prepareOSPath adds the \\?\ prefix on Windows to prevent the Win32 API
// from automatically stripping trailing dots and spaces from file names.
func prepareOSPath(p string) string {
	if runtime.GOOS != "windows" {
		return p
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if strings.HasPrefix(abs, `\\?\`) {
		return abs
	}
	if strings.HasPrefix(abs, `\\`) {
		return `\\?\UNC\` + abs[2:]
	}
	return `\\?\` + abs
}

// stripExtendedPrefix removes the \\?\ prefix from paths returned by OS functions
// (like EvalSymlinks) so they display nicely in the UI.
func stripExtendedPrefix(p string) string {
	if runtime.GOOS != "windows" {
		return p
	}
	if strings.HasPrefix(p, `\\?\UNC\`) {
		return `\\` + p[8:]
	}
	if strings.HasPrefix(p, `\\?\`) {
		return p[4:]
	}
	return p
}

// Readlink and Symlink make OSVFS a SymlinkVFS. The local file system is the
// one backend where a symbolic link is exactly what the word means.

func (v *OSVFS) Readlink(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	abs, err := v.Abs(path)
	if err != nil {
		return "", err
	}
	return os.Readlink(prepareOSPath(abs))
}

func (v *OSVFS) Symlink(ctx context.Context, target, linkPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	abs, err := v.Abs(linkPath)
	if err != nil {
		return err
	}
	return os.Symlink(target, prepareOSPath(abs))
}

// OpenWriteAt makes OSVFS a RandomWriteVFS. A local file is the case where
// staging a second copy on the same disk buys nothing at all.
func (v *OSVFS) OpenWriteAt(ctx context.Context, path string) (WriterAtCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	abs, err := v.Abs(path)
	if err != nil {
		return nil, err
	}
	return os.OpenFile(prepareOSPath(abs), os.O_RDWR|os.O_CREATE, 0o644)
}
