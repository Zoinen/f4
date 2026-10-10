package app

import (
	"strings"
	"testing"

	"github.com/unxed/f4/plugins/netfox/fishplus"
)

func TestRunFishServerServesASession(t *testing.T) {
	in := fishplus.NativeHelloLine("tok") + "1 noop\n2 exit\n"
	var out, errOut strings.Builder
	if code := runFishServer(strings.NewReader(in), &out, &errOut); code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, errOut.String())
	}
	for _, want := range []string{".tok 0 ok FISHPLUS 1 ", ".tok 1 ok", ".tok 2 ok"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output %q lacks %q", out.String(), want)
		}
	}
}

func TestRunFishServerReportsAStreamItCannotFollow(t *testing.T) {
	var out, errOut strings.Builder
	if code := runFishServer(strings.NewReader("not a hello\n"), &out, &errOut); code != 1 {
		t.Fatalf("exit code %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "bad hello") {
		t.Errorf("stderr = %q", errOut.String())
	}
}
