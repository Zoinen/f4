//go:build windows

package vfs

import "testing"

func TestEnumerateWindowsCodepagesListsWithoutOpening(t *testing.T) {
	ids := enumerateWindowsCodepages()
	if len(ids) == 0 {
		t.Fatal("EnumSystemCodePagesW reported nothing")
	}
	seen := make(map[int]struct{}, len(ids))
	for _, cp := range ids {
		if cp <= 0 {
			t.Fatalf("code page id %d", cp)
		}
		if _, dup := seen[cp]; dup {
			t.Fatalf("code page %d listed twice", cp)
		}
		seen[cp] = struct{}{}
	}
}

func TestWindowsCodepageNameFallsBackToNumber(t *testing.T) {
	if got := windowsCodepageName(10007); got != "Mac Cyrillic" {
		t.Fatalf("10007 = %q", got)
	}
	if got := windowsCodepageName(424242); got != "Windows codepage 424242" {
		t.Fatalf("unknown = %q", got)
	}
}

func TestPlatformCodepagesSkipBuiltins(t *testing.T) {
	for _, cp := range platformCodepages() {
		if cp.ID == 1251 || cp.ID == 65001 || cp.ID == systemANSI {
			t.Fatalf("built-in code page %d came back from the platform list", cp.ID)
		}
		if cp.Name == "" {
			t.Fatalf("code page %d has no name", cp.ID)
		}
	}
}
