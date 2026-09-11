//go:build !windows

package gui

import (
	"reflect"
	"testing"
)

func TestGuiFontChoicesUnixCaseSensitivePaths(t *testing.T) {
	installed := []string{"/fonts/Mono.ttf", "/fonts/mono.ttf", "/fonts/./Mono.ttf"}
	got := GuiFontChoicesFromInstalled("", installed)
	want := []string{"/fonts/Mono.ttf", "/fonts/mono.ttf"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("choices = %q, want %q", got, want)
	}
}
