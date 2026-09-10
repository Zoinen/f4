//go:build windows

package hostfs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	winescape "github.com/unxed/libwinescape/go"
)

func TestWindowsHostFSForwardsNativeOperations(t *testing.T) {
	t.Setenv("F4_WINE_POSIX", "0")
	root := t.TempDir()
	nested := filepath.Join(root, "nested", "deep")
	if err := MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	single := filepath.Join(root, "single")
	if err := Mkdir(single, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	path := filepath.Join(single, "source.txt")

	f, err := OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if n, err := f.Write([]byte("hello")); err != nil || n != 5 {
		t.Fatalf("Write: n=%d err=%v", n, err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	buf := make([]byte, 5)
	if n, err := f.Read(buf); err != nil || n != len(buf) || string(buf) != "hello" {
		t.Fatalf("Read: n=%d err=%v data=%q", n, err, buf)
	}
	if _, err := f.ReadAt(make([]byte, 2), 0); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if _, err := f.WriteAt([]byte("!"), 5); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	if err := f.Truncate(3); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	if info, err := f.Stat(); err != nil || info.Size() != 3 {
		t.Fatalf("File.Stat: info=%v err=%v", info, err)
	}
	if f.Fd() == 0 {
		t.Fatal("File.Fd returned stdin")
	}
	if err := f.Close(); err != nil {
		t.Fatalf("File.Close: %v", err)
	}

	f, err = Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Open.Close: %v", err)
	}
	if _, err := Stat(path); err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if _, err := Lstat(path); err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if entries, err := ReadDir(single); err != nil || len(entries) != 1 {
		t.Fatalf("ReadDir: entries=%v err=%v", entries, err)
	}

	renamed := filepath.Join(single, "renamed.txt")
	if err := Rename(path, renamed); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if err := Chmod(renamed, 0o640); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	when := time.Unix(123, 456)
	if err := Chtimes(renamed, when, when); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	_ = Chown(renamed, 0, 0)
	_ = Lchown(renamed, 0, 0)
	if err := Remove(renamed); err != nil {
		t.Fatalf("Remove file: %v", err)
	}
	if err := Remove(single); err != nil {
		t.Fatalf("Remove directory: %v", err)
	}
	if err := RemoveAll(filepath.Join(root, "nested")); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
}

func TestWindowsHostFSHelpers(t *testing.T) {
	for _, tc := range []struct {
		path, want string
	}{
		{"/tmp/file.txt", "file.txt"},
		{"file.txt", "file.txt"},
		{"/", ""},
	} {
		if got := stdBase(tc.path); got != tc.want {
			t.Errorf("stdBase(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}

	flags, err := translateOpenFlags(os.O_RDWR | os.O_APPEND | os.O_CREATE | os.O_EXCL | os.O_TRUNC)
	if err != nil {
		t.Fatalf("translateOpenFlags: %v", err)
	}
	wantFlags := winescape.O_RDWR | winescape.O_APPEND | winescape.O_CREAT | winescape.O_EXCL | winescape.O_TRUNC
	if flags != wantFlags {
		t.Fatalf("translateOpenFlags = %#x, want %#x", flags, wantFlags)
	}

	st := winescape.Stat_t{
		Mode: 0o100754,
		Size: 42,
		Mtim: winescape.Timespec{Sec: 7, Nsec: 8},
	}
	info := wineStatToInfo("", "/tmp/item", st)
	if info.Name() != "item" || info.Size() != 42 || info.Mode().Perm() != 0o754 {
		t.Fatalf("wineStatToInfo = name=%q size=%d mode=%v", info.Name(), info.Size(), info.Mode())
	}
	if !info.ModTime().Equal(time.Unix(7, 8)) {
		t.Fatalf("ModTime = %v", info.ModTime())
	}
	if got, ok := info.Sys().(*winescape.Stat_t); !ok || got.Size != 42 {
		t.Fatalf("Sys = %#v", info.Sys())
	}
	for _, tc := range []struct {
		mode os.FileMode
		want os.FileMode
	}{
		{0o040755, os.ModeDir},
		{0o120777, os.ModeSymlink},
	} {
		got := wineStatToInfo("entry", "/tmp/entry", winescape.Stat_t{Mode: uint32(tc.mode)}).Mode()
		if got&tc.want == 0 {
			t.Errorf("mode %#o = %v, want %v", tc.mode, got, tc.want)
		}
	}

	marker := errors.New("marker")
	if n, err := wineIOResult(3, marker); n != 3 || !errors.Is(err, marker) {
		t.Fatalf("wineIOResult = %d, %v", n, err)
	}
	if got := (errNotImplemented("Truncate")).Error(); got != "Truncate: not implemented in posix mode yet" {
		t.Fatalf("errNotImplemented = %q", got)
	}
	if hostErr(nil) != nil || hostErr(marker) != marker {
		t.Fatal("hostErr should preserve nil and non-errno errors")
	}
	for _, tc := range []struct {
		errno  winescape.Errno
		target error
	}{
		{winescape.ENOENT, os.ErrNotExist},
		{winescape.ENOTDIR, os.ErrNotExist},
		{winescape.EEXIST, os.ErrExist},
		{winescape.EACCES, os.ErrPermission},
		{winescape.EPERM, os.ErrPermission},
		{winescape.EINVAL, os.ErrInvalid},
	} {
		if !errors.Is(hostErr(tc.errno), tc.target) {
			t.Errorf("hostErr(%v) does not match %v", tc.errno, tc.target)
		}
	}

	for _, tc := range []struct {
		typeID uint8
		want   os.FileMode
	}{
		{winescape.DT_DIR, os.ModeDir},
		{winescape.DT_LNK, os.ModeSymlink},
		{winescape.DT_REG, 0},
		{winescape.DT_UNKNOWN, os.ModeIrregular},
	} {
		entry := wineDirEntry{name: "entry", dtype: tc.typeID}
		if entry.Name() != "entry" || entry.IsDir() != (tc.typeID == winescape.DT_DIR) || entry.Type() != tc.want {
			t.Errorf("wineDirEntry type=%d: name=%q dir=%v mode=%v", tc.typeID, entry.Name(), entry.IsDir(), entry.Type())
		}
	}
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := (&wineDirEntry{name: "missing", dirPath: filepath.Dir(missing)}).Info(); err == nil {
		t.Fatal("wineDirEntry.Info unexpectedly succeeded")
	}
	if err := winescapeRemove(missing); err == nil {
		t.Fatal("winescapeRemove unexpectedly succeeded")
	}
	if _, err := winescapeOpenFile(missing, os.O_RDONLY, 0); err == nil {
		t.Fatal("winescapeOpenFile unexpectedly succeeded")
	}
	if _, err := winescapeReadDir(missing); err == nil {
		t.Fatal("winescapeReadDir unexpectedly succeeded")
	}

	file := &wineFile{fd: -1, name: "missing"}
	if _, err := file.Read(make([]byte, 1)); err == nil {
		t.Fatal("wineFile.Read unexpectedly succeeded")
	}
	if _, err := file.Write([]byte("x")); err == nil {
		t.Fatal("wineFile.Write unexpectedly succeeded")
	}
	if _, err := file.ReadAt(make([]byte, 1), 0); err == nil {
		t.Fatal("wineFile.ReadAt unexpectedly succeeded")
	}
	if _, err := file.WriteAt([]byte("x"), 0); err == nil {
		t.Fatal("wineFile.WriteAt unexpectedly succeeded")
	}
	if _, err := file.Seek(0, 0); err == nil {
		t.Fatal("wineFile.Seek unexpectedly succeeded")
	}
	if _, err := file.Stat(); err == nil {
		t.Fatal("wineFile.Stat unexpectedly succeeded")
	}
	if err := file.Truncate(0); err == nil {
		t.Fatal("wineFile.Truncate unexpectedly succeeded")
	}
	if err := file.Close(); err == nil {
		t.Fatal("wineFile.Close unexpectedly succeeded")
	}
	if file.Fd() != ^uintptr(0) {
		t.Fatalf("wineFile.Fd = %d, want max uintptr", file.Fd())
	}
}
