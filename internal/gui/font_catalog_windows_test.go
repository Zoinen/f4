//go:build windows

package gui

import (
	"reflect"
	"testing"
)

func TestGuiFontChoicesWindowsPathEquality(t *testing.T) {
	for _, test := range []struct {
		name      string
		current   string
		installed []string
		want      []string
	}{
		{
			name:      "drive slash dot and case",
			current:   `C:/Fonts/Sub/../Mono.ttf`,
			installed: []string{`c:\fonts\MONO.TTF`, `C:\Fonts\Other.ttf`},
			want:      []string{`C:/Fonts/Sub/../Mono.ttf`, `C:\Fonts\Other.ttf`},
		},
		{
			name:      "unicode simple folding",
			current:   `C:\Fonts\Σ.ttf`,
			installed: []string{`c:\fonts\ς.TTF`, `C:\Fonts\σ.ttf`, `C:\Fonts\K.ttf`, `c:\fonts\k.TTF`, `C:\Fonts\ſ.ttf`, `c:\fonts\s.TTF`},
			want:      []string{`C:\Fonts\Σ.ttf`, `C:\Fonts\K.ttf`, `C:\Fonts\ſ.ttf`},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := GuiFontChoicesFromInstalled(test.current, test.installed)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("choices = %q, want %q", got, test.want)
			}
		})
	}
}

func TestGuiFontBatchLabelsWindowsPathEquality(t *testing.T) {
	previous := newGuiFontDisplayNameResolver
	t.Cleanup(func() { newGuiFontDisplayNameResolver = previous })
	newGuiFontDisplayNameResolver = func([]string) func(string) string {
		return func(value string) string { return "Family: " + value }
	}
	values := []string{`C:/fonts/sub/../MONO.ttf`, `C:\fonts\σ.ttf`, `C:\Custom\Mono.ttf`}
	installed := []string{`c:\Fonts\mono.TTF`, `c:/fonts/ς.TTF`}
	got := GuiFontDisplayValuesFromInstalled(values, installed)
	want := []string{"Family: " + values[0], "Family: " + values[1], values[2]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("labels = %q, want %q", got, want)
	}
}
