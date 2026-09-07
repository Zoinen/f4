//go:build !windows
// +build !windows

package vfs

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/unxed/localecp"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/ianaindex"
)

func platformCodepages() []Codepage {
	return discoverIconvCodepages()
}

func systemCodepageIDs() (int, int) {
	return codepageIDForEncoding(localecp.ANSIEncoding, 1252), codepageIDForEncoding(localecp.OEMEncoding, 437)
}

func systemCodepageNames() (string, string) {
	return fmt.Sprintf("System ANSI (%d)", systemANSI), fmt.Sprintf("System OEM (%d)", systemOEM)
}

func codepageIDForEncoding(enc encoding.Encoding, fallback int) int {
	name, err := ianaindex.IANA.Name(enc)
	if err != nil {
		return fallback
	}
	name = strings.ToLower(name)
	if strings.HasPrefix(name, "windows-") {
		if id, err := strconv.Atoi(strings.TrimPrefix(name, "windows-")); err == nil {
			return id
		}
	}
	if strings.HasPrefix(name, "ibm") {
		if id, err := strconv.Atoi(strings.TrimPrefix(name, "ibm")); err == nil {
			return id
		}
	}
	if strings.HasPrefix(name, "iso-8859-") {
		if id, err := strconv.Atoi(strings.TrimPrefix(name, "iso-8859-")); err == nil {
			return 28590 + id
		}
	}
	switch name {
	case "koi8-r":
		return 20866
	case "koi8-u":
		return 21866
	case "euc-jp":
		return 51932
	case "euc-kr":
		return 51949
	case "gbk":
		return 936
	case "gb18030":
		return 54936
	case "big5":
		return 950
	case "shift_jis":
		return 932
	}
	return fallback
}
