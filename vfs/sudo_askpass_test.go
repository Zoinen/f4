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
	"syscall"
	"testing"
	"time"

	"github.com/unxed/vtui"
)

func TestSudoAskpassHelperProcess(t *testing.T) {
	if os.Getenv("F4_RUN_SUDO_ASKPASS") != "1" {
		return
	}
	RunSudoAskpass()
}

func TestRunSudoAskpassSendsPasswordToHelper(t *testing.T) {
	sockPath := getAskpassSocketPath(os.Getpid())
	_ = os.Remove(sockPath)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: sockPath, Net: "unix"})
	if errors.Is(err, syscall.EPERM) {
		t.Skipf("Unix socket bind is not permitted: %v", err)
	}
	if err != nil {
		t.Fatalf("listen on askpass socket: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
		_ = os.Remove(sockPath)
	})

	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.AcceptUnix()
		if err != nil {
			serverErr <- err
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		request := make([]byte, len("GET\n"))
		if _, err := io.ReadFull(conn, request); err != nil {
			serverErr <- err
			return
		}
		if string(request) != "GET\n" {
			serverErr <- errors.New("unexpected askpass request")
			return
		}
		_, err = conn.Write([]byte("secret"))
		serverErr <- err
	}()

	cmd := askpassHelperCommand(t, strconv.Itoa(os.Getpid()))
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("askpass helper failed: %v", err)
	}
	if string(output) != "secret\n" {
		t.Fatalf("askpass output = %q, want %q", output, "secret\n")
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("askpass server: %v", err)
	}
}

func TestRunSudoAskpassRejectsMissingParent(t *testing.T) {
	cmd := askpassHelperCommand(t, "")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("askpass exit error = %v, want exit status 1", err)
	}
}

func TestAskpassServerReturnsWhenSocketCannotBeCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "askpass.sock")

	(&SudoClient{}).runAskpassServer(path)
}

func TestHandleAskpassRequestStopsAfterThreeAttempts(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = client.Close() }()

	sudo := &SudoClient{attempts: 3}
	sudo.handleAskpassRequest(server)
}

func TestRunOnUIWithoutFrameManagerDoesNothing(t *testing.T) {
	original := vtui.FrameManager
	vtui.FrameManager = nil
	t.Cleanup(func() { vtui.FrameManager = original })

	(&SudoClient{}).RunOnUI(func() {
		t.Fatal("callback should not run without a frame manager")
	})
}

func askpassHelperCommand(t *testing.T, parent string) *exec.Cmd {
	t.Helper()
	env := make([]string, 0, len(os.Environ())+2)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "F4_RUN_SUDO_ASKPASS=") || strings.HasPrefix(value, "F4_ASKPASS_PARENT=") {
			continue
		}
		env = append(env, value)
	}
	env = append(env, "F4_RUN_SUDO_ASKPASS=1", "F4_ASKPASS_PARENT="+parent)

	// #nosec G204 G702 -- os.Args[0] is the current test binary and the arguments are fixed.
	cmd := exec.Command(os.Args[0], "-test.run=TestSudoAskpassHelperProcess", "--")
	cmd.Env = env
	return cmd
}
