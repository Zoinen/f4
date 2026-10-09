//go:build !windows

package install

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeEnv stubs UserHomeDir/Getenv/Confirm for one test and restores the
// originals afterward, so RunCLI never touches the real home directory,
// environment or stdin.
func fakeEnv(t *testing.T, home string, env map[string]string, confirm bool) {
	t.Helper()
	oldHome, oldGetenv, oldConfirm := UserHomeDir, Getenv, Confirm
	t.Cleanup(func() { UserHomeDir, Getenv, Confirm = oldHome, oldGetenv, oldConfirm })

	UserHomeDir = func() (string, error) { return home, nil }
	Getenv = func(key string) string { return env[key] }
	Confirm = func(prompt string) bool { return confirm }
}

func writeFakeExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("fake f4 binary"), 0o755); err != nil { // #nosec G306 -- test fixture executable, needs the exec bit
		t.Fatal(err)
	}
	return path
}

func TestRunCLIInstallsAndReportsAlreadyOnPath(t *testing.T) {
	home := t.TempDir()
	srcDir := t.TempDir()
	exe := writeFakeExecutable(t, srcDir, "f4")

	fakeEnv(t, home, map[string]string{
		"PATH":  PreferredDir(home) + string(os.PathListSeparator) + "/usr/bin",
		"SHELL": "/bin/bash",
	}, false)

	if got := RunCLI(exe, Options{}); got != 0 {
		t.Fatalf("RunCLI = %d, want 0", got)
	}
	dst := filepath.Join(PreferredDir(home), "f4")
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("installed binary missing: %v", err)
	}
	if string(data) != "fake f4 binary" {
		t.Errorf("installed content = %q", data)
	}
	if _, err := os.Stat(filepath.Join(home, ".bashrc")); !os.IsNotExist(err) {
		t.Errorf(".bashrc should be untouched when the dir is already on PATH, stat err = %v", err)
	}
}

func TestRunCLIPromptsAndAppendsOnConfirm(t *testing.T) {
	home := t.TempDir()
	srcDir := t.TempDir()
	exe := writeFakeExecutable(t, srcDir, "f4")

	fakeEnv(t, home, map[string]string{
		"PATH":  "/usr/bin:/bin",
		"SHELL": "/bin/zsh",
	}, true) // Confirm() answers yes

	if got := RunCLI(exe, Options{}); got != 0 {
		t.Fatalf("RunCLI = %d, want 0", got)
	}
	data, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil {
		t.Fatalf(".zshrc not written: %v", err)
	}
	profile, _ := DetectShellProfile(home, "/bin/zsh")
	want := PathLine(profile, home, PreferredDir(home))
	if !strings.Contains(string(data), want) {
		t.Errorf(".zshrc = %q, want it to mention %q", data, want)
	}
}

func TestRunCLIDeclinesWithoutConfirmation(t *testing.T) {
	home := t.TempDir()
	srcDir := t.TempDir()
	exe := writeFakeExecutable(t, srcDir, "f4")

	fakeEnv(t, home, map[string]string{
		"PATH":  "/usr/bin:/bin",
		"SHELL": "/bin/bash",
	}, false) // Confirm() answers no

	if got := RunCLI(exe, Options{}); got != 0 {
		t.Fatalf("RunCLI = %d, want 0", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".bashrc")); !os.IsNotExist(err) {
		t.Errorf(".bashrc should be untouched when the user declines, stat err = %v", err)
	}
}

func TestRunCLIAutoConfirmSkipsPrompt(t *testing.T) {
	home := t.TempDir()
	srcDir := t.TempDir()
	exe := writeFakeExecutable(t, srcDir, "f4")

	promptAsked := false
	fakeEnv(t, home, map[string]string{
		"PATH":  "/usr/bin:/bin",
		"SHELL": "/bin/bash",
	}, false)
	Confirm = func(prompt string) bool { promptAsked = true; return false }

	if got := RunCLI(exe, Options{AutoConfirm: true}); got != 0 {
		t.Fatalf("RunCLI = %d, want 0", got)
	}
	if promptAsked {
		t.Errorf("Confirm was called despite --yes")
	}
	data, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatalf(".bashrc not written: %v", err)
	}
	profile, _ := DetectShellProfile(home, "/bin/bash")
	want := PathLine(profile, home, PreferredDir(home))
	if !strings.Contains(string(data), want) {
		t.Errorf(".bashrc = %q, want it to mention %q", data, want)
	}
}

