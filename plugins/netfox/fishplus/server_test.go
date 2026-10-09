package fishplus

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// serverSession connects a client Session to a Server through two pipes, the
// way a client reaches an f4 started as the remote command.
func serverSession(t *testing.T, srv *Server) (*Session, <-chan error) {
	t.Helper()
	cr, sw := io.Pipe() // server -> client
	sr, cw := io.Pipe() // client -> server
	done := make(chan error, 1)
	go func() {
		err := srv.Serve(sr, sw)
		_ = sw.Close()
		done <- err
	}()
	sess := NewSession(cw, cr, closerFunc(func() error { _ = cw.Close(); return cr.Close() }))
	t.Cleanup(func() { _ = sess.Close() })
	return sess, done
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

func TestServerNativeSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dir := t.TempDir()
	sess, done := serverSession(t, &Server{Dir: dir})

	if err := sess.HandshakeWithOptions(ctx, HandshakeOptions{Bootstrap: BootstrapNative}); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if !sess.Features().Has("native") || sess.Features().Proto != ProtocolVersion {
		t.Fatalf("features = %#v, want the native marker at protocol %d", sess.Features(), ProtocolVersion)
	}
	if err := sess.Noop(ctx); err != nil {
		t.Fatalf("noop: %v", err)
	}
	resp, err := sess.Exec(ctx, "pwd")
	if err != nil || !resp.OK() || strings.Join(resp.Lines, "\n") != dir {
		t.Fatalf("pwd = %#v, %v, want %q", resp, err, dir)
	}
	// A payload the line protocol has to escape survives the round trip.
	for _, payload := range []string{"plain", "with space", "~tilde", "two\nlines"} {
		got, err := sess.Ping(ctx, payload)
		if err != nil || got != payload {
			t.Fatalf("ping(%q) = %q, %v", payload, got, err)
		}
	}
	// A command the server does not implement is refused after its path lines
	// were read, and the session stays usable.
	resp, err = sess.ExecPaths(ctx, "grep", []string{"pattern", "/nowhere"}, "f", "10")
	if err != nil || resp.OK() || !strings.Contains(resp.Msg, "unknown command") {
		t.Fatalf("grep = %#v, %v, want an unknown command error", resp, err)
	}
	if err := sess.Noop(ctx); err != nil {
		t.Fatalf("noop after a refused command: %v", err)
	}
	resp, err = sess.Exec(ctx, "exit")
	if err != nil || !resp.OK() {
		t.Fatalf("exit = %#v, %v", resp, err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Serve: %v", err)
	}
}

func TestServerDefaultsToTheWorkingDirectory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sess, _ := serverSession(t, &Server{})
	if err := sess.HandshakeWithOptions(ctx, HandshakeOptions{Bootstrap: BootstrapNative}); err != nil {
		t.Fatal(err)
	}
	want, _ := os.Getwd()
	resp, err := sess.Exec(ctx, "pwd")
	if err != nil || strings.Join(resp.Lines, "\n") != want {
		t.Fatalf("pwd = %#v, %v, want %q", resp, err, want)
	}
}

func TestServerRejectsABadHelloAndAnUnfollowableRequest(t *testing.T) {
	var out strings.Builder
	if err := (&Server{}).Serve(strings.NewReader("hello\n"), &out); err == nil {
		t.Fatal("a hello without the native prefix must be refused")
	}
	out.Reset()
	in := NativeHelloLine("tok") + "1 patch 1 raw\n/x\n/y\nseg\n"
	if err := (&Server{}).Serve(strings.NewReader(in), &out); err == nil {
		t.Fatal("a patch segment that cannot be followed must end the session")
	}
	if !strings.Contains(out.String(), ".tok 1 err bad patch segment") {
		t.Fatalf("the refusal was not sent: %q", out.String())
	}
}

