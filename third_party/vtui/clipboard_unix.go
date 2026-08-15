//go:build !windows

package vtui

import (
	"os"
	"os/exec"
	"strings"
)

func setOSClipboard(text string) bool {
	if _, err := exec.LookPath("wl-copy"); err == nil && os.Getenv("WAYLAND_DISPLAY") != "" {
		cmd := exec.Command("wl-copy")
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	if _, err := exec.LookPath("xclip"); err == nil && os.Getenv("DISPLAY") != "" {
		cmd := exec.Command("xclip", "-selection", "clipboard", "-in")
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	if _, err := exec.LookPath("xsel"); err == nil && os.Getenv("DISPLAY") != "" {
		cmd := exec.Command("xsel", "--clipboard", "--input")
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	if _, err := exec.LookPath("pbcopy"); err == nil {
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	return false
}

func getOSClipboard() (string, bool) {
	if _, err := exec.LookPath("wl-paste"); err == nil && os.Getenv("WAYLAND_DISPLAY") != "" {
		if out, err := exec.Command("wl-paste", "--no-newline").Output(); err == nil {
			return string(out), true
		}
	}
	if _, err := exec.LookPath("xclip"); err == nil && os.Getenv("DISPLAY") != "" {
		if out, err := exec.Command("xclip", "-selection", "clipboard", "-out").Output(); err == nil {
			return string(out), true
		}
	}
	if _, err := exec.LookPath("xsel"); err == nil && os.Getenv("DISPLAY") != "" {
		if out, err := exec.Command("xsel", "--clipboard", "--output").Output(); err == nil {
			return string(out), true
		}
	}
	if _, err := exec.LookPath("pbpaste"); err == nil {
		if out, err := exec.Command("pbpaste").Output(); err == nil {
			return string(out), true
		}
	}
	return "", false
}
