package panel

import "github.com/unxed/f4/internal/winshell"

func openAssociatedPathOS(path, dir string) error {
	return winshell.OpenAssociatedPath(path, dir)
}

func AssociatedFileCommand(path string) (string, []string, bool) {
	return "rundll32", []string{"url.dll,FileProtocolHandler", path}, true
}