func TestRunCLIRerunIsIdempotent(t *testing.T) {
	home := t.TempDir()
	srcDir := t.TempDir()
	exe := writeFakeExecutable(t, srcDir, "f4")

	fakeEnv(t, home, map[string]string{
		"PATH":  "/usr/bin:/bin",
		"SHELL": "/bin/bash",
	}, true)

	if got := RunCLI(exe, Options{}); got != 0 {
		t.Fatalf("first RunCLI = %d, want 0", got)
	}
	first, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatal(err)
	}

	// Confirm would now refuse, but re-running should not even need to ask:
	// the profile already has the line, and the binary is just overwritten.
	Confirm = func(prompt string) bool { t.Fatal("Confirm called on a re-run that needed no prompt"); return false }

	if got := RunCLI(exe, Options{}); got != 0 {
		t.Fatalf("second RunCLI = %d, want 0", got)
	}
	second, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf(".bashrc changed on re-run: first %q, second %q", first, second)
	}
	profile, _ := DetectShellProfile(home, "/bin/bash")
	line := PathLine(profile, home, PreferredDir(home))
	if n := strings.Count(string(second), line); n != 1 {
		t.Errorf("PATH line appears %d times after two runs, want 1: %q", n, second)
	}
}

func TestRunCLIUnknownShellPrintsLineInsteadOfPrompting(t *testing.T) {
	home := t.TempDir()
	srcDir := t.TempDir()
	exe := writeFakeExecutable(t, srcDir, "f4")

	fakeEnv(t, home, map[string]string{
		"PATH":  "/usr/bin:/bin",
		"SHELL": "/bin/tcsh",
	}, false)
	Confirm = func(prompt string) bool { t.Fatal("Confirm called for an unrecognized shell"); return false }

	if got := RunCLI(exe, Options{}); got != 0 {
		t.Fatalf("RunCLI = %d, want 0", got)
	}
}

func TestRunCLIFallsBackWhenPreferredDirUnavailable(t *testing.T) {
	home := t.TempDir()
	srcDir := t.TempDir()
	exe := writeFakeExecutable(t, srcDir, "f4")

	// Make ~/.local a plain file, so ~/.local/bin can be neither statted as a
	// directory nor created, and ~/bin must be used instead.
	if err := os.WriteFile(filepath.Join(home, ".local"), []byte("x"), 0o644); err != nil { // #nosec G306 -- test fixture, not sensitive
		t.Fatal(err)
	}
	fakeEnv(t, home, map[string]string{
		"PATH":  FallbackDir(home) + string(os.PathListSeparator) + "/usr/bin",
		"SHELL": "/bin/bash",
	}, false)

	if got := RunCLI(exe, Options{}); got != 0 {
		t.Fatalf("RunCLI = %d, want 0", got)
	}
	if _, err := os.Stat(filepath.Join(FallbackDir(home), "f4")); err != nil {
		t.Fatalf("binary not installed into fallback dir: %v", err)
	}
}

func TestRunCLIFailsWhenHomeDirUnavailable(t *testing.T) {
	oldHome := UserHomeDir
	UserHomeDir = func() (string, error) { return "", errors.New("no home for you") }
	defer func() { UserHomeDir = oldHome }()

	srcDir := t.TempDir()
	exe := writeFakeExecutable(t, srcDir, "f4")

	if got := RunCLI(exe, Options{}); got != 1 {
		t.Fatalf("RunCLI = %d, want 1", got)
	}
}

