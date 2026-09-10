package update

import (
	"testing"
)

func TestUpdater_ParseUpdateHelperArgs(t *testing.T) {
	archive, kind, found, err := ParseHelperArgs([]string{HelperFlag, `C:\Users\Test User\f4-update.archive`, "zip"})
	if err != nil || !found {
		t.Fatalf("ParseHelperArgs() failed: found=%v err=%v", found, err)
	}
	if archive != `C:\Users\Test User\f4-update.archive` || kind != "zip" {
		t.Fatalf("ParseHelperArgs() = %q, %q; want archive path and zip", archive, kind)
	}

	if _, _, found, err := ParseHelperArgs([]string{HelperFlag, "archive.zip"}); !found || err == nil {
		t.Fatalf("malformed helper invocation: found=%v err=%v", found, err)
	}
	if _, _, found, err := ParseHelperArgs([]string{"--gui=win32"}); found || err != nil {
		t.Fatalf("normal invocation parsed as update helper: found=%v err=%v", found, err)
	}
}

func TestUpdater_ManualBuildVersionUsesBuildTimestamp(t *testing.T) {
	manual := Build{Version: "manual-build-sha", TimeText: "2026-08-21T12:00:00Z"}

	newerLocalBuild := Release{TagName: "v0.2.0-beta", PublishedAt: "2026-08-20T12:00:00Z"}
	if stableReleaseNeedsUpdate(newerLocalBuild, manual, "") {
		t.Fatal("a manual build newer than the release must not request a downgrade")
	}

	newerRelease := Release{TagName: "v0.2.0-beta", PublishedAt: "2026-08-22T12:00:00Z"}
	if !stableReleaseNeedsUpdate(newerRelease, manual, "") {
		t.Fatal("a release newer than a manual build must be offered")
	}

	exact := Build{Version: "v0.2.0-beta", IsRelease: true, TimeText: manual.TimeText}
	if stableReleaseNeedsUpdate(newerRelease, exact, "") {
		t.Fatal("an exact release build must not request an update to itself")
	}

	// LastVersion is the "already installed" marker and suppresses the offer
	// on its own, whatever the build calls itself.
	if stableReleaseNeedsUpdate(newerRelease, manual, "v0.2.0-beta") {
		t.Fatal("an already installed release must not be offered again")
	}
}

func TestFormatBuildTimeUsesOneClockForVCSAndNightlyMetadata(t *testing.T) {
	fromVCS := FormatBuildTime("2026-08-23T06:49:17Z")
	fromReleaseBody := FormatBuildTime("2026-08-23 06:49:17")
	if fromVCS != fromReleaseBody {
		t.Fatalf("VCS time %q and release-body time %q diverged", fromVCS, fromReleaseBody)
	}
	if got := FormatBuildTime("not a timestamp"); got != "not a timestamp" {
		t.Fatalf("invalid timestamp = %q, want unchanged input", got)
	}
}
