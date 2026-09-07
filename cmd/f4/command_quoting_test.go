package main

import (
	"errors"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestQuoteCommandArgument(t *testing.T) {
	tests := []struct {
		name    string
		dialect vfs.CommandDialect
		value   string
		want    string
	}{
		{name: "POSIX empty", dialect: vfs.CommandDialectPOSIX, value: "", want: "''"},
		{name: "POSIX apostrophe", dialect: vfs.CommandDialectPOSIX, value: "a'b c", want: "'a'\"'\"'b c'"},
		{name: "cmd empty", dialect: vfs.CommandDialectCmd, value: "", want: `""`},
		{name: "cmd spaces", dialect: vfs.CommandDialectCmd, value: `C:\Program Files\f4`, want: `"C:\Program Files\f4"`},
		{name: "cmd trailing slash", dialect: vfs.CommandDialectCmd, value: `C:\tmp\`, want: `"C:\tmp\\"`},
		{name: "cmd percent", dialect: vfs.CommandDialectCmd, value: `%PATH% report`, want: `"%F4_APPLY_LITERAL_PERCENT_8C1E%PATH%F4_APPLY_LITERAL_PERCENT_8C1E% report"`},
		{name: "PowerShell apostrophe", dialect: vfs.CommandDialectPowerShell, value: "a'b c", want: "'a''b c'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := QuoteCommandArgument(tt.dialect, tt.value)
			if err != nil {
				t.Fatalf("QuoteCommandArgument: %v", err)
			}
			if got != tt.want {
				t.Fatalf("QuoteCommandArgument(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestQuoteCommandArgumentRejectsUnknownAndNUL(t *testing.T) {
	if _, err := QuoteCommandArgument(vfs.CommandDialectUnknown, "value"); !errors.Is(err, ErrUnknownCommandDialect) {
		t.Fatalf("unknown dialect error = %v", err)
	}
	if _, err := QuoteCommandArgument(vfs.CommandDialectPOSIX, "a\x00b"); err == nil {
		t.Fatal("NUL argument was accepted")
	}
	if _, err := QuoteCommandArgument(vfs.CommandDialectCmd, `x"&whoami&"`); err == nil {
		t.Fatal("cmd argument containing a shell-closing quote was accepted")
	}
	if _, err := QuoteCommandArgument(vfs.CommandDialectCmd, "safe\r\nwhoami"); err == nil {
		t.Fatal("cmd argument containing a command line break was accepted")
	}
}

func TestWrapCommandInDirectory(t *testing.T) {
	tests := []struct {
		dialect vfs.CommandDialect
		dir     string
		command string
		want    string
	}{
		{vfs.CommandDialectPOSIX, "/tmp/a'b", "echo ok # note", "cd '/tmp/a'\"'\"'b' && (\necho ok # note\n)"},
		{vfs.CommandDialectCmd, `D:\work dir`, "echo ok & REM note", "cd /D \"D:\\work dir\" && (\r\necho ok & REM note\r\n)"},
		{vfs.CommandDialectPowerShell, `C:\it's here`, "Write-Output ok # note", "Set-Location -LiteralPath 'C:\\it''s here' -ErrorAction Stop; & {\nWrite-Output ok # note\n}"},
	}
	for _, tt := range tests {
		got, err := WrapCommandInDirectory(tt.dialect, tt.dir, tt.command)
		if err != nil {
			t.Fatalf("WrapCommandInDirectory: %v", err)
		}
		if got != tt.want {
			t.Fatalf("WrapCommandInDirectory = %q, want %q", got, tt.want)
		}
	}
}

func TestWrapCommandInDirectoryRejectsEmptyCommand(t *testing.T) {
	if _, err := WrapCommandInDirectory(vfs.CommandDialectPOSIX, "/tmp", " \t"); err == nil {
		t.Fatal("empty command was accepted")
	}
}
