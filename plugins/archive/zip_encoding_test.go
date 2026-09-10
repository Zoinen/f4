package archive

import (
	"testing"

	"github.com/unxed/zip"
)

func TestDecodeZipName(t *testing.T) {
	tests := []struct {
		name           string
		encoded        string
		want           string
		flags          uint16
		creatorVersion uint16
	}{
		{
			name:    "utf8 flag preserves name",
			encoded: "already UTF-8",
			want:    "already UTF-8",
			flags:   0x800,
		},
		{
			name:           "creator OS 0 uses code page 866",
			encoded:        string([]byte{0xe2, 0xa5, 0xe1, 0xe2}),
			want:           "тест",
			creatorVersion: 0x0000,
		},
		{
			name:           "creator OS 6 uses code page 866",
			encoded:        string([]byte{0xe2, 0xa5, 0xe1, 0xe2}),
			want:           "тест",
			creatorVersion: 0x0600,
		},
		{
			name:           "old creator OS 11 uses code page 866",
			encoded:        string([]byte{0xe2, 0xa5, 0xe1, 0xe2}),
			want:           "тест",
			creatorVersion: 0x0b13,
		},
		{
			name:           "modern creator OS 11 uses Windows 1251",
			encoded:        string([]byte{0xf2, 0xe5, 0xf1, 0xf2}),
			want:           "тест",
			creatorVersion: 0x0b14,
		},
		{
			name:           "unknown creator uses code page 437",
			encoded:        string([]byte{0x82}),
			want:           "é",
			creatorVersion: 0x0300,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hdr := &zip.FileHeader{
				Name:           test.encoded,
				Flags:          test.flags,
				CreatorVersion: test.creatorVersion,
			}
			if got := DecodeZipName(hdr); got != test.want {
				t.Fatalf("DecodeZipName() = %q, want %q", got, test.want)
			}
		})
	}
}
