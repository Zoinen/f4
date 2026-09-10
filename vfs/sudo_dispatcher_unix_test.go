//go:build !windows

package vfs

import (
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRunSudoDispatcherHelperProcess(t *testing.T) {
	if os.Getenv("F4_RUN_SUDO_DISPATCHER") != "1" {
		return
	}
	RunSudoDispatcher(filepath.Join(os.TempDir(), "f4-sudo-dispatcher-test.sock"))
}

func TestRunSudoDispatcherRejectsInvalidIdentity(t *testing.T) {
	cmd := sudoDispatcherHelperCommand(t, "not-a-uid", "not-a-gid")
	output, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("dispatcher exit error = %v, output = %q; want exit status 1", err, output)
	}
	if !strings.Contains(string(output), "Invalid SUDO_UID/SUDO_GID") {
		t.Fatalf("dispatcher output = %q, want invalid identity diagnostic", output)
	}
}

func TestRunSudoDispatcherRejectsNonRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("RunSudoDispatcher would enter its root-only socket server")
	}
	uid := strconv.Itoa(os.Getuid())
	gid := strconv.Itoa(os.Getgid())
	debugPath := filepath.Join(os.TempDir(), "f4-sudo-debug-"+uid+".txt")
	_ = os.Remove(debugPath)
	t.Cleanup(func() { _ = os.Remove(debugPath) })

	cmd := sudoDispatcherHelperCommand(t, uid, gid)
	output, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("dispatcher exit error = %v, output = %q; want exit status 1", err, output)
	}
	if !strings.Contains(string(output), "f4 dispatcher must run as root") {
		t.Fatalf("dispatcher output = %q, want root diagnostic", output)
	}
}

func sudoDispatcherHelperCommand(t *testing.T, uid, gid string) *exec.Cmd {
	t.Helper()
	env := make([]string, 0, len(os.Environ())+3)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "F4_RUN_SUDO_DISPATCHER=") ||
			strings.HasPrefix(value, "SUDO_UID=") || strings.HasPrefix(value, "SUDO_GID=") {
			continue
		}
		env = append(env, value)
	}
	env = append(env, "F4_RUN_SUDO_DISPATCHER=1", "SUDO_UID="+uid, "SUDO_GID="+gid)

	// #nosec G204 G702 -- os.Args[0] is the current test binary and the arguments are fixed.
	cmd := exec.Command(os.Args[0], "-test.run=TestRunSudoDispatcherHelperProcess", "--")
	cmd.Env = env
	return cmd
}

func TestHandleSudoClientDispatchesFilesystemCommands(t *testing.T) {
	tmpDir := shortSocketDir(t)
	sockPath := filepath.Join(tmpDir, "dispatcher.sock")
	addr, err := net.ResolveUnixAddr("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	listener := listenUnixForTest(t, addr)
	defer func() { _ = listener.Close() }()

	serverDone := make(chan struct{})
	serverErr := make(chan error, 1)
	go func() {
		conn, acceptErr := listener.AcceptUnix()
		if acceptErr != nil {
			serverErr <- acceptErr
			close(serverDone)
			return
		}
		handleSudoClient(conn)
		close(serverDone)
	}()

	client, err := net.DialUnix("unix", nil, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	request := func(req SudoRequest) (SudoResponse, *os.File) {
		t.Helper()
		if err := sendMsg(client, req, -1); err != nil {
			t.Fatal(err)
		}
		var resp SudoResponse
		fd, err := recvMsg(client, &resp)
		if err != nil {
			t.Fatal(err)
		}
		return resp, fd
	}
	requireSuccess := func(req SudoRequest) SudoResponse {
		t.Helper()
		resp, fd := request(req)
		if fd != nil {
			_ = fd.Close()
			t.Fatal("unexpected file descriptor in response")
		}
		if resp.Error != "" {
			t.Fatalf("command %d failed: %s", req.Cmd, resp.Error)
		}
		return resp
	}

	hiddenPath := filepath.Join(tmpDir, ".hidden")
	if err := os.WriteFile(hiddenPath, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	contentPath := filepath.Join(tmpDir, "content.txt")
	if err := os.WriteFile(contentPath, []byte("elevated content"), 0600); err != nil {
		t.Fatal(err)
	}
	dirPath := filepath.Join(tmpDir, "target-dir")
	if err := os.Mkdir(dirPath, 0755); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(tmpDir, "dir-link")
	if err := os.Symlink(dirPath, linkPath); err != nil {
		t.Fatal(err)
	}

	requireSuccess(SudoRequest{Cmd: CmdPing})

	stat := requireSuccess(SudoRequest{Cmd: CmdStat, Path: hiddenPath}).Item
	if stat.Name != ".hidden" || stat.Size != int64(len("payload")) || !stat.IsHidden {
		t.Fatalf("stat = %+v", stat)
	}

	readDir := requireSuccess(SudoRequest{Cmd: CmdReadDir, Path: tmpDir})
	var sawLink, sawDir bool
	for _, item := range readDir.Items {
		if item.Name == "dir-link" {
			sawLink = item.IsDir
		}
		if item.Name == "target-dir" {
			sawDir = item.IsDir
		}
	}
	if !sawLink || !sawDir {
		t.Fatalf("ReadDir items = %+v, want directory link and directory", readDir.Items)
	}

	opened, fd := request(SudoRequest{Cmd: CmdOpen, Path: contentPath, Flags: os.O_RDONLY})
	if opened.Error != "" || fd == nil {
		t.Fatalf("open response = %+v, fd=%v", opened, fd)
	}
	data, err := io.ReadAll(fd)
	_ = fd.Close()
	if err != nil || string(data) != "elevated content" {
		t.Fatalf("opened data = %q, err=%v", data, err)
	}

	special, fd := request(SudoRequest{Cmd: CmdOpen, Path: sockPath, Flags: os.O_RDONLY})
	if fd != nil {
		_ = fd.Close()
	}
	if special.Error != "cannot open special file" {
		t.Fatalf("special open error = %q", special.Error)
	}

	createdDir := filepath.Join(tmpDir, "created", "nested")
	requireSuccess(SudoRequest{Cmd: CmdMkDir, Path: createdDir, Mode: 0750})
	if info, err := os.Stat(createdDir); err != nil || !info.IsDir() {
		t.Fatalf("created directory stat = %v, %v", info, err)
	}

	oldPath := filepath.Join(tmpDir, "rename-me")
	newPath := filepath.Join(tmpDir, "renamed")
	if err := os.WriteFile(oldPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	requireSuccess(SudoRequest{Cmd: CmdRename, Path: oldPath, Path2: newPath})
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("renamed path is missing: %v", err)
	}

	attrTime := time.Unix(123456, 0)
	requireSuccess(SudoRequest{Cmd: CmdSetAttributes, Path: newPath, Item: VFSItem{
		UnixMode: 0600,
		Uid:      os.Getuid(),
		Gid:      os.Getgid(),
		ATime:    attrTime,
		MTime:    attrTime,
	}})
	if info, err := os.Stat(newPath); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("attributes stat = %v, %v", info, err)
	}

	requireSuccess(SudoRequest{Cmd: CmdRemove, Path: newPath})
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatalf("removed path stat error = %v", err)
	}

	missing, fd := request(SudoRequest{Cmd: CmdStat, Path: filepath.Join(tmpDir, "missing")})
	if fd != nil {
		_ = fd.Close()
	}
	if missing.Error == "" {
		t.Fatal("missing stat unexpectedly succeeded")
	}

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-serverErr:
		t.Fatal(err)
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Fatal("dispatcher did not stop after client disconnect")
	}
}
