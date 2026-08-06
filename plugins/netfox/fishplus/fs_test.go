package fishplus

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseFindEntry(t *testing.T) {
	e, err := parseFindEntry("f f 5 1785869231.1077299730 1785869231.1037299730 1785869231.1077299733 644 1000 100 a file.txt")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if e.Name != "a file.txt" {
		t.Errorf("Name = %q", e.Name)
	}
	if e.Size != 5 || e.Uid != 1000 || e.Gid != 100 {
		t.Errorf("Size/Uid/Gid = %d/%d/%d", e.Size, e.Uid, e.Gid)
	}
	if e.Mode != 0100644 {
		t.Errorf("Mode = %o, want %o", e.Mode, 0100644)
	}
	if !e.IsRegular() || e.IsDir() || e.IsSymlink() {
		t.Error("regular file misclassified")
	}
	if e.MTime.Unix() != 1785869231 || e.MTime.Nanosecond() != 107729973 {
		t.Errorf("MTime = %v", e.MTime)
	}
	if e.CTime.Unix() != 1785869231 {
		t.Errorf("CTime = %v", e.CTime)
	}
}

func TestParseFindEntrySymlinkToDir(t *testing.T) {
	e, err := parseFindEntry("l d 7 1785869231.10 1785869231.10 1785869231.10 777 0 0 link to dir")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !e.IsSymlink() {
		t.Errorf("Mode = %o, want a symlink", e.Mode)
	}
	if !e.TargetIsDir {
		t.Error("TargetIsDir not detected although find resolved the link to a directory")
	}
	if e.IsDir() {
		t.Error("a symlink must not be reported as a directory itself")
	}
}

func TestParseStatEntry(t *testing.T) {
	e, err := parseStatEntry("41ed 4096 1785869231 1785869230 1785869229 0 0 /tmp/x/sub dir")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if e.Name != "sub dir" {
		t.Errorf("Name = %q", e.Name)
	}
	if !e.IsDir() {
		t.Errorf("Mode = %o, want a directory", e.Mode)
	}
	if e.Perm() != 0755 {
		t.Errorf("Perm = %o", e.Perm())
	}
	if e.MTime.Unix() != 1785869231 || e.ATime.Unix() != 1785869230 || e.CTime.Unix() != 1785869229 {
		t.Errorf("timestamps mixed up: %v %v %v", e.MTime, e.ATime, e.CTime)
	}
}

func TestParseBSDStatEntry(t *testing.T) {
	e, err := parseBSDStatEntry("120777 7 1785869231 1785869231 1785869231 501 20 /tmp/x/link")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !e.IsSymlink() {
		t.Errorf("Mode = %o, want a symlink", e.Mode)
	}
	if e.Uid != 501 || e.Gid != 20 {
		t.Errorf("Uid/Gid = %d/%d", e.Uid, e.Gid)
	}
}

