package terminal

import (
	"context"
	"os"
	"strings"
)

// RunLocalCommandCapture returns the merged stdout/stderr of one local shell
// command. The Wine/POSIX implementation is selected by the same personality
// bit as the file VFS and terminal PTY; it must not silently become cmd.exe.
func RunLocalCommandCapture(ctx context.Context, dir, command string) ([]byte, error) {
	if handled, output, err := runNativeLocalCommandCapture(ctx, dir, command); handled {
		return output, err
	}

	cmd := newLocalShellCommandContext(ctx, command)
	cmd.Env = localCommandEnvironment(os.Environ())
	cmd.Stdin = strings.NewReader("")
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.CombinedOutput()
}
