//go:build darwin

package sysinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/unix"
)

// Host must be called off the UI thread. system_profiler supplies localized
// model, chip and display names without a hardcoded Mac model database.
func Host(ctx context.Context) (HostInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/sbin/system_profiler",
		"SPHardwareDataType", "SPSoftwareDataType", "SPDisplaysDataType",
		"-json", "-detailLevel", "mini", "-timeout", "4")
	data, err := cmd.Output()
	if err != nil {
		return HostInfo{}, fmt.Errorf("read macOS host information: %w", err)
	}
	info, err := parseDarwinProfiler(data)
	if err != nil {
		return HostInfo{}, err
	}
	info.Architecture = "x86-64"
	if arm, _ := unix.SysctlUint32("hw.optional.arm64"); arm == 1 {
		// This is hardware architecture, even when running under Rosetta.
		info.Architecture = "ARM64"
	}
	if api := loadDarwinDisplayAPI(); api != nil {
		for i := range info.Displays {
			display := &info.Displays[i]
			if display.id == 0 {
				continue
			}
			size := api.screenSize(display.id)
			if size.Width > 0 && size.Height > 0 {
				// Core Graphics can estimate dimensions when EDID is unavailable.
				display.Diagonal = fmt.Sprintf("~%.1f\"", math.Hypot(size.Width, size.Height)/25.4)
			}
		}
	}
	darwinInfoDebug("host model=%q chip=%q os=%q arch=%q displays=%d",
		info.Model, info.Chip, info.OS, info.Architecture, len(info.Displays))
	return info, nil
}

func parseDarwinProfiler(data []byte) (HostInfo, error) {
	var report struct {
		Hardware []struct {
			Name string `json:"machine_name"`
			Chip string `json:"chip_type"`
			CPU  string `json:"cpu_type"`
		} `json:"SPHardwareDataType"`
		Software []struct {
			OS string `json:"os_version"`
		} `json:"SPSoftwareDataType"`
		GPUs []struct {
			Displays []struct {
				Name             string `json:"_name"`
				Type             string `json:"spdisplays_display_type"`
				ID               string `json:"_spdisplays_displayID"`
				Pixels           string `json:"_spdisplays_pixels"`
				Resolution       string `json:"spdisplays_resolution"`
				LegacyResolution string `json:"_spdisplays_resolution"`
			} `json:"spdisplays_ndrvs"`
		} `json:"SPDisplaysDataType"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return HostInfo{}, fmt.Errorf("decode macOS host information: %w", err)
	}
	info := HostInfo{Displays: []DisplayInfo{}}
	if len(report.Hardware) > 0 {
		hw := report.Hardware[0]
		info.Model, info.Chip = hw.Name, hw.Chip
		if info.Chip == "" {
			info.Chip = hw.CPU
		}
	}
	if len(report.Software) > 0 {
		info.OS = report.Software[0].OS
	}
	for _, gpu := range report.GPUs {
		for _, d := range gpu.Displays {
			resolution := d.Pixels
			if resolution == "" {
				resolution = d.Resolution
			}
			if resolution == "" {
				resolution = d.LegacyResolution
			}
			id, _ := strconv.ParseUint(strings.TrimSpace(d.ID), 10, 32)
			info.Displays = append(info.Displays, DisplayInfo{
				Name: darwinDisplayName(d.Name, d.Type), Resolution: resolution, id: uint32(id),
			})
		}
	}
	return info, nil
}

// system_profiler supplies the descriptive display type separately from its
// legacy Color LCD name. Preserve user/product names for external displays.
func darwinDisplayName(name, displayType string) string {
	switch displayType {
	case "spdisplays_built-in-liquid-retina-xdr":
		return "Built-in Liquid Retina XDR Display"
	case "spdisplays_built-in-liquid-retina":
		return "Built-in Liquid Retina Display"
	case "spdisplays_built-in-retina":
		return "Built-in Retina Display"
	default:
		return name
	}
}

type darwinScreenSize struct{ Width, Height float64 }
type darwinDisplayAPI struct{ screenSize func(uint32) darwinScreenSize }

var loadDarwinDisplayAPI = sync.OnceValue(func() *darwinDisplayAPI {
	handle, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if err != nil {
		darwinInfoDebug("CoreGraphics unavailable: %v", err)
		return nil
	}
	address, err := purego.Dlsym(handle, "CGDisplayScreenSize")
	if err != nil {
		_ = purego.Dlclose(handle)
		darwinInfoDebug("display size unavailable: %v", err)
		return nil
	}
	api := &darwinDisplayAPI{}
	purego.RegisterFunc(&api.screenSize, address)
	return api
})