// TestParseFoundListingKeepsFullPaths guards the difference between the two
// parsers. A listing wants the bare name; a search answers with paths, and
// the stat backends print the path they were handed, so reducing it to a
// base name silently loses the directory the hit was in. This needs no shell
// and therefore catches it on any machine, including one whose stat is GNU.
func TestParseFoundListingKeepsFullPaths(t *testing.T) {
	cases := []struct {
		mode string
		line string
	}{
		{"find", "f f 12 1785869231.0 1785869231.0 1785869231.0 644 1000 1000 /tmp/x/sub dir/b.txt"},
		{"stat", "81a4 12 1785869231 1785869231 1785869231 1000 1000 /tmp/x/sub dir/b.txt"},
		{"statbsd", "100644 12 1785869231 1785869231 1785869231 1000 1000 /tmp/x/sub dir/b.txt"},
	}
	for _, tc := range cases {
		mode, entries, err := ParseFoundListing([]string{"M " + tc.mode, tc.line})
		if err != nil {
			t.Fatalf("%s: %v", tc.mode, err)
		}
		if mode != tc.mode {
			t.Errorf("mode = %q, want %q", mode, tc.mode)
		}
		if len(entries) != 1 {
			t.Fatalf("%s: got %d entries", tc.mode, len(entries))
		}
		if entries[0].Name != "/tmp/x/sub dir/b.txt" {
			t.Errorf("%s: Name = %q, want the full path", tc.mode, entries[0].Name)
		}
		if entries[0].Size != 12 {
			t.Errorf("%s: Size = %d, want 12", tc.mode, entries[0].Size)
		}

		// The stat backends print the path they were handed, so the listing
		// parser has to reduce it or every panel would show full paths in
		// its name column. The find backend is exempt: a listing there is
		// printed with %f, which is the bare name to begin with, and only a
		// search asks for %p.
		if tc.mode == "find" {
			continue
		}
		_, entries, err = ParseListing([]string{"M " + tc.mode, tc.line})
		if err != nil {
			t.Fatalf("%s: %v", tc.mode, err)
		}
		if len(entries) != 1 || entries[0].Name != "b.txt" {
			t.Errorf("%s: listing Name = %+v, want b.txt", tc.mode, entries)
		}
	}
}

func TestParseListingSkipsDotsAndGarbage(t *testing.T) {
	lines := []string{
		"M stat",
		"41c0 4096 1785869231 1785869231 1785869231 0 0 /tmp/x/.",
		"43ff 4096 1785869231 1785869231 1785869231 0 0 /tmp/x/..",
		"stat: cannot statx '/tmp/x/gone': No such file or directory",
		"81a4 5 1785869231 1785869231 1785869231 0 0 /tmp/x/a file.txt",
	}
	mode, entries, err := ParseListing(lines)
	if err != nil {
		t.Fatalf("ParseListing: %v", err)
	}
	if mode != "stat" {
		t.Errorf("mode = %q", mode)
	}
	if len(entries) != 1 || entries[0].Name != "a file.txt" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParseListingRejectsMissingMarker(t *testing.T) {
	if _, _, err := ParseListing([]string{"81a4 5 1 1 1 0 0 /tmp/x"}); err == nil {
		t.Error("listing without a mode marker accepted")
	}
	if _, _, err := ParseListing(nil); err == nil {
		t.Error("empty listing accepted")
	}
}

func TestTargetDirsUsesOneRequestAndPreservesPathEncoding(t *testing.T) {
	paths := []string{"/tmp/link to dir", "/tmp/link\nwith newline", "/tmp/broken"}
	seen := make(chan mockRequest, 1)
	sess := newMockPeer(t, "ok FISHPLUS 1 stat", func(w io.Writer, token string, req mockRequest) {
		seen <- req
		fmt.Fprintln(w, "1")
		fmt.Fprintln(w, "0")
		fmt.Fprintln(w, "0")
		fmt.Fprintf(w, ".%s %s ok\n", token, req.ID)
	}, len(paths))
	if err := sess.Handshake(context.Background()); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	c := NewClient(sess)

	empty, err := c.TargetDirs(context.Background(), nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("TargetDirs(nil) = %v, %v", empty, err)
	}
	got, err := c.TargetDirs(context.Background(), paths)
	if err != nil {
		t.Fatalf("TargetDirs: %v", err)
	}
	want := []bool{true, false, false}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("TargetDirs = %v, want %v", got, want)
	}

	req := <-seen
	if req.Cmd != "isdirs" || len(req.Args) != 1 || req.Args[0] != "3" {
		t.Fatalf("request = %q %q, want isdirs 3", req.Cmd, req.Args)
	}
	if decoded := req.decodePaths(t); fmt.Sprint(decoded) != fmt.Sprint(paths) {
		t.Errorf("paths = %q, want %q", decoded, paths)
	}
}

