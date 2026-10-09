//go:build linux || windows || darwin

package proclist

import "unicode/utf16"

// splitWindowsEnvironmentBlock turns the environment block another process
// keeps in its memory -- UTF-16 little-endian strings each ended by a NUL,
// the whole block ended by an empty string -- into one "NAME=value" line per
// variable. The block may be cut short (the size read is a cap, not a
// guarantee), so a last unterminated string is kept as it stands. The hidden
// per-drive entries ("=C:=C:\dir") that cmd.exe keeps are left out, as
// "set" leaves them out.
func splitWindowsEnvironmentBlock(units []uint16) []string {
	var lines []string
	start := 0
	flush := func(end int) bool {
		if end == start {
			return false // the empty string that ends the block
		}
		entry := string(utf16.Decode(units[start:end]))
		if entry != "" && entry[0] != '=' {
			lines = append(lines, entry)
		}
		return true
	}
	for i, u := range units {
		if u != 0 {
			continue
		}
		if !flush(i) {
			return lines
		}
		start = i + 1
	}
	if start < len(units) {
		flush(len(units))
	}
	return lines
}
