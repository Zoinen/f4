package panel

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

func TestPromptFormattingOptions(t *testing.T) {
	old := config.App
	t.Cleanup(func() { config.App = old })

	config.App.UsePromptFormat = false
	if promptFormatEnabled("") {
		t.Fatal("prompt format enabled while the setting is off")
	}
	config.App.UsePromptFormat = true
	if promptFormatEnabled("virtual") {
		t.Fatal("virtual filesystem prompt must use its provider title")
	}
	if !promptFormatEnabled("") {
		t.Fatal("configured local prompt format was not enabled")
	}

	config.App.PromptFormat = "$u@$n:$p "
	spans := promptFormatSpans("/home/alice/src", "/home/alice", "alice", "workstation", 80)
	if got := cmdline.PromptText(spans); got != "alice@workstation:~/src " {
		t.Fatalf("configured prompt = %q", got)
	}
	config.App.PromptFormat = ""
	spans = promptFormatSpans("/home/alice/src", "/home/alice", "alice", "workstation", 80)
	if got := cmdline.PromptText(spans); !strings.Contains(got, "alice@workstation:") {
		t.Fatalf("default prompt = %q", got)
	}
}

func TestPromptSpanRendering(t *testing.T) {
	spans := []cmdline.PromptSpan{
		{Text: "user", Kind: cmdline.PromptIdentity},
		{Text: "@host:~/src$ ", Kind: cmdline.PromptLiteral},
	}
	base := vtui.SetRGBBoth(0, 0x112233, 0x445566)
	got := promptSpansToCharInfo(spans, base)
	if len(got) != len("user@host:~/src$ ") {
		t.Fatalf("rendered %d cells, want %d", len(got), len("user@host:~/src$ "))
	}
	if vtui.GetRGBFore(got[0].Attributes) != 0x8AE234 {
		t.Fatalf("identity foreground = %#x", vtui.GetRGBFore(got[0].Attributes))
	}
	if vtui.GetRGBFore(got[len("user")].Attributes) != 0x112233 {
		t.Fatalf("literal foreground = %#x", vtui.GetRGBFore(got[len("user")].Attributes))
	}

	inactive := promptSpansInactive(spans)
	if inactive[0].Char != uint64('u') {
		t.Fatalf("inactive first rune = %#x", inactive[0].Char)
	}
	if len(inactive) != len(got) {
		t.Fatalf("inactive rendered %d cells, want %d", len(inactive), len(got))
	}
}

// The path of the prompt is painted with CommandLine.Path (f4#234), the
// separators around it with the prompt colour.
func TestPromptSpanRenderingPathHasItsOwnColour(t *testing.T) {
	old := vtui.Palette[theme.ColCommandLinePath]
	t.Cleanup(func() { vtui.Palette[theme.ColCommandLinePath] = old })
	vtui.Palette[theme.ColCommandLinePath] = vtui.SetRGBBoth(0, 0x00FFFF, 0x000000)

	spans := []cmdline.PromptSpan{
		{Text: "/src", Kind: cmdline.PromptPath},
		{Text: "$ ", Kind: cmdline.PromptLiteral},
	}
	base := vtui.SetRGBBoth(0, 0x112233, 0x445566)
	got := promptSpansToCharInfo(spans, base)
	if vtui.GetRGBFore(got[0].Attributes) != 0x00FFFF {
		t.Fatalf("path foreground = %#x, want CommandLine.Path", vtui.GetRGBFore(got[0].Attributes))
	}
	if vtui.GetRGBFore(got[len("/src")].Attributes) != 0x112233 {
		t.Fatalf("literal foreground = %#x, want the prompt colour", vtui.GetRGBFore(got[len("/src")].Attributes))
	}
}
