package theme

import (
	"github.com/unxed/f4/vfs"
)

func SemanticFileIconColor(fileName string) string {
	if fileName == "" || GlobalFileHighlighter == nil {
		return ""
	}
	_, style := GlobalFileHighlighter.SemanticStyle(
		&vfs.VFSItem{Name: fileName}, false)
	return style.Normal.Foreground
}
