//go:build !windows

package gui

// windowsFontFile is a no-op outside Windows: font name resolution there is
// left entirely to vtui.
func windowsFontFile(fontName string) string {
	return ""
}