func TestTargetDirsRejectsMalformedAnswers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines []string
	}{
		{name: "too few", lines: []string{"1"}},
		{name: "invalid value", lines: []string{"1", "yes"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sess := newMockPeer(t, "ok FISHPLUS 1 stat", func(w io.Writer, token string, req mockRequest) {
				for _, line := range tc.lines {
					fmt.Fprintln(w, line)
				}
				fmt.Fprintf(w, ".%s %s ok\n", token, req.ID)
			}, 2)
			if err := sess.Handshake(context.Background()); err != nil {
				t.Fatalf("handshake: %v", err)
			}
			if _, err := NewClient(sess).TargetDirs(context.Background(), []string{"/a", "/b"}); err == nil {
				t.Fatal("malformed isdirs response accepted")
			}
		})
	}
}

// newLocalShellClient starts the real helper in a local POSIX shell.
func newLocalShellClient(t *testing.T) *Client {
	t.Helper()
	return newLocalShellClientEnv(t)
}

// newLocalShellClientEnv is the same with extra environment entries. That is
// how a host with different tools is simulated: a PATH pointing at stubs
// makes the helper take the branch a macOS or BSD box would take.
func newLocalShellClientEnv(t *testing.T, extraEnv ...string) *Client {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX shell on Windows")
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell available")
	}
	cmd := exec.Command(shell)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	stderr := &syncBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", shell, err)
	}
	sess := NewSession(stdin, stdout, stdin)
	t.Cleanup(func() {
		sess.Close()
		done := make(chan struct{})
		go func() {
			cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			cmd.Process.Kill()
		}
	})
	if err := sess.Handshake(context.Background()); err != nil {
		if strings.Contains(err.Error(), "base64") {
			t.Skipf("no base64 on this host: %v", err)
		}
		t.Fatalf("handshake: %v (shell stderr: %s)", err, stderr.String())
	}
	return NewClient(sess)
}

// TestListingAgainstLocalShell exercises every metadata backend the local
// host happens to provide, so the parsers are checked against real tool
// output and not only against captured samples.
func TestListingAgainstLocalShell(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a file.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hidden"), []byte("0123456789"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub dir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("sub dir", filepath.Join(dir, "link to dir")); err != nil {
		t.Skipf("symlinks not supported here: %v", err)
	}

	if mode := c.Session().Features().ListingMode(); mode == "" {
		t.Fatal("handshake announced no listing mode")
	}

	tried := 0
	for _, mode := range ListingModes {
		if err := c.SetListingMode(ctx, mode); err != nil {
			t.Logf("listing mode %q unavailable here: %v", mode, err)
			continue
		}
		tried++
		t.Run(mode, func(t *testing.T) {
			entries, err := c.Enum(ctx, dir)
			if err != nil {
				t.Fatalf("enum: %v", err)
			}
			byName := make(map[string]Entry, len(entries))
			for _, e := range entries {
				byName[e.Name] = e
			}
			if len(byName) != 4 {
				t.Fatalf("got %d entries: %+v", len(byName), entries)
			}

			file, ok := byName["a file.txt"]
			if !ok {
				t.Fatal("a file with a space in its name got lost")
			}
			if file.Size != 5 {
				t.Errorf("Size = %d, want 5", file.Size)
			}
			if !file.IsRegular() {
				t.Errorf("Mode = %o, want a regular file", file.Mode)
			}
			if file.Perm() != 0644 {
				t.Errorf("Perm = %o, want 644", file.Perm())
			}
			if time.Since(file.MTime) > time.Hour || time.Since(file.MTime) < -time.Hour {
				t.Errorf("MTime = %v, which is nowhere near now", file.MTime)
			}

			if hidden, ok := byName[".hidden"]; !ok {
				t.Error("hidden file missing from the listing")
			} else if hidden.Size != 10 {
				t.Errorf("hidden Size = %d, want 10", hidden.Size)
			}

			if sub, ok := byName["sub dir"]; !ok {
				t.Error("subdirectory missing from the listing")
			} else if !sub.IsDir() {
				t.Errorf("sub dir Mode = %o, want a directory", sub.Mode)
			}

			if link, ok := byName["link to dir"]; !ok {
				t.Error("symlink missing from the listing")
			} else if !link.IsSymlink() {
				t.Errorf("link Mode = %o, want a symlink", link.Mode)
			}

			linkPath := filepath.Join(dir, "link to dir")
			st, err := c.Stat(ctx, linkPath)
			if err != nil {
				t.Fatalf("stat: %v", err)
			}
			if !st.IsDir() {
				t.Errorf("Stat followed no symlink: Mode = %o", st.Mode)
			}
			if st.Name != "link to dir" {
				t.Errorf("Stat Name = %q", st.Name)
			}
			lst, err := c.Lstat(ctx, linkPath)
			if err != nil {
				t.Fatalf("lstat: %v", err)
			}
			if !lst.IsSymlink() {
				t.Errorf("Lstat resolved the symlink: Mode = %o", lst.Mode)
			}

			target, err := c.ReadLink(ctx, linkPath)
			if err != nil {
				t.Fatalf("readlink: %v", err)
			}
			if target != "sub dir" {
				t.Errorf("ReadLink = %q, want %q", target, "sub dir")
			}

			if _, err := c.Stat(ctx, filepath.Join(dir, "no such file")); err == nil {
				t.Error("stat of a missing file succeeded")
			}
			if _, err := c.Enum(ctx, filepath.Join(dir, "a file.txt")); err == nil {
				t.Error("enum of a regular file succeeded")
			}
		})
	}
	if tried == 0 {
		t.Fatal("no metadata backend available on this host")
	}
}

