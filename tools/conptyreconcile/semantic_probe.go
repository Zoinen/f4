package main

func semanticProbeWorkload(kind, begin, end string) string {
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
