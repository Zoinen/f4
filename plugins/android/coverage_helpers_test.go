package androidfs

import (
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestAndroidInfoParsersCoverFallbacksAndMalformedRows(t *testing.T) {
	if got := normalizeDeviceInfoPath("relative/path"); got != "/" {
		t.Fatalf("normalizeDeviceInfoPath(relative) = %q, want /", got)
	}
	if got := normalizeDeviceInfoPath("/a/../b/"); got != "/b" {
		t.Fatalf("normalizeDeviceInfoPath(abs) = %q, want /b", got)
	}
	command := buildDeviceInfoCommand()
	for _, marker := range []string{deviceInfoPropMarker, deviceInfoDFMarker, deviceInfoMemMarker, deviceInfoUpMarker, deviceInfoBatMarker, deviceInfoKernelMark} {
		if !strings.Contains(command, marker) {
			t.Fatalf("device info command misses marker %q: %q", marker, command)
		}
	}

	if got := parseUptime([]string{"bad"}); got != "" {
		t.Fatalf("parseUptime(bad) = %q, want empty", got)
	}
	if got := parseUptime([]string{"3600"}); got != "1h 00m" {
		t.Fatalf("parseUptime(hour) = %q, want 1h 00m", got)
	}
	if got := parseUptime([]string{"172800"}); got != "2d 00h 00m" {
		t.Fatalf("parseUptime(days) = %q, want 2d 00h 00m", got)
	}
	if got := parseBattery([]string{"level: 0", "wireless powered: true", "temperature: -10"}); got != "0% · wireless · -1.0 °C" {
		t.Fatalf("parseBattery(fallbacks) = %q", got)
	}
	if got := parseBattery([]string{"level: 101", "temperature: nope"}); got != "" {
		t.Fatalf("parseBattery(invalid) = %q, want empty", got)
	}

	total, available := parseMemInfo([]string{"MemFree: 10 kB", "Buffers: 20 kB", "Cached: 30 kB", "bad"})
	if total != 0 || available != 60*1024 {
		t.Fatalf("parseMemInfo(fallback) = %d, %d; want 0, %d", total, available, 60*1024)
	}
	totalBytes, availableBytes, mount := parseDF([]string{
		"Filesystem 1K-blocks Used Available Use% Mounted on",
		"broken row",
		"/dev/a nope 1 2% /bad",
		"/dev/b 10 2 7 30% /storage/emulated/0",
	})
	if totalBytes != 10*1024 || availableBytes != 7*1024 || mount != "/storage/emulated/0" {
		t.Fatalf("parseDF = %d, %d, %q", totalBytes, availableBytes, mount)
	}

	props := parseGetprop([]string{"[ro.model]: [Phone]", "ro.brand = Brand", "invalid"})
	if props["ro.model"] != "Phone" || props["ro.brand"] != "Brand" {
		t.Fatalf("parseGetprop = %#v", props)
	}
}

func TestAndroidNamesAndRPCPathHelpers(t *testing.T) {
	if got := DeviceDisplayName(DeviceInfo{Serial: "  s1 ", Model: " Phone ", State: DeviceStateOnline}); got != "Phone (s1)" {
		t.Fatalf("online display name = %q", got)
	}
	if got := DeviceDisplayName(DeviceInfo{Serial: "s2", State: ""}); got != "s2 [unknown]" {
		t.Fatalf("unknown display name = %q", got)
	}
	if got := DeviceDisplayName(DeviceInfo{Serial: "s3", State: "offline"}); got != "s3 [offline]" {
		t.Fatalf("offline display name = %q", got)
	}

	for _, tc := range []struct {
		raw, name, rest string
	}{
		{raw: `\Phone (s1)\Download`, name: "Phone (s1)", rest: "Download"},
		{raw: "/Phone (s1)", name: "Phone (s1)", rest: ""},
		{raw: "/", name: "", rest: ""},
	} {
		name, rest := splitDevicePath(tc.raw)
		if name != tc.name || rest != tc.rest {
			t.Errorf("splitDevicePath(%q) = %q, %q; want %q, %q", tc.raw, name, rest, tc.name, tc.rest)
		}
	}

	item := convertItem(vfs.VFSItem{Name: "file", Size: 7, IsDir: true, IsHidden: true, IsExecutable: true})
	if item.Name != "file" || item.Size != 7 || !item.IsDir || !item.IsHidden || !item.IsExecutable {
		t.Fatalf("convertItem = %#v", item)
	}
}
