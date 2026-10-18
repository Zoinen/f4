package panel

import (
	"testing"

	"github.com/unxed/f4/internal/sysinfo"
)

func TestDeviceLabel(t *testing.T) {
	cases := []struct {
		dev  sysinfo.DeviceInfo
		want string
	}{
		{sysinfo.DeviceInfo{Class: sysinfo.DeviceSSD, Name: "nvme0n1"}, "SSD (nvme0n1)"},
		{sysinfo.DeviceInfo{Class: sysinfo.DeviceHDD, Name: "sda+sdb"}, "HDD (sda+sdb)"},
	}
	for _, c := range cases {
		if got := deviceLabel(c.dev); got != c.want {
			t.Errorf("deviceLabel(%+v) = %q, want %q", c.dev, got, c.want)
		}
	}
	if got := deviceLabel(sysinfo.DeviceInfo{Class: sysinfo.DeviceUnknown}); got == "" || got == "unknown ()" {
		t.Errorf("an unknown device without a name must render as the bare class word, got %q", got)
	}
}
