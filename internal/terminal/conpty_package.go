package terminal

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// The ConPTY that f4 runs its Windows shell in.
//
// Current ConPTY hands a VT-mode write to the terminal verbatim:
// DoWriteConsole in src/host/_stream.cpp (tag v1.25.1912.0) sends it to
// WriteCharsVT whenever ENABLE_VIRTUAL_TERMINAL_PROCESSING and
// ENABLE_PROCESSED_OUTPUT are both set, which ConPTY does for its clients.
// A long line therefore arrives as one run of text and f4 wraps it itself,
// exactly as on Unix, so the wrap is f4's own record and the line can be
// reflowed and handed to F3/F4 whole. A resize sent through the signal pipe
// ends in SCREEN_INFORMATION::ResizeScreenBuffer and writes nothing to the
// terminal (ConhostInternalGetSet::ResizeWindow, src/host/outputStream.cpp),
// so a reflow on f4's side is not followed by a repaint from the host.
//
// Most in-box ConPTYs f4 runs on do not behave like that (the ones known to
// are listed in inboxConhostPreservingLines, pty_windows.go), so the
// redistributable is fetched on first use into the profile, next to the
// Colorer schemes, and f4 itself stays a single binary. conpty.dll starts
// OpenConsole.exe from its own directory, so the two always come from the
// same package and live in the same directory.
const (
	conPTYPackageVersion = "1.25.260710002-preview"
	conPTYPackageSHA256  = "05fe9b571ea4fb198f5012405cb39a132cf23eee50feaa496524c149b2502692"
	// maxConPTYPackageBytes bounds the download; the package is 1 717 498
	// bytes.
	maxConPTYPackageBytes = 16 << 20
	// maxConPTYMemberBytes bounds one extracted file; the largest is
	// 1 110 368 bytes.
	maxConPTYMemberBytes = 8 << 20
)

// conPTYPackageURL is a variable so that a test can point it at a local
// server; the address is upstream's release asset and is not configurable at
// runtime.
var conPTYPackageURL = "https://github.com/microsoft/terminal/releases/download/v1.25.1912.0/Microsoft.Windows.Console.ConPTY." + conPTYPackageVersion + ".nupkg"

// conPTYPackage describes the files f4 takes from the package for one
// architecture. Every file is checked against its own SHA-256 as well as the
// package's, so a copy in the profile is trusted only byte for byte.
type conPTYPackage struct {
	Version string
	URL     string
	SHA256  string
	Runtime string
	Files   []conPTYPackageFile
}

type conPTYPackageFile struct {
	Name   string // file name in the installed directory
	Entry  string // member name inside the package
	SHA256 string
}

// conPTYPackageFor returns the package files for a Go architecture. The
// DLL is loaded into f4's own process, so it must match f4's architecture,
// not the machine's.
func conPTYPackageFor(goarch string) (conPTYPackage, bool) {
	var runtimeName, dllSHA, exeSHA string
	switch goarch {
	case "amd64":
		runtimeName = "x64"
		dllSHA = "e2fe87e2258c4e46ffc5157f727218cc25f34a174902f72eb8a5b49edd9a6458"
		exeSHA = "2525c351aa136d555e5df9a3c9d6ce9be43f785e37e3c993b8f23b3f0a53c7fa"
	case "arm64":
		runtimeName = "arm64"
		dllSHA = "36a5a3977e83b888f353ce96bae2b5283708630fc43d0f518eaaf2235da8902c"
		exeSHA = "197a765e0a0b67a03b142ce9b93b1f428d92f7eee310b110bdf08383ed8d0d73"
	case "386":
		runtimeName = "x86"
		dllSHA = "399df767224480502cefaba207df422a01ef18e404300f9e922d3ba01429a77f"
		exeSHA = "1a76d1ef14f34cebeb2ab33088ff48b5456a839f369cfbccb6352497769956de"
	default:
		return conPTYPackage{}, false
	}
	return conPTYPackage{
		Version: conPTYPackageVersion,
		URL:     conPTYPackageURL,
		SHA256:  conPTYPackageSHA256,
		Runtime: "win-" + runtimeName,
		Files: []conPTYPackageFile{
			{Name: "conpty.dll", Entry: "runtimes/win-" + runtimeName + "/native/conpty.dll", SHA256: dllSHA},
			{Name: "OpenConsole.exe", Entry: "build/native/runtimes/" + runtimeName + "/OpenConsole.exe", SHA256: exeSHA},
		},
	}, true
}

// conPTYPackageDir is where a package is installed under the profile root.
func conPTYPackageDir(root string, pkg conPTYPackage) string {
	return filepath.Join(root, "conpty", pkg.Version, pkg.Runtime)
}

