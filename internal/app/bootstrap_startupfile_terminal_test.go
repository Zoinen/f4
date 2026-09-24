//go:build linux || darwin

package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/unxed/f4/internal/terminal"
)

// TestTerminalStartOpensNamedFileInViewer starts `f4 sub/notes.txt` in a
// terminal, the way issue #991 asks to use it, and reads what f4 draws there:
// the file's text, which only the viewer shows. It takes the whole road the
// file travels -- the command line, the client, the ATTACH datagram to the
// session daemon, and the viewer opened on attach -- that the unit tests
// check a piece at a time. The harness is the one of
// TestTerminalStartOpensItsDirectoryOverTheSession.
func TestTerminalStartOpensNamedFileInViewer(t *testing.T) {
	if testing.Short() {
		t.Skip("starts f4 and its session daemon in a pty")
	}
	f4 := newStartupFileTest(t)
	here := f4.dir(t, "here")
	sub := f4.dir(t, filepath.Join("here", "sub"))
	// The text differs from the file name, which a panel shows too.
	const contentMarker = "viewmark991"
	writeStartupFixture(t, filepath.Join(sub, "notes.txt"), "first line\n"+contentMarker+" in the file\n")

	// A relative path, resolved against the directory the shell is in.
	started := f4.start(t, here, filepath.Join("sub", "notes.txt"))
	started.waitFor(t, f4, contentMarker,
		"`f4 sub/notes.txt` in a terminal must show the file in the viewer (issue #991)")
}

// TestTerminalEditOpensFileOfItsOwnDirectory runs `f4 -e notes.txt` while an
// f4 session daemon started in another directory is already running. That
// client does not start a daemon of its own: it attaches to the running one,
// which draws into its terminal. The relative name must still mean the file
// in the directory the command was typed in, not in the one the daemon was
// started from.
func TestTerminalEditOpensFileOfItsOwnDirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("starts f4 and its session daemon in a pty")
	}
	f4 := newStartupFileTest(t)
	first := f4.dir(t, "first")
	second := f4.dir(t, "second")
	const firstMarker, contentMarker = "firstdirmark", "editmark991"
	writeStartupFixture(t, filepath.Join(first, firstMarker), "")
	// The daemon's own directory has a notes.txt too, with other text.
	writeStartupFixture(t, filepath.Join(first, "notes.txt"), "wrongdirmark\n")
	writeStartupFixture(t, filepath.Join(second, "notes.txt"), contentMarker+" in the file\n")

	running := f4.start(t, first)
	running.waitFor(t, f4, firstMarker, "the first f4 must show its directory before the second one starts")

	editing := f4.start(t, second, "-e", "notes.txt")
	editing.waitFor(t, f4, contentMarker,
		"`f4 -e notes.txt` attaching to a daemon started elsewhere must edit the notes.txt of its own directory")
}

// TestTerminalEditOpensFileOverADialog is the same, with a dialog left open in
// the running session: the Create Folder dialog, from F7. The file must still
// open for the client that asked for it.
func TestTerminalEditOpensFileOverADialog(t *testing.T) {
	if testing.Short() {
		t.Skip("starts f4 and its session daemon in a pty")
	}
	f4 := newStartupFileTest(t)
	first := f4.dir(t, "first")
	second := f4.dir(t, "second")
	const firstMarker, contentMarker = "firstdirmark", "editmark991"
	writeStartupFixture(t, filepath.Join(first, firstMarker), "")
	writeStartupFixture(t, filepath.Join(second, "notes.txt"), contentMarker+" in the file\n")

	running := f4.start(t, first)
	running.waitFor(t, f4, firstMarker, "the first f4 must show its directory before the second one starts")
	if _, err := running.pty.Master.Write([]byte("\x1b[18~")); err != nil {
		t.Fatalf("press F7: %v", err)
	}
	running.waitFor(t, f4, "Create Folder", "F7 must open the Create Folder dialog in the first f4")

	editing := f4.start(t, second, "-e", "notes.txt")
	editing.waitFor(t, f4, contentMarker,
		"`f4 -e notes.txt` attaching to a daemon with a dialog open must still edit the file")
}

