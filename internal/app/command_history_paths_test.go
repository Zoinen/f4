package app

import (
	runewidth "github.com/mattn/go-runewidth"
	"github.com/unxed/f4/internal/history"
	"github.com/unxed/vtui"
	"strings"
	"testing"
)

func TestCommandHistoryPathsFollowDeduplicatedCommands(t *testing.T) {
	previous := vtui.GlobalHistoryProvider
	provider := stubHistoryProvider{}
	vtui.GlobalHistoryProvider = &provider
	t.Cleanup(func() { vtui.GlobalHistoryProvider = previous })

	commands := []string{"git status", "go test ./..."}
	history.RememberCommandHistoryPath("git status", "/work/first", commands)
	history.RememberCommandHistoryPath("go test ./...", "/work/tests", commands)
	history.RememberCommandHistoryPath("git status", "/work/latest", commands)

	paths := history.LoadCommandHistoryPaths(commands)
	if got, want := paths, []string{"/work/latest", "/work/tests"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("command paths = %v, want %v", got, want)
	}

	history.SaveCommandHistoryPaths(commands[:1], paths[:1])
	if got := history.LoadCommandHistoryPaths(commands); got[1] != "" {
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
