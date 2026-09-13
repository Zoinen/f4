package androidfs

import "testing"

func TestDeviceInfoNormalizesPublicDirectory(t *testing.T) {
	if got := normalizeDeviceInfoPath("android://Pixel 3/sdcard/a'b %23%3F"); got != "/sdcard/a'b #?" {
		t.Fatalf("panel info directory = %q", got)
	}
}
