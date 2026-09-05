//go:build !windows

package vfs

import "testing"

func TestParseSysfsBlockSize(t *testing.T) {
	tests := []struct {
		name string
		data string
		want int64
		ok   bool
	}{
		{name: "valid", data: "8\n", want: 4096, ok: true},
		{name: "surrounding whitespace", data: " \t8\r\n", want: 4096, ok: true},
		{name: "empty", data: "\n"},
		{name: "non-numeric", data: "unknown\n"},
		{name: "numeric prefix", data: "8 sectors\n"},
		{name: "zero", data: "0\n"},
		{name: "negative", data: "-1\n"},
		{name: "overflowing bytes", data: "18014398509481984\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseSysfsBlockSize([]byte(tt.data))
			if got != tt.want || ok != tt.ok {
				t.Fatalf("parseSysfsBlockSize(%q) = (%d, %v), want (%d, %v)", tt.data, got, ok, tt.want, tt.ok)
			}
		})
	}
}
