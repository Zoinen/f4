package main

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtui"
)

func TestCommandHistoryPathsFollowDeduplicatedCommands(t *testing.T) {
	previous := vtui.GlobalHistoryProvider
	provider := stubHistoryProvider{}
	vtui.GlobalHistoryProvider = &provider
	t.Cleanup(func() { vtui.GlobalHistoryProvider = previous })

	commands := []string{"git status", "go test ./..."}
	rememberCommandHistoryPath("git status", "/work/first", commands)
	rememberCommandHistoryPath("go test ./...", "/work/tests", commands)
	rememberCommandHistoryPath("git status", "/work/latest", commands)

	paths := loadCommandHistoryPaths(commands)
	if got, want := paths, []string{"/work/latest", "/work/tests"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("command paths = %v, want %v", got, want)
	}

	saveCommandHistoryPaths(commands[:1], paths[:1])
	if got := loadCommandHistoryPaths(commands); got[1] != "" {
		t.Fatalf("deleted command retained path %q", got[1])
	}
}

func TestTruncateHistoryPathKeepsBothEnds(t *testing.T) {
	path := `C:\Users\designer\projects\f4\sources\panels`
	got := truncateHistoryPath(path, 24)
	if runewidth.StringWidth(got) > 24 {
		t.Fatalf("truncated path width = %d, want <= 24: %q", runewidth.StringWidth(got), got)
	}
	if !strings.HasPrefix(got, `C:\Users`) || !strings.HasSuffix(got, `\panels`) || !strings.Contains(got, "...") {
		t.Fatalf("middle-elided path did not preserve both ends: %q", got)
	}
}