// verifyConPTYPackage reports whether dir holds exactly the package's files.
func verifyConPTYPackage(dir string, pkg conPTYPackage) error {
	for _, file := range pkg.Files {
		path := filepath.Join(dir, file.Name)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", path)
		}
		sum, err := fileSHA256(path)
		if err != nil {
			return err
		}
		if sum != file.SHA256 {
			return fmt.Errorf("%s: SHA-256 %s, want %s", path, sum, file.SHA256)
		}
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- a file of the installed package
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// installConPTYPackage makes sure the package is installed under root and
// returns its directory. A verified copy is used as is; anything else is
// downloaded, checked, staged in a sibling directory and moved into place,
// so no half-written package is ever visible under the final name.
func installConPTYPackage(ctx context.Context, client *http.Client, root string, pkg conPTYPackage) (string, error) {
	dir := conPTYPackageDir(root, pkg)
	if verifyConPTYPackage(dir, pkg) == nil {
		return dir, nil
	}
	data, err := downloadConPTYPackage(ctx, client, pkg)
	if err != nil {
		return "", err
	}
	members, err := extractConPTYPackage(data, pkg)
	if err != nil {
		return "", err
	}

	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(parent, ".stage-")
	if err != nil {
		return "", err
	}
	staged := true
	defer func() {
		if staged {
			_ = os.RemoveAll(stage)
		}
	}()
	for _, file := range pkg.Files {
		if err := os.WriteFile(filepath.Join(stage, file.Name), members[file.Name], 0o600); err != nil {
			return "", err
		}
	}

	if _, err := os.Lstat(dir); err == nil {
		// Another f4 may have finished first; its copy is as good as ours.
		if verifyConPTYPackage(dir, pkg) == nil {
			return dir, nil
		}
		damaged, err := os.MkdirTemp(parent, ".damaged-")
		if err != nil {
			return "", err
		}
		_ = os.Remove(damaged)
		if err := os.Rename(dir, damaged); err != nil {
			return "", fmt.Errorf("replace damaged ConPTY package %s: %w", dir, err)
		}
		defer func() { _ = os.RemoveAll(damaged) }()
	}
	if err := os.Rename(stage, dir); err != nil {
		if verifyConPTYPackage(dir, pkg) == nil {
			return dir, nil
		}
		return "", err
	}
	staged = false
	if err := verifyConPTYPackage(dir, pkg); err != nil {
		return "", err
	}
	return dir, nil
}

func downloadConPTYPackage(ctx context.Context, client *http.Client, pkg conPTYPackage) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pkg.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "f4-conpty-downloader")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: status %d", pkg.URL, resp.StatusCode)
	}
	if resp.ContentLength > maxConPTYPackageBytes {
		return nil, fmt.Errorf("download %s: %d bytes exceeds %d", pkg.URL, resp.ContentLength, maxConPTYPackageBytes)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxConPTYPackageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxConPTYPackageBytes {
		return nil, fmt.Errorf("download %s: exceeds %d bytes", pkg.URL, maxConPTYPackageBytes)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != pkg.SHA256 {
		return nil, fmt.Errorf("download %s: SHA-256 %s, want %s", pkg.URL, got, pkg.SHA256)
	}
	return data, nil
}

func extractConPTYPackage(data []byte, pkg conPTYPackage) (map[string][]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	members := make(map[string][]byte, len(pkg.Files))
	for _, file := range pkg.Files {
		var entry *zip.File
		for _, f := range zr.File {
			if f.Name == file.Entry {
				if entry != nil {
					return nil, fmt.Errorf("ConPTY package holds %s twice", file.Entry)
				}
				entry = f
			}
		}
		if entry == nil {
			return nil, fmt.Errorf("ConPTY package has no %s", file.Entry)
		}
		body, err := readZipMember(entry)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file.Entry, err)
		}
		sum := sha256.Sum256(body)
		if got := hex.EncodeToString(sum[:]); got != file.SHA256 {
			return nil, fmt.Errorf("%s: SHA-256 %s, want %s", file.Entry, got, file.SHA256)
		}
		members[file.Name] = body
	}
	return members, nil
}

func readZipMember(f *zip.File) ([]byte, error) {
	if f.UncompressedSize64 > maxConPTYMemberBytes {
		return nil, fmt.Errorf("%d bytes exceeds %d", f.UncompressedSize64, maxConPTYMemberBytes)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	body, err := io.ReadAll(io.LimitReader(rc, maxConPTYMemberBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxConPTYMemberBytes {
		return nil, errors.New("member exceeds its size bound")
	}
	return body, nil
}
