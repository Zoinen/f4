//go:build !windows

package vfs

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/unxed/vtui"
)

// RunSudoDispatcher starts the elevated backend server loop.
// It is spawned by SudoClient via `sudo -A`.
func RunSudoDispatcher(sockPath string) {
	fmt.Fprintf(os.Stderr, "SUDO_DISPATCHER: STARTING (EUID=%d, PID=%d)\n", os.Geteuid(), os.Getpid())
	// Use a dedicated log file because stderr might be unreliable under sudo
	sudoUid := os.Getenv("SUDO_UID")
	if sudoUid == "" {
		sudoUid = fmt.Sprintf("%d", os.Getuid())
	}
	debugLogPath := filepath.Join(os.TempDir(), fmt.Sprintf("f4-sudo-debug-%s.txt", sudoUid))
	debugLog, _ := os.OpenFile(debugLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if debugLog != nil {
		fmt.Fprintf(debugLog, "[%s] DISPATCHER STARTING: EUID=%d PID=%d Sock=%q\n", time.Now().Format("15:04:05"), os.Geteuid(), os.Getpid(), sockPath)
		defer debugLog.Close()
	}

	// Create a canary file to prove execution
	canaryPath := filepath.Join(os.TempDir(), fmt.Sprintf("f4-canary-%d.txt", os.Getpid()))
	os.WriteFile(canaryPath, []byte(fmt.Sprintf("EUID=%d", os.Geteuid())), 0666)

	fmt.Fprintf(os.Stderr, "SUDO_DISPATCHER: STARTING (EUID=%d, PID=%d)\n", os.Geteuid(), os.Getpid())
	if os.Geteuid() != 0 {
		vtui.DebugLog("SUDO_DISPATCHER: FATAL: Not running as root (EUID: %d)", os.Geteuid())
		fmt.Fprintf(os.Stderr, "f4 dispatcher must run as root\n")
		os.Exit(1)
	}
	vtui.DebugLog("SUDO_DISPATCHER: Initializing on %q as root", sockPath)

	os.Remove(sockPath)
	vtui.DebugLog("SUDO_DISPATCHER: Creating socket %q", sockPath)
	addr, err := net.ResolveUnixAddr("unix", sockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SUDO_DISPATCHER: ResolveUnixAddr failed: %v\n", err)
		os.Exit(1)
	}

	l, err := net.ListenUnix("unix", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SUDO_DISPATCHER: ListenUnix failed: %v\n", err)
		os.Exit(1)
	}
	defer l.Close()

	if debugLog != nil {
		fmt.Fprintf(debugLog, "[%s] DISPATCHER: Socket created.\n", time.Now().Format("15:04:05"))
	}

	fi, _ := os.Stat(sockPath)
	fmt.Fprintf(os.Stderr, "SUDO_DISPATCHER: Socket created. Initial perms: %v\n", fi.Mode())
	if debugLog != nil {
		fmt.Fprintf(debugLog, "[%s] DISPATCHER: Initial perms: %v\n", time.Now().Format("15:04:05"), fi.Mode())
	}

	fmt.Fprintf(os.Stderr, "SUDO_DISPATCHER: Setting permissions 0666...\n")
	if debugLog != nil {
		fmt.Fprintf(debugLog, "[%s] DISPATCHER: Chmod 0666 starting...\n", time.Now().Format("15:04:05"))
	}
	// Permissions 0666 allow the non-root f4 process to connect to the root-owned socket.
	err = os.Chmod(sockPath, 0666)
	if debugLog != nil {
		fmt.Fprintf(debugLog, "[%s] DISPATCHER: Chmod result: %v\n", time.Now().Format("15:04:05"), err)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "SUDO_DISPATCHER: Chmod failed: %v\n", err)
	}

	// In this design, we accept a single connection, process its requests,
	// and exit when it drops (or when f4 finishes).
	conn, err := l.AcceptUnix()
	if err == nil {
		handleSudoClient(conn)
	}
	os.Exit(0)
}

