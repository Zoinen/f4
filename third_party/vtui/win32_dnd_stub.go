//go:build !windows

package vtui

func win32DoDragDrop(paths []string, allowed DropAction) (DropAction, error) {
	return DropNone, ErrDragUnsupported
}
