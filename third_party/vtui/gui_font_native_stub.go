//go:build !windows

package vtui

import "golang.org/x/image/font"

func wrapGUIFace(_ string, face font.Face, _, _ float64) font.Face {
	return face
}
