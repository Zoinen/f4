//go:build !windows

package svcmgr

// listServices has nothing to ask off Windows.
func listServices(string) ([]service, error) { return nil, errUnsupported }