func TestTargetDirsAgainstLocalShell(t *testing.T) {
	c := newLocalShellClient(t)
	dir := t.TempDir()
	targetDir := filepath.Join(dir, "target dir")
	targetFile := filepath.Join(dir, "target file")
	if err := os.Mkdir(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetFile, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	dirLink := filepath.Join(dir, "directory link")
	fileLink := filepath.Join(dir, "file link")
	brokenLink := filepath.Join(dir, "broken link")
	if err := os.Symlink(targetDir, dirLink); err != nil {
		t.Skipf("symlinks not supported here: %v", err)
	}
	if err := os.Symlink(targetFile, fileLink); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "missing"), brokenLink); err != nil {
		t.Fatal(err)
	}

	got, err := c.TargetDirs(context.Background(), []string{dirLink, fileLink, brokenLink})
	if err != nil {
		t.Fatalf("TargetDirs: %v", err)
	}
	want := []bool{true, false, false}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("TargetDirs = %v, want %v", got, want)
	}
}

func TestListingSurvivesWeirdNames(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()
	dir := t.TempDir()
	names := []string{"trailing space ", "back\\slash", "tab\there", "юникод", "-dash", "~tilde"}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Skipf("cannot create %q here: %v", name, err)
		}
	}
	entries, err := c.Enum(ctx, dir)
	if err != nil {
		t.Fatalf("enum: %v", err)
	}
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		seen[e.Name] = true
	}
	for _, name := range names {
		if !seen[name] {
			t.Errorf("entry %q lost, got %+v", name, entries)
		}
	}
	for _, name := range names {
		if _, err := c.Stat(ctx, filepath.Join(dir, name)); err != nil {
			t.Errorf("stat %q: %v", name, err)
		}
	}
}
func TestPwdAgainstLocalShell(t *testing.T) {
	c := newLocalShellClient(t)
	cwd, err := c.Pwd(context.Background())
	if err != nil {
		t.Fatalf("pwd: %v", err)
	}
	if !strings.HasPrefix(cwd, "/") {
		t.Errorf("Pwd = %q, want an absolute path", cwd)
	}
	if _, err := c.Enum(context.Background(), cwd); err != nil {
		t.Errorf("the directory Pwd reported cannot be listed: %v", err)
	}
}
