package panel

import (
	"fmt"
	"os"
	"strings"

	"github.com/unxed/vtui"
)

// Keep the interactive line (including the managed C/D wrapper) below the
// terminal's input-queue limit even while the shell is changing tty modes.
const localCommandInlineBytes = 256

func prepareLocalCommandEvaluation(cmd string) (string, func(), error) {
	quoted := ShellSingleQuote(cmd)
	if len(quoted) <= localCommandInlineBytes && !strings.ContainsAny(cmd, "\r\n") {
		return quoted, func() {}, nil
	}
	// Reuse the private shell-transport session (0700 directory/0600 files),
	// whose normal shutdown and stale-session sweep also cover an app crash.
	// Store a quoted eval invocation, not raw user text: command substitution
	// strips trailing newlines, which may be meaningful inside the command.
	path, err := writePrivateProcessEnvironmentFile("command-posix-*", []byte("eval "+quoted))
	if err != nil {
		return "", nil, fmt.Errorf("prepare shell command: %w", err)
	}
	cleanup := func() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			vtui.DebugLog("[FIX:command-handoff] cleanup failed: %v", err)
		}
	}
	// Both evals run in the existing shell, so aliases, cwd, variables, $0,
	// positional parameters and tty stdin retain their ordinary semantics.
	// A read failure is visible on stderr and evaluates false; the outer
	// managed wrapper still emits D instead of silently wedging execution.
	arg := `"$(command cat -- ` + ShellSingleQuote(path) + ` || printf false)"`
	if len(arg) > localCommandInlineBytes {
		cleanup()
		return "", nil, fmt.Errorf("private shell command path is too long for the terminal")
	}
	vtui.DebugLog("[FIX:command-handoff] staged %d command bytes; eval argument %d bytes", len(cmd), len(arg))
	return arg, cleanup, nil
}

func (pf *PanelsFrame) clearCommandPayload() {
	if cleanup := pf.commandPayloadCleanup; cleanup != nil {
		pf.commandPayloadCleanup = nil
		cleanup()
	}
}
