package app

import (
	"testing"
	"time"

	"github.com/unxed/f4/internal/vtvibe"
)

func TestParseBotArgs(t *testing.T) {
	for _, tc := range []struct {
		in     string
		source string
		pause  time.Duration
	}{
		{"https://example.org/LUNOBOT.md 30m", "https://example.org/LUNOBOT.md", 30 * time.Minute},
		{"/home/me/bot.md", "/home/me/bot.md", vtvibe.DefaultBotPause},
		{"/home/me/my bot.md 1h", "/home/me/my bot.md", time.Hour},
		{"/home/me/my bot.md", "/home/me/my bot.md", vtvibe.DefaultBotPause},
	} {
		source, pause := parseBotArgs(tc.in)
		if source != tc.source || pause != tc.pause {
			t.Errorf("parseBotArgs(%q) = %q, %s", tc.in, source, pause)
		}
	}
}
