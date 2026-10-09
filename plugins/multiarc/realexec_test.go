package multiarc

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

// The *_realexec_test.go files in this package run the actual archivers
// against real archives, with runTool/lookupTool left as production has
// them. Each one skips when the tool it needs is not on PATH, which is what
// lets the same files run on every CI cell: a Linux runner has GNU tar, zip
// and unzip, a macOS runner has bsdtar, and a Windows runner has bsdtar as
// tar.exe and usually nothing else. The helpers below are shared by them.

// runProbe reports whether running path with args exits with one of the
// NTSTATUS-range codes Windows' own loader produces (0xC0000000 and up) --
// 0xC0000135 for STATUS_DLL_NOT_FOUND among them -- rather than ever
// running the tool's own code.
func runProbe(path string, args ...string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...) // #nosec G204 -- path came from exec.LookPath for a tool name the test itself chose; args are fixed literals or a throwaway probe file this same function made.
	err := cmd.Run()
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && uint32(exitErr.ExitCode()) >= 0xC0000000
}

// toolCannotStart reports whether the executable at path, resolved for
// name, is on PATH but fails even to start. A CI runner's own copy of a
// tool can be present yet broken -- missing a DLL it needs, say -- which
// exec.LookPath cannot see. Every real-exec test that needs a working
// tool, not just one that resolves on PATH, treats that exactly like the
// tool being absent.
//
// unzip gets its own probe rather than a bare invocation: the DLL it turned
// out to be missing on a Windows runner is one it only loads to read a
// real zip's central directory, not on a bare "unzip" with no archive to
// open, so a bare probe never saw the failure at all -- unzipCannotStart
// gives it one.
func toolCannotStart(name, path string) bool {
	if name == "unzip" {
		return unzipCannotStart(path)
	}
	return runProbe(path)
}

// unzipCannotStart is toolCannotStart's probe for unzip: a real listing of
// a real (if trivial) zip file, the same operation the zip backend depends
// on unzip for, made with Go's own archive/zip so the probe needs nothing
// beyond a tool that resolved on PATH.
func unzipCannotStart(path string) bool {
	dir, err := os.MkdirTemp("", "f4-unzip-probe-")
	if err != nil {
		return false // cannot tell; the real test will find out
	}
	defer func() { _ = os.RemoveAll(dir) }()
	probe := filepath.Join(dir, "probe.zip")
	f, err := os.Create(probe) // #nosec G304 -- probe is this function's own t.TempDir-style file.
	if err != nil {
		return false
	}
	zw := zip.NewWriter(f)
	if w, err := zw.Create("probe.txt"); err == nil {
		_, _ = w.Write([]byte("probe"))
	}
	_ = zw.Close()
	_ = f.Close()
	return runProbe(path, "-Z1", probe)
}

// requireRealTool skips the test unless every one of names is on PATH and
// actually runs.
func requireRealTool(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Skipf("%s is not on PATH", name)
		}
		if toolCannotStart(name, path) {
			t.Skipf("%s is on PATH but does not start", name)
		}
	}
}

// writeTree creates files under root. A key ending in "/" makes an empty
// directory; any other key is a file holding its value.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		full := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(name, "/")))
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", full, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
}

// runReal runs a tool in dir to build a test fixture, failing the test if
// the tool fails. It execs directly rather than through runTool, so a
// fixture never depends on the code it is there to test.
func runReal(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...) // #nosec G204 -- test fixture: a known archiver and paths under t.TempDir.
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v (%s)", name, args, err, out)
	}
}

// openReal opens the archive at localPath the way the provider would: by
// its name, through whichever backend detectFormat picks from the real PATH.
func openReal(t *testing.T, localPath string) *MultiArcVFS {
	t.Helper()
	b, id, ok := detectFormat(filepath.Base(localPath))
	if !ok {
		t.Fatalf("detectFormat(%s): no backend", localPath)
	}
	return NewMultiArcVFS(vfs.NewOSVFS(filepath.Dir(localPath)), localPath, filepath.Base(localPath), b, id)
}

