package sheet

import (
	"bufio"
	"bytes"
	"strings"
)

// DetectCSVSeparator guesses the field delimiter of a CSV-like text from its
// first lines: tab, semicolon or comma, whichever splits every sampled line
// into the same number of fields, more than one, and the most of them; ties
// go in that order. A file of one column, or of anything else, keeps the
// comma (nz256, f4#1892: TSV and semicolon files opened as one column).
func DetectCSVSeparator(sample []byte) rune {
	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(sample))
	scanner.Buffer(make([]byte, 0, 64*1024), len(sample)+1)
	for scanner.Scan() && len(lines) < 20 {
		if line := strings.TrimRight(scanner.Text(), "\r"); strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	// The last line of a sample cut in the middle may be partial.
	if len(lines) > 1 && len(sample) > 0 && sample[len(sample)-1] != '\n' {
		lines = lines[:len(lines)-1]
	}
	best, bestFields := ',', 1
	for _, sep := range []rune{'\t', ';', ','} {
		fields := -1
		for _, line := range lines {
			n := csvFieldCount(line, sep)
			if fields < 0 {
				fields = n
			} else if n != fields {
				fields = 0
				break
			}
		}
		if fields > bestFields {
			best, bestFields = sep, fields
		}
	}
	return best
}

// csvFieldCount counts the fields of one line, skipping separators inside
// double quotes.
func csvFieldCount(line string, sep rune) int {
	n, quoted := 1, false
	for _, r := range line {
		switch {
		case r == '"':
			quoted = !quoted
		case r == sep && !quoted:
			n++
		}
	}
	return n
}
