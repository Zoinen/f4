//go:build darwin

package panel

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/terminal"
)

type shellLaunch struct {
	name string
	args []string
}

type recordingShellPTY struct {
	mockPty
	launch chan shellLaunch
}

func (p *recordingShellPTY) Run(name string, args ...string) error {
	p.launch <- shellLaunch{name: name, args: args}
	return errors.New("launch captured for real PTY probe")
}

func TestPanelsFrameMacShellLoadsLoginProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("ZDOTDIR", dir)
	t.Setenv("F4_PROFILE_PROBE", "")
	t.Setenv("PATH", "/usr/bin:/bin:/usr/sbin:/sbin")
	for name, content := range map[string]string{
		".zprofile": "export F4_PROFILE_PROBE=profile\n",
		".zshrc":    "export F4_PROFILE_PROBE=${F4_PROFILE_PROBE}-rc\nPS1=''\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	pf := setupMockPanelsFrame(t)
	defer pf.Close()
	oldSpawn, oldFactory := SpawnLocalShellPTY, newLocalPTY
	t.Cleanup(func() { SpawnLocalShellPTY, newLocalPTY = oldSpawn, oldFactory })
	capture := &recordingShellPTY{launch: make(chan shellLaunch, 1)}
	newLocalPTY = func() (terminal.PtyBackend, error) { return capture, nil }
	SpawnLocalShellPTY = true
	pf.Pty = nil
	pf.InitPTY()
	var launch shellLaunch
	select {
	case launch = <-capture.launch:
	case <-time.After(5 * time.Second):
		t.Fatal("shell was not started")
	}
	p, err := terminal.NewPTY()
	if err != nil {
		t.Fatal(err)
	}
	started := false
	defer func() {
		_ = p.Close()
		if started {
			_ = p.Wait()
		}
	}()
	if err := p.Run(launch.name, launch.args...); err != nil {
		t.Fatal(err)
	}
	started = true
	// Split the marker so terminal echo cannot satisfy the assertion.
	command := "printf 'F4_%s:%s:%s:%s\\n' RESULT $options[login] $options[interactive] $F4_PROFILE_PROBE\r"
	if _, err := p.Write([]byte(command)); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	buf := make([]byte, 4096)
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(out.String(), "F4_RESULT:") && time.Now().Before(deadline) {
		_ = p.Master.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, err := p.Read(buf)
		out.Write(buf[:n])
		if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Fatal(err)
		}
	}
	if !strings.Contains(out.String(), "F4_RESULT:on:on:profile-rc") {
		t.Fatalf("login profile missing from real shell: %q", out.String())
	}
}
