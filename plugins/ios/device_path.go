package iosfs

import (
	"strings"

	"github.com/unxed/f4/vfs"
)

func iosPaths(device DeviceInfo, title string) vfs.DevicePath {
	_, domain, _ := strings.Cut(title, ":/")
	name := device.pathName
	if name == "" {
		name = deviceLabel(device)
	}
	return vfs.DevicePath{Scheme: "ios", Device: name, Domain: strings.Trim(domain, "/")}
}
