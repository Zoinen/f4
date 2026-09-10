// Package unpack extracts a downloaded archive over a directory on disk.
//
// Three places in f4 do that with an archive it did not create: the updater
// unpacks a release, the plugin catalogue unpacks a plugin, and the colorer
// downloader unpacks a scheme bundle. SanitizePath is the guard all three need
// — an archive member named "../../etc/passwd" must land nowhere — and a guard
// that lives in one of the three is a guard the fourth caller will not find.
package unpack

import (
	"archive/tar"
	"bytes"
	"fmt"
	gzip "github.com/klauspost/pgzip"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/sevenzip"
	"github.com/unxed/vtui"
	"github.com/unxed/zip"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

func WriteFileSafe(targetPath string, r io.Reader, mode os.FileMode) error {
	primaryOldPath := targetPath + ".old"
	oldPath := primaryOldPath

	err := os.Remove(oldPath)
	if err != nil && os.IsPermission(err) && vfs.GetSudoClient().IsAvailable() {
		_ = vfs.GetSudoClient().Remove(oldPath)
	}

	if _, err := os.Stat(oldPath); err == nil {
		for i := 1; i < 1000; i++ {
			cand := fmt.Sprintf("%s.%d", primaryOldPath, i)
			err := os.Remove(cand)
			if err != nil && os.IsPermission(err) && vfs.GetSudoClient().IsAvailable() {
				_ = vfs.GetSudoClient().Remove(cand)
			}
			if _, err := os.Stat(cand); os.IsNotExist(err) {
				oldPath = cand
				break
			}
		}
	}

	if _, err := os.Stat(targetPath); err == nil {
		errRename := os.Rename(targetPath, oldPath)
		if errRename != nil && os.IsPermission(errRename) && vfs.GetSudoClient().IsAvailable() {
			_ = vfs.GetSudoClient().Rename(targetPath, oldPath)
		}
	}

	dir := filepath.Dir(targetPath)
	// #nosec G301 -- updater targets may be shared/system installations whose directories must remain searchable by other users.
	errMkdir := os.MkdirAll(dir, 0755)
	if errMkdir != nil && os.IsPermission(errMkdir) && vfs.GetSudoClient().IsAvailable() {
		_ = vfs.GetSudoClient().MkDir(dir, 0755)
	}

	var f *os.File
	f, err = os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil && os.IsPermission(err) && vfs.GetSudoClient().IsAvailable() {
		vtui.DebugLog("UPDATER: Permission denied for %q, attempting elevated write via sudo...", targetPath)
		f, err = vfs.GetSudoClient().Open(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, uint32(mode))
	}

	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = io.Copy(f, r)
	if err != nil {
		return err
	}

	errRemove := os.Remove(primaryOldPath)
	if errRemove != nil && os.IsPermission(errRemove) && vfs.GetSudoClient().IsAvailable() {
		_ = vfs.GetSudoClient().Remove(primaryOldPath)
	}

	return nil
}

func SanitizePath(name, destDir string) (string, error) {
	// Archive paths are always slash-separated
	if strings.ContainsRune(name, '\\') || strings.ContainsRune(name, '\x00') {
		return "", fmt.Errorf("invalid path in archive: %s", name)
	}
	cleanName := path.Clean(name)
	if path.IsAbs(cleanName) || strings.HasPrefix(cleanName, "../") || cleanName == ".." {
		return "", fmt.Errorf("invalid path in archive: %s", name)
	}
	return filepath.Join(destDir, filepath.FromSlash(cleanName)), nil
}

type ArchiveEntry struct {
	name  string
	isDir bool
	mode  os.FileMode
	open  func() (io.ReadCloser, error)
}

func ExtractEntry(e ArchiveEntry, destDir string) error {
	targetPath, err := SanitizePath(e.name, destDir)
	if err != nil {
		return nil // Skip malicious/invalid paths
	}

	if e.isDir {
		// #nosec G301 -- release archive directories may belong to a shared installation and must remain searchable by other users.
		errMkdir := os.MkdirAll(targetPath, 0755)
		if errMkdir != nil && os.IsPermission(errMkdir) && vfs.GetSudoClient().IsAvailable() {
			_ = vfs.GetSudoClient().MkDir(targetPath, 0755)
		}
		return nil
	}

	rc, err := e.open()
	if err != nil {
		return err
	}

	mode := e.mode
	if mode == 0 {
		mode = 0644
	}
	err = WriteFileSafe(targetPath, rc, mode)
	_ = rc.Close()
	return err
}

func TarGz(data []byte, destDir string) error {
	r := bytes.NewReader(data)
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		// tar.Header.Mode is a signed container, but only its low permission
		// bits are meaningful to the extracted file.
		// #nosec G115 -- masking to 0777 bounds the os.FileMode conversion.
		mode := os.FileMode(hdr.Mode & 0o777)
		if err := ExtractEntry(ArchiveEntry{
			name:  hdr.Name,
			isDir: hdr.Typeflag == tar.TypeDir,
			mode:  mode,
			open:  func() (io.ReadCloser, error) { return io.NopCloser(tr), nil },
		}, destDir); err != nil {
			return err
		}
	}
}

func Zip(data []byte, destDir string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		if err := ExtractEntry(ArchiveEntry{
			name:  f.Name,
			isDir: f.FileInfo().IsDir(),
			mode:  f.Mode(),
			open:  f.Open,
		}, destDir); err != nil {
			return err
		}
	}
	return nil
}

// ZipParallel: Zip with entries in parallel; falls back when <2 entries.
func ZipParallel(data []byte, destDir string, workers int) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	if workers < 2 || len(zr.File) < 2 {
		return Zip(data, destDir)
	}
	if workers > len(zr.File) {
		workers = len(zr.File)
	}

	entries := make([]ArchiveEntry, len(zr.File))
	for i, f := range zr.File {
		entries[i] = ArchiveEntry{
			name:  f.Name,
			isDir: f.FileInfo().IsDir(),
			mode:  f.Mode(),
			open:  f.Open,
		}
	}

	errCh := make(chan error, 1)
	done := make(chan struct{})
	jobs := make(chan ArchiveEntry)
	go func() {
		defer close(jobs)
		for _, e := range entries {
			select {
			case jobs <- e:
			case <-done:
				return
			}
		}
	}()

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range jobs {
				if err := ExtractEntry(e, destDir); err != nil {
					select {
					case errCh <- err:
					default:
					}
					return
				}
			}
		}()
	}
	wg.Wait()
	close(done)
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

func SevenZip(data []byte, destDir string) error {
	szr, err := sevenzip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range szr.File {
		if err := ExtractEntry(ArchiveEntry{
			name:  f.Name,
			isDir: f.FileInfo().IsDir(),
			mode:  f.Mode(),
			open:  f.Open,
		}, destDir); err != nil {
			return err
		}
	}
	return nil
}
