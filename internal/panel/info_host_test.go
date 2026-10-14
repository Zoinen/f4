package panel

import (
	"context"
	"testing"
	"time"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/sysinfo"
)

func TestInfoPanelHostCacheSurvivesReopen(t *testing.T) {
	cache := sysinfo.NewHostCache()
	if err := cache.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	want, _ := cache.Snapshot()
	if want.Model == "" {
		t.Skip("host collector unavailable")
	}
	frame, _ := panelsFrameWithMouseSelect(t)
	first := NewInfoPanel(frame.GetActivePanel())
	first.hostCache = cache
	first.currentHostInfo()
	first.Close()
	reopened := NewInfoPanel(frame.GetActivePanel())
	reopened.hostCache = cache
	defer reopened.Close()
	start := time.Now()
	got := reopened.currentHostInfo()
	t.Logf("reopened snapshot read: %s", time.Since(start))
	if got.Model != want.Model || got.Architecture != want.Architecture || reopened.hostTask != nil {
		t.Fatalf("reopened panel did not immediately reuse cache: %+v", got)
	}
}

func TestInfoPanelHostRows(t *testing.T) {
	frame, _ := panelsFrameWithMouseSelect(t)
	ip := NewInfoPanel(frame.GetActivePanel())
	ip.hostCache = sysinfo.NewHostCache()
	ip.hostRetryAfter = time.Now().Add(time.Hour)
	ip.hostSnapshot = sysinfo.HostInfo{
		Model: "MacBook Pro (Mac17,9)", Chip: "Apple M5 Pro",
		OS: "macOS 27.0.1", Architecture: "ARM64 (arm64)",
		Displays: []sysinfo.DisplayInfo{
			{Name: "Color LCD", Resolution: "3024 x 1964", Diagonal: "~14.1\""},
			{Name: "External", Resolution: "3840 x 2160"},
		},
	}
	defer ip.Close()
	model := ip.semanticModel(0, true)
	values := map[string]string{}
	section := ""
	for _, row := range model.Rows {
		if row.Kind == "section" {
			section = row.Label
		}
		if row.Kind == "row" {
			values[section+"|"+row.Label] = row.Value
		}
	}
	checks := []struct{ name, section, key, want string }{
		{name: "model", key: "InfoPanel.ComputerType", want: ip.hostSnapshot.Model},
		{name: "chip", key: "InfoPanel.Chip", want: ip.hostSnapshot.Chip},
		{name: "OS", key: "InfoPanel.OS", want: ip.hostSnapshot.OS},
		{name: "hardware architecture", key: "InfoPanel.Architecture", want: ip.hostSnapshot.Architecture},
		{name: "Retina physical pixels", section: i18n.Msg("InfoPanel.Display") + " 1", key: "InfoPanel.DisplayResolution", want: "3024 x 1964"},
		{name: "diagonal", section: i18n.Msg("InfoPanel.Display") + " 1", key: "InfoPanel.DisplayDiagonal", want: "~14.1\""},
		{name: "external name", section: i18n.Msg("InfoPanel.Display") + " 2", key: "InfoPanel.DisplayName", want: "External"},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if got := values[check.section+"|"+i18n.Msg(check.key)]; got != check.want {
				t.Fatalf("got %q, want %q", got, check.want)
			}
		})
	}
	if _, exists := values[i18n.Msg("InfoPanel.Display")+" 2|"+i18n.Msg("InfoPanel.DisplayDiagonal")]; exists {
		t.Fatal("unknown diagonal must not be invented")
	}
}
