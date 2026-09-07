package archive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

// issue816EncryptedFixture creates a password-protected archive of the given
// kind with the password "Correct". It skips when the required tool is missing.
func issue816EncryptedFixture(t *testing.T, kind string) string {
	t.Helper()
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "secret.txt"), []byte("secret data"), 0600); err != nil {
		t.Fatal(err)
	}
	var p string
	var cmd *exec.Cmd
	switch kind {
	case "zip":
		p = filepath.Join(tmp, "s.zip")
		cmd = exec.Command("zip", "-q", "-P", "Correct", p, "secret.txt")
	case "7z":
		p = filepath.Join(tmp, "s.7z")
		cmd = exec.Command("7z", "a", "-t7z", "-pCorrect", "-bd", p, "secret.txt")
	case "7z-encrypted-headers":
		p = filepath.Join(tmp, "s.7z")
		cmd = exec.Command("7z", "a", "-t7z", "-pCorrect", "-mhe=on", "-bd", p, "secret.txt")
	case "rar":
		p = filepath.Join(tmp, "s.rar")
		cmd = exec.Command("rar", "a", "-pCorrect", "-idq", p, "secret.txt")
	case "rar-encrypted-headers":
		p = filepath.Join(tmp, "s.rar")
		cmd = exec.Command("rar", "a", "-hpCorrect", "-idq", p, "secret.txt")
	default:
		t.Fatalf("unknown fixture kind %q", kind)
	}
	if _, err := exec.LookPath(cmd.Path); err != nil {
		t.Skipf("%s command is not installed", cmd.Path)
	}
	cmd.Dir = tmp
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("create %s fixture: %v: %s", kind, err, out)
	}
	return p
}

// runIssue816Scenario opens the archive, lists it and reads secret.txt while
// the password prompt returns the given answers (a string is a typed
// password, an error means the dialog was closed). It returns a short
// description of the outcome and fails on a hang.
func runIssue816Scenario(t *testing.T, p string, answers []any, withProgress bool) (string, int) {
	t.Helper()
	prompts := 0
	prev := archivePasswordPrompt
	archivePasswordPrompt = func(context.Context, string) (string, error) {
		i := prompts
		prompts++
		if i >= len(answers) {
			return "", errors.New("too many prompts")
		}
		switch a := answers[i].(type) {
		case string:
			return a, nil
		case error:
			return "", a
		}
		return "", nil
	}
	defer func() { archivePasswordPrompt = prev }()

	done := make(chan string, 1)
	go func() {
		ctx := context.Background()
		if withProgress {
			ctx = context.WithValue(ctx, vfs.ProgressKey, vfs.ProgressCallback(func(string, int) {}))
		}
		v, err := NewArchiveVFSContext(ctx, vfs.NewOSVFS(filepath.Dir(p)), p)
		if err != nil {
			done <- "open: " + err.Error()
			return
		}
		defer v.Close()
		if err := v.ReadDir(ctx, v.GetPath(), func([]vfs.VFSItem) {}); err != nil {
			done <- "readdir: " + err.Error()
			return
		}
		f, err := v.Open(ctx, v.Join(p, "secret.txt"))
		if err != nil {
			done <- "member: " + err.Error()
			return
		}
		defer f.Close()
		data, err := io.ReadAll(ctxReader{r: f, ctx: ctx})
		if err != nil {
			done <- "read: " + err.Error()
			return
		}
		done <- fmt.Sprintf("data=%q", data)
	}()
	select {
	case s := <-done:
		return s, prompts
	case <-time.After(15 * time.Second):
		t.Fatalf("%s answers=%v progress=%v: hung after %d prompts", filepath.Base(p), answers, withProgress, prompts)
		return "", prompts
	}
}

// TestIssue816_WrongPasswordRepromptsUntilCorrect covers the follow-up in
// issue #816: a wrong password must bring the password dialog back (not an
// endless "Opening..." screen), an empty answer must re-prompt, and closing
// the dialog must abort the operation promptly.
func TestIssue816_WrongPasswordRepromptsUntilCorrect(t *testing.T) {
	kinds := []string{"zip", "7z", "7z-encrypted-headers", "rar", "rar-encrypted-headers"}
	for _, kind := range kinds {
		for _, withProgress := range []bool{false, true} {
			name := fmt.Sprintf("%s/progress=%v", kind, withProgress)
			t.Run(name, func(t *testing.T) {
				p := issue816EncryptedFixture(t, kind)

				res, prompts := runIssue816Scenario(t, p, []any{"Wrong", "", "Wrong2", "Correct"}, withProgress)
				if res != `data="secret data"` {
					t.Fatalf("wrong then correct: got %s", res)
				}
				if prompts != 4 {
					t.Fatalf("wrong then correct: prompts = %d, want 4", prompts)
				}

				res, prompts = runIssue816Scenario(t, p, []any{"Wrong", context.Canceled}, withProgress)
				if !strings.Contains(res, context.Canceled.Error()) {
					t.Fatalf("wrong then cancel: got %s", res)
				}
				if prompts != 2 {
					t.Fatalf("wrong then cancel: prompts = %d, want 2", prompts)
				}
			})
		}
	}
}
