package terminal

import "strings"

// pbcopyLocale is the locale pbcopy and pbpaste run with on macOS. They take
// the encoding of the text from the locale of their environment, and with no
// UTF-8 locale there every letter outside ASCII comes out as "?" (f4#2057:
// f4 started from a terminal whose shell sets no LANG). en_US.UTF-8 is
// installed on every macOS.
const pbcopyLocale = "en_US.UTF-8"

// utf8LocaleEnv is env with every locale variable (LANG, LC_ALL, LC_CTYPE,
// LC_MESSAGES...) removed and LANG set to pbcopyLocale. One left behind that
// is not UTF-8 is enough for "?" (sogonov measured LANG=C LC_ALL=en_US.UTF-8
// and LANG=en_US.UTF-8 LC_COLLATE=C, both "???"); the other variables stay.
func utf8LocaleEnv(env []string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if name == "LANG" || strings.HasPrefix(name, "LC_") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "LANG="+pbcopyLocale)
}
