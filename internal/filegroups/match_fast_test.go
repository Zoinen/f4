package filegroups

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestSimpleGlobMatchesRegexpSemantics(t *testing.T) {
	for _, pattern := range []string{"*", "*.go", "main*", "a*b", "main.go", "*.tar.gz", "файл*Я", "a?b", "a[bc]", `a\*b`, "a/b*"} {
		for _, ignore := range []bool{false, true} {
			masks, err := compileMasks(pattern, ignore, false)
			if err != nil {
				t.Fatal(err)
			}
			compiledPattern := pattern
			if ignore {
				compiledPattern = strings.ToLower(pattern)
			}
			want := regexp.MustCompile(globRegexp(compiledPattern, false))
			for _, name := range []string{"", "a", "ab", "aab", "acb", "main.go", "MAIN.GO", "main\n.go", "a\nb", "a/b", `a\b`, "a/bx", "file.tar.gz", "файлЯ", "ФАЙЛя", string(filepath.Separator)} {
				candidate := name
				if ignore {
					candidate = strings.ToLower(name)
				}
				if got := masks[0].MatchString(name); got != want.MatchString(candidate) {
					t.Errorf("pattern=%q name=%q ignore=%v: got %v", pattern, name, ignore, got)
				}
			}
		}
	}
}

func BenchmarkSimpleGlob(b *testing.B) {
	masks, err := compileMasks("*.go", true, false)
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		masks[0].MatchString("qt_semantic.go")
	}
}
