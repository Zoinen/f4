package main

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersionInfoBuildMetadataTakesPrecedence(t *testing.T) {
	info := &debug.BuildInfo{
		Main: debug.Module{Version: "v1.0.0"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "1111111111111111"},
			{Key: "vcs.modified", Value: "false"},
			{Key: "vcs.time", Value: "2020-01-02T03:04:05Z"},
		},
	}

	got := resolveVersionInfo(rawVersionInfo{
		version:  "v2.3.4",
		revision: "abcdef0123456789",
		modified: "true",
		time:     "2026-08-16T12:34:56+03:00",
	}, info)

	if want := "v2.3.4-dirty"; got.short() != want {
		t.Fatalf("short version = %q, want %q", got.short(), want)
	}
	if want := "v2.3.4-dirty [2026-08-16 12:34]"; got.long() != want {
		t.Fatalf("long version = %q, want %q", got.long(), want)
	}
	if want := "abcdef0"; got.revision != want {
		t.Fatalf("revision = %q, want %q", got.revision, want)
	}
}

func TestResolveVersionInfoUsesGoBuildSettings(t *testing.T) {
	info := &debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef"},
			{Key: "vcs.modified", Value: "true"},
			{Key: "vcs.time", Value: "2025-11-12T13:14:15Z"},
		},
	}

	got := resolveVersionInfo(rawVersionInfo{}, info)
	if want := "0123456-dirty"; got.short() != want {
		t.Fatalf("short version = %q, want %q", got.short(), want)
	}
	if want := "0123456-dirty [2025-11-12 13:14]"; got.long() != want {
		t.Fatalf("long version = %q, want %q", got.long(), want)
	}
}

func TestResolveVersionInfoUsesModuleReleaseVersion(t *testing.T) {
	info := &debug.BuildInfo{
		Main: debug.Module{Version: "v4.5.6"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "fedcba9876543210"},
			{Key: "vcs.modified", Value: "false"},
		},
	}

	got := resolveVersionInfo(rawVersionInfo{}, info)
	if want := "v4.5.6"; got.short() != want {
		t.Fatalf("short version = %q, want %q", got.short(), want)
	}
}

func TestResolveVersionInfoFallsBackWithoutVCSMetadata(t *testing.T) {
	got := resolveVersionInfo(rawVersionInfo{}, nil)
	if want := "(devel)"; got.short() != want {
		t.Fatalf("short version = %q, want %q", got.short(), want)
	}
	if want := "(devel)"; got.long() != want {
		t.Fatalf("long version = %q, want %q", got.long(), want)
	}
}

func TestResolveVersionInfoIgnoresIncompleteTime(t *testing.T) {
	got := resolveVersionInfo(rawVersionInfo{
		revision: "1234567",
		time:     "not-a-time",
	}, nil)
	if want := "1234567"; got.long() != want {
		t.Fatalf("long version = %q, want %q", got.long(), want)
	}
}
