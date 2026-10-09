package vfs

import "testing"

// TestLinkKindOf pins the classification the panel's Size column relies on
// to say Symlink or Junction (#392). Tag values are from the Windows
// "Reparse Point Tags" documentation.
func TestLinkKindOf(t *testing.T) {
	const (
		tagDedup     uint32 = 0x80000013 // IO_REPARSE_TAG_DEDUP: the file itself, no N bit
		tagSurrogate uint32 = 0xA000001D // an N-bit tag other than the two named ones
	)
	cases := []struct {
		name string
		item VFSItem
		want LinkKind
	}{
		{"plain file", VFSItem{Name: "a.txt"}, LinkNone},
		{"plain dir", VFSItem{Name: "d", IsDir: true}, LinkNone},
		{"posix symlink has no tag", VFSItem{Name: "l", IsSymlink: true}, LinkSymlink},
		{"posix symlink to dir", VFSItem{Name: "l", IsDir: true, IsSymlink: true}, LinkSymlink},
		{"windows symlink", VFSItem{Name: "l", IsDir: true, IsSymlink: true, ReparseTag: ReparseTagSymlink}, LinkSymlink},
		{"windows junction", VFSItem{Name: "j", IsDir: true, IsSymlink: true, ReparseTag: ReparseTagMountPoint}, LinkJunction},
		{"junction tag alone", VFSItem{Name: "j", IsDir: true, ReparseTag: ReparseTagMountPoint}, LinkJunction},
		{"other surrogate", VFSItem{Name: "s", IsSymlink: true, ReparseTag: tagSurrogate}, LinkSymlink},
		{"non-surrogate reparse point", VFSItem{Name: "f", Size: 10, IsSymlink: true, ReparseTag: tagDedup}, LinkNone},
	}
	for _, tc := range cases {
		if got := LinkKindOf(&tc.item); got != tc.want {
			t.Errorf("%s: LinkKindOf = %d, want %d", tc.name, got, tc.want)
		}
	}
}
