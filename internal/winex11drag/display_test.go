package winex11drag

import "testing"

func TestParseDisplay(t *testing.T) {
	for _, tt := range []struct {
		name    string
		display string
		want    DisplaySpec
		wantErr bool
	}{
		{name: "local, no screen", display: ":0", want: DisplaySpec{Display: 0}},
		{name: "local, double digit", display: ":10", want: DisplaySpec{Display: 10}},
		{name: "local, with screen", display: ":1.2", want: DisplaySpec{Display: 1, Screen: 2, HasScreen: true}},
		{name: "remote host, no screen", display: "myhost:0", want: DisplaySpec{Hostname: "myhost", Display: 0}},
		{name: "remote host, with screen", display: "myhost:0.1", want: DisplaySpec{Hostname: "myhost", Display: 0, Screen: 1, HasScreen: true}},
		{name: "explicit unix pseudo-host", display: "unix:0", want: DisplaySpec{Hostname: "unix", Display: 0}},
		{name: "no colon at all", display: "garbage", wantErr: true},
		{name: "colon with nothing after it", display: ":", wantErr: true},
		{name: "non-numeric display", display: ":abc", wantErr: true},
		{name: "non-numeric screen", display: ":0.abc", wantErr: true},
		{name: "negative display", display: ":-1", wantErr: true},
		{name: "empty string", display: "", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDisplay(tt.display)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseDisplay(%q) succeeded with %+v, want an error", tt.display, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDisplay(%q) = %v", tt.display, err)
			}
			if got != tt.want {
				t.Errorf("ParseDisplay(%q) = %+v, want %+v", tt.display, got, tt.want)
			}
		})
	}
}

func TestDisplaySpecSocketPath(t *testing.T) {
	for _, tt := range []struct {
		name     string
		spec     DisplaySpec
		wantPath string
		wantOK   bool
	}{
		{name: "local", spec: DisplaySpec{Display: 0}, wantPath: "/tmp/.X11-unix/X0", wantOK: true},
		{name: "local, double digit display", spec: DisplaySpec{Display: 12}, wantPath: "/tmp/.X11-unix/X12", wantOK: true},
		{name: "explicit unix pseudo-host", spec: DisplaySpec{Hostname: "unix", Display: 3}, wantPath: "/tmp/.X11-unix/X3", wantOK: true},
		{name: "remote host has no local socket", spec: DisplaySpec{Hostname: "myhost", Display: 0}, wantOK: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path, ok := tt.spec.SocketPath()
			if ok != tt.wantOK || (ok && path != tt.wantPath) {
				t.Errorf("SocketPath() = (%q, %v), want (%q, %v)", path, ok, tt.wantPath, tt.wantOK)
			}
		})
	}
}

func TestDisplaySpecIsLocal(t *testing.T) {
	for _, tt := range []struct {
		hostname string
		want     bool
	}{
		{hostname: "", want: true},
		{hostname: "unix", want: true},
		{hostname: "myhost", want: false},
		{hostname: "localhost", want: false}, // a real hostname, not the "unix" pseudo-host
	} {
		spec := DisplaySpec{Hostname: tt.hostname}
		if got := spec.IsLocal(); got != tt.want {
			t.Errorf("DisplaySpec{Hostname: %q}.IsLocal() = %v, want %v", tt.hostname, got, tt.want)
		}
	}
}
