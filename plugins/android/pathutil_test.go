package androidfs

import "testing"

func TestQuoteShellArg(t *testing.T) {
	for input, want := range map[string]string{
		"":                              "''",
		"plain":                         "'plain'",
		"white space":                   "'white space'",
		"a'b":                           "'a'\"'\"'b'",
		"$(touch /data/local/injected)": "'$(touch /data/local/injected)'",
	} {
		if got := quoteShellArg(input); got != want {
			t.Errorf("quoteShellArg(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMutationPath(t *testing.T) {
	for _, bad := range []string{"", ".", "relative", "/", "//", "/a/../b", "/a/..", "/a\x00b"} {
		if got, err := mutationPath(bad); err == nil {
			t.Errorf("mutationPath(%q) = %q, want error", bad, got)
		}
	}
	if got, err := mutationPath("/data/local/tmp/a/./b"); err != nil || got != "/data/local/tmp/a/b" {
		t.Fatalf("mutationPath(valid) = %q, %v", got, err)
	}
}