// memberPaths lists every member the archive holds, sorted, directories
// marked with a trailing "/", read back from a fresh listing rather than
// from any state a MultiArcVFS might have cached. A member stored twice
// (a tar -r that did not delete first) shows up twice.
func memberPaths(t *testing.T, localPath string) []string {
	t.Helper()
	b, _, ok := detectFormat(filepath.Base(localPath))
	if !ok {
		t.Fatalf("detectFormat(%s): no backend", localPath)
	}
	entries, err := b.list(context.Background(), localPath)
	if err != nil {
		t.Fatalf("list %s: %v", localPath, err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		p := e.Path
		if e.IsDir {
			p += "/"
		}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// writeMember stores content as the member p through Create, the way the
// copy engine and the editor do.
func writeMember(t *testing.T, v *MultiArcVFS, p, content string) {
	t.Helper()
	w, err := v.Create(context.Background(), p)
	if err != nil {
		t.Fatalf("Create %s: %v", p, err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatalf("Write %s: %v", p, err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close %s: %v", p, err)
	}
}

// pathOnly replaces PATH, for the rest of the test, with a directory
// holding just the named tools, each a symlink to the real binary found on
// the current PATH under the target name (so "tar": "bsdtar" puts bsdtar on
// PATH as tar). It skips the test when a tool is missing, cannot start
// through the symlink, or the symlink cannot be made (Windows without the
// privilege).
//
// The cannot-start check runs after PATH is switched, through the symlink
// itself rather than the original path LookPath found target at: on
// Windows, CreateProcess resolves the DLL search directory from the path
// it was actually asked to run, not from wherever a symlink points, so a
// tool needing a DLL that sits next to its own real binary can start fine
// run directly and still fail once it is only reachable through a symlink
// in an otherwise-empty directory (unzip.exe was one, on the CI runners).
func pathOnly(t *testing.T, tools map[string]string) {
	t.Helper()
	bin := t.TempDir()
	for name, target := range tools {
		real, err := exec.LookPath(target)
		if err != nil {
			t.Skipf("%s is not on PATH", target)
		}
		link := filepath.Join(bin, name+filepath.Ext(real))
		if err := os.Symlink(real, link); err != nil {
			t.Skipf("cannot symlink %s: %v", real, err)
		}
	}
	t.Setenv("PATH", bin)
	for name := range tools {
		p, err := exec.LookPath(name)
		if err != nil {
			t.Skipf("%s is not on the replaced PATH", name)
		}
		// name, not target: a caller giving target as an already-resolved
		// path (realToolPath's callers do) rather than a bare tool name
		// would otherwise never match unzipCannotStart's dispatch.
		if toolCannotStart(name, p) {
			t.Skipf("%s does not start through its symlink", name)
		}
	}
}

// realTarFlavor is what the tar on the real PATH is.
func realTarFlavor(t *testing.T) tarFlavor {
	t.Helper()
	requireRealTool(t, "tar")
	return probeTar(context.Background()).flavor
}

func assertMembers(t *testing.T, localPath string, want ...string) {
	t.Helper()
	sort.Strings(want)
	got := memberPaths(t, localPath)
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("members of %s =\n\t%q\nwant\n\t%q", filepath.Base(localPath), got, want)
	}
}

// readMember reads one member back through MultiArcVFS.Open.
func readMember(t *testing.T, v *MultiArcVFS, p string) string {
	t.Helper()
	f, err := v.Open(context.Background(), p)
	if err != nil {
		t.Fatalf("Open %s: %v", p, err)
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, f.Size())
	n, err := f.ReadAt(context.Background(), buf, 0)
	if err != nil && n != len(buf) {
		t.Fatalf("ReadAt %s: %v", p, err)
	}
	return string(buf[:n])
}