func handleSudoClient(conn *net.UnixConn) {
	defer conn.Close()

	for {
		var req SudoRequest
		vtui.DebugLog("SUDO_DISPATCHER: Waiting for message from client...")
		_, err := recvMsg(conn, &req)
		if err == nil {
			vtui.DebugLog("SUDO_DISPATCHER: Received Cmd=%d for Path=%q", req.Cmd, req.Path)
			fmt.Fprintf(os.Stderr, "SUDO_DISPATCHER: Received Cmd=%d for Path=%q\n", req.Cmd, req.Path)
		} else {
			vtui.DebugLog("SUDO_DISPATCHER: recvMsg error: %v", err)
		}
		if err != nil {
			return // Client disconnected or protocol error
		}

		resp := SudoResponse{}
		fd := -1

		vtui.DebugLog("SUDO_DISPATCHER: Processing Cmd=%d, Path=%q", req.Cmd, req.Path)

		switch req.Cmd {
		case CmdPing:
			// Just return success

		case CmdOpen:
			fi, err := os.Stat(req.Path)
			if err == nil && (fi.Mode()&(os.ModeNamedPipe|os.ModeSocket) != 0) {
				resp.Error = "cannot open special file"
			} else {
				f, err := os.OpenFile(req.Path, req.Flags, os.FileMode(req.Mode))
				if err != nil {
					vtui.DebugLog("SUDO_DISPATCHER: Open(%q) FAILED: %v", req.Path, err)
					resp.Error = err.Error()
				} else {
					fd = int(f.Fd())
					vtui.DebugLog("SUDO_DISPATCHER: Open(%q) SUCCESS, FD=%d", req.Path, fd)
					defer f.Close() // Safe to close in dispatcher, FD is duplicated across Unix socket
				}
			}

		case CmdStat:
			info, err := os.Stat(req.Path)
			if err != nil {
				resp.Error = err.Error()
			} else {
				resp.Item = VFSItem{
					Name:         info.Name(),
					Size:         info.Size(),
					IsDir:        info.IsDir(),
					MTime:        info.ModTime(),
					IsExecutable: info.Mode().Perm()&0111 != 0,
					IsHidden:     strings.HasPrefix(info.Name(), "."),
				}
			}

		case CmdMkDir:
			err := os.MkdirAll(req.Path, os.FileMode(req.Mode))
			if err != nil {
				resp.Error = err.Error()
			}

		case CmdRemove:
			err := os.RemoveAll(req.Path)
			if err != nil {
				resp.Error = err.Error()
			}

		case CmdRename:
			err := os.Rename(req.Path, req.Path2)
			if err != nil {
				resp.Error = err.Error()
			}
		case CmdSetAttributes:
			// Apply all 3 metadata types at once under root
			err := os.Chmod(req.Path, os.FileMode(req.Item.UnixMode))
			if err == nil {
				err = os.Chown(req.Path, req.Item.Uid, req.Item.Gid)
			}
			if err == nil {
				err = os.Chtimes(req.Path, req.Item.ATime, req.Item.MTime)
			}
			if err == nil {
				err = applyPlatformAttributes(req.Path, req.Item)
			}
			if err != nil {
				resp.Error = err.Error()
			}

		case CmdReadDir:
			entries, err := os.ReadDir(req.Path)
			if err != nil {
				resp.Error = err.Error()
			} else {
				for _, e := range entries {
					info, _ := e.Info()
					var size int64
					var mtime time.Time
					var isExec bool
					if info != nil {
						size = info.Size()
						mtime = info.ModTime()
						isExec = info.Mode().Perm()&0111 != 0
					}
					isDir := e.IsDir()

					// Resolve symlinks/special objects to check if they are directories
					if !isDir && !e.Type().IsRegular() {
						if target, err := os.Stat(filepath.Join(req.Path, e.Name())); err == nil {
							isDir = target.IsDir()
						}
					}

					resp.Items = append(resp.Items, VFSItem{
						Name:         e.Name(),
						Size:         size,
						IsDir:        isDir,
						MTime:        mtime,
						IsExecutable: isExec,
						IsHidden:     strings.HasPrefix(e.Name(), "."),
					})
				}
			}
		}

		err = sendMsg(conn, resp, fd)
		if err != nil {
			return
		}
	}
}
