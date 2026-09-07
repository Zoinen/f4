package main

// commandHasUnmatchedQuote reports whether command leaves a shell quote open.
// The command line is a single-shot input field, so it cannot provide the
// continuation prompt that an interactive shell would normally use; sending
// such input would make f4 appear to hang while the child shell waits for the
// closing quote.
func commandHasUnmatchedQuote(command string, windowsShell bool) bool {
	quotes := make([]rune, 0, 2)
	escaped := false
	for _, ch := range command {
		if escaped {
			escaped = false
			continue
		}
		var current rune
		if len(quotes) > 0 {
			current = quotes[len(quotes)-1]
		}
		if ch == '^' && windowsShell && current != '\'' {
			escaped = true
			continue
		}
		if ch == '\\' && !windowsShell && current != '\'' {
			escaped = true
			continue
		}

		switch current {
		case '\'':
			if ch == '\'' {
				quotes = quotes[:len(quotes)-1]
			}
		case '"':
			switch ch {
			case '"':
				quotes = quotes[:len(quotes)-1]
			case '`':
				if !windowsShell {
					quotes = append(quotes, ch)
				}
			}
		case '`':
			switch ch {
			case '`':
				quotes = quotes[:len(quotes)-1]
			case '\'', '"':
				quotes = append(quotes, ch)
			}
		default:
			if ch == '"' || (!windowsShell && (ch == '\'' || ch == '`')) {
				quotes = append(quotes, ch)
			}
		}
	}
	return len(quotes) > 0
}
