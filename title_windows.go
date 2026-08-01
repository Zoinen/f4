//go:build windows

package main

import "golang.org/x/sys/windows"

func isAdmin() bool {
	var token windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token)
	if err != nil {
		return false
	}
	defer token.Close()
	return token.IsElevated()
}

func getAdminString() string {
	if isAdmin() {
		return "[Administrator]"
	}
	return ""
}
