package fileops

import "testing"

func TestFoldOSPathCase(t *testing.T) {
	t.Run("case-insensitive filesystem folds both sides", func(t *testing.T) {
		src, dst := foldOSPathCase("/a/Foo", "/A/foo", true)
		if src != dst {
			t.Fatalf("folded paths differ: %q vs %q", src, dst)
		}
	})
	t.Run("case-sensitive filesystem keeps them apart", func(t *testing.T) {
		src, dst := foldOSPathCase("/a/Foo", "/a/foo", false)
		if src == dst {
			t.Fatalf("Foo and foo compared equal on a case-sensitive filesystem: %q", src)
		}
		if src != "/a/Foo" || dst != "/a/foo" {
			t.Fatalf("paths were changed: %q, %q", src, dst)
		}
	})
}
