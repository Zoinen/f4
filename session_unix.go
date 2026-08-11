//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

type SessionInfo struct {
	PID      int
	Title    string
	SockPath string
}

func SupportsBackgrounding() bool {
	return true
}

func sessionDir() string {
	uid := os.Getuid()
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("f4-sessions-%d", uid))
	os.MkdirAll(dir, 0700)
	return dir
}

func isProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func listSessions() []SessionInfo {
	dir := sessionDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var sessions []SessionInfo
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			path := filepath.Join(dir, e.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			var info SessionInfo
			if err := json.Unmarshal(data, &info); err == nil {
				if isProcessAlive(info.PID) {
					sessions = append(sessions, info)
				} else {
					os.Remove(path)
					os.Remove(info.SockPath)
				}
			}
		}
	}
	return sessions
}

func writeSessionInfo(sockPath string) {
	pid := os.Getpid()
	infoPath := filepath.Join(sessionDir(), fmt.Sprintf("f4-%d.json", pid))
	title := "f4"
	if top := vtui.FrameManager.GetTopFrame(); top != nil {
		title = top.GetTitle()
	}
	info := SessionInfo{
		PID:      pid,
		Title:    title,
		SockPath: sockPath,
	}
	data, _ := json.Marshal(info)
	os.WriteFile(infoPath, data, 0600)
}

func removeSessionInfo(sockPath string) {
	pid := os.Getpid()
	infoPath := filepath.Join(sessionDir(), fmt.Sprintf("f4-%d.json", pid))
	os.Remove(infoPath)
	os.Remove(sockPath)
}

func ManageSessions() {
	if len(os.Args) > 1 && os.Args[1] == "--server" {
		runServer(os.Args[2])
		return
	}

	if os.Getenv("F4_NESTED") != "" {
		startNewSession()
		return
	}

	sessions := listSessions()
	if len(sessions) > 0 {
		selected := runSessionPicker(sessions)
		if selected != nil {
			if selected.PID == 0 {
				startNewSession()
			} else {
				runClient(selected.SockPath)
			}
			return
		} else {
			return // Picker cancelled
		}
	}
	startNewSession()
}

func startNewSession() {
	pid := os.Getpid()
	sockPath := filepath.Join(sessionDir(), fmt.Sprintf("f4-new-%d-%d.sock", pid, time.Now().Unix()))
	vtui.DebugLog("SESSION: Starting new daemon server at %s", sockPath)

	cmd := exec.Command(os.Args[0], "--server", sockPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // Detach from terminal

	// Crucial for GUI startup: redirect daemon's own I/O to null so it doesn't
	// hold the parent's pipe/TTY.
	null, _ := os.Open(os.DevNull)
	cmd.Stdin = null
	cmd.Stdout = null
	cmd.Stderr = null

	if err := cmd.Start(); err != nil {
		vtui.DebugLog("SESSION: CRITICAL: Failed to spawn daemon process (path: %s): %v", os.Args[0], err)
		if null != nil {
			null.Close()
		}
		fmt.Println("Failed to start daemon:", err)
		return
	}
	vtui.DebugLog("SESSION: Daemon spawned successfully (PID: %d). Attaching client.", cmd.Process.Pid)
	if null != nil {
		null.Close()
	}

	// Wait for the server to create the socket
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	runClient(sockPath)
}

func runClient(sockPath string) {
	vtui.DebugLog("CLIENT: Start runClient, target socket: %s", sockPath)
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		vtui.DebugLog("CLIENT: WARNING: os.Stdin (fd %d) is not a terminal!", os.Stdin.Fd())
	}

	var serverPID int
	for _, s := range listSessions() {
		if s.SockPath == sockPath {
			serverPID = s.PID
			break
		}
	}
	if serverPID > 0 {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGWINCH)
		go func() {
			for range sigChan {
				syscall.Kill(serverPID, syscall.SIGWINCH)
			}
		}()
	}

	clntPath := filepath.Join(sessionDir(), fmt.Sprintf("clnt-%d-%d.ipc", os.Getpid(), time.Now().UnixNano()))
	laddr, _ := net.ResolveUnixAddr("unixgram", clntPath)

	conn, err := net.ListenUnixgram("unixgram", laddr)
	if err != nil {
		vtui.DebugLog("CLIENT: CRITICAL: Failed to create client socket: %v", err)
		return
	}
	defer os.Remove(clntPath)
	defer conn.Close()

	raddr, _ := net.ResolveUnixAddr("unixgram", sockPath)

	notifyPipe := make([]int, 2)
	if err := syscall.Pipe(notifyPipe); err != nil {
		vtui.DebugLog("CLIENT: CRITICAL: Failed to create notify pipe: %v", err)
		return
	}
	defer syscall.Close(notifyPipe[0])

	oob := syscall.UnixRights(0, 1, notifyPipe[1])
	vtui.DebugLog("CLIENT: FDs to send: In:0 Out:1 Pipe:%d", notifyPipe[1])

	n, oobn, err := conn.WriteMsgUnix([]byte("ATTACH"), oob, raddr)
	if err != nil {
		vtui.DebugLog("CLIENT: ATTACH FAILURE: Failed to send FDs to daemon at %s: %v", sockPath, err)
		return
	}
	vtui.DebugLog("CLIENT: FDs transmitted (sent %d bytes, %d oob). Relinquishing terminal control.", n, oobn)
	syscall.Close(notifyPipe[1])

	vtui.DebugLog("CLIENT: Waiting for server signal on pipe %d...", notifyPipe[0])
	dummy := make([]byte, 1)
	nRead, err := syscall.Read(notifyPipe[0], dummy)
	vtui.DebugLog("CLIENT: Server released pipe. nRead=%d, err=%v", nRead, err)
}