func TestServerFileSystemCommands(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a file.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hidden"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	haveLinks := os.Symlink("sub", filepath.Join(root, "link")) == nil

	sess, _ := serverSession(t, &Server{Dir: root})
	if err := sess.HandshakeWithOptions(ctx, HandshakeOptions{Bootstrap: BootstrapNative}); err != nil {
		t.Fatal(err)
	}
	if sess.Features().ListingMode() != "find" {
		t.Fatalf("listing mode = %q, want find", sess.Features().ListingMode())
	}
	c := NewClient(sess)

	entries, err := c.Enum(ctx, root)
	if err != nil {
		t.Fatalf("enum: %v", err)
	}
	byName := map[string]Entry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	if f, ok := byName["a file.txt"]; !ok || !f.IsRegular() || f.Size != 5 || (runtime.GOOS != "windows" && f.Perm() != 0o600) {
		t.Errorf("a file.txt = %#v", f)
	}
	if d, ok := byName["sub"]; !ok || !d.IsDir() {
		t.Errorf("sub = %#v", d)
	}
	if _, ok := byName[".hidden"]; !ok {
		t.Error("hidden entries must be listed")
	}
	if haveLinks {
		if l, ok := byName["link"]; !ok || !l.IsSymlink() || !l.TargetIsDir {
			t.Errorf("link = %#v, want a symlink to a directory", l)
		}
		if target, err := c.ReadLink(ctx, filepath.Join(root, "link")); err != nil || target != "sub" {
			t.Errorf("ReadLink = %q, %v", target, err)
		}
		if e, err := c.Stat(ctx, filepath.Join(root, "link")); err != nil || !e.IsDir() {
			t.Errorf("Stat through a link = %#v, %v, want a directory", e, err)
		}
		if e, err := c.Lstat(ctx, filepath.Join(root, "link")); err != nil || !e.IsSymlink() {
			t.Errorf("Lstat of a link = %#v, %v, want the link", e, err)
		}
	}
	if e, err := c.Stat(ctx, filepath.ToSlash(filepath.Join(root, "a file.txt"))); err != nil || e.Size != 5 || e.Name != "a file.txt" {
		t.Errorf("Stat = %#v, %v", e, err)
	}
	if _, err := c.Stat(ctx, filepath.Join(root, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat of a missing path = %v, want os.ErrNotExist", err)
	}
	if _, err := c.Stat(ctx, "relative"); err == nil {
		t.Error("a relative path must be refused")
	}
	big := make([]byte, 700*1024+13) // more than two chunks
	for i := range big {
		big[i] = byte(i * 7)
	}
	bigPath := filepath.Join(root, "big.bin")
	if err := os.WriteFile(bigPath, big, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := c.ReadFile(ctx, bigPath); err != nil || !bytes.Equal(got, big) {
		t.Errorf("ReadFile = %d bytes, %v, want the %d bytes of the file", len(got), err, len(big))
	}
	if part, size, err := c.Read(ctx, bigPath, 100, 50); err != nil || size != int64(len(big)) || !bytes.Equal(part, big[100:150]) {
		t.Errorf("Read(100, 50) = %d bytes, size %d, %v", len(part), size, err)
	}
	if part, _, err := c.Read(ctx, bigPath, int64(len(big))+10, 5); err != nil || len(part) != 0 {
		t.Errorf("Read past the end = %d bytes, %v, want none", len(part), err)
	}
	if _, _, err := c.Read(ctx, root, 0, 10); err == nil {
		t.Error("reading a directory must fail")
	}
	dirs, err := c.TargetDirs(ctx, []string{root, filepath.Join(root, "a file.txt"), filepath.Join(root, "missing")})
	if err != nil || len(dirs) != 3 || !dirs[0] || dirs[1] || dirs[2] {
		t.Errorf("TargetDirs = %v, %v, want [true false false]", dirs, err)
	}
}

func TestServerMutations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	sess, _ := serverSession(t, &Server{Dir: root})
	if err := sess.HandshakeWithOptions(ctx, HandshakeOptions{Bootstrap: BootstrapNative}); err != nil {
		t.Fatal(err)
	}
	c := NewClient(sess)
	at := func(parts ...string) string { return filepath.Join(append([]string{root}, parts...)...) }

	if err := c.MkDir(ctx, at("a", "b")); err != nil {
		t.Fatalf("MkDir: %v", err)
	}
	if fi, err := os.Stat(at("a", "b")); err != nil || !fi.IsDir() {
		t.Fatalf("MkDir did not create the directory: %v", err)
	}
	if err := os.WriteFile(at("a", "f.txt"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := c.Copy(ctx, at("a"), at("copy")); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if got, err := os.ReadFile(at("copy", "f.txt")); err != nil || string(got) != "data" {
		t.Fatalf("copied file = %q, %v", got, err)
	}
	if err := c.Rename(ctx, at("copy"), at("moved")); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := os.Stat(at("copy")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Rename left the source behind")
	}

	if err := c.Remove(ctx, at("moved")); err == nil {
		t.Error("rm of a directory must fail")
	}
	if err := c.RemoveDir(ctx, at("moved")); err == nil {
		t.Error("rmdir of a non-empty directory must fail")
	}
	if err := c.Remove(ctx, at("moved", "f.txt")); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := c.RemoveAll(ctx, at("moved")); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	if _, err := os.Stat(at("moved")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("RemoveAll left the tree behind")
	}

	if runtime.GOOS != "windows" {
		if err := c.Chmod(ctx, at("a", "f.txt"), 0o640); err != nil {
			t.Fatalf("Chmod: %v", err)
		}
		if fi, _ := os.Stat(at("a", "f.txt")); fi.Mode().Perm() != 0o640 {
			t.Errorf("mode after Chmod = %v", fi.Mode().Perm())
		}
		if err := c.Symlink(ctx, at("a", "lnk"), "f.txt"); err != nil {
			t.Fatalf("Symlink: %v", err)
		}
		if target, _ := os.Readlink(at("a", "lnk")); target != "f.txt" {
			t.Errorf("link target = %q", target)
		}
		if err := c.Symlink(ctx, at("a", "lnk"), "other"); err == nil {
			t.Error("an existing link path must be refused")
		}
	}
	when := time.Unix(1_700_000_000, 0)
	if err := c.Chtimes(ctx, at("a", "f.txt"), when, when); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	if fi, _ := os.Stat(at("a", "f.txt")); !fi.ModTime().Equal(when) {
		t.Errorf("mtime after Chtimes = %v", fi.ModTime())
	}

	// The guard: relative paths, .. components and the root are refused, and
	// nothing outside the tree is touched.
	if err := c.RemoveAll(ctx, root+string(filepath.Separator)+"a"+string(filepath.Separator)+".."+string(filepath.Separator)+"a"); err == nil {
		t.Error("a .. component must be refused")
	}
	if err := c.RemoveAll(ctx, string(filepath.Separator)); err == nil {
		t.Error("the root directory must be refused")
	}
	if err := c.MkDir(ctx, "relative/dir"); err == nil {
		t.Error("a relative path must be refused")
	}
	if _, err := os.Stat(at("a", "f.txt")); err != nil {
		t.Fatalf("a refused request damaged the tree: %v", err)
	}
	if err := sess.Noop(ctx); err != nil {
		t.Fatalf("the session must stay usable after refusals: %v", err)
	}
}

func TestServerWrite(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	sess, _ := serverSession(t, &Server{Dir: root})
	if err := sess.HandshakeWithOptions(ctx, HandshakeOptions{Bootstrap: BootstrapNative}); err != nil {
		t.Fatal(err)
	}
	if sess.Features().WriteMode() != "ddbytes" {
		t.Fatalf("write mode = %q, want ddbytes", sess.Features().WriteMode())
	}
	c := NewClient(sess)
	file := filepath.Join(root, "out.bin")

	if err := c.Write(ctx, file, 0, []byte("hello world")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// A range in the middle leaves what follows it alone.
	if err := c.Write(ctx, file, 6, []byte("WORLD")); err != nil {
		t.Fatalf("Write at an offset: %v", err)
	}
	if got, _ := os.ReadFile(file); string(got) != "hello WORLD" {
		t.Fatalf("file = %q, want %q", got, "hello WORLD")
	}
	// A range past the end leaves a hole of zeros.
	if err := c.Write(ctx, file, 14, []byte("!")); err != nil {
		t.Fatalf("Write past the end: %v", err)
	}
	if got, _ := os.ReadFile(file); !bytes.Equal(got, []byte("hello WORLD\x00\x00\x00!")) {
		t.Fatalf("file = %q", got)
	}
	// Bytes that look like a request stay payload.
	tricky := []byte("1 noop\n2 exit\n")
	if err := c.Write(ctx, file, 0, tricky); err != nil {
		t.Fatalf("Write of request-shaped bytes: %v", err)
	}
	if got, _ := os.ReadFile(file); !bytes.HasPrefix(got, tricky) {
		t.Fatalf("file = %q", got)
	}
	if err := c.Truncate(ctx, file, 4); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	if got, _ := os.ReadFile(file); string(got) != "1 no" {
		t.Fatalf("file after Truncate = %q", got)
	}

	// A refused write still consumes its payload and says so, so the session
	// carries on.
	if err := c.Write(ctx, "relative.bin", 0, tricky); err == nil {
		t.Fatal("a relative path must be refused")
	}
	if err := sess.Noop(ctx); err != nil {
		t.Fatalf("the session must survive a refused write: %v", err)
	}
	if sess.Broken() {
		t.Fatal("a refused write must not mark the session broken")
	}
}

func TestServerPatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	sess, _ := serverSession(t, &Server{Dir: root})
	if err := sess.HandshakeWithOptions(ctx, HandshakeOptions{Bootstrap: BootstrapNative}); err != nil {
		t.Fatal(err)
	}
	c := NewClient(sess)
	if !c.CanPatch() {
		t.Fatal("the server must announce what patch needs")
	}
	src := filepath.Join(root, "src.txt")
	dst := filepath.Join(root, "dst.txt")
	if err := os.WriteFile(src, []byte("hello world"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Two copies around a literal: one changed byte range crosses the wire, the
	// rest is copied where the file is.
	segs := []PatchSegment{Copy(0, 6), Literal([]byte("F4\n1 exit\n")), Copy(8, 3)}
	if err := c.Patch(ctx, src, dst, segs); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "hello F4\n1 exit\nrld" {
		t.Fatalf("patched file = %q", got)
	}
	if err := c.Patch(ctx, src, dst, []PatchSegment{Copy(0, 5)}); err != nil {
		t.Fatalf("second Patch: %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "hello" {
		t.Fatalf("second patched file = %q, want the destination rebuilt from nothing", got)
	}

	// A refusal still leaves the stream where the next request expects it.
	if err := c.Patch(ctx, "relative", dst, segs); err == nil {
		t.Fatal("a relative source must be refused")
	}
	if err := c.Patch(ctx, src, src, segs); err == nil {
		t.Fatal("source and destination must differ")
	}
	if err := sess.Noop(ctx); err != nil || sess.Broken() {
		t.Fatalf("the session must survive a refused patch: %v, broken=%v", err, sess.Broken())
	}
}
