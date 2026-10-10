//go:build extralite

package vfs

// cjkCodepages is empty in the extra-lite profile (unxed/f4#1671): the East
// Asian code page tables are about 0.6 MB of a binary meant for routers.
// UTF-8, UTF-16 and the single-byte code pages remain.
func cjkCodepages() []Codepage { return nil }