// startupFileTest is one isolated f4 profile and temporary directory that any
// number of f4 clients can be started in, so a later client finds the session
// daemon an earlier one spawned.
type startupFileTest struct {
	root, logDir, tmp string
	env               []string
}

func newStartupFileTest(t *testing.T) *startupFileTest {
	t.Helper()
	root := startupTestRoot(t)
	f := &startupFileTest{root: root, logDir: filepath.Join(root, "log"), tmp: filepath.Join(root, "tmp")}
	home := f.dir(t, "home")
	configHome := f.dir(t, "config")
	f.dir(t, "tmp")
	f.dir(t, "log")
	// No update check: its prompt may pop up over the panels at any moment,
	// and whatever the command line asked for is opened on the panels, not
	// over a dialog.
	settingsPath := filepath.Join(childUserConfigDir(home, configHome), "f4", "settings.ini")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o700); err != nil {
		t.Fatal(err)
	}
	writeStartupFixture(t, settingsPath, "[Update]\nInterval = 0\n")
	f.env = []string{
		runAsF4Env + "=1",
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + configHome,
		"TMPDIR=" + f.tmp,
		"PATH=" + os.Getenv("PATH"),
		"SHELL=/bin/sh",
		"HISTFILE=/dev/null",
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"LANG=en_US.UTF-8",
		"VTUI_DEBUG=" + filepath.Join(f.logDir, "f4.log"),
	}
	for _, name := range []string{"USER", "LOGNAME"} {
		if value, ok := os.LookupEnv(name); ok {
			f.env = append(f.env, name+"="+value)
		}
	}
	return f
}

func (f *startupFileTest) dir(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(f.root, name)
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

type startedF4 struct {
	client *startedProcess
	screen *startupScreen
	pty    *terminal.PTY
}

// start runs f4 with args in a pty of its own, in dir.
func (f *startupFileTest) start(t *testing.T, dir string, args ...string) *startedF4 {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("cannot locate the test binary to run as f4: %v", err)
	}
	pty, err := terminal.NewPTY()
	if err != nil {
		t.Skipf("PTY allocation unavailable in this environment: %v", err)
	}
	pty.SetSize(startTermCols, startTermRows)
	screen := newStartupScreen(startTermCols, startTermRows)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 32*1024)
		for {
			n, err := pty.Master.Read(buf)
			if n > 0 {
				screen.Feed(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	// #nosec G204 G702 -- exe is this test binary, started as f4 on purpose.
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.Env = append(append([]string(nil), f.env...), "PWD="+dir)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = pty.Slave, pty.Slave, pty.Slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		_ = pty.Close()
		t.Fatalf("start f4 in a pty: %v", err)
	}
	client := &startedProcess{cmd: cmd, done: make(chan struct{})}
	go func() {
		client.err = cmd.Wait()
		close(client.done)
	}()
	t.Cleanup(func() { stopStartupTest(t, client, pty, readDone, f.tmp) })
	return &startedF4{client: client, screen: screen, pty: pty}
}

// waitFor waits for marker to appear on the screen of this f4, and fails with
// why otherwise.
func (s *startedF4) waitFor(t *testing.T, f *startupFileTest, marker, why string) {
	t.Helper()
	shown := func(rows [][]rune) bool {
		for _, row := range rows {
			if strings.Contains(string(row), marker) {
				return true
			}
		}
		return false
	}
	// The very first start with an empty profile builds its configuration and
	// takes seconds.
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-s.client.done:
			t.Fatalf("%s; f4 exited first: %v\nscreen:\n%s\nf4 debug log (tail):\n%s",
				why, s.client.err, formatStartupScreen(s.screen.Settled(time.Second)), startupLogTail(f.logDir, 120))
		default:
		}
		if rows, complete := s.screen.Snapshot(); complete && shown(rows) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	rows := s.screen.Settled(5 * time.Second)
	if !shown(rows) {
		t.Fatalf("%s; %q is not on the screen.\nscreen:\n%s\nf4 debug log (tail):\n%s",
			why, marker, formatStartupScreen(rows), startupLogTail(f.logDir, 120))
	}
}