func runServer(sockPath string) {
	vtui.DebugLog("SERVER: Starting daemon at %s", sockPath)

	// Prevent the server from dying if the terminal drops
	signal.Ignore(syscall.SIGPIPE)
	signal.Ignore(syscall.SIGHUP)
	signal.Ignore(syscall.SIGTTOU)
	signal.Ignore(syscall.SIGTTIN)

	scr := InitCore()

	addr, _ := net.ResolveUnixAddr("unixgram", sockPath)
	conn, err := net.ListenUnixgram("unixgram", addr)
	if err != nil {
		vtui.DebugLog("SERVER: Listen error: %v", err)
		return
	}
	defer conn.Close()

	writeSessionInfo(sockPath)
	defer removeSessionInfo(sockPath)

	vtui.DebugLog("SERVER: Daemon listener active on %s. Standing by.", sockPath)
	for {
		buf := make([]byte, 32)
		oob := make([]byte, 1024)

		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, oobn, _, from, err := conn.ReadMsgUnix(buf, oob)
		if err != nil {
			if strings.Contains(err.Error(), "timeout") {
				continue
			}
			vtui.DebugLog("SERVER: IPC error on %s: %v", sockPath, err)
			time.Sleep(1 * time.Second)
			continue
		}

		vtui.DebugLog("SERVER: Connection received from client %v (Message: %q).", from, string(buf[:n]))

		scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
		if err != nil {
			vtui.DebugLog("SERVER: ParseSocketControlMessage error: %v", err)
			continue
		}
		if len(scms) == 0 {
			vtui.DebugLog("SERVER: SCM_RIGHTS list is empty")
			continue
		}
		fds, err := syscall.ParseUnixRights(&scms[0])
		if err != nil || len(fds) < 3 {
			vtui.DebugLog("SERVER: Failed to parse Unix rights")
			continue
		}

		vtui.DebugLog("SERVER: FDs received (In:%d Out:%d Pipe:%d). Goroutines: %d. Attaching terminal.", fds[0], fds[1], fds[2], runtime.NumGoroutine())
		// We don't log individual FD flags in production to keep logs clean.

		newStdin := os.NewFile(uintptr(fds[0]), "/dev/stdin")
		newStdout := os.NewFile(uintptr(fds[1]), "/dev/stdout")
		notifyPipeWriteEnd := fds[2]

		// Save global state to restore later
		oldStdin, oldStdout := os.Stdin, os.Stdout
		os.Stdin, os.Stdout = newStdin, newStdout

		vtui.DebugLog("SERVER: Enabling raw mode on new Stdin...")

		// Set new Stdin/Stdout before calling Enable
		os.Stdin, os.Stdout = newStdin, newStdout

		var restore func()
		// Only try to enable raw mode if we actually have a terminal.
		if term.IsTerminal(int(os.Stdin.Fd())) {
			r, err := vtui.PrepareTerminal()
			if err != nil {
				vtui.DebugLog("SERVER: WARNING: Failed to prepare terminal: %v", err)
			} else {
				restore = r
				vtui.DebugLog("SERVER: Raw mode and environment enabled successfully.")
			}
		} else {
			vtui.DebugLog("SERVER: FD %d is NOT a terminal, raw mode skipped.", os.Stdin.Fd())
		}

		// Sync terminal size
		if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 && h > 0 {
			vtui.DebugLog("SERVER: Terminal size: %dx%d", w, h)
			scr.AllocBuf(w, h)
			for _, s := range vtui.FrameManager.Screens {
				for _, f := range s.Frames {
					f.ResizeConsole(w, h)
				}
			}
		}
		scr.HardReset()
		vtui.FrameManager.Redraw()

		vtui.DebugLog("SERVER: PRE-RUN: Stdin FD: %d, Stdout FD: %d", os.Stdin.Fd(), os.Stdout.Fd())
		reader := vtinput.NewReader(os.Stdin, false)

		vtui.DebugLog("SERVER: Entering fm.Run()...")
		vtui.FrameManager.Run(reader)
		vtui.DebugLog("SERVER: fm.Run() EXITED.")

		if restore != nil {
			// Ensure all pending escape sequences are sent before restoring terminal
			os.Stdout.Sync()

			// 2. CRITICAL: Clear O_NONBLOCK that Go automatically sets.
			// Shared FD description means bash will also get EAGAIN if we don't.
			clearNonBlock := func(f *os.File) {
				flags, _, _ := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), syscall.F_GETFL, 0)
				syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), syscall.F_SETFL, flags & ^uintptr(syscall.O_NONBLOCK))
			}
			clearNonBlock(os.Stdin)
			clearNonBlock(os.Stdout)

			vtui.DebugLog("SERVER: Calling terminal restore()...")
			restore()
			vtui.DebugLog("SERVER: terminal restore() done.")
		}

		// CLOSE the notify pipe to signal the client it can exit.

		// CRITICAL: Redirect standard descriptors to /dev/null to fully release the PTY.
		// If the daemon keeps PTY FDs open as its own 0,1,2, the host shell hangs.
		devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err == nil {
			dnfd := int(devNull.Fd())
			_ = unix.Dup2(dnfd, 0)
			_ = unix.Dup2(dnfd, 1)
			_ = unix.Dup2(dnfd, 2)
			devNull.Close()
		}

		// CLOSE the notify pipe to signal the client it can exit.
		syscall.Close(notifyPipeWriteEnd)

		// Ensure we don't hold the terminal if the session is logically closed
		os.Stdout.Sync()

		// CRITICAL: If the system assigned us standard FDs (0, 1, 2) for the new session,
		// we MUST NOT close them, or os.Stdin/os.Stdout in the server process will become
		// invalid (EBADF) for all future connections.
		if fds[0] > 2 {
			newStdin.Close()
		}
		if fds[1] > 2 {
			newStdout.Close()
		}

		// Restore original server Stdin/Stdout pointers so they aren't garbage collected.
		os.Stdin, os.Stdout = oldStdin, oldStdout

		if vtui.FrameManager.IsShutdown() {
			vtui.DebugLog("SERVER: Shutdown requested. Exiting.")
			break
		}
		writeSessionInfo(sockPath)
	}
}

