//go:build !windows

package vfs

import "context"

func readCompleteOSDirectoryBase(context.Context, string) ([]VFSItem, bool, error) {
	return nil, false, nil
}

func readCompleteOSDirectoryBasePhased(
	context.Context, string, func([]VFSItem),
) ([]VFSItem, bool, error) {
	return nil, false, nil
}
