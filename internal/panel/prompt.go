package panel

import (
	"time"

	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

// promptFormatEnabled reports whether this prompt comes from the configured
// format string rather than from BuildPrompt's built-in layout.
//
// A panel showing a virtual filesystem is excluded. Its prompt names the
// provider instead of the user and the host, and there is no local home
// directory to abbreviate against, so $u, $n and $p would all describe a
// machine the panel is not on.
func promptFormatEnabled(vfsTitle string) bool {
	return config.App.UsePromptFormat && vfsTitle == ""
}

// promptFormatSpans renders the configured format for the current panel.
func promptFormatSpans(path, home, username, host string, maxPromptLen int) []cmdline.PromptSpan {
	format := config.App.PromptFormat
	if format == "" {
		format = cmdline.DefaultPromptFormat
	}
	spans := cmdline.ExpandPrompt(format, cmdline.PromptContext{
		Path:       path,
		Home:       home,
		User:       username,
		Host:       host,
		Admin:      cmdline.PromptUserIsAdmin(),
		AdminLabel: cmdline.PromptAdminLabel(),
		Now:        time.Now(),
	})
	return cmdline.FitPrompt(spans, maxPromptLen)
}

// promptSpansToCharInfo paints the spans: the user and host names keep the
// colour bash gives them, everything else follows the theme.
func promptSpansToCharInfo(spans []cmdline.PromptSpan, baseAttr uint64) []vtui.CharInfo {
	identityAttr := vtui.SetRGBFore(baseAttr, 0x8AE234)
	var out []vtui.CharInfo
	for _, s := range spans {
		attr := baseAttr
		if s.Kind == cmdline.PromptIdentity {
			attr = identityAttr
		}
		out = append(out, vtui.StringToCharInfo(s.Text, attr)...)
	}
	return out
}

// promptSpansInactive is the same prompt in the one colour the search-first
// mode uses while the command line does not have focus.
func promptSpansInactive(spans []cmdline.PromptSpan) []vtui.CharInfo {
	return vtui.StringToCharInfo(cmdline.PromptText(spans), vtui.Palette[theme.ColCommandLineInactivePrompt])
}