func runSessionPicker(sessions []SessionInfo) *SessionInfo {
	restore, err := vtui.PrepareTerminal()
	if err != nil {
		return nil
	}
	defer restore()

	width, height, _ := term.GetSize(0)
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(width, height)
	vtui.FrameManager.Init(scr)
	SetDefaultF4Palette()

	dlg := vtui.NewCenteredDialog(50, 15, Msg("Session.Title"))

	var items []string
	for _, s := range sessions {
		items = append(items, fmt.Sprintf("PID: %d - %s", s.PID, s.Title))
	}
	items = append(items, "--- Start New Session ---")

	lb := vtui.NewListBox(0, 0, 10, 9, items)
	dlg.AddItem(lb)

	var selected *SessionInfo
	lb.OnAction = func(idx int) {
		if idx < len(sessions) {
			selected = &sessions[idx]
		} else {
			selected = &SessionInfo{PID: 0}
		}
		dlg.SetExitCode(1)
	}

	btnOk := vtui.NewButton(0, 0, Msg("vtui.Ok"))
	btnCancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))
	dlg.AddItem(btnOk)
	dlg.AddItem(btnCancel)

	// Layout Engine
	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, 50-4, 15-4)
	vbox.Add(lb, vtui.Margins{}, vtui.AlignFill)

	hbox := vtui.NewHBoxLayout(0, 0, 50-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(hbox, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	btnOk.OnClick = func() {
		if lb.OnAction != nil {
			lb.OnAction(lb.SelectPos)
		}
	}
	btnCancel.OnClick = func() { dlg.SetExitCode(-1) }

	vtui.FrameManager.Push(dlg)
	reader := vtinput.NewReader(os.Stdin, false)
	vtui.FrameManager.Run(reader)

	vtui.FrameManager.Shutdown()

	return selected
}
