package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/unxed/f4/vfs"
)

// LocalCommandRunner adapts the platform's non-interactive command shell to
// vfs.CommandRunner. It is intentionally separate from OSVFS and is selected
// only for local OS panels; unsupported remote VFSes must never fall back to it.
type LocalCommandRunner struct{}

// cmd expands percent-delimited variables even inside double quotes. Generated
// Apply arguments spell a literal percent through this variable; cmd performs
// variable expansion only once, so a resulting filename such as %PATH% is not
// interpreted a second time.
const applyCommandLiteralPercentEnv = vfs.CommandLiteralPercentEnv

func NewLocalCommandRunner() *LocalCommandRunner { return &LocalCommandRunner{} }

func (*LocalCommandRunner) CommandRunnerInfo() vfs.CommandRunnerInfo {
	return vfs.CommandRunnerInfo{Dialect: localCommandDialect(), MaxParallel: 0}
}

func (*LocalCommandRunner) RunCommand(ctx context.Context, dir, command string, cb func(line string)) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if strings.TrimSpace(command) == "" {
		return 0, errors.New("local command is empty")
	}

	cmd := newLocalShellCommand(command)
	cmd.Env = localCommandEnvironment(os.Environ())
	if dir != "" {
		cmd.Dir = dir
	}
	// A command applied to panel items is non-interactive. An empty reader gives
	// children immediate EOF without keeping an inherited terminal descriptor.
	cmd.Stdin = strings.NewReader("")
	lines := newCommandLineWriter(cb)
	cmd.Stdout = lines
	cmd.Stderr = lines
	configureLocalProcessTree(cmd)

	if err := cmd.Start(); err != nil {
		return 0, err
	}
	tree := attachLocalProcessTree(cmd)
	defer tree.Close()

	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()

	select {
	case err := <-wait:
		lines.Flush()
		return localCommandExitStatus(err)
	case <-ctx.Done():
		_ = tree.Kill()
		<-wait
		lines.Flush()
		return 0, ctx.Err()
	}
}

func commandEnvironmentWith(environment []string, key, value string) []string {
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, key) {
			continue
		}
		result = append(result, entry)
	}
	return append(result, key+"="+value)
}

func localCommandExitStatus(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 0, err
}

// commandLineWriter is shared by stdout and stderr. Assigning the same writer
// preserves their observed write order, and the mutex also makes callbacks
// serialized when a transport happens to write both streams concurrently.
type commandLineWriter struct {
	mu      sync.Mutex
	pending []byte
	cb      func(string)
}

const commandOutputChunkBytes = 64 << 10

func newCommandLineWriter(cb func(string)) *commandLineWriter {
	return &commandLineWriter{cb: cb}
}

func (w *commandLineWriter) Write(p []byte) (int, error) {
	n := len(p)
	if n == 0 || w.cb == nil {
		return n, nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, p...)
	for {
		i := bytes.IndexByte(w.pending, '\n')
		if i < 0 && len(w.pending) <= commandOutputChunkBytes {
			break
		}
		end, consumed := i, i+1
		if i < 0 || i > commandOutputChunkBytes {
			end = commandOutputChunkEnd(w.pending, commandOutputChunkBytes)
			consumed = end
		}
		line := w.pending[:end]
		if len(line) != 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		w.cb(normalizeCommandOutput(line))
		w.pending = w.pending[consumed:]
	}
	return n, nil
}

func commandOutputChunkEnd(data []byte, limit int) int {
	if len(data) <= limit {
		return len(data)
	}
	end := limit
	for end > 0 && end < len(data) && !utf8.RuneStart(data[end]) {
		end--
	}
	if end == 0 {
		return limit
	}
	return end
}

func (w *commandLineWriter) Flush() {
	if w.cb == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) == 0 {
		return
	}
	line := w.pending
	if line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	w.cb(normalizeCommandOutput(line))
	w.pending = nil
}

type localProcessTree interface {
	Kill() error
	Close() error
}

var (
	_ io.Writer                     = (*commandLineWriter)(nil)
	_ vfs.CommandRunner             = (*LocalCommandRunner)(nil)
	_ vfs.CommandRunnerInfoProvider = (*LocalCommandRunner)(nil)
)
