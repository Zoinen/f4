package vfs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// NullVFS is a mock filesystem for testing UI responsiveness and file operations.
// It provides virtual files of specific sizes and discards any written data,
// simulating network or disk delays via a speed limit.
type NullVFS struct {
	currentPath string
	speedLimit  int64 // Bytes per second. 0 means unlimited.
}

var nullFiles = map[string]int64{
	"1KB.bin":   1024,
	"1MB.bin":   1024 * 1024,
	"10MB.bin":  10 * 1024 * 1024,
	"100MB.bin": 100 * 1024 * 1024,
	"huge.bin":  512 * 1024 * 1024, // Centralized huge.bin size
	"1GB.bin":   1024 * 1024 * 1024,
}

// NewNullVFS creates a new NullVFS with the specified speed limit in bytes per second.
func NewNullVFS(speedLimit int64) *NullVFS {
	return &NullVFS{
		currentPath: "/",
		speedLimit:  speedLimit,
	}
}

func (v *NullVFS) GetPath() string {
	return filepath.FromSlash(v.currentPath)
}
func (v *NullVFS) IsAbs(p string) bool { return path.IsAbs(p) }

func (v *NullVFS) IsAtRoot() bool {
	return v.currentPath == "/" || v.currentPath == ""
}

func (v *NullVFS) SetPath(p string) error {
	v.currentPath = filepath.ToSlash(filepath.Clean(p))
	return nil
}

