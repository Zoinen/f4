package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/unxed/f4/vfs"
)

// ErrUnknownCommandDialect reports that a runner did not identify the shell
// syntax needed to quote a value safely.
var ErrUnknownCommandDialect = errors.New("command dialect is unknown")

// QuoteCommandArgument returns one conventionally quoted shell word for value
// in the requested command language. Command dialects are properties of
// runners, not of the machine displaying the panel, which matters for remote
// filesystems. Cmd runners receive literal percent signs through
// vfs.CommandLiteralPercentEnv so percent-delimited text in a generated
// filename is not reinterpreted.
func QuoteCommandArgument(dialect vfs.CommandDialect, value string) (string, error) {
	if strings.IndexByte(value, 0) >= 0 {
		return "", errors.New("command argument contains NUL")
	}

	switch dialect {
	case vfs.CommandDialectPOSIX:
		return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'", nil
	case vfs.CommandDialectCmd:
		// cmd.exe does not recognize the C-runtime \" convention as shell
		// escaping. Win32 filenames cannot contain a quote; reject arbitrary
		// provider values that could otherwise terminate this shell word.
		if strings.ContainsAny(value, "\"\r\n") {
			return "", errors.New("cmd argument contains a double quote or line break")
		}
		value = strings.ReplaceAll(value, "%", "%"+vfs.CommandLiteralPercentEnv+"%")
		return quoteCmdArgument(value), nil
	case vfs.CommandDialectPowerShell:
		return "'" + strings.ReplaceAll(value, "'", "''") + "'", nil
	default:
		return "", ErrUnknownCommandDialect
	}
}

// QuoteCommandPath is the path-named counterpart to QuoteCommandArgument. It
// intentionally uses the same quoting rules: neither POSIX shells nor
// PowerShell give path strings a distinct lexical form, and cmd paths are
// ordinary command arguments too.
func QuoteCommandPath(dialect vfs.CommandDialect, value string) (string, error) {
	return QuoteCommandArgument(dialect, value)
}

// WrapCommandInDirectory makes command execute in dir without changing the
// caller's process directory. The command text itself is deliberately left
// untouched; it is user-authored shell syntax, not an argument to quote.
func WrapCommandInDirectory(dialect vfs.CommandDialect, dir, command string) (string, error) {
	quoted, err := QuoteCommandPath(dialect, dir)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(command) == "" {
		return "", errors.New("command is empty")
	}

	switch dialect {
	case vfs.CommandDialectPOSIX:
		return "cd " + quoted + " && (\n" + command + "\n)", nil
	case vfs.CommandDialectCmd:
		return "cd /D " + quoted + " && (\r\n" + command + "\r\n)", nil
	case vfs.CommandDialectPowerShell:
		return "Set-Location -LiteralPath " + quoted + " -ErrorAction Stop; & {\n" + command + "\n}", nil
	default:
		return "", fmt.Errorf("%w: %d", ErrUnknownCommandDialect, dialect)
	}
}

// quoteCmdArgument follows the quoting accepted by the Microsoft C runtime,
// which is also the convention understood by the overwhelming majority of
// native commands launched through cmd.exe. Wrapping every value keeps cmd
// metacharacters such as &, |, <, >, and parentheses inert. Delayed expansion
// is disabled by the LocalCommandRunner, so exclamation marks remain literal.
func quoteCmdArgument(value string) string {
	var b strings.Builder
	b.Grow(len(value) + 2)
	b.WriteByte('"')

	backslashes := 0
	for _, r := range value {
		switch r {
		case '\\':
			backslashes++
		case '"':
			b.WriteString(strings.Repeat("\\", backslashes*2+1))
			b.WriteRune(r)
			backslashes = 0
		default:
			b.WriteString(strings.Repeat("\\", backslashes))
			backslashes = 0
			b.WriteRune(r)
		}
	}
	b.WriteString(strings.Repeat("\\", backslashes*2))
	b.WriteByte('"')
	return b.String()
}
