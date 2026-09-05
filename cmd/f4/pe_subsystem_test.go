package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The subsystem check is what tells `notepad` (GUI, cmd does not wait) from
// `ping` (console, cmd waits). Build one of each and read them back.
func TestExecutableIsGUI(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go toolchain on PATH")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	build := func(out string, gui bool) {
		args := []string{"build", "-o", out}
		if gui {
			args = append(args, "-ldflags", "-H=windowsgui")
		}
		args = append(args, src)
		cmd := exec.Command(goBin, args...)
		cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0")
		if outb, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("cannot cross-compile a Windows binary here: %v\n%s", err, outb)
		}
	}
	console := filepath.Join(dir, "console.exe")
	gui := filepath.Join(dir, "gui.exe")
	build(console, false)
	build(gui, true)

	if got, err := executableIsGUI(console); err != nil || got {
		t.Errorf("console exe: gui=%v err=%v, want false", got, err)
	}
	if got, err := executableIsGUI(gui); err != nil || !got {
		t.Errorf("gui exe: gui=%v err=%v, want true", got, err)
	}
	// Second lookup comes from the cache and must agree.
	if got, _ := executableIsGUI(gui); !got {
		t.Error("cached answer differs")
	}
}
