package fileops

import "testing"

func TestDeviceURIHistoryIdentity(t *testing.T) {
	for _, tt := range []struct {
		name, a, b string
		equal      bool
	}{
		{"ios case", "ios://Alexander's iPhone/DCIM", "ios://alexander's iPhone/DCIM", false},
		{"android case", "android://Pixel 3/sdcard", "android://pixel 3/sdcard", false},
		{"scheme case", "IOS://Alexander’s iPhone/DCIM", "ios://Alexander’s iPhone/DCIM", true},
		{"android scheme case", "ANDROID://Pixel 3/sdcard", "android://Pixel 3/sdcard", true},
		{"escaped authority case", "ios://Phone%2FOne/DCIM", "ios://Phone%2Fone/DCIM", false},
		{"escaped authority delimiter", "ios://Phone%2FOne/DCIM", "ios://Phone%252FOne/DCIM", false},
		{"escaped path delimiter", "android://Pixel 3/sdcard/a%23b", "android://Pixel 3/sdcard/a%2523b", false},
		{"device trailing slash", "ios://Phone/DCIM/", "ios://Phone/DCIM", true},
		{"web DNS case", "HTTPS://EXAMPLE.COM/path", "https://example.com/path", true},
		{"cloud authority case", "CLOUD://GDRIVE/profile/item", "cloud://gdrive/profile/item", true},
		{"network DNS case", "SFTP://HOST/path", "sftp://host/path", true},
		{"network path case", "https://example.com/Path", "https://example.com/path", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := SameFolderHistoryPath(tt.a, tt.b); got != tt.equal {
				t.Fatalf("SameFolderHistoryPath(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.equal)
			}
		})
	}
}
