//go:build darwin

package sysinfo

import (
	"context"
	"testing"
)

func TestDarwinHostInformation(t *testing.T) {
	info, err := Host(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Model == "" || info.Chip == "" || info.OS == "" || info.Architecture == "" {
		t.Fatalf("missing general information: %+v", info)
	}
	t.Logf("host: %+v", info)
}

func TestDarwinProfilerInformation(t *testing.T) {
	data := []byte(`{"SPHardwareDataType":[{"machine_name":"MacBook Pro","machine_model":"Mac17,9","chip_type":"Apple M5 Pro"}],"SPSoftwareDataType":[{"os_version":"macOS 27.0.1 (26A434)"}],"SPDisplaysDataType":[{"spdisplays_ndrvs":[{"_name":"Color LCD","spdisplays_display_type":"spdisplays_built-in-liquid-retina-xdr","_spdisplays_displayID":"1","_spdisplays_pixels":"3024 x 1964","_spdisplays_resolution":"1512 x 982 @ 120.00Hz"},{"_name":"External","spdisplays_resolution":"3840 x 2160"}]}]}`)
	info, err := parseDarwinProfiler(data)
	if err != nil {
		t.Fatal(err)
	}
	if info.Chip != "Apple M5 Pro" || info.Model != "MacBook Pro" || len(info.Displays) != 2 {
		t.Fatalf("unexpected host: %+v", info)
	}
	if info.Displays[0].Name != "Built-in Liquid Retina XDR Display" || info.Displays[1].Name != "External" {
		t.Fatalf("wrong display names: %+v", info.Displays)
	}
	if info.Displays[0].Resolution != "3024 x 1964" || info.Displays[1].Resolution != "3840 x 2160" {
		t.Fatalf("wrong display resolution: %+v", info.Displays)
	}
}

func TestDarwinProfilerPartialInformation(t *testing.T) {
	tests := []struct {
		name, data, chip string
		invalid          bool
	}{
		{name: "Intel", data: `{"SPHardwareDataType":[{"machine_name":"iMac","cpu_type":"Intel Core i7"}]}`, chip: "Intel Core i7"},
		{name: "no displays", data: `{}`},
		{name: "invalid JSON", data: `{`, invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := parseDarwinProfiler([]byte(tt.data))
			if (err != nil) != tt.invalid {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.invalid && (info.Chip != tt.chip || info.Displays == nil) {
				t.Fatalf("unexpected partial information: %+v", info)
			}
		})
	}
}

func TestDarwinHostCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Host(ctx); err == nil {
		t.Fatal("canceled collection succeeded")
	}
}
