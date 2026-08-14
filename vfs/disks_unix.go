//go:build !windows

package vfs

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func resolveDevicePath(name string) string {
	if !strings.HasPrefix(name, "/dev/") {
		return "/dev/" + name
	}
	return name
}

func getPlatformBlockDevices(ctx context.Context) []VFSItem {
	var items []VFSItem

	// Linux /sys/class/block enumeration (world-readable)
	entries, err := os.ReadDir("/sys/class/block")
	if err == nil {
		for _, e := range entries {
			if ctx.Err() != nil {
				break
			}
			name := e.Name()

			// Read sector size from sysfs
			sizeData, err := os.ReadFile("/sys/class/block/" + name + "/size")
			if err != nil {
				continue
			}
			var sectors int64
			fmt.Sscanf(strings.TrimSpace(string(sizeData)), "%d", &sectors)
			if sectors <= 0 {
				continue
			}
			size := sectors * 512

			displayName := name
			if dmName, err := os.ReadFile("/sys/class/block/" + name + "/dm/name"); err == nil {
				trimmed := strings.TrimSpace(string(dmName))
				if trimmed != "" {
					displayName = "mapper/" + trimmed
				}
			}

			items = append(items, VFSItem{
				Name:      displayName,
				Size:      size,
				SizeKnown: true,
				MTime:     time.Now(),
			})
		}
		if len(items) > 0 {
			return items
		}
	}

	// Fallback for macOS / BSDs: scan /dev
	devEntries, err := os.ReadDir("/dev")
	if err == nil {
		for _, e := range devEntries {
			if ctx.Err() != nil {
				break
			}
			info, err := e.Info()
			if err == nil && (info.Mode()&os.ModeDevice != 0) {
				items = append(items, VFSItem{
					Name:      e.Name(),
					Size:      getDeviceSize("/dev/"+e.Name(), nil),
					SizeKnown: true,
					MTime:     info.ModTime(),
				})
			}
		}
	}
	return items
}

func getDeviceSize(devPath string, f *os.File) int64 {
	if f != nil {
		if pos, err := f.Seek(0, io.SeekEnd); err == nil && pos > 0 {
			f.Seek(0, io.SeekStart)
			return pos
		}
	}
	// Try sysfs lookup if devPath is /dev/xxx
	sysName := strings.TrimPrefix(devPath, "/dev/")
	sysName = strings.TrimPrefix(sysName, "mapper/")
	if sizeData, err := os.ReadFile("/sys/class/block/" + sysName + "/size"); err == nil {
		var sectors int64
		fmt.Sscanf(strings.TrimSpace(string(sizeData)), "%d", &sectors)
		if sectors > 0 {
			return sectors * 512
		}
	}
	// Direct open seek fallback
	if f == nil {
		if localF, err := os.Open(devPath); err == nil {
			defer localF.Close()
			if pos, err := localF.Seek(0, io.SeekEnd); err == nil && pos > 0 {
				return pos
			}
		}
	}
	return 0
}
