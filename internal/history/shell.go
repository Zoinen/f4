package history

import (
	"bytes"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ShellKind names a shell whose on-disk history file f4 knows how to read.
// Scope for this first slice (f4#1505) is POSIX bash/zsh only: cmd.exe and
// PowerShell have no comparable plain-text history file to read (cmd.exe
// keeps none at all; PowerShell's is a different enough format and location
// that it is deliberately left for a follow-up), and reverse export (f4 ->
// shell) is explicitly optional in the ticket and not attempted here.
type ShellKind string

const (
	ShellBash ShellKind = "bash"
	ShellZsh  ShellKind = "zsh"
)

// DefaultShellHistoryFileName is the conventional history file name under
// $HOME for a shell that does not set (or whose environment f4 cannot see)
// $HISTFILE.
func defaultShellHistoryFileName(shell ShellKind) string {
	if shell == ShellZsh {
		return ".zsh_history"
	}
	return ".bash_history"
}

// ShellHistoryPath answers the file ImportShellHistory should read: histfile
// (the shell's own $HISTFILE, when set -- both bash and zsh honor it) or,
// failing that, the shell's conventional default location under home. This
// mirrors install.DetectShellProfile's existing $SHELL-based pattern: the
// caller reads the environment (os.Getenv("HISTFILE"), hostmode.UserHomeDir())
// and passes plain strings in, keeping this package free of an os.Getenv of
// its own and easy to unit test.
func ShellHistoryPath(shell ShellKind, histfile, home string) string {
	if strings.TrimSpace(histfile) != "" {
		return histfile
	}
	return filepath.Join(home, defaultShellHistoryFileName(shell))
}

// isCleanHistoryLine reports whether line is plausible command text rather
// than binary garbage: valid UTF-8 and free of C0 control bytes other than
// tab. Real-world shell history files sometimes contain a stray line like
// this -- typically from a paste that hit the terminal mid-escape-sequence
// or in the wrong encoding -- and the importer skips exactly that line
// rather than failing the whole import or corrupting later entries.
func isCleanHistoryLine(line []byte) bool {
	if !utf8.Valid(line) {
		return false
	}
	for _, b := range line {
		if b < 0x20 && b != '\t' {
			return false
		}
	}
	return true
}

// ParseBashHistory parses the plain format bash writes to $HISTFILE /
// ~/.bash_history: one command per line, oldest first. When HISTTIMEFORMAT
// was set at some point, bash also writes a "#<unix-seconds>" comment line
// immediately before the command it timestamps; that form is recognized too
// and turned into HistoryRecord.Timestamp, but its absence (the common case)
// is not an error -- the record is simply left with a zero Timestamp, the
// same as any other history entry that has never had one.
func ParseBashHistory(data []byte) []HistoryRecord {
	var records []HistoryRecord
	var pendingTS time.Time
	havePending := false
	for _, raw := range bytes.Split(data, []byte("\n")) {
		line := bytes.TrimRight(raw, "\r")
		if !isCleanHistoryLine(line) {
			havePending = false
			continue
		}
		text := strings.TrimSpace(string(line))
		if text == "" {
			havePending = false
			continue
		}
		if strings.HasPrefix(text, "#") {
			if sec, err := strconv.ParseInt(text[1:], 10, 64); err == nil {
				pendingTS = time.Unix(sec, 0)
				havePending = true
				continue
			}
			// Starts with '#' but is not a bash timestamp comment (e.g. an
			// actual command that happens to start with '#'): fall through
			// and record it as a normal line.
		}
		rec := HistoryRecord{Name: text}
		if havePending {
			rec.Timestamp = pendingTS
		}
		havePending = false
		records = append(records, rec)
	}
	return records
}

// zshExtendedPrefix splits a zsh EXTENDED_HISTORY line, ": <begin>:<elapsed>;
// <command>", into its timestamp and command text. ok is false for a plain
// (non-extended) line, in which case text is the input unchanged.
func zshExtendedPrefix(text string) (ts time.Time, command string, ok bool) {
	if !strings.HasPrefix(text, ": ") {
		return time.Time{}, text, false
	}
	rest := text[2:]
	colon := strings.IndexByte(rest, ':')
	if colon < 0 {
		return time.Time{}, text, false
	}
	beginPart, rest := rest[:colon], rest[colon+1:]
	semi := strings.IndexByte(rest, ';')
	if semi < 0 {
		return time.Time{}, text, false
	}
	elapsedPart, command := rest[:semi], rest[semi+1:]
	begin, err := strconv.ParseInt(beginPart, 10, 64)
	if err != nil {
		return time.Time{}, text, false
	}
	if _, err := strconv.ParseInt(elapsedPart, 10, 64); err != nil {
		return time.Time{}, text, false
	}
	return time.Unix(begin, 0), command, true
}

// ParseZshHistory parses zsh's history file, ~/.zsh_history by default. Zsh
// writes either a plain line (one command per line, like bash) or, when
// EXTENDED_HISTORY is set, a line prefixed with ": <begin>:<elapsed>;" giving
// the command's start time and duration; ParseZshHistory recognizes both,
// stripping the metadata prefix rather than importing it as command text and
// filling in HistoryRecord.Timestamp whenever it is present.
//
// A command zsh itself split across several physical lines (each continued
// line ending in a literal backslash) is read back as separate entries
// rather than being rejoined -- reconstructing that is not attempted in this
// first slice.
func ParseZshHistory(data []byte) []HistoryRecord {
	var records []HistoryRecord
	for _, raw := range bytes.Split(data, []byte("\n")) {
		line := bytes.TrimRight(raw, "\r")
		if !isCleanHistoryLine(line) {
			continue
		}
		text := string(line)
		if strings.TrimSpace(text) == "" {
			continue
		}
		rec := HistoryRecord{Name: text}
		if ts, command, ok := zshExtendedPrefix(text); ok {
			rec.Timestamp = ts
			rec.Name = command
		}
		rec.Name = strings.TrimSpace(rec.Name)
		if rec.Name == "" {
			continue
		}
		records = append(records, rec)
	}
	return records
}

// ParseShellHistory dispatches to ParseBashHistory or ParseZshHistory by
// shell. Any other ShellKind value parses as bash, the more forgiving of the
// two formats (plain lines with no required prefix to strip).
func ParseShellHistory(shell ShellKind, data []byte) []HistoryRecord {
	if shell == ShellZsh {
		return ParseZshHistory(data)
	}
	return ParseBashHistory(data)
}

// MergeShellHistoryRecords merges imported records into current, f4's
// existing "cmdline" rich history, keeping current's entries (and their
// order) first and appending only the imported records whose command text
// does not already appear -- word for word -- somewhere in current. Shell
// history commonly repeats the same command many times over, and importing
// every repetition would just crowd out real f4 history with noise.
//
// The merged result is capped to limit the same way importing far2l history
// already is (actionImportFar2lHistory): a limit of 0 or less leaves it
// uncapped. added is the number of records actually appended, for a status
// toast.
func MergeShellHistoryRecords(current, imported []HistoryRecord, limit int) (merged []HistoryRecord, added int) {
	seen := make(map[string]bool, len(current)+len(imported))
	merged = make([]HistoryRecord, 0, len(current)+len(imported))
	for _, rec := range current {
		if seen[rec.Name] {
			continue
		}
		seen[rec.Name] = true
		merged = append(merged, rec)
	}
	for _, rec := range imported {
		if rec.Name == "" || seen[rec.Name] {
			continue
		}
		seen[rec.Name] = true
		merged = append(merged, rec)
		added++
	}
	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}
	return merged, added
}
