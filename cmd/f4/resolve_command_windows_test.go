//go:build windows

package main

import "testing"

func TestFindCmdTokenRejectsUnmatchedQuote(t *testing.T) {
	for _, command := range []string{`"`, `"program`} {
		if _, _, _, ok := findCmdToken(command); ok {
			t.Fatalf("findCmdToken(%q) accepted an unmatched quote", command)
		}
	}
	if _, _, name, ok := findCmdToken(`'`); !ok || name != `'` {
		t.Fatalf("findCmdToken did not treat a Windows single quote as a literal token: name=%q ok=%v", name, ok)
	}
}
