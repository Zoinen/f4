package panel

import (
	"fmt"
	"time"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/vtui"
)

// Owned by the local information-panel subsystem, not by a disposable view.
var localHostCache = sysinfo.NewHostCache()

// Both presentations consume the same cached snapshot. Probing runs outside
// paint and refreshes at most once per 30 seconds, including display changes.
func (ip *InfoPanel) currentHostInfo() sysinfo.HostInfo {
	if ip.hostCache == nil {
		ip.hostCache = localHostCache
	}
	cached, fresh := ip.hostCache.Snapshot()
	if cached.Model != "" || cached.OS != "" {
		ip.hostSnapshot = cached
	}
	if fresh {
		return cached
	}
	if ip.hostTask == nil && !time.Now().Before(ip.hostRetryAfter) {
		cache := ip.hostCache
		ip.hostTask = vtui.RunAsync(func(ctx *vtui.TaskContext) {
			err := cache.Refresh(ctx.Context)
			info, _ := cache.Snapshot()
			ctx.RunOnUI(func() {
				if ip.hostTask != ctx {
					return
				}
				ip.hostTask = nil
				ip.hostRetryAfter = time.Now().Add(30 * time.Second)
				if err != nil {
					vtui.DebugLog("[FIX:host-info] probe failed: %v", err)
					return
				}
				ip.hostSnapshot = info
				vtui.FrameManager.Redraw()
			})
		})
	}
	return ip.hostSnapshot
}

func (ip *InfoPanel) visitHostInfo(row func(string, string), section func(string), blank func()) {
	info := ip.currentHostInfo()
	fields := []struct{ key, value string }{
		{key: "InfoPanel.ComputerType", value: info.Model},
		{key: "InfoPanel.Chip", value: info.Chip},
		{key: "InfoPanel.OS", value: info.OS},
		{key: "InfoPanel.Architecture", value: info.Architecture},
	}
	for _, field := range fields {
		if field.value != "" {
			row(i18n.Msg(field.key), field.value)
		}
	}
	for i, display := range info.Displays {
		blank()
		section(fmt.Sprintf("%s %d", i18n.Msg("InfoPanel.Display"), i+1))
		if display.Name != "" {
			row(i18n.Msg("InfoPanel.DisplayName"), display.Name)
		}
		if display.Diagonal != "" {
			row(i18n.Msg("InfoPanel.DisplayDiagonal"), display.Diagonal)
		}
		if display.Resolution != "" {
			row(i18n.Msg("InfoPanel.DisplayResolution"), display.Resolution)
		}
	}
}
