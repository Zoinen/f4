package netfox

import (
	"bufio"
	"context"
	"fmt"
	"github.com/unxed/f4/internal/netproxy"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/plugins/netfox/fishplus"
	"github.com/unxed/f4/vfs"
)

type fishPanelInfoStub struct {
	key          string
	snapshot     vfs.PanelInfoSnapshot
	refreshCalls int
}

func (p *fishPanelInfoStub) PanelInfoKey(vfs.PanelInfoRequest) string { return p.key }
func (p *fishPanelInfoStub) CachedPanelInfo(vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, bool) {
	return p.snapshot, false
}
func (p *fishPanelInfoStub) RefreshPanelInfo(context.Context, vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
	p.refreshCalls++
	return p.snapshot, nil
}

func TestFishVFSPanelInfoProviderSurvivesCloneForParent(t *testing.T) {
	provider := &fishPanelInfoStub{
		key: "android:serial:/sdcard",
		snapshot: vfs.PanelInfoSnapshot{Authoritative: true, Sections: []vfs.PanelInfoSection{{
			ID: "android", Fields: []vfs.PanelInfoField{{ID: "model", Value: "phone"}},
		}}},
	}
	original := &FishVFS{path: "/sdcard", title: "phone"}
	original.SetPanelInfoProvider(provider)
	original.SetPanelTitleFormatter(func(title, path string) string {
		return title + "|" + path
	})
	parent := vfs.NewNullVFS(0)
	clone := original.CloneForParent(parent)
	if clone == original || clone.ParentVFS() != parent || clone.GetPath() != "/sdcard" {
		t.Fatalf("clone identity = cloneSame %v parent %T path %q", clone == original, clone.ParentVFS(), clone.GetPath())
	}
	if clone.panelInfoProvider() != provider {
		t.Fatal("clone did not retain the attached panel-info provider")
	}
	if got := clone.PanelTitle("/sdcard/Download"); got != "phone|/sdcard/Download" {
		t.Fatalf("clone PanelTitle = %q", got)
	}
	req := vfs.PanelInfoRequest{Path: "/sdcard"}
	if got := clone.PanelInfoKey(req); got != provider.key {
		t.Fatalf("PanelInfoKey = %q", got)
	}
	if snapshot, fresh := clone.CachedPanelInfo(req); fresh || !snapshot.Authoritative || len(snapshot.Sections) != 1 {
		t.Fatalf("CachedPanelInfo = %#v, fresh %v", snapshot, fresh)
	}
	if _, err := clone.RefreshPanelInfo(context.Background(), req); err != nil || provider.refreshCalls != 1 {
		t.Fatalf("RefreshPanelInfo = %v, calls %d", err, provider.refreshCalls)
	}
}

func TestValidateFishEntryNameRejectsHostTraversal(t *testing.T) {
	for _, name := range []string{"", "a/b", "../outside", "bad\x00name"} {
		if err := validateFishEntryName(name); err == nil {
			t.Errorf("validateFishEntryName(%q) succeeded", name)
		}
	}
	if err := validateFishEntryName("ordinary name"); err != nil {
		t.Fatalf("ordinary name rejected: %v", err)
	}
	if err := validateFishEntryName(`..\outside`); runtime.GOOS == "windows" && err == nil {
		t.Fatal("Windows traversal name was accepted")
	} else if runtime.GOOS != "windows" && err != nil {
		t.Fatalf("legal Unix backslash name rejected: %v", err)
	}
}

