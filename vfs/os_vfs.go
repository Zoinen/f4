package vfs

import (
	"context"
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

func NewOSVFS(initialPath string) *OSVFS {
	abs, _ := filepath.Abs(initialPath)
	return &OSVFS{currentPath: abs}
}

func (v *OSVFS) GetPath() string        { return v.currentPath }
func (v *OSVFS) IsAbs(path string) bool { return filepath.IsAbs(path) }

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
	if st, errStat := os.Stat(prepareOSPath(abs)); errStat == nil && st.IsDir() {
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
	st, err := os.Stat(prepareOSPath(abs))
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
			// If it's not a direct directory, it might be a symlink or a Windows Junction.
			// If it's not a regular file, ask the OS to resolve the final target.
			if !isDir && !e.Type().IsRegular() {
				if target, err := os.Stat(filepath.Join(dirPath, e.Name())); err == nil {
					isDir = target.IsDir()
				}
			}

			items = append(items, VFSItem{
				Name:         e.Name(),
				Size:         size,
				IsDir:        isDir,
				MTime:        mtime,
				IsExecutable: isExec,
				IsHidden:     isHidden(filepath.Join(dirPath, e.Name()), e.Name(), info),
			})
		}

		if len(items) > 0 && onChunk != nil {
			onChunk(items)
		}
	}
	return nil
}

func (v *OSVFS) Stat(ctx context.Context, path string) (VFSItem, error) {
	if ctx.Err() != nil {
		return VFSItem{}, ctx.Err()
	}
	info, err := os.Stat(prepareOSPath(path))
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

	item := VFSItem{
		Name:         info.Name(),
		Size:         info.Size(),
		IsDir:        info.IsDir(),
		MTime:        info.ModTime(),
		UnixMode:     uint32(info.Mode().Perm()),
		IsExecutable: info.Mode().Perm()&0111 != 0,
		IsHidden:     isHidden(path, info.Name(), info),
	}

	// Platform specific time extraction
	fillPlatformTimes(&item, info)

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
	err := os.Rename(prepareOSPath(old), prepareOSPath(new))
	if err != nil && os.IsPermission(err) && globalSudoClient.IsAvailable() {
		return globalSudoClient.Rename(prepareOSPath(old), prepareOSPath(new))
	}
	return err
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

func (v *OSVFS) GetCapabilities() VFSCapabilities {
	return VFSCapabilities{
		HasServerSideCopy:  true,
		HasServerSideMove:  true,
		HasRandomAccess:    true,
		HasSearch:          false,
		HasUnixPermissions: runtime.GOOS != "windows",
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

func (f *osFileWrapper) Size() int64 { return f.size }
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
	if err == nil && (fi.Mode()&(os.ModeNamedPipe|os.ModeSocket|os.ModeDevice|os.ModeCharDevice) != 0) {
		return nil, os.ErrInvalid
	}
	f, err := os.Open(prepareOSPath(path))
	if err != nil {
		if os.IsPermission(err) && globalSudoClient.IsAvailable() {
			vtui.DebugLog("VFS: Permission denied for Open(%q), attempting sudo...", path)
			sudoF, sudoErr := globalSudoClient.Open(prepareOSPath(path), os.O_RDONLY, 0)
			if sudoErr == nil {
				info, _ := sudoF.Stat()
				vtui.DebugLog("VFS: Sudo Open(%q) SUCCESS, size: %d", path, info.Size())
				return &osFileWrapper{File: sudoF, size: info.Size()}, nil
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
	return &osFileWrapper{File: f, size: info.Size()}, nil
}

func (v *OSVFS) Create(ctx context.Context, path string) (io.WriteCloser, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	fi, err := os.Stat(prepareOSPath(path))
	if err == nil && (fi.Mode()&(os.ModeNamedPipe|os.ModeSocket|os.ModeDevice|os.ModeCharDevice) != 0) {
		return nil, os.ErrInvalid
	}
	f, err := os.Create(prepareOSPath(path))
	if err != nil && os.IsPermission(err) && globalSudoClient.IsAvailable() {
		vtui.DebugLog("VFS: Permission denied for Create(%q), attempting sudo...", path)
		return globalSudoClient.Open(prepareOSPath(path), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	}
	return f, err
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
