//go:build windows

// The netfox "wsl" site type's transport (f4#1494): a locally spawned
// wsl.exe in place of the \\wsl.localhost\ UNC projection. wsl.exe attaches
// stdin/stdout to a shell it starts inside the distribution, which is
// exactly the duplex byte stream FISH+ already knows how to bootstrap over
// -- fish_dialer_lite.go tells the same "wrap a console tool as a
// subprocess" story for ssh, and this is the same shape with a different
// argv and no auth story at all: wsl.exe starts the distribution (if it is
// not running already) and needs no host, port, user or password.
//
// Going through wsl.exe rather than the UNC path means a copy or a move
// within the distribution's own filesystem never leaves it -- helper.sh
// runs cp/mv locally, at Linux disk speed, instead of every byte crossing
// the projection out to this side and back -- and a directory listing is
// one FISH+ request instead of a plan9 readdir plus a stat per entry.

package netfox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
)

// wslLookPath is exec.LookPath, overridden in tests so they can exercise
// the "wsl.exe not on PATH" error path without touching the real PATH.
var wslLookPath = exec.LookPath

// buildWSLArgs is the pure part of the dialer: the argument list handed to
// wsl.exe, kept separate from exec.Command so it can be asserted on
// directly in tests without spawning anything.
//
// "-d <distro>" selects the distribution the way "wsl.exe -l -q" names it;
// an empty distro omits the flag entirely and leaves wsl.exe to pick
// whichever one it treats as the default. "--" tells wsl.exe that
// everything after it is the command to run inside the distribution rather
// than one of wsl.exe's own flags, and naming /bin/sh directly -- instead
// of asking for an interactive login shell -- sidesteps whatever the
// distribution's default user shell happens to be, the same reason
// sshFishDialer execs /bin/sh rather than requesting a plain shell. No
// pseudo terminal is requested: wsl.exe attaches a plain pipe unless one is
// asked for, which is what lets the FISH+ bootstrap send raw bytes instead
// of the base64 encoding a real terminal would force.
func buildWSLArgs(distro string) []string {
	args := make([]string, 0, 4)
	if distro != "" {
		args = append(args, "-d", distro)
	}
	return append(args, "--", "/bin/sh")
}

// wslProcess ties the lifetime of the local wsl.exe subprocess -- and,
// through it, the shell it started inside the distribution -- to the
// FISH+ session speaking over its stdin/stdout. Closing it is how the
// remote shell is told to leave: closing stdin delivers the EOF that ends
// "/bin/sh" the same way sshProcess.Close ends the remote shell over ssh,
// and wsl.exe itself then exits once that shell does.
type wslProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func (p *wslProcess) Close() error {
	stdinErr := p.stdin.Close()
	_ = p.stdout.Close() // Best effort: stdin's EOF is what actually ends the remote side.
	waitErr := p.cmd.Wait()
	if stdinErr != nil {
		return stdinErr
	}
	// The remote shell exiting non-zero -- including "killed" once wsl.exe
	// tears down after stdin's EOF -- is not this Close's problem to
	// report; the session already knows it is the one closing.
	var exitErr *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) {
		return waitErr
	}
	return nil
}

// dialWSLSubprocess spawns wsl.exe with the distribution's /bin/sh attached
// to its stdin/stdout, wiring that pair up as the FISH+ transport. There is
// no dial-level failure to distinguish from a handshake failure the way SSH
// has one (wrong host, refused, auth): if wsl.exe itself is missing this
// fails before Start; if the named distribution does not exist or is not
// installed, wsl.exe exits almost immediately and the FISH+ handshake sees
// the resulting EOF, reported the same way a Windows sshd peer answering
// the wrong shell flavor is (see isHandshakeFailure in ssh_dial.go).
func dialWSLSubprocess(ctx context.Context, distro string) (io.Writer, io.Reader, io.Closer, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}
	wslPath, err := wslLookPath("wsl.exe")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fishplus: no wsl.exe on PATH: %w", err)
	}

	// #nosec G204 -- distro comes from a site the user configured, the same
	// trust boundary the ssh dialer's host/user/keyPath already runs at.
	cmd := exec.CommandContext(ctx, wslPath, buildWSLArgs(distro)...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		proc := &wslProcess{cmd: cmd, stdin: stdin, stdout: stdout}
		_ = proc.Close() // Preserve cancellation as the primary error.
		return nil, nil, nil, err
	}
	return stdin, stdout, &wslProcess{cmd: cmd, stdin: stdin, stdout: stdout}, nil
}

// wslFishDialer is the FishDialer for a WSL distribution reached through a
// locally spawned wsl.exe instead of SSH. distro is the distribution name
// "wsl.exe -l -q" enumerates; an empty distro dials whichever distribution
// wsl.exe treats as the default.
func wslFishDialer(distro string) FishDialer {
	return func(ctx context.Context) (io.Writer, io.Reader, io.Closer, error) {
		return dialWSLSubprocess(ctx, distro)
	}
}