// TestFishVFSReadDirResolvesAllSymlinksInOneRequest exercises the VFS over a
// tiny protocol peer. It is intentionally not a local-shell test: besides
// running on Windows, it proves the wire contains one isdirs request rather
// than merely proving that both implementations eventually classify links.
func TestFishVFSReadDirResolvesAllSymlinksInOneRequest(t *testing.T) {
	peerR, clientW := io.Pipe()
	clientR, peerW := io.Pipe()
	sess := fishplus.NewSession(clientW, clientR, nil)
	t.Cleanup(func() {
		clientW.Close()
		peerW.Close()
	})

	done := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(peerR)
		if !scanner.Scan() {
			done <- fmt.Errorf("missing enum header: %v", scanner.Err())
			return
		}
		enumHeader := strings.Fields(scanner.Text())
		if len(enumHeader) != 2 || enumHeader[1] != "enum" {
			done <- fmt.Errorf("first request = %q, want enum", scanner.Text())
			return
		}
		if !scanner.Scan() || scanner.Text() != "/" {
			done <- fmt.Errorf("enum path = %q, want /", scanner.Text())
			return
		}
		fmt.Fprintln(peerW, "M stat")
		fmt.Fprintln(peerW, "a1ff 8 1 1 1 0 0 /directory-link")
		fmt.Fprintln(peerW, "a1ff 8 1 1 1 0 0 /file-link")
		fmt.Fprintln(peerW, "81a4 1 1 1 1 0 0 /ordinary")
		fmt.Fprintf(peerW, ".%s %s ok\n", sess.Token(), enumHeader[0])

		if !scanner.Scan() {
			done <- fmt.Errorf("missing isdirs header: %v", scanner.Err())
			return
		}
		batchHeader := strings.Fields(scanner.Text())
		if len(batchHeader) != 3 || batchHeader[1] != "isdirs" || batchHeader[2] != "2" {
			done <- fmt.Errorf("second request = %q, want isdirs 2", scanner.Text())
			return
		}
		wantPaths := []string{"/directory-link", "/file-link"}
		for _, want := range wantPaths {
			if !scanner.Scan() || scanner.Text() != want {
				done <- fmt.Errorf("isdirs path = %q, want %q", scanner.Text(), want)
				return
			}
		}
		fmt.Fprintln(peerW, "1")
		fmt.Fprintln(peerW, "0")
		fmt.Fprintf(peerW, ".%s %s ok\n", sess.Token(), batchHeader[0])
		done <- nil
	}()

	v := &FishVFS{
		conn: &fishConn{client: fishplus.NewClient(sess), refs: 1},
		path: "/",
	}
	items := make(map[string]vfs.VFSItem)
	err := v.ReadDir(context.Background(), "/", func(chunk []vfs.VFSItem) {
		for _, item := range chunk {
			items[item.Name] = item
		}
	})
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if item := items["directory-link"]; !item.IsSymlink || !item.IsDir {
		t.Errorf("directory link = %+v", item)
	}
	if item := items["file-link"]; !item.IsSymlink || item.IsDir {
		t.Errorf("file link = %+v", item)
	}
	if item := items["ordinary"]; item.IsSymlink || item.IsDir {
		t.Errorf("ordinary file = %+v", item)
	}
}

// newLocalFishVFS runs the real helper in a local shell and wraps it in a
// FishVFS, which is the only way to check the mapping against output real
// tools produced rather than against captured samples.
func newLocalFishVFS(t *testing.T) *FishVFS {
	return newLocalFishVFSWithTitle(t, "local")
}

