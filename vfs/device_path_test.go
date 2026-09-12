package vfs

import "testing"

func TestDevicePathRoundTrip(t *testing.T) {
	tests := []struct{ name, device, domain, remote, public string }{
		{name: "iphone", device: "Alexander's iPhone", remote: "/DCIM/100APPLE/IMG_0007.JPG",
			public: "ios://Alexander's iPhone/DCIM/100APPLE/IMG_0007.JPG"},
		{name: "unicode", device: "Саша’s iPhone", remote: "/Фото/снимок.jpg",
			public: "ios://Саша’s iPhone/Фото/снимок.jpg"},
		{name: "delimiters", device: "phone/test", remote: "/a%2F b#?.jpg",
			public: "ios://phone%2Ftest/a%252F b%23%3F.jpg"},
		{name: "export", device: "Phone", domain: "[Applications]/App (org.app)", remote: "/Documents/a.txt",
			public: "ios://Phone/[Applications]/App (org.app)/Documents/a.txt"},
		{name: "root", device: "Phone", remote: "/", public: "ios://Phone/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := DevicePath{Scheme: "ios", Device: tt.device, Domain: tt.domain}
			if got := d.Public(tt.remote); got != tt.public {
				t.Fatalf("Public = %q, want %q", got, tt.public)
			}
			if got, err := d.Remote("/other", tt.public); err != nil || got != tt.remote {
				t.Fatalf("Remote = %q, %v, want %q", got, err, tt.remote)
			}
			if got := d.Join(d.Dir(tt.public), d.Base(tt.public)); tt.remote != "/" && got != tt.public {
				t.Fatalf("Dir/Base/Join round trip = %q", got)
			}
			if d.Dir(d.Root()) != d.Root() {
				t.Fatal("Dir escaped export root")
			}
		})
	}
}

func TestDevicePathRejectsForeignAndMalformedAddresses(t *testing.T) {
	d := DevicePath{Scheme: "ios", Device: "Phone", Domain: "[Applications]/App"}
	for _, candidate := range []string{
		"android://Phone/[Applications]/App/a", "ios://Other/[Applications]/App/a",
		"ios://Phone/DCIM/a", "ios://Phone/[Applications]/App/a%2Fb", "ios://Phone/%ZZ",
		"ios://Phone/[Applications]/App/%00", "ios:///[Applications]/App",
	} {
		t.Run(candidate, func(t *testing.T) {
			if _, err := d.Remote("/", candidate); err == nil {
				t.Fatal("invalid address accepted")
			}
		})
	}
}

func TestDevicePathNativePathsAndEscapedNames(t *testing.T) {
	d := DevicePath{Scheme: "android", Device: "Pixel 3"}
	if got, err := d.Remote("/sdcard/DCIM", "../Download"); err != nil || got != "/sdcard/Download" {
		t.Fatalf("relative = %q, %v", got, err)
	}
	if got := d.Join(d.Public("/sdcard"), "a%2F#?.txt"); got != "android://Pixel 3/sdcard/a%252F%23%3F.txt" {
		t.Fatalf("literal filename was decoded: %q", got)
	}
}
