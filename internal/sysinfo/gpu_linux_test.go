//go:build linux

package sysinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseWmicNames(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "two adapters",
			in:   "Name=NVIDIA GeForce RTX 4090\r\n\r\nName=Intel(R) UHD Graphics\r\n\r\n",
			want: []string{"NVIDIA GeForce RTX 4090", "Intel(R) UHD Graphics"},
		},
		{
			name: "blank lines and unrelated fields ignored",
			in:   "\r\nSomeOtherField=x\r\nName=Adapter One\r\n",
			want: []string{"Adapter One"},
		},
		{
			name: "empty value skipped",
			in:   "Name=\r\nName=Adapter\r\n",
			want: []string{"Adapter"},
		},
		{
			name: "no matches",
			in:   "SomeOtherField=x\r\n",
			want: nil,
		},
		{
			name: "empty input",
			in:   "",
			want: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseWmicNames(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("parseWmicNames(%q) = %#v, want %#v", c.in, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("parseWmicNames(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestScanPCIIDs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pci.ids")
	content := "# comment line, ignored\n" +
		"8086  Intel Corporation\n" +
		"\t1234  Some Intel Device\n" +
		"\t\t5678 9abc Subsystem, skipped as a sub-vendor line\n" +
		"10de  NVIDIA Corporation\n" +
		"\t2684  AD102 [GeForce RTX 4090]\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	open := func(t *testing.T) *os.File {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = f.Close() })
		return f
	}

	if vName, dName := scanPCIIDs(open(t), "8086", "1234"); vName != "Intel Corporation" || dName != "Some Intel Device" {
		t.Errorf("scanPCIIDs(8086,1234) = (%q, %q), want (Intel Corporation, Some Intel Device)", vName, dName)
	}
	if vName, dName := scanPCIIDs(open(t), "10de", "2684"); vName != "NVIDIA Corporation" || dName != "AD102 [GeForce RTX 4090]" {
		t.Errorf("scanPCIIDs(10de,2684) = (%q, %q), want (NVIDIA Corporation, AD102 [GeForce RTX 4090])", vName, dName)
	}
	// Vendor matches but device id does not exist under it: vendor name is
	// still returned, device name stays empty.
	if vName, dName := scanPCIIDs(open(t), "8086", "ffff"); vName != "Intel Corporation" || dName != "" {
		t.Errorf("scanPCIIDs(8086,ffff) = (%q, %q), want (Intel Corporation, \"\")", vName, dName)
	}
	// Vendor id absent from the file entirely.
	if vName, dName := scanPCIIDs(open(t), "ffff", "0000"); vName != "" || dName != "" {
		t.Errorf("scanPCIIDs(ffff,0000) = (%q, %q), want (\"\", \"\")", vName, dName)
	}
}

func TestReadDriverFromUevent(t *testing.T) {
	dir := t.TempDir()

	withDriver := filepath.Join(dir, "uevent-with-driver")
	if err := os.WriteFile(withDriver, []byte("MODALIAS=x\nDRIVER=amdgpu\nOTHER=y\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readDriverFromUevent(withDriver); got != "amdgpu" {
		t.Errorf("readDriverFromUevent(with driver) = %q, want %q", got, "amdgpu")
	}

	withoutDriver := filepath.Join(dir, "uevent-without-driver")
	if err := os.WriteFile(withoutDriver, []byte("MODALIAS=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readDriverFromUevent(withoutDriver); got != "" {
		t.Errorf("readDriverFromUevent(without driver) = %q, want empty", got)
	}

	if got := readDriverFromUevent(filepath.Join(dir, "does-not-exist")); got != "" {
		t.Errorf("readDriverFromUevent(missing file) = %q, want empty", got)
	}
}

func TestReadFileTrim(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	if err := os.WriteFile(path, []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readFileTrim(path); got != "hello\n" {
		t.Errorf("readFileTrim(existing) = %q, want %q", got, "hello\n")
	}
	if got := readFileTrim(filepath.Join(dir, "missing")); got != "" {
		t.Errorf("readFileTrim(missing) = %q, want empty", got)
	}
}

func TestReadNvidiaGPUsNoModuleLoaded(t *testing.T) {
	// The GitHub Actions ubuntu-latest runner this test executes on has no
	// NVIDIA driver loaded, so /proc/driver/nvidia never exists: this
	// exercises the real "no proprietary driver" branch.
	if got := readNvidiaGPUs(); got != nil {
		t.Errorf("readNvidiaGPUs() = %#v, want nil without an NVIDIA driver", got)
	}
}

func TestLookupPCINameEmptyVendor(t *testing.T) {
	if got := lookupPCIName("", "1234"); got != "" {
		t.Errorf("lookupPCIName(empty vendor) = %q, want empty", got)
	}
}

func TestLookupPCINameUnknownVendor(t *testing.T) {
	// "zzzz" is not a hex id at all, so it cannot appear in either
	// /usr/share/hwdata/pci.ids or /usr/share/misc/pci.ids on any real
	// system (unlike the reserved "ffff", which pci.ids does list as
	// "Illegal Vendor ID") -- this exercises the full path-search loop (both
	// candidate files opened or skipped) down to the final "not found"
	// return, regardless of whether either file happens to exist on the runner.
	if got := lookupPCIName("zzzz", "zzzz"); got != "" {
		t.Errorf("lookupPCIName(zzzz, zzzz) = %q, want empty for a vendor id no pci.ids lists", got)
	}
}

func TestQueryHostGPUsFromWSL(t *testing.T) {
	_, wmicErr := exec.LookPath("wmic.exe")
	_, psErr := exec.LookPath("powershell.exe")

	got := queryHostGPUsFromWSL()

	if wmicErr != nil && psErr != nil {
		// Neither Windows interop binary is on PATH, which is true for
		// every non-WSL Linux CI runner: both exec.LookPath calls must
		// fail closed and the function must return nil rather than
		// panicking, blocking, or running past its 3s timeout.
		if got != nil {
			t.Errorf("queryHostGPUsFromWSL() = %#v, want nil when neither wmic.exe nor powershell.exe is on PATH", got)
		}
		return
	}
	// On a machine that does happen to expose one of the interop
	// binaries, just make sure the call completes without panicking;
	// its output depends on that binary's real output, which this test
	// doesn't control.
	t.Logf("queryHostGPUsFromWSL() = %#v (wmic.exe present=%v, powershell.exe present=%v)", got, wmicErr == nil, psErr == nil)
}

func TestGPU(t *testing.T) {
	// GPU() must not panic regardless of what hardware the CI runner
	// exposes, and its ok flag must agree with whether it returned any
	// entries.
	gpus, ok := GPU()
	if ok != (len(gpus) > 0) {
		t.Errorf("GPU() ok = %v but len(gpus) = %d, they must agree", ok, len(gpus))
	}
}
