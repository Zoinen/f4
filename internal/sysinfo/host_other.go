//go:build !darwin

package sysinfo

import "context"

// Host is not yet implemented on other platforms.
func Host(context.Context) (HostInfo, error) { return HostInfo{Displays: []DisplayInfo{}}, nil }
