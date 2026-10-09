//go:build !extralite

package vfs

import (
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

// cjkCodepages are the East Asian multi-byte code pages. Their tables are
// about 0.6 MB of the binary, which is why the extra-lite profile leaves
// them out (codepages_nocjk.go).
func cjkCodepages() []Codepage {
	return []Codepage{
		{ID: 932, Name: "Shift JIS (Japanese)", Enc: japanese.ShiftJIS, group: codepageOther},
		{ID: 50220, Name: "ISO-2022-JP (Japanese)", Enc: japanese.ISO2022JP, group: codepageOther},
		{ID: 51932, Name: "EUC-JP (Japanese)", Enc: japanese.EUCJP, group: codepageOther},
		{ID: 51949, Name: "EUC-KR (Korean)", Enc: korean.EUCKR, group: codepageOther},
		{ID: 936, Name: "GBK (Simplified Chinese)", Enc: simplifiedchinese.GBK, group: codepageOther},
		{ID: 52936, Name: "HZ-GB-2312 (Simplified Chinese)", Enc: simplifiedchinese.HZGB2312, group: codepageOther},
		{ID: 54936, Name: "GB18030 (Simplified Chinese)", Enc: simplifiedchinese.GB18030, group: codepageOther},
		{ID: 950, Name: "Big5 (Traditional Chinese)", Enc: traditionalchinese.Big5, group: codepageOther},
	}
}