func TestRunCLIFailsWhenNeitherInstallDirCanBeCreated(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission checks")
	}
	parent := t.TempDir()
	roParent := filepath.Join(parent, "ro")
	if err := os.Mkdir(roParent, 0o555); err != nil {
		t.Fatal(err)
	}
	// home itself does not exist yet, and its parent forbids creating it, so
	// both PreferredDir and FallbackDir fail the same way.
	home := filepath.Join(roParent, "home")
	srcDir := t.TempDir()
	exe := writeFakeExecutable(t, srcDir, "f4")

	fakeEnv(t, home, map[string]string{
		"PATH":  "/usr/bin:/bin",
		"SHELL": "/bin/bash",
	}, false)

	if got := RunCLI(exe, Options{}); got != 1 {
		t.Fatalf("RunCLI = %d, want 1", got)
	}
}

func TestRunCLIFailsWhenCopyExecutableFails(t *testing.T) {
	home := t.TempDir()
	srcDir := t.TempDir()
	// exe points at a file that was never written, so CopyExecutable's own
	// os.Stat fails.
	exe := filepath.Join(srcDir, "does-not-exist")

	fakeEnv(t, home, map[string]string{
		"PATH":  "/usr/bin:/bin",
		"SHELL": "/bin/bash",
	}, false)

	if got := RunCLI(exe, Options{}); got != 1 {
		t.Fatalf("RunCLI = %d, want 1", got)
	}
}

func TestRunCLIFailsWhenProfileFileIsUnreadable(t *testing.T) {
	home := t.TempDir()
	srcDir := t.TempDir()
	exe := writeFakeExecutable(t, srcDir, "f4")

	// .bashrc is a directory here, so os.ReadFile fails with an error other
	// than "not exist" (EISDIR), which RunCLI must not treat as "no profile
	// yet".
	if err := os.MkdirAll(filepath.Join(home, ".bashrc"), 0o755); err != nil {
		t.Fatal(err)
	}

	fakeEnv(t, home, map[string]string{
		"PATH":  "/usr/bin:/bin",
		"SHELL": "/bin/bash",
	}, false)

	if got := RunCLI(exe, Options{}); got != 1 {
		t.Fatalf("RunCLI = %d, want 1", got)
	}
}

func TestRunCLIFailsWhenAppendProfileLineFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission checks")
	}
	home := t.TempDir()
	srcDir := t.TempDir()
	exe := writeFakeExecutable(t, srcDir, "f4")

	// fish's config lives under ~/.config/fish/; making ~/.config read-only
	// lets AppendProfileLine's os.MkdirAll(.../fish) fail while the profile
	// path itself is still a plain "does not exist yet" ENOENT for
	// os.ReadFile.
	configDir := filepath.Join(home, ".config")
	if err := os.Mkdir(configDir, 0o555); err != nil {
		t.Fatal(err)
	}

	fakeEnv(t, home, map[string]string{
		"PATH":  "/usr/bin:/bin",
		"SHELL": "/usr/bin/fish",
	}, false)

	if got := RunCLI(exe, Options{AutoConfirm: true}); got != 1 {
		t.Fatalf("RunCLI = %d, want 1", got)
	}
}

func TestConfirmStdinReadsAnswer(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"y", "y\n", true},
		{"yes", "yes\n", true},
		{"uppercase Y", "Y\n", true},
		{"mixed case Yes", "Yes\n", true},
		{"explicit no", "n\n", false},
		{"empty line", "\n", false},
		{"garbage", "maybe\n", false},
		{"trailing spaces", "  yes  \n", true},
		{"no trailing newline before EOF", "yes", true},
		{"empty input, immediate EOF", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			oldStdin := os.Stdin
			os.Stdin = r
			defer func() {
				os.Stdin = oldStdin
				_ = r.Close()
			}()

			done := make(chan struct{})
			go func() {
				_, _ = w.WriteString(c.input)
				_ = w.Close()
				close(done)
			}()

			if got := confirmStdin("Add it now? [y/N] "); got != c.want {
				t.Errorf("confirmStdin(%q) = %v, want %v", c.input, got, c.want)
			}
			<-done
		})
	}
}