func newLocalFishVFSWithTitle(t *testing.T, title string) *FishVFS {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX shell on Windows")
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell available")
	}
	cmd := exec.Command(shell)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", shell, err)
	}
	v, err := NewFishVFSOnStream(context.Background(), nil, stdin, stdout, stdin, title)
	if err != nil {
		cmd.Process.Kill()
		if strings.Contains(err.Error(), "base64") {
			t.Skipf("no base64 on this host: %v", err)
		}
		t.Fatalf("handshake: %v", err)
	}
	t.Cleanup(func() {
		v.Close()
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
	return v
}

func TestFishVFSOptimisticPathChangeNeedsNoSession(t *testing.T) {
	v := &FishVFS{path: "/"}
	if err := v.SetPathOptimistic("sdcard"); err != nil {
		t.Fatalf("SetPathOptimistic: %v", err)
	}
	if got := v.GetPath(); got != "/sdcard" {
		t.Fatalf("optimistic path = %q, want /sdcard", got)
	}
	if v.PtyAvailable() {
		t.Fatal("a FISH+ view without a PTY transport reported one available")
	}
}

func TestFishVFSBrowsesLocalShell(t *testing.T) {
	v := newLocalFishVFS(t)
	ctx := context.Background()

	if !strings.HasPrefix(v.GetPath(), "/") {
		t.Errorf("GetPath = %q, want the remote working directory", v.GetPath())
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a file.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hidden"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub dir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("sub dir", filepath.Join(dir, "link to dir")); err != nil {
		t.Skipf("symlinks not supported here: %v", err)
	}

	if err := v.SetPath(dir); err != nil {
		t.Fatalf("SetPath(%q): %v", dir, err)
	}
	if v.GetPath() != filepath.Clean(dir) {
		t.Errorf("GetPath = %q, want %q", v.GetPath(), dir)
	}
	if err := v.SetPath(filepath.Join(dir, "a file.txt")); err == nil {
		t.Error("SetPath accepted a regular file")
	}

	byName := map[string]vfs.VFSItem{}
	if err := v.ReadDir(ctx, ".", func(chunk []vfs.VFSItem) {
		for _, item := range chunk {
			byName[item.Name] = item
		}
	}); err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(byName) != 4 {
		t.Fatalf("got %d entries: %v", len(byName), byName)
	}

	file, ok := byName["a file.txt"]
	if !ok {
		t.Fatal("a file with a space in its name got lost")
	}
	if file.Size != 5 || file.IsDir || file.IsSymlink {
		t.Errorf("file mapped wrong: %+v", file)
	}
	if file.UnixMode&0777 != 0644 {
		t.Errorf("UnixMode = %o, want 644 in the low bits", file.UnixMode)
	}
	if time.Since(file.MTime) > time.Hour {
		t.Errorf("MTime = %v, which is nowhere near now", file.MTime)
	}

	if hidden, ok := byName[".hidden"]; !ok {
		t.Error("hidden file missing")
	} else if !hidden.IsHidden {
		t.Error("a dot file was not marked hidden")
	}

	if sub, ok := byName["sub dir"]; !ok || !sub.IsDir {
		t.Errorf("subdirectory mapped wrong: %+v", sub)
	}

	link, ok := byName["link to dir"]
	if !ok {
		t.Fatal("symlink missing")
	}
	if !link.IsSymlink {
		t.Error("symlink not marked as one")
	}
	if !link.IsDir {
		t.Error("a symlink to a directory must be enterable")
	}
}

func TestFishVFSStatAndOpen(t *testing.T) {
	v := newLocalFishVFS(t)
	ctx := context.Background()

	dir := t.TempDir()
	body := strings.Repeat("0123456789", 5000)
	p := filepath.Join(dir, "payload.bin")
	if err := os.WriteFile(p, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}

	item, err := v.Stat(ctx, p)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if item.Name != "payload.bin" || item.Size != int64(len(body)) {
		t.Errorf("Stat mapped wrong: %+v", item)
	}
	if !item.IsExecutable {
		t.Error("an executable file was not marked as one")
	}
	if _, err := v.Stat(ctx, filepath.Join(dir, "no such file")); err == nil {
		t.Error("Stat of a missing file succeeded")
	}

	f, err := v.Open(ctx, p)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	if f.Size() != int64(len(body)) {
		t.Errorf("Size = %d, want %d", f.Size(), len(body))
	}
	buf := make([]byte, 20)
	if n, err := f.ReadAt(ctx, buf, 12345); err != nil || n != len(buf) {
		t.Fatalf("ReadAt = %d, %v", n, err)
	}
	if string(buf) != body[12345:12365] {
		t.Errorf("ReadAt returned %q", buf)
	}

	if !v.GetCapabilities().HasRandomAccess {
		t.Error("HasRandomAccess must be set, the viewer depends on it")
	}
}

func TestFishVFSMutations(t *testing.T) {
	v := newLocalFishVFS(t)
	ctx := context.Background()
	root := t.TempDir()

	dir := filepath.Join(root, "new dir")
	if err := v.MkDir(ctx, dir); err != nil {
		t.Fatalf("MkDir: %v", err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("MkDir did not create %q: %v", dir, err)
	}

	file := filepath.Join(dir, "payload.txt")
	if err := os.WriteFile(file, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(dir, "renamed.txt")
	if err := v.Rename(ctx, file, moved); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("Rename did not produce %q: %v", moved, err)
	}

	// Mode, ownership and timestamps in one call, which is what the copier
	// does once a file has arrived. A negative uid and gid mean the copier's
	// "keep whatever the remote host decided".
	stamp := time.Unix(1400000000, 0)
	attrs := vfs.VFSItem{UnixMode: 0100600, Uid: -1, Gid: -1, MTime: stamp}
	if err := v.SetAttributes(ctx, moved, attrs); err != nil {
		t.Fatalf("SetAttributes: %v", err)
	}
	info, err := os.Stat(moved)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("mode = %o, want 600", info.Mode().Perm())
	}
	if info.ModTime().Unix() != stamp.Unix() {
		t.Errorf("mtime = %v, want %v", info.ModTime(), stamp)
	}
	if item, err := v.Stat(ctx, moved); err != nil {
		t.Errorf("Stat after SetAttributes: %v", err)
	} else if item.MTime.Unix() != stamp.Unix() {
		t.Errorf("mtime seen through the panel = %v, want %v", item.MTime, stamp)
	}
	// An item carrying nothing to set must leave the file alone.
	if err := v.SetAttributes(ctx, moved, vfs.VFSItem{Uid: -1, Gid: -1}); err != nil {
		t.Errorf("SetAttributes with nothing to set: %v", err)
	}
	if info, err = os.Stat(moved); err != nil || info.Mode().Perm() != 0600 {
		t.Errorf("mode after an empty SetAttributes = %v (%v)", info.Mode().Perm(), err)
	}

	created := filepath.Join(dir, "brand new.txt")
	w, err := v.Create(ctx, created)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := w.Write([]byte("first line\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := w.Write([]byte("second line\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got, err := os.ReadFile(created); err != nil || string(got) != "first line\nsecond line\n" {
		t.Fatalf("created file = %q (%v)", got, err)
	}
	// Creating an existing file truncates it, so a shorter second round must
	// not leave the tail of the first one behind.
	if w, err = v.Create(ctx, created); err != nil {
		t.Fatalf("Create again: %v", err)
	}
	if _, err := w.Write([]byte("short")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got, err := os.ReadFile(created); err != nil || string(got) != "short" {
		t.Fatalf("recreated file = %q (%v)", got, err)
	}
	if item, err := v.Stat(ctx, created); err != nil || item.Size != 5 {
		t.Errorf("Stat after Create = %+v (%v)", item, err)
	}

	// Remove takes the whole tree, the file inside it included.
	if err := v.Remove(ctx, dir); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("Remove left the directory behind")
	}
	if err := v.Remove(ctx, dir); err == nil {
		t.Error("Remove of a missing directory succeeded")
	}
}
func TestFishVFSSearch(t *testing.T) {
	v := newLocalFishVFS(t)
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "log.txt")
	content := "alpha\nbeta\ngamma beta\n"
	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	ch, err := v.Search(ctx, file, "beta")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if ch == nil {
		if v.GetCapabilities().HasSearch {
			t.Fatal("Search answered nothing although the host announced it")
		}
		t.Skip("no server-side search on this host")
	}
	var got []int64
	for off := range ch {
		got = append(got, off)
	}
	want := []int64{int64(strings.Index(content, "beta")), int64(strings.LastIndex(content, "beta"))}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("offsets = %v, want %v", got, want)
	}
	if !v.GetCapabilities().HasSearch {
		t.Error("HasSearch is false although the search worked")
	}
	// An empty pattern is the caller saying it has nothing to search for,
	// and must not cost a round trip.
	if ch, err = v.Search(ctx, file, ""); ch != nil || err != nil {
		t.Errorf("Search with an empty pattern = %v, %v", ch, err)
	}
}
func TestFishVFSCloneHasItsOwnDirectory(t *testing.T) {
	v := newLocalFishVFS(t)
	dirA := t.TempDir()
	dirB := t.TempDir()
	if err := v.SetPath(dirA); err != nil {
		t.Fatalf("SetPath: %v", err)
	}

	clone, ok := v.Clone().(*FishVFS)
	if !ok {
		t.Fatal("Clone did not return a FishVFS")
	}
	if clone == v {
		t.Fatal("Clone returned the same instance, so two panels would share one current directory")
	}
	if clone.GetPath() != v.GetPath() {
		t.Errorf("clone starts at %q, want %q", clone.GetPath(), v.GetPath())
	}
	if err := clone.SetPath(dirB); err != nil {
		t.Fatalf("SetPath on the clone: %v", err)
	}
	if v.GetPath() != filepath.Clean(dirA) {
		t.Errorf("moving the clone moved the original to %q", v.GetPath())
	}
	if clone.GetPath() != filepath.Clean(dirB) {
		t.Errorf("the clone did not move: %q", clone.GetPath())
	}
}

func TestFishVFSSessionOutlivesItsClones(t *testing.T) {
	v := newLocalFishVFS(t)
	ctx := context.Background()
	dir := t.TempDir()
	clone := v.Clone().(*FishVFS)

	if err := clone.Close(); err != nil {
		t.Fatalf("closing the clone: %v", err)
	}
	if _, err := v.Stat(ctx, dir); err != nil {
		t.Fatalf("closing a clone tore the shared session down: %v", err)
	}
	// A double close must not drop the reference count twice.
	if err := clone.Close(); err != nil {
		t.Errorf("closing the clone again: %v", err)
	}
	if _, err := v.Stat(ctx, dir); err != nil {
		t.Fatalf("a second close of the clone tore the session down: %v", err)
	}

	if err := v.Close(); err != nil {
		t.Fatalf("closing the last view: %v", err)
	}
	if _, err := v.Stat(ctx, dir); err == nil {
		t.Error("the session survived its last user")
	}
}

func TestFishVFSClonesShareOneSessionSafely(t *testing.T) {
	v := newLocalFishVFS(t)
	dir := t.TempDir()
	for _, name := range []string{"one", "two", "three"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	clone := v.Clone().(*FishVFS)

	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 16; i++ {
		for _, target := range []*FishVFS{v, clone} {
			wg.Add(1)
			go func(x *FishVFS) {
				defer wg.Done()
				count := 0
				err := x.ReadDir(context.Background(), dir, func(chunk []vfs.VFSItem) {
					count += len(chunk)
				})
				if err != nil {
					errs <- err
					return
				}
				if count != 3 {
					errs <- fmt.Errorf("listing returned %d entries, want 3", count)
				}
			}(target)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent use of one session: %v", err)
	}
}
func TestFishProtocolIsRegistered(t *testing.T) {
	found := false
	for _, p := range GetProtocols() {
		if p == "fish+" {
			found = true
		}
	}
	if !found {
		t.Fatalf("fish+ missing from the protocol list: %v", GetProtocols())
	}
	ph := &fishProtocolHandler{}
	if ph.Prefix() != "fish+" {
		t.Errorf("Prefix = %q", ph.Prefix())
	}
	if ph.DefaultPort() != "22" {
		t.Errorf("DefaultPort = %q, want 22", ph.DefaultPort())
	}
	ui, apply := ph.BuildExtraUI(&NetFoxConfig{}, 0, 0, 10, 10)
	if ui != nil {
		t.Error("the fish+ handler needs no extra UI yet")
	}
	apply()
}

func TestFishTypeMatches(t *testing.T) {
	for _, good := range []string{"fish+", "fish"} {
		if !fishTypeMatches(good) {
			t.Errorf("type %q not recognized as FISH+", good)
		}
	}
	// An empty type belongs to SFTP, which claims it as its default; taking
	// it here would hijack every site saved before FISH+ existed.
	for _, bad := range []string{"", "sftp", "ftp", "fishy"} {
		if fishTypeMatches(bad) {
			t.Errorf("type %q wrongly claimed by FISH+", bad)
		}
	}
}

func TestSSHTimeoutDefaults(t *testing.T) {
	if got := sshTimeout(0); got != 15*time.Second {
		t.Errorf("sshTimeout(0) = %v, want 15s", got)
	}
	if got := sshTimeout(-3); got != 15*time.Second {
		t.Errorf("sshTimeout(-3) = %v, want 15s", got)
	}
	if got := sshTimeout(7); got != 7*time.Second {
		t.Errorf("sshTimeout(7) = %v, want 7s", got)
	}
}

func TestDialSSHFailsOnAClosedPort(t *testing.T) {
	client, err := DialSSH("127.0.0.1", "1", "nobody", "", 2, netproxy.Settings{})
	if err == nil {
		client.Close()
		t.Fatal("dialing a closed port succeeded")
	}
}
func TestFishVFSLineIndex(t *testing.T) {
	v := newLocalFishVFS(t)
	ctx := context.Background()
	if !v.Client().CanIndexLines() {
		t.Skip("no awk on this host")
	}

	dir := t.TempDir()
	p := filepath.Join(dir, "log with spaces.txt")
	if err := os.WriteFile(p, []byte("alpha\nbeta\ngamma\n"), 0644); err != nil {
		t.Fatal(err)
	}

	idx, err := v.LineIndex(ctx, p, 1, 0)
	if err != nil {
		t.Fatalf("LineIndex: %v", err)
	}
	if idx.Total != 3 {
		t.Errorf("Total = %d, want 3", idx.Total)
	}
	if len(idx.Offsets) != 0 {
		t.Errorf("a count of zero returned %d offsets", len(idx.Offsets))
	}

	idx, err = v.LineIndex(ctx, p, 2, 2)
	if err != nil {
		t.Fatalf("LineIndex: %v", err)
	}
	want := []int64{6, 11}
	if len(idx.Offsets) != len(want) {
		t.Fatalf("got %d offsets, want %d", len(idx.Offsets), len(want))
	}
	for i, off := range idx.Offsets {
		if off != want[i] {
			t.Errorf("offset %d = %d, want %d", i, off, want[i])
		}
	}
	if idx.First != 2 {
		t.Errorf("First = %d, want 2", idx.First)
	}

	// The VFS is a vfs.LineIndexer, which is what the viewer asserts for.
	if _, ok := interface{}(v).(vfs.LineIndexer); !ok {
		t.Error("FishVFS does not satisfy vfs.LineIndexer")
	}
}
func TestFishVFSScan(t *testing.T) {
	v := newLocalFishVFS(t)
	ctx := context.Background()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tree", "a deep dir"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, size := range map[string]int{
		"tree/one.txt":              100,
		"tree/two.txt":              250,
		"tree/a deep dir/three.txt": 1000,
		"loose.txt":                 7,
	} {
		if err := os.WriteFile(filepath.Join(root, name), make([]byte, size), 0644); err != nil {
			t.Fatal(err)
		}
	}

	names := []string{"tree", "loose.txt"}
	var seen int
	got, err := v.Scan(ctx, root, names, func(string, vfs.OpStats) { seen++ })
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// The generic walk is the definition of the right answer: whichever way
	// the numbers were obtained, a panel must show the same ones.
	want, err := vfs.GenericScan(ctx, v, root, names, nil)
	if err != nil {
		t.Fatalf("GenericScan: %v", err)
	}
	if got.Files != want.Files || got.Dirs != want.Dirs || got.Bytes != want.Bytes {
		t.Errorf("remote scan = %+v, generic walk = %+v", got, want)
	}
	if got.Bytes != 1357 {
		t.Errorf("Bytes = %d, want 1357", got.Bytes)
	}
	if seen == 0 {
		t.Error("the scan reported no progress at all")
	}

	// A host that cannot run a remote scan still has to produce numbers.
	if !v.Client().CanScan() {
		t.Log("this host has no remote scan; the generic path was the one tested")
	}

	if _, ok := interface{}(v).(vfs.FastScanner); !ok {
		t.Error("FishVFS does not satisfy vfs.FastScanner")
	}
}

func TestFishVFSFindDuplicates(t *testing.T) {
	v := newLocalFishVFS(t)
	ctx := context.Background()
	if !v.Client().CanHash() {
		t.Skip("this host cannot hash a tree remotely")
	}

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub dir"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"twin one.txt":     "the same content\n",
		"sub dir/twin two": "the same content\n",
		"different.txt":    "the same lengthX\n",
		"alone.txt":        "on its own\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	var progress int
	groups, err := v.FindDuplicates(ctx, root, func(p vfs.DuplicateProgress) { progress++ })
	if err != nil {
		t.Fatalf("FindDuplicates: %v", err)
	}
	if len(groups) != 1 || len(groups[0]) != 2 {
		t.Fatalf("got %v, want one group of two", groups)
	}
	names := []string{filepath.Base(groups[0][0]), filepath.Base(groups[0][1])}
	sort.Strings(names)
	if names[0] != "twin one.txt" || names[1] != "twin two" {
		t.Errorf("the group is %v", names)
	}
	if progress == 0 {
		t.Error("no progress was reported")
	}

	if _, ok := interface{}(v).(vfs.DuplicateFinder); !ok {
		t.Error("FishVFS does not satisfy vfs.DuplicateFinder")
	}
}
func TestFishVFSPatchFile(t *testing.T) {
	v := newLocalFishVFS(t)
	ctx := context.Background()
	if !v.Client().CanPatch() {
		t.Skip("this host cannot assemble a file remotely")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "original.txt")
	dst := filepath.Join(dir, "rebuilt.txt")
	body := []byte("the quick brown fox jumps over the lazy dog")
	if err := os.WriteFile(src, body, 0644); err != nil {
		t.Fatal(err)
	}

	// One word replaced: two ranges of the original and one literal.
	err := v.PatchFile(ctx, src, dst, []vfs.PatchPiece{
		{Offset: 0, Length: 4},
		{Length: 5, Data: []byte("slow ")},
		{Offset: 10, Length: 33},
	})
	if err != nil {
		t.Fatalf("PatchFile: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "the slow brown fox jumps over the lazy dog" {
		t.Errorf("rebuilt file is %q", got)
	}

	if _, ok := interface{}(v).(vfs.DeltaWriter); !ok {
		t.Error("FishVFS does not satisfy vfs.DeltaWriter")
	}
}
func TestFishVFSFindFiles(t *testing.T) {
	v := newLocalFishVFS(t)
	ctx := context.Background()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "nested dir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "top.txt"), []byte("needle here\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested dir", "deep.txt"), []byte("nothing\n"), 0755); err != nil {
		t.Fatal(err)
	}

	hits, err := v.FindFiles(ctx, root, vfs.FindQuery{Masks: []string{"*.txt"}})
	if err != nil {
		t.Fatalf("FindFiles: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("found %d files, want 2: %+v", len(hits), hits)
	}
	byName := map[string]vfs.FoundEntry{}
	for _, hit := range hits {
		byName[hit.Item.Name] = hit
	}
	deep, ok := byName["deep.txt"]
	if !ok {
		t.Fatal("the nested file was not found")
	}
	if !strings.HasPrefix(deep.Path, root) || !strings.HasSuffix(deep.Path, "deep.txt") {
		t.Errorf("Path = %q, want a full path below %q", deep.Path, root)
	}
	if !deep.Item.IsExecutable || deep.Item.IsDir {
		t.Errorf("nested file mapped wrong: %+v", deep.Item)
	}

	if !v.Client().CanGrep() {
		t.Skip("no remote grep on this host")
	}
	hits, err = v.FindFiles(ctx, root, vfs.FindQuery{Masks: []string{"*"}, Text: "NEEDLE", IgnoreCase: true})
	if err != nil {
		t.Fatalf("FindFiles with a pattern: %v", err)
	}
	if len(hits) != 1 || hits[0].Item.Name != "top.txt" {
		t.Errorf("content search returned %+v, want top.txt alone", hits)
	}

	// The VFS is a vfs.FileFinder, which is what the search asserts for.
	if _, ok := interface{}(v).(vfs.FileFinder); !ok {
		t.Error("FishVFS does not satisfy vfs.FileFinder")
	}
}

func TestFishVFSServerSideCopyAndMove(t *testing.T) {
	v1 := newLocalFishVFS(t)
	ctx := context.Background()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	content := []byte("Hello Server-Side Copy/Move")
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	// Verify Capabilities
	caps := v1.GetCapabilities()
	if !caps.HasServerSideCopy {
		t.Error("expected HasServerSideCopy to be true")
	}
	if !caps.HasServerSideMove {
		t.Error("expected HasServerSideMove to be true")
	}

	// Assert v1 implements ServerSideCopier
	ssc, ok := interface{}(v1).(vfs.ServerSideCopier)
	if !ok {
		t.Fatal("FishVFS does not satisfy vfs.ServerSideCopier")
	}

	// Test Copy
	dstCopy := filepath.Join(tmpDir, "copied.txt")
	if err := ssc.Copy(ctx, srcPath, dstCopy); err != nil {
		t.Fatalf("Copy: %v", err)
	}

	gotCopy, err := os.ReadFile(dstCopy)
	if err != nil {
		t.Fatalf("os.ReadFile copied: %v", err)
	}
	if string(gotCopy) != string(content) {
		t.Errorf("got copy content %q, want %q", gotCopy, content)
	}

	// Test SameSession helper
	v2 := v1.Clone()
	defer v2.Close()

	if !vfs.SameSession(v1, v2) {
		t.Error("expected SameSession to be true for clones")
	}

	// Test different session (with different titles)
	v3 := newLocalFishVFSWithTitle(t, "local-diff")
	defer v3.Close()

	if vfs.SameSession(v1, v3) {
		t.Error("expected SameSession to be false for distinct sessions with different titles")
	}
}

func TestFishVFSServerToServerInfo(t *testing.T) {
	v1 := newLocalFishVFS(t)
	defer v1.Close()

	v1.host = "runcity.org"
	v1.port = "22"
	v1.user = "unxed"

	cip, ok := interface{}(v1).(vfs.ConnectionInfoProvider)
	if !ok {
		t.Fatal("FishVFS does not satisfy vfs.ConnectionInfoProvider")
	}

	h, p, u, ok := cip.ConnectionInfo()
	if !ok || h != "runcity.org" || p != "22" || u != "unxed" {
		t.Errorf("ConnectionInfo = (%q, %q, %q, %t), want (runcity.org, 22, unxed, true)", h, p, u, ok)
	}

	v2 := v1.Clone().(*FishVFS)
	defer v2.Close()

	h2, p2, u2, ok2 := interface{}(v2).(vfs.ConnectionInfoProvider).ConnectionInfo()
	if !ok2 || h2 != h || p2 != p || u2 != u {
		t.Errorf("cloned ConnectionInfo mismatch: (%q, %q, %q, %t)", h2, p2, u2, ok2)
	}
}
