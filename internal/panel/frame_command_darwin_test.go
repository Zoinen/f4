//go:build darwin && integration

package panel

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/internal/terminal"
)

func TestLocalCommandLongInteractiveRoundTrip(t *testing.T) {
	p, err := terminal.NewPTY()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	if err := p.Run("/bin/zsh", "-f"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Wait() })
	// Close before waiting for the shell, including on a failed assertion.
	t.Cleanup(func() { _ = p.Close() })
	out := make(chan struct{}, 1)
	var output strings.Builder
	var outputMu sync.Mutex
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(out)
		buf := make([]byte, 4096)
		for {
			n, err := p.Read(buf)
			if n > 0 {
				outputMu.Lock()
				output.Write(buf[:n])
				outputMu.Unlock()
				select {
				case out <- struct{}{}:
				default:
				}
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() { _ = p.Close(); <-done })
	waitFor := func(want string) {
		t.Helper()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		for {
			select {
			case _, ok := <-out:
				if !ok {
					t.Fatalf("shell closed before %q", want[:min(30, len(want))])
				}
				outputMu.Lock()
				matched := strings.Contains(output.String(), want)
				outputMu.Unlock()
				if matched {
					return
				}
			case <-timer.C:
				outputMu.Lock()
				got := output.String()
				outputMu.Unlock()
				t.Fatalf("shell did not execute full command; received %d bytes, tail %q",
					len(got), got[max(0, len(got)-200):])
			}
		}
	}
	if _, err := p.Write([]byte("PS1=''; printf '\\033]133;D\\007'\r")); err != nil {
		t.Fatal(err)
	}
	waitFor("\x1b]133;D\a")
	for i := range 20 {
		_ = p.Master.SetWriteDeadline(time.Now().Add(5 * time.Second))
		payload := fmt.Sprintf("%d:", i) + strings.Repeat("abcdefghijklmnopqrstuvwxyz0123456789", 64)
		arg, cleanup, err := prepareLocalCommandEvaluation("printf 'BEGIN:%s:END\\n' '" + payload + "'")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(cleanup)
		cmd := " { printf '\\033]133;C\\007'; eval " + arg + "; printf '\\033]133;D\\007'; }\r"
		if n, err := p.Write([]byte(cmd)); err != nil || n != len(cmd) {
			t.Fatalf("Write: %d/%d, %v", n, len(cmd), err)
		}
		waitFor("BEGIN:" + payload + ":END")
		cleanup()
	}
}

func TestLocalCommandEvaluationShellSemantics(t *testing.T) {
	for _, shell := range []string{"/bin/sh", "/bin/bash", "/bin/zsh"} {
		for _, tt := range []struct {
			name string
			cmd  string
		}{
			{name: "quotes and unicode", cmd: "printf '%s' " + ShellSingleQuote("it's $literal `text` 😀\n\n")},
			{name: "persistent state", cmd: "F4_TEST_STATE=after; cd /; chosen; printf '%s' \"$0:$1:$2\""},
			{name: "stdin", cmd: "read F4_TEST_INPUT; printf '%s' \"$F4_TEST_INPUT\""},
			{name: "syntax error", cmd: "echo 'unterminated"},
			{name: "trailing escaped newline", cmd: "printf '%s' hello\\\n"},
			{name: "status", cmd: "false"},
		} {
			t.Run(shell+"/"+tt.name, func(t *testing.T) {
				cmd := "# " + strings.Repeat("padding", 100) + "\n" + tt.cmd
				arg, cleanup, err := prepareLocalCommandEvaluation(cmd)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(cleanup)
				prefix := "alias chosen='printf alias-ok'; set -- first second; F4_TEST_STATE=before\n"
				if shell == "/bin/bash" {
					prefix = "shopt -s expand_aliases\n" + prefix
				}
				suffix := "; result=$?; printf '\\nSTATE:%s:%s:%s:%s\\n' \"$0\" \"$1\" \"$F4_TEST_STATE\" \"$PWD\"; exit $result"
				run := func(argument string) (string, int) {
					t.Helper()
					child := exec.CommandContext(t.Context(), shell, "-c", prefix+"eval "+argument+suffix, "f4-test-shell")
					child.Dir = "/tmp"
					child.Stdin = strings.NewReader("tty-input\n")
					out, _ := child.Output()
					return string(out), child.ProcessState.ExitCode()
				}
				want, wantCode := run(ShellSingleQuote(cmd))
				got, gotCode := run(arg)
				if got != want || gotCode != wantCode {
					t.Fatalf("staging changed shell behavior: got (%q,%d), want (%q,%d)", got, gotCode, want, wantCode)
				}
			})
		}
	}
}

func TestLocalCommandMissingPayloadReportsCompletion(t *testing.T) {
	arg, cleanup, err := prepareLocalCommandEvaluation(strings.Repeat("long", 200))
	if err != nil {
		t.Fatal(err)
	}
	cleanup()
	script := "printf '[C]'; eval " + arg + "; result=$?; printf '[D]'; exit $result"
	child := exec.CommandContext(t.Context(), "/bin/zsh", "-c", script)
	out, err := child.CombinedOutput()
	if err == nil || !strings.HasPrefix(string(out), "[C]") || !strings.HasSuffix(string(out), "[D]") {
		t.Fatalf("read error stranded command lifecycle: output=%q error=%v", out, err)
	}
}
