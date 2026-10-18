//go:build !windows

package app

import "errors"

// showExplorerMenu has nothing to show outside Windows; the action is not
// offered there.
func showExplorerMenu(string) error {
	return errors.New("the Explorer context menu exists only on Windows")
}