func (v *NullVFS) ReadDir(ctx context.Context, p string, onChunk func([]VFSItem)) error {
	p = path.Clean(p)
	v.throttleMeta(ctx, p)

	var items []VFSItem
	if p == "/" {
		for name, size := range nullFiles {
			items = append(items, VFSItem{Name: name, Size: size, IsDir: false, MTime: time.Now()})
		}
		items = append(items, VFSItem{Name: "upload", IsDir: true, MTime: time.Now()})
		items = append(items, VFSItem{Name: "scenarios", IsDir: true, MTime: time.Now()})
	} else if p == "/scenarios" {
		items = append(items, VFSItem{Name: "bandwidth", IsDir: true}, VFSItem{Name: "iops", IsDir: true})
		items = append(items, VFSItem{Name: "deep", IsDir: true}, VFSItem{Name: "slow", IsDir: true}, VFSItem{Name: "fast", IsDir: true})
	} else if strings.HasPrefix(p, "/scenarios/bandwidth") {
		items = append(items, VFSItem{Name: "huge.bin", Size: nullFiles["huge.bin"], IsDir: false})
	} else if strings.HasPrefix(p, "/scenarios/iops") {
		for i := 0; i < 10000; i++ {
			items = append(items, VFSItem{Name: fmt.Sprintf("small_%d.txt", i), Size: 1024, IsDir: false})
		}
	} else if strings.HasPrefix(p, "/scenarios/deep") {
		parts := strings.Split(strings.TrimPrefix(p, "/scenarios/deep"), "/")
		level := len(parts) - 1
		if level < 10 {
			items = append(items, VFSItem{Name: "next_level", IsDir: true})
		}
		for i := 0; i < 10; i++ {
			items = append(items, VFSItem{Name: fmt.Sprintf("file_%d.txt", i), Size: 512, IsDir: false})
		}
	} else if p == "/scenarios/slow" || p == "/scenarios/fast" {
		items = append(items, VFSItem{Name: "test.bin", Size: 50 * 1024 * 1024, IsDir: false})
	}

	if onChunk == nil || len(items) == 0 {
		return nil
	}

	// Realistic paging: split into chunks of 100
	chunkSize := 100
	for i := 0; i < len(items); i += chunkSize {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		end := i + chunkSize
		if end > len(items) {
			end = len(items)
		}
		onChunk(items[i:end])

		// Simulate inter-chunk delay on slow connections
		if strings.Contains(p, "/slow") && end < len(items) {
			select {
			case <-time.After(20 * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return nil
}

func (v *NullVFS) Stat(ctx context.Context, p string) (VFSItem, error) {
	p = path.Clean(p)
	v.throttleMeta(ctx, p)
	base := path.Base(p)

	// 1. Fixed directory resolution
	if p == "/" || p == "/upload" || p == "/scenarios" ||
		p == "/scenarios/bandwidth" || p == "/scenarios/iops" ||
		p == "/scenarios/slow" || p == "/scenarios/fast" {
		return VFSItem{Name: base, IsDir: true, MTime: time.Now()}, nil
	}

	// 2. Scenario-specific logic (Scoped to /scenarios prefix)
	if strings.HasPrefix(p, "/scenarios/") {
		if strings.HasPrefix(p, "/scenarios/deep") && (base == "next_level" || base == "deep") {
			return VFSItem{Name: base, IsDir: true, MTime: time.Now()}, nil
		}
		if p == "/scenarios/bandwidth/huge.bin" {
			return VFSItem{Name: base, Size: nullFiles["huge.bin"], IsDir: false}, nil
		}
		if strings.HasPrefix(p, "/scenarios/iops/") {
			return VFSItem{Name: base, Size: 1024, IsDir: false}, nil
		}
		if strings.HasPrefix(p, "/scenarios/deep/") {
			return VFSItem{Name: base, Size: 512, IsDir: false}, nil
		}
		if (strings.HasPrefix(p, "/scenarios/slow/") || strings.HasPrefix(p, "/scenarios/fast/")) && base == "test.bin" {
			return VFSItem{Name: base, Size: 50 * 1024 * 1024, IsDir: false}, nil
		}
	}

	// 3. Root static files
	if size, ok := nullFiles[base]; ok && path.Dir(p) == "/" {
		return VFSItem{Name: base, Size: size, IsDir: false, MTime: time.Now()}, nil
	}

	// 4. Upload zone logic: allow recursive directory creation
	if strings.HasPrefix(p, "/upload/") {
		ext := path.Ext(p)
		if ext == ".bin" || ext == ".txt" {
			return VFSItem{Name: base, Size: 0, IsDir: false, MTime: time.Now()}, nil
		}
		return VFSItem{Name: base, IsDir: true, MTime: time.Now()}, nil
	}

	return VFSItem{}, os.ErrNotExist
}

func (v *NullVFS) Join(elem ...string) string { return path.Join(elem...) }

func (v *NullVFS) Abs(p string) (string, error) {
	if path.IsAbs(p) {
		return path.Clean(p), nil
	}
	return path.Join(v.currentPath, p), nil
}

func (v *NullVFS) Base(p string) string { return path.Base(p) }
func (v *NullVFS) Dir(p string) string  { return path.Dir(p) }

// Mutations succeed silently
func (v *NullVFS) MkDir(ctx context.Context, p string) error {
	v.throttleMeta(ctx, p)
	return nil
}

func (v *NullVFS) Remove(ctx context.Context, p string) error {
	v.throttleMeta(ctx, p)
	return nil
}

func (v *NullVFS) Rename(ctx context.Context, old, new string) error {
	v.throttleMeta(ctx, old)
	v.throttleMeta(ctx, new)
	return nil
}

func (v *NullVFS) SetAttributes(ctx context.Context, path string, item VFSItem) error {
	v.throttleMeta(ctx, path)
	return nil
}

func (v *NullVFS) GetCapabilities() VFSCapabilities {
	return VFSCapabilities{
		HasServerSideCopy:  false,
		HasServerSideMove:  false,
		HasRandomAccess:    true,
		ReadAccess:         ReadAccessNativeRange,
		StorageClass:       StorageClassVirtual,
		HasSearch:          false,
		HasUnixPermissions: false,
	}
}

func (v *NullVFS) Search(ctx context.Context, p string, pattern string) (chan int64, error) {
	return nil, nil
}

func (v *NullVFS) Open(ctx context.Context, p string) (ReadAtCloser, error) {
	v.throttleMeta(ctx, p)
	stat, _ := v.Stat(ctx, p)

	speed := v.speedLimit
	if strings.Contains(p, "/scenarios/slow") {
		speed = 128 * 1024 // 128 KB/s
	} else if strings.Contains(p, "/scenarios/fast") {
		speed = 500 * 1024 * 1024 // 500 MB/s
	}

	return &nullReader{size: stat.Size, speed: speed}, nil
}

func (v *NullVFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	v.throttleMeta(ctx, p)
	// Refuse overwriting static files, allow everything else
	base := path.Base(p)
	if _, ok := nullFiles[base]; ok && path.Dir(p) == "/" {
		return nil, os.ErrPermission
	}
	return &nullWriter{ctx: ctx, speed: v.speedLimit}, nil
}

func (v *NullVFS) ParentVFS() VFS { return nil }
func (v *NullVFS) Clone() VFS     { return NewNullVFS(v.speedLimit) }
func (v *NullVFS) Close() error   { return nil }

// --- Throttled Reader ---

type nullReader struct {
	size   int64
	offset int64
	speed  int64
}

func (r *nullReader) Size() int64 { return r.size }

func (r *nullReader) Read(ctx context.Context, p []byte) (n int, err error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if r.offset >= r.size {
		return 0, io.EOF
	}
	n = len(p)
	if r.offset+int64(n) > r.size {
		n = int(r.size - r.offset)
	}

	// Zero-fill
	for i := 0; i < n; i++ {
		p[i] = 0
	}

	r.offset += int64(n)
	err = throttle(ctx, n, r.speed)
	return n, err
}

func (r *nullReader) ReadAt(ctx context.Context, p []byte, off int64) (n int, err error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if off >= r.size {
		return 0, io.EOF
	}
	n = len(p)
	if off+int64(n) > r.size {
		n = int(r.size - off)
	}

	for i := 0; i < n; i++ {
		p[i] = 0
	}

	err = throttle(ctx, n, r.speed)
	return n, err
}

func (r *nullReader) Close() error { return nil }

// --- Throttled Writer ---

type nullWriter struct {
	ctx   context.Context
	speed int64
}

func (w *nullWriter) Write(p []byte) (n int, err error) {
	n = len(p)
	err = throttle(w.ctx, n, w.speed)
	return n, err
}

func (w *nullWriter) Close() error { return nil }

// --- Throttle Helper ---

func throttle(ctx context.Context, n int, speed int64) error {
	if speed <= 0 || n <= 0 {
		return nil
	}
	dur := time.Duration(float64(n) / float64(speed) * float64(time.Second))

	timer := time.NewTimer(dur)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (v *NullVFS) throttleMeta(ctx context.Context, p string) {
	if strings.Contains(p, "/slow") {
		timer := time.NewTimer(100 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
		}
	}
}
