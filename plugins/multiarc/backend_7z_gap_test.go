package multiarc

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// backend_7z_test.go covers list's success path and extractOne's happy
// path plus its odd-name argument shapes, but never a single failure: not
// the 7z/7za/7zr binary missing, not the tool itself exiting nonzero for
// list, extractAll or extractOne, and not extractAll or extractOne's own
// "no member path" precondition at all. This file covers those.

func TestSevenZipBackendID(t *testing.T) {
	if got := (sevenZipBackend{}).id(); got != "7z" {
		t.Errorf("id() = %q, want 7z", got)
	}
}

func TestSevenZipBackendListToolFailure(t *testing.T) {
	f := &fakeArchiver{
		tools: map[string]bool{"7z": true},
		fail:  map[string]error{"7z l": errors.New("multiarc test: 7z exited 2")},
	}
	f.install(t)

	_, err := (sevenZipBackend{}).list(context.Background(), "/a.7z")
	if err == nil || !strings.Contains(err.Error(), "7z exited 2") {
		t.Fatalf("list = %v, want the tool's own failure wrapped", err)
	}
}

func TestSevenZipBackendExtractAll(t *testing.T) {
	tests := []struct {
		name    string
		tools   map[string]bool
		fail    map[string]error
		wantErr string
	}{
		{
			name:    "no 7z binary on PATH",
			wantErr: "no 7z/7za/7zr on PATH",
		},
		{
			name:    "tool exits nonzero",
			tools:   map[string]bool{"7z": true},
			fail:    map[string]error{"7z x": errors.New("multiarc test: extract failed")},
			wantErr: "extract failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeArchiver{tools: tt.tools, fail: tt.fail}
			f.install(t)

			err := (sevenZipBackend{}).extractAll(context.Background(), "/a.7z", "/dest")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("extractAll = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestSevenZipBackendExtractAllRunsTheRightCommand(t *testing.T) {
	f := &fakeArchiver{tools: map[string]bool{"7z": true}}
	f.install(t)

	if err := (sevenZipBackend{}).extractAll(context.Background(), "/a.7z", "/dest"); err != nil {
		t.Fatalf("extractAll: %v", err)
	}
	want := []string{"7z x -y -o/dest /a.7z"}
	if got := f.commands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %q, want %q", got, want)
	}
}

func TestSevenZipBackendExtractOneRefusals(t *testing.T) {
	tests := []struct {
		name    string
		tools   map[string]bool
		member  string
		wantErr string
	}{
		{
			name:    "empty member",
			tools:   map[string]bool{"7z": true},
			member:  "",
			wantErr: "extractOne needs a member path",
		},
		{
			name:    "no 7z binary on PATH",
			member:  "sub/data.bin",
			wantErr: "no 7z/7za/7zr on PATH",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeArchiver{tools: tt.tools}
			f.install(t)

			err := (sevenZipBackend{}).extractOne(context.Background(), "/a.7z", "/dest", tt.member)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("extractOne(%q) = %v, want it to mention %q", tt.member, err, tt.wantErr)
			}
		})
	}
}

// extractOne's "@name" branch and its plain-name branch each run their own
// 7z command and each wrap a failure through toolFailure independently.
func TestSevenZipBackendExtractOneToolFailure(t *testing.T) {
	tests := []struct {
		name   string
		member string
	}{
		{"plain member", "sub/data.bin"},
		{"at-prefixed member (listfile form)", "@list.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeArchiver{
				tools: map[string]bool{"7z": true},
				fail:  map[string]error{"7z x": errors.New("multiarc test: extract failed")},
			}
			f.install(t)

			err := (sevenZipBackend{bin: "7z"}).extractOne(context.Background(), "/a.7z", "/dest", tt.member)
			if err == nil || !strings.Contains(err.Error(), "extract failed") {
				t.Fatalf("extractOne(%q) = %v, want it to mention the tool's failure", tt.member, err)
			}
		})
	}
}
