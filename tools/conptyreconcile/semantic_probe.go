package main

import (
	"bytes"
	"strings"
)

// passthroughSequence is one escape sequence a terminal program can send that
// the pinned host does not itself interpret as far as we know: it should reach
// the consumer unchanged (unxed/f4#1681, idea 3 -- "does sixel and the kitty
// protocol pass through", which decides whether f4's own picture output works
// under the bundled host). The claim is a prediction from the host's source, so
// the probe measures it instead of trusting it.
type passthroughSequence struct {
	Name string
	Data string
}

var passthroughSequences = []passthroughSequence{
	{"sixel", "\x1bPq\"1;1;6;6#0;2;100;0;0#0!6~\x1b\\"},
	{"kitty-graphics", "\x1b_Ga=T,f=24,s=1,v=1;AAAA\x1b\\"},
	{"iterm2-image", "\x1b]1337;File=inline=1;size=3:AAAA\x07"},
	{"osc-133-prompt", "\x1b]133;A\x07"},
	{"osc-7-cwd", "\x1b]7;file:///C:/pinned\x07"},
	{"osc-8-link", "\x1b]8;;https://example.test\x1b\\x\x1b]8;;\x1b\\"},
	{"kitty-keyboard-query", "\x1b[?u"},
}

// passthroughResults reports, for every sequence, whether it reached raw output
// byte for byte.
func passthroughResults(raw []byte) map[string]bool {
	out := make(map[string]bool, len(passthroughSequences))
	for _, seq := range passthroughSequences {
		out[seq.Name] = bytes.Contains(raw, []byte(seq.Data))
	}
	return out
}

func semanticProbeWorkload(kind, begin, end string) string {
	if kind == "passthrough" {
		var body strings.Builder
		for _, seq := range passthroughSequences {
			body.WriteString(seq.Data)
			body.WriteString("\r\n")
		}
		return "\x1b[?25l" + begin + "\r\n" + body.String() + end + "\r\n\x1b[?25h"
	}
	if kind == "tabs" {
		return "\x1b[?25l" + begin + "\r\ntabs:\tX\tY\r\n" + end + "\r\n\x1b[?25h"
	}
	if kind == "progress" {
		return "\x1b[?25l" + begin + "\r\nprogress: 0%\rprogress: 50%\rprogress: 100%\r\n" + end + "\r\n\x1b[?25h"
	}
	if kind == "unicode" {
		return "\x1b[?25l" + begin + "\r\nunicode: 漢字 e\u0301 ☕️ 😀 👩‍💻 אבג العربية\r\n" + end + "\r\n\x1b[?25h"
	}
	return "\x1b[?25l" + begin + "\r\n\x1b]8;;https://example.test\x1b\\link\x1b]8;;\x1b\\\r\n" + end + "\r\n\x1b[?25h"
}
