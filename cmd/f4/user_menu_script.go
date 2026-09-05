package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/unxed/f4/vfs"
)

// userMenuInterpreter recognizes a standard shebang on the first command line.
// A menu item can therefore carry a complete script while each
// item remains free to choose its own interpreter:
//
//	#!/usr/bin/env bash
//	printf '%s\\n' "hello"
//
// The shebang is metadata for f4 and is not sent to the interpreter as part of
// the script body.
func userMenuInterpreter(commands []string) (interpreter string, scriptStart bool) {
	if len(commands) == 0 {
		return "", false
	}
	line := strings.TrimSpace(commands[0])
	if !strings.HasPrefix(line, "#!") {
		return "", false
	}
	interpreter = strings.TrimSpace(strings.TrimPrefix(line, "#!"))
	return interpreter, interpreter != ""
}

// userMenuCommandDialect selects the shell syntax used by the command window.
// Remote command runners can advertise their dialect; ordinary local panels
// use the native shell. The POSIX path deliberately does not create a local
// temporary file, so the same menu item also works through a remote POSIX PTY.
func userMenuCommandDialect(pf *PanelsFrame) vfs.CommandDialect {
	if pf != nil {
		if fsp := pf.getActivePanel(); fsp != nil {
			if provider, ok := fsp.vfs.(vfs.CommandRunnerInfoProvider); ok {
				if dialect := provider.CommandRunnerInfo().Dialect; dialect != vfs.CommandDialectUnknown {
					return dialect
				}
			}
		}
	}
	if runtime.GOOS == "windows" {
		return vfs.CommandDialectCmd
	}
	return vfs.CommandDialectPOSIX
}

// buildUserMenuScriptCommand turns a shebang-selected script into one command
// line suitable for the existing command-window path. POSIX interpreters read
// the decoded script from stdin; this avoids assuming that /tmp is visible to
// a remote PTY. Windows uses a temporary script file because cmd.exe has no
// portable equivalent of a binary-safe stdin pipeline for arbitrary scripts.
func buildUserMenuScriptCommand(interpreter, script string, dialect vfs.CommandDialect) (string, error) {
	interpreter = strings.TrimSpace(interpreter)
	if interpreter == "" {
		return "", errors.New("script interpreter is empty")
	}
	if strings.IndexByte(script, 0) >= 0 {
		return "", errors.New("script contains NUL")
	}

	switch dialect {
	case vfs.CommandDialectPOSIX:
		quoted, err := QuoteCommandArgument(vfs.CommandDialectPOSIX, script)
		if err != nil {
			return "", err
		}
		// `-` is the conventional stdin script operand for sh, bash, python,
		// perl, ruby and node. The interpreter itself is user-authored, so
		// flags may be placed before the final stdin operand in the shebang.
		return fmt.Sprintf("printf '%%s' %s | %s -", quoted, interpreter), nil
	case vfs.CommandDialectCmd:
		file, err := os.CreateTemp("", "f4-usermenu-*.script")
		if err != nil {
			return "", fmt.Errorf("create temporary script: %w", err)
		}
		path := file.Name()
		cleanup := func() {
			_ = file.Close()
			_ = os.Remove(path)
		}
		if _, err := file.WriteString(strings.ReplaceAll(script, "\n", "\r\n")); err != nil {
			cleanup()
			return "", fmt.Errorf("write temporary script: %w", err)
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(path)
			return "", fmt.Errorf("close temporary script: %w", err)
		}
		quoted, err := QuoteCommandPath(vfs.CommandDialectCmd, path)
		if err != nil {
			_ = os.Remove(path)
			return "", err
		}
		// cmd.exe's '&' is sequential, so the temporary file is removed after
		// the interpreter returns, including when the script exits non-zero.
		return fmt.Sprintf("%s %s & del /Q %s", interpreter, quoted, quoted), nil
	default:
		return "", fmt.Errorf("unsupported command dialect %d", dialect)
	}
}
